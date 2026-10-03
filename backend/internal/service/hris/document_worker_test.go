package hris

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/config"
	"github.com/kana-consultant/kantor/backend/internal/docgen"
	"github.com/kana-consultant/kantor/backend/internal/security"
	auditservice "github.com/kana-consultant/kantor/backend/internal/service/audit"
	"github.com/kana-consultant/kantor/backend/internal/tenant"
)

const (
	workerTestTenantID = "00000000-0000-0000-0000-00000000000a"
	workerTestActorID  = "11111111-1111-1111-1111-111111111111"
)

var workerTestTenant = tenant.Info{ID: workerTestTenantID, Slug: "acme", Name: "Acme"}

type workerConnKey struct{}

// fakeConnector hands out a marker "connection" and counts how many are held.
type fakeConnector struct {
	held     atomic.Int32
	acquired atomic.Int32
	failNext atomic.Bool
}

func (c *fakeConnector) connect(ctx context.Context, tenantID string) (context.Context, func(), error) {
	if c.failNext.CompareAndSwap(true, false) {
		return nil, nil, errors.New("pool exhausted")
	}
	c.acquired.Add(1)
	c.held.Add(1)
	var once sync.Once
	return context.WithValue(ctx, workerConnKey{}, tenantID), func() { once.Do(func() { c.held.Add(-1) }) }, nil
}

type fakeRow struct {
	status      string
	claim       int
	parts       []DocumentPart
	files       []StoredDocument
	failure     string
	actor       string
	stalePaths  []string
	supersede   bool
	prepareErr  error
	completions int
	panicOnFail bool
}

type fakeRenderHandler struct {
	t    *testing.T
	kind string

	mu          sync.Mutex
	rows        map[string]*fakeRow
	staleBefore time.Time
	failClaims  []string
	done        chan string
}

func newFakeRenderHandler(t *testing.T, kind string) *fakeRenderHandler {
	return &fakeRenderHandler{t: t, kind: kind, rows: map[string]*fakeRow{}, done: make(chan string, 64)}
}

func (h *fakeRenderHandler) requireConn(ctx context.Context) {
	if tenantID, _ := ctx.Value(workerConnKey{}).(string); tenantID != workerTestTenantID {
		h.t.Errorf("%s handler called without a tenant connection", h.kind)
	}
}

// requireTenant: Prepare/Complete/Fail must see the tenant itself, not only
// its connection, so tenant-aware services (company profile logo) work.
func (h *fakeRenderHandler) requireTenant(ctx context.Context) {
	h.requireConn(ctx)
	if info, ok := tenant.FromContext(ctx); !ok || info.ID != workerTestTenantID {
		h.t.Errorf("%s handler called without tenant.Info in ctx (ok=%v)", h.kind, ok)
	}
}

func (h *fakeRenderHandler) Kind() string { return h.kind }

func (h *fakeRenderHandler) ListPending(ctx context.Context, staleBefore time.Time) ([]string, error) {
	h.requireConn(ctx)
	h.mu.Lock()
	defer h.mu.Unlock()
	h.staleBefore = staleBefore
	var ids []string
	for id, row := range h.rows {
		if row.status == "pending" {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func (h *fakeRenderHandler) Prepare(ctx context.Context, id string) (PreparedDocument, error) {
	h.requireTenant(ctx)
	h.mu.Lock()
	defer h.mu.Unlock()
	row, ok := h.rows[id]
	if !ok || row.status != "pending" {
		return PreparedDocument{}, ErrDocumentJobSkipped
	}
	if row.prepareErr != nil {
		return PreparedDocument{}, row.prepareErr
	}
	row.status = "rendering"
	row.claim++
	return PreparedDocument{Claim: fmt.Sprint(row.claim), ActorID: row.actor, Parts: row.parts}, nil
}

func (h *fakeRenderHandler) Complete(ctx context.Context, id string, claim string, files []StoredDocument) ([]string, error) {
	h.requireTenant(ctx)
	h.mu.Lock()
	defer h.mu.Unlock()
	row := h.rows[id]
	row.completions++
	defer func() { h.done <- id }()
	if row.supersede || claim != fmt.Sprint(row.claim) {
		row.status = "pending"
		return nil, ErrDocumentJobSuperseded
	}
	row.status = "ready"
	row.files = files
	return row.stalePaths, nil
}

func (h *fakeRenderHandler) Fail(ctx context.Context, id string, claim string, reason string) error {
	h.requireTenant(ctx)
	h.mu.Lock()
	defer h.mu.Unlock()
	row := h.rows[id]
	if row.panicOnFail {
		panic("handler bug in Fail")
	}
	row.status = "failed"
	row.failure = reason
	h.failClaims = append(h.failClaims, claim)
	h.done <- id
	return nil
}

func (h *fakeRenderHandler) row(id string) fakeRow {
	h.mu.Lock()
	defer h.mu.Unlock()
	return *h.rows[id]
}

// fakeConverter "converts" by prefixing %PDF-; documents containing "BAD"
// fail the whole call, like a real soffice batch would.
type fakeConverter struct {
	t         *testing.T
	available bool
	conns     *fakeConnector

	mu    sync.Mutex
	calls [][]int
}

func (c *fakeConverter) Available() bool { return c.available }

func (c *fakeConverter) ConvertBatch(_ context.Context, docs [][]byte) ([][]byte, error) {
	if held := c.conns.held.Load(); held != 0 {
		c.t.Errorf("conversion ran while %d DB connection(s) were held", held)
	}
	c.mu.Lock()
	sizes := make([]int, len(docs))
	for i, doc := range docs {
		sizes[i] = len(doc)
	}
	c.calls = append(c.calls, sizes)
	c.mu.Unlock()

	out := make([][]byte, len(docs))
	for i, doc := range docs {
		if bytes.Contains(doc, []byte("BAD")) {
			return nil, errors.New("soffice failed")
		}
		out[i] = append([]byte("%PDF-1.7 "), doc...)
	}
	return out, nil
}

func (c *fakeConverter) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.calls)
}

type workerHarness struct {
	worker    *DocumentWorker
	store     *DocumentStore
	conns     *fakeConnector
	converter *fakeConverter
	handler   *fakeRenderHandler
	uploads   string

	auditMu sync.Mutex
	audits  []auditservice.Entry
}

func newWorkerHarness(t *testing.T, pdfAvailable bool) *workerHarness {
	t.Helper()
	encrypter, err := security.NewEncrypter("worker-test-secret-0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	h := &workerHarness{
		conns:   &fakeConnector{},
		uploads: t.TempDir(),
		handler: newFakeRenderHandler(t, "payslip"),
	}
	h.store = NewDocumentStore(h.uploads, encrypter)
	h.converter = &fakeConverter{t: t, available: pdfAvailable, conns: h.conns}
	h.worker = newDocumentWorker(h.converter, h.store, h.conns.connect, func(ctx context.Context, entry auditservice.Entry) {
		if ctx.Value(workerConnKey{}) == nil {
			t.Error("audit written without a tenant connection")
		}
		h.auditMu.Lock()
		defer h.auditMu.Unlock()
		h.audits = append(h.audits, entry)
	})
	h.worker.render = func(part DocumentPart) ([]byte, error) {
		if msg, _ := part.Payload["fail"].(string); msg != "" {
			return nil, fmt.Errorf("%w: missing %s", docgen.ErrRender, msg)
		}
		return []byte(fmt.Sprintf("DOCX[%s:%v]", part.Name, part.Payload["body"])), nil
	}
	if err := h.worker.Register(h.handler); err != nil {
		t.Fatal(err)
	}
	return h
}

func (h *workerHarness) addRow(id string, row *fakeRow) {
	if row.status == "" {
		row.status = "pending"
	}
	if row.actor == "" {
		row.actor = workerTestActorID
	}
	h.handler.rows[id] = row
}

func slipPart(body string) DocumentPart {
	return DocumentPart{Name: "slip", Template: docgen.TemplateSlip, Payload: docgen.Payload{"body": body}}
}

func jobsFor(kind string, ids ...string) []DocumentJob {
	jobs := make([]DocumentJob, 0, len(ids))
	for _, id := range ids {
		jobs = append(jobs, DocumentJob{Tenant: workerTestTenant, Kind: kind, ID: id})
	}
	return jobs
}

func storedFiles(t *testing.T, uploads string) []string {
	t.Helper()
	var files []string
	root := filepath.Join(uploads, "documents")
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			rel, _ := filepath.Rel(uploads, path)
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	return files
}

const (
	docA = "aaaaaaaa-0000-0000-0000-000000000001"
	docB = "aaaaaaaa-0000-0000-0000-000000000002"
	docC = "aaaaaaaa-0000-0000-0000-000000000003"
)

func TestDocumentWorkerBatchesTenantIntoOneConversion(t *testing.T) {
	h := newWorkerHarness(t, true)

	old, err := h.store.Write(workerTestTenantID, "payslip", docA, "slip", []byte("%PDF-old"))
	if err != nil {
		t.Fatal(err)
	}
	h.addRow(docA, &fakeRow{parts: []DocumentPart{slipPart("a")}, stalePaths: []string{old.Path}})
	h.addRow(docB, &fakeRow{parts: []DocumentPart{slipPart("b")}})
	h.addRow(docC, &fakeRow{parts: []DocumentPart{
		{Name: "pkwt", Template: docgen.TemplatePKWT, Payload: docgen.Payload{"body": "c1"}},
		{Name: "nda", Template: docgen.TemplateNDA, Payload: docgen.Payload{"body": "c2"}},
	}})

	h.worker.ProcessTenantBatch(context.Background(), workerTestTenant, jobsFor("payslip", docA, docB, docC))

	if calls := h.converter.callCount(); calls != 1 {
		t.Fatalf("soffice calls = %d, want 1 for the whole tenant batch", calls)
	}
	if got := len(h.converter.calls[0]); got != 4 {
		t.Fatalf("documents in the batch = %d, want 4", got)
	}
	if got := h.conns.acquired.Load(); got != 2 {
		t.Fatalf("connections acquired = %d, want 2 (claim, record)", got)
	}
	if held := h.conns.held.Load(); held != 0 {
		t.Fatalf("%d connection(s) leaked", held)
	}

	pathRe := regexp.MustCompile(`^documents/` + workerTestTenantID + `/payslip/[0-9a-f-]{36}-(slip|pkwt|nda)-[0-9a-f]{12}\.pdf\.enc$`)
	for id, wantParts := range map[string][]string{docA: {"slip"}, docB: {"slip"}, docC: {"pkwt", "nda"}} {
		row := h.handler.row(id)
		if row.status != "ready" {
			t.Fatalf("%s status = %s (%s), want ready", id, row.status, row.failure)
		}
		if len(row.files) != len(wantParts) {
			t.Fatalf("%s files = %d, want %d", id, len(row.files), len(wantParts))
		}
		for i, file := range row.files {
			if file.Part != wantParts[i] || !pathRe.MatchString(file.Path) || !strings.HasPrefix(file.Path, "documents/"+workerTestTenantID+"/payslip/"+id+"-") {
				t.Fatalf("unexpected stored file %+v", file)
			}
			info, err := os.Stat(filepath.Join(h.uploads, filepath.FromSlash(file.Path)))
			if err != nil {
				t.Fatal(err)
			}
			if mode := info.Mode().Perm(); mode != 0o640 {
				t.Fatalf("file mode = %o, want 640", mode)
			}
			raw, _ := os.ReadFile(filepath.Join(h.uploads, filepath.FromSlash(file.Path)))
			if bytes.Contains(raw, []byte("%PDF")) || bytes.Contains(raw, []byte("DOCX[")) {
				t.Fatal("stored file is not encrypted")
			}
			pdf, err := h.store.Read(workerTestTenantID, file.Path, file.SHA256)
			if err != nil {
				t.Fatalf("Read: %v", err)
			}
			sum := sha256.Sum256(pdf)
			if hex.EncodeToString(sum[:]) != file.SHA256 || !bytes.HasPrefix(pdf, []byte("%PDF-1.7 DOCX["+wantParts[i])) {
				t.Fatalf("stored PDF / sha256 mismatch for %s", file.Path)
			}
		}
	}

	// The superseded PDF of docA is gone; only the 4 new files remain.
	if _, err := os.Stat(filepath.Join(h.uploads, filepath.FromSlash(old.Path))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("superseded PDF still exists: %v", err)
	}
	if files := storedFiles(t, h.uploads); len(files) != 4 {
		t.Fatalf("stored files = %v, want 4", files)
	}

	if len(h.audits) != 3 {
		t.Fatalf("audit entries = %d, want 3", len(h.audits))
	}
	for _, entry := range h.audits {
		value, _ := json.Marshal(entry.NewValue)
		if entry.Action != "render" || entry.Resource != "payslip" || entry.UserID != workerTestActorID || !strings.Contains(string(value), `"status":"ready"`) {
			t.Fatalf("unexpected audit entry %+v %s", entry, value)
		}
		if strings.Contains(string(value), "DOCX") || strings.Contains(string(value), "body") {
			t.Fatalf("audit holds document content: %s", value)
		}
	}
}

func TestDocumentWorkerWithoutConverterMarksFailed(t *testing.T) {
	h := newWorkerHarness(t, false)
	h.addRow(docA, &fakeRow{parts: []DocumentPart{slipPart("a")}})

	h.worker.ProcessTenantBatch(context.Background(), workerTestTenant, jobsFor("payslip", docA))

	row := h.handler.row(docA)
	if row.status != "failed" || row.failure != DocumentRenderFailedUnavailable {
		t.Fatalf("row = %s / %q, want failed / %q", row.status, row.failure, DocumentRenderFailedUnavailable)
	}
	if h.converter.callCount() != 0 {
		t.Fatal("converter called although unavailable")
	}
	if files := storedFiles(t, h.uploads); len(files) != 0 {
		t.Fatalf("files written without a converter: %v", files)
	}
	if h.worker.PDFAvailable() {
		t.Fatal("PDFAvailable must be false")
	}
}

func TestDocumentWorkerFallsBackToSingleConversions(t *testing.T) {
	h := newWorkerHarness(t, true)
	h.addRow(docA, &fakeRow{parts: []DocumentPart{slipPart("a")}})
	h.addRow(docB, &fakeRow{parts: []DocumentPart{slipPart("BAD")}})
	h.addRow(docC, &fakeRow{parts: []DocumentPart{slipPart("c")}})

	h.worker.ProcessTenantBatch(context.Background(), workerTestTenant, jobsFor("payslip", docA, docB, docC))

	if calls := h.converter.callCount(); calls != 4 {
		t.Fatalf("soffice calls = %d, want 1 batch + 3 single retries", calls)
	}
	if row := h.handler.row(docB); row.status != "failed" || row.failure != documentRenderFailedConvert {
		t.Fatalf("bad document = %s / %q", row.status, row.failure)
	}
	for _, id := range []string{docA, docC} {
		if row := h.handler.row(id); row.status != "ready" {
			t.Fatalf("%s = %s / %q, want ready", id, row.status, row.failure)
		}
	}
	if files := storedFiles(t, h.uploads); len(files) != 2 {
		t.Fatalf("stored files = %v, want 2", files)
	}
}

func TestDocumentWorkerRenderErrorFailsOnlyThatDocument(t *testing.T) {
	h := newWorkerHarness(t, true)
	h.addRow(docA, &fakeRow{parts: []DocumentPart{{Name: "slip", Payload: docgen.Payload{"fail": "total_gaji"}}}})
	h.addRow(docB, &fakeRow{parts: []DocumentPart{slipPart("b")}})

	h.worker.ProcessTenantBatch(context.Background(), workerTestTenant, jobsFor("payslip", docA, docB))

	if row := h.handler.row(docA); row.status != "failed" || row.failure != documentRenderFailedTemplate {
		t.Fatalf("docA = %s / %q", row.status, row.failure)
	}
	if row := h.handler.row(docB); row.status != "ready" {
		t.Fatalf("docB = %s / %q", row.status, row.failure)
	}
	if got := len(h.converter.calls[0]); got != 1 {
		t.Fatalf("converted %d documents, want only the renderable one", got)
	}
	var failed bool
	for _, entry := range h.audits {
		value, _ := json.Marshal(entry.NewValue)
		if entry.ResourceID == docA {
			failed = strings.Contains(string(value), `"status":"failed"`)
		}
	}
	if !failed {
		t.Fatal("expected a failed render audit entry")
	}
}

func TestDocumentWorkerDiscardsSupersededResult(t *testing.T) {
	h := newWorkerHarness(t, true)
	h.addRow(docA, &fakeRow{parts: []DocumentPart{slipPart("a")}, supersede: true})

	h.worker.ProcessTenantBatch(context.Background(), workerTestTenant, jobsFor("payslip", docA))

	if row := h.handler.row(docA); row.completions != 1 || row.status != "pending" {
		t.Fatalf("row = %+v", row)
	}
	if files := storedFiles(t, h.uploads); len(files) != 0 {
		t.Fatalf("superseded render left files behind: %v", files)
	}
	if len(h.audits) != 0 {
		t.Fatalf("a discarded render must not be audited as ready: %+v", h.audits)
	}
}

func TestDocumentWorkerPrepareOutcomes(t *testing.T) {
	h := newWorkerHarness(t, true)
	h.addRow(docA, &fakeRow{status: "ready", parts: []DocumentPart{slipPart("a")}})
	h.addRow(docB, &fakeRow{parts: []DocumentPart{slipPart("b")}, prepareErr: errors.New("decrypt failed")})

	h.worker.ProcessTenantBatch(context.Background(), workerTestTenant,
		append(jobsFor("payslip", docA, docB), DocumentJob{Tenant: workerTestTenant, Kind: "unknown", ID: docC}))

	if row := h.handler.row(docA); row.status != "ready" || row.completions != 0 {
		t.Fatalf("skipped row was processed: %+v", row)
	}
	row := h.handler.row(docB)
	if row.status != "failed" || row.failure != documentRenderFailedPrepare {
		t.Fatalf("prepare error = %s / %q", row.status, row.failure)
	}
	if len(h.handler.failClaims) != 1 || h.handler.failClaims[0] != "" {
		t.Fatalf("Fail after a prepare error must get an empty claim, got %q", h.handler.failClaims)
	}
	if h.converter.callCount() != 0 {
		t.Fatal("nothing to convert, converter must not be called")
	}
	if held := h.conns.held.Load(); held != 0 {
		t.Fatalf("%d connection(s) leaked", held)
	}
}

func TestDocumentWorkerConnectionFailureLeavesRowsPending(t *testing.T) {
	h := newWorkerHarness(t, true)
	h.addRow(docA, &fakeRow{parts: []DocumentPart{slipPart("a")}})
	h.conns.failNext.Store(true)

	h.worker.ProcessTenantBatch(context.Background(), workerTestTenant, jobsFor("payslip", docA))

	if row := h.handler.row(docA); row.status != "pending" {
		t.Fatalf("row = %s, want pending for the next sweep", row.status)
	}
}

func TestDocumentWorkerSweepEnqueuesAndRunDrains(t *testing.T) {
	h := newWorkerHarness(t, true)
	h.addRow(docA, &fakeRow{parts: []DocumentPart{slipPart("a")}})
	h.addRow(docB, &fakeRow{parts: []DocumentPart{slipPart("b")}})
	h.addRow(docC, &fakeRow{status: "ready", parts: []DocumentPart{slipPart("c")}})

	now := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	sweepCtx, release, _ := h.conns.connect(context.Background(), workerTestTenantID)
	if err := h.worker.Sweep(sweepCtx, workerTestTenant, now); err != nil {
		t.Fatal(err)
	}
	// A second sweep before the worker ran must not queue duplicates.
	if err := h.worker.Sweep(sweepCtx, workerTestTenant, now); err != nil {
		t.Fatal(err)
	}
	release()
	if want := now.Add(-5 * time.Minute); !h.handler.staleBefore.Equal(want) {
		t.Fatalf("staleBefore = %s, want %s", h.handler.staleBefore, want)
	}
	if got := len(h.worker.queue); got != 2 {
		t.Fatalf("queued jobs = %d, want 2", got)
	}

	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		h.worker.Run(ctx)
		close(stopped)
	}()
	for i := 0; i < 2; i++ {
		select {
		case <-h.handler.done:
		case <-time.After(5 * time.Second):
			t.Fatal("worker did not process the queued jobs")
		}
	}
	cancel()
	<-stopped

	if calls := h.converter.callCount(); calls != 1 {
		t.Fatalf("soffice calls = %d, want 1 (queued jobs drained into one batch)", calls)
	}
	for _, id := range []string{docA, docB} {
		if row := h.handler.row(id); row.status != "ready" {
			t.Fatalf("%s = %s", id, row.status)
		}
	}
}

func TestDocumentWorkerRegisterRejectsDuplicatesAndBadKinds(t *testing.T) {
	h := newWorkerHarness(t, true)
	if err := h.worker.Register(newFakeRenderHandler(t, "payslip")); err == nil {
		t.Fatal("duplicate kind accepted")
	}
	if err := h.worker.Register(newFakeRenderHandler(t, "../x")); err == nil {
		t.Fatal("invalid kind accepted")
	}
}

// The default render path uses the embedded templates.
func TestDocumentWorkerRendersEmbeddedTemplate(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "docgen", "testdata", "kamus_variabel.json"))
	if err != nil {
		t.Fatalf("read kamus: %v", err)
	}
	var kamus struct {
		Templates map[string]struct {
			ContohPayload map[string]any `json:"contoh_payload"`
		} `json:"templates"`
	}
	if err := json.Unmarshal(raw, &kamus); err != nil {
		t.Fatal(err)
	}
	payload := docgen.Payload(kamus.Templates[string(docgen.TemplateSlip)].ContohPayload)

	docx, err := renderDocumentPart(DocumentPart{Name: "slip", Template: docgen.TemplateSlip, Payload: payload, Options: []docgen.Option{docgen.WithLogo(nil)}})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(docx), int64(len(docx)))
	if err != nil {
		t.Fatalf("rendered slip is not a zip: %v", err)
	}
	found := false
	for _, f := range zr.File {
		found = found || f.Name == "word/document.xml"
	}
	if !found {
		t.Fatal("rendered slip has no word/document.xml")
	}

	delete(payload, "total_gaji")
	if _, err := renderDocumentPart(DocumentPart{Name: "slip", Template: docgen.TemplateSlip, Payload: payload}); !errors.Is(err, docgen.ErrRender) {
		t.Fatalf("missing variable: err = %v, want ErrRender", err)
	}
}

func TestDocumentStorePathsAndIntegrity(t *testing.T) {
	encrypter, err := security.NewEncrypter("worker-test-secret-0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	uploads := t.TempDir()
	store := NewDocumentStore(uploads, encrypter)

	if _, err := store.Write(workerTestTenantID, "payslip", docA, "slip", []byte("not a pdf")); err == nil {
		t.Fatal("non-PDF accepted")
	}
	for _, bad := range []struct{ tenant, kind, id, part string }{
		{"../x", "payslip", docA, "slip"},
		{workerTestTenantID, "../payslip", docA, "slip"},
		{workerTestTenantID, "payslip", "x/../y", "slip"},
		{workerTestTenantID, "payslip", docA, "Slip"},
	} {
		if _, err := store.Write(bad.tenant, bad.kind, bad.id, bad.part, []byte("%PDF-1")); !errors.Is(err, ErrDocumentPathInvalid) {
			t.Fatalf("Write(%+v) err = %v, want ErrDocumentPathInvalid", bad, err)
		}
	}

	first, err := store.Write(workerTestTenantID, "contract", docA, "pkwt", []byte("%PDF-1 one"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Write(workerTestTenantID, "contract", docA, "pkwt", []byte("%PDF-1 two"))
	if err != nil {
		t.Fatal(err)
	}
	if first.Path == second.Path {
		t.Fatal("every render must get its own file")
	}

	if _, err := store.Read(workerTestTenantID, first.Path, second.SHA256); !errors.Is(err, ErrDocumentHashMismatch) {
		t.Fatalf("sha mismatch err = %v", err)
	}
	if pdf, err := store.Read(workerTestTenantID, first.Path, strings.ToUpper(first.SHA256)); err != nil || string(pdf) != "%PDF-1 one" {
		t.Fatalf("Read = %q, %v", pdf, err)
	}
	otherTenant := "00000000-0000-0000-0000-00000000000b"
	if _, err := store.Read(otherTenant, first.Path, ""); !errors.Is(err, ErrDocumentPathInvalid) {
		t.Fatalf("cross-tenant read err = %v", err)
	}
	for _, bad := range []string{
		"",
		"/etc/passwd",
		"documents/../../etc/passwd",
		"documents/" + workerTestTenantID + "/contract/../../x.pdf.enc",
		"documents/" + workerTestTenantID + "/contract/x.pdf",
		"branding/" + workerTestTenantID + "/logo.png",
		`documents\` + workerTestTenantID + `\contract\x.pdf.enc`,
	} {
		if _, err := store.Read(workerTestTenantID, bad, ""); !errors.Is(err, ErrDocumentPathInvalid) {
			t.Fatalf("Read(%q) err = %v, want ErrDocumentPathInvalid", bad, err)
		}
		if err := store.Delete(workerTestTenantID, bad); !errors.Is(err, ErrDocumentPathInvalid) {
			t.Fatalf("Delete(%q) err = %v, want ErrDocumentPathInvalid", bad, err)
		}
	}

	if err := store.Delete(workerTestTenantID, first.Path); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(workerTestTenantID, first.Path); err != nil {
		t.Fatalf("deleting a missing file must be a no-op: %v", err)
	}
	if _, err := store.Read(workerTestTenantID, first.Path, ""); !errors.Is(err, ErrDocumentNotFound) {
		t.Fatalf("Read deleted err = %v", err)
	}
}

// With SOFFICE_BIN set, the worker drives the real converter: two payslips
// in one soffice call, stored encrypted, each a one-page PDF.
func TestDocumentWorkerWithRealConverter(t *testing.T) {
	// Opt-in (a real conversion takes 10-20 s): the server auto-detects
	// LibreOffice, the tests only run it when SOFFICE_BIN names it.
	bin := os.Getenv("SOFFICE_BIN")
	if bin == "" || config.IsOffValue(bin) {
		t.Skip("SOFFICE_BIN not set")
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "docgen", "testdata", "kamus_variabel.json"))
	if err != nil {
		t.Fatal(err)
	}
	var kamus struct {
		Templates map[string]struct {
			ContohPayload map[string]any `json:"contoh_payload"`
		} `json:"templates"`
	}
	if err := json.Unmarshal(raw, &kamus); err != nil {
		t.Fatal(err)
	}

	h := newWorkerHarness(t, true)
	h.worker.render = renderDocumentPart
	converter := docgen.NewConverter(docgen.ConverterConfig{Bin: bin, ProfileDir: os.Getenv("DOCUMENT_LO_PROFILE_DIR"), Timeout: 3 * time.Minute})
	h.worker.converter = converter
	for _, id := range []string{docA, docB} {
		payload := docgen.Payload(kamus.Templates[string(docgen.TemplateSlip)].ContohPayload)
		h.addRow(id, &fakeRow{parts: []DocumentPart{{Name: "slip", Template: docgen.TemplateSlip, Payload: payload, Options: []docgen.Option{docgen.WithLogo(nil)}}}})
	}

	h.worker.ProcessTenantBatch(context.Background(), workerTestTenant, jobsFor("payslip", docA, docB))

	pageRe := regexp.MustCompile(`/Type\s*/Page[^s]`)
	for _, id := range []string{docA, docB} {
		row := h.handler.row(id)
		if row.status != "ready" || len(row.files) != 1 {
			t.Fatalf("%s = %s / %q", id, row.status, row.failure)
		}
		pdf, err := h.store.Read(workerTestTenantID, row.files[0].Path, row.files[0].SHA256)
		if err != nil {
			t.Fatal(err)
		}
		if pages := len(pageRe.FindAll(pdf, -1)); pages != 1 {
			t.Fatalf("%s: %d pages, want 1", id, pages)
		}
	}
}

// TestDocumentWorkerReleasesConnectionWhenHandlerPanics: processQueued
// recovers a handler panic; the tenant connection taken to record the
// outcome must still go back to the pool.
func TestDocumentWorkerReleasesConnectionWhenHandlerPanics(t *testing.T) {
	h := newWorkerHarness(t, true)
	for i, id := range []string{docA, docB, docC} {
		h.addRow(id, &fakeRow{parts: []DocumentPart{{Name: "slip", Template: docgen.TemplateSlip, Payload: docgen.Payload{"fail": fmt.Sprint("x", i)}}}, panicOnFail: true})
		h.worker.processQueued(context.Background(), jobsFor("payslip", id))
	}
	if held := h.conns.held.Load(); held != 0 {
		t.Fatalf("%d tenant connection(s) leaked after panicking handlers", held)
	}
	if acquired := h.conns.acquired.Load(); acquired != 6 {
		t.Fatalf("acquired = %d, want 6 (prepare + outcome per batch)", acquired)
	}
}
