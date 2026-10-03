package hris

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sort"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kana-consultant/kantor/backend/internal/docgen"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	repository "github.com/kana-consultant/kantor/backend/internal/repository"
	auditservice "github.com/kana-consultant/kantor/backend/internal/service/audit"
	"github.com/kana-consultant/kantor/backend/internal/tenant"
)

// Document worker: renders queued documents (payslips, contracts) to PDF in
// the background.
//
//   - Queue: an in-process channel of {tenant, kind, id}. Generation enqueues
//     right away; a startup + per-minute sweep (Sweep, driven by the
//     per-tenant ticker in app.go) re-enqueues rows still 'pending', or stuck
//     in 'rendering' for more than 5 minutes (e.g. after a crash).
//   - Batch: the worker waits documentBatchGather after the first job, then
//     drains every job queued by then, and all documents of one tenant go
//     into a single soffice call.
//   - Connections: a tenant connection is acquired to claim and load the
//     snapshots, released, and acquired again only to record the outcome.
//     Rendering, the (slow) PDF conversion and file writes hold no DB
//     connection.
//   - Storage: PDFs are encrypted (DocumentStore) and the superseded file is
//     deleted once the row points at the new one.
//
// The worker knows nothing about payslips or contracts: each document kind
// registers a DocumentRenderHandler.

const (
	documentRenderStaleAfter = 5 * time.Minute
	documentQueueCapacity    = 1024
	// documentBatchGather is how long the worker waits after the first job
	// for the rest of a selection (Generate queues every slip at once, a
	// sweep a whole tenant) so they share one soffice call.
	documentBatchGather = 400 * time.Millisecond
	// maxDocumentsPerConversion caps one soffice call (the converter adds a
	// few seconds of timeout per extra document).
	maxDocumentsPerConversion = 40

	DocumentRenderFailedUnavailable = "Konverter PDF belum tersedia"
	documentRenderFailedPrepare     = "Gagal memuat data dokumen"
	documentRenderFailedTemplate    = "Gagal menyusun dokumen dari template"
	documentRenderFailedConvert     = "Gagal mengonversi dokumen ke PDF"
	documentRenderFailedStore       = "Gagal menyimpan PDF"
)

var (
	// ErrDocumentJobSkipped is returned by Prepare when the row no longer
	// needs rendering (not pending, freshly claimed elsewhere, deleted).
	ErrDocumentJobSkipped = errors.New("document job skipped")
	// ErrDocumentJobSuperseded is returned by Complete when the row changed
	// after it was claimed (e.g. edited and re-queued); the new files are
	// discarded and the newer render wins.
	ErrDocumentJobSuperseded = errors.New("document job superseded")
)

// DocumentJob identifies one document row to render.
type DocumentJob struct {
	Tenant tenant.Info
	Kind   string
	ID     string
}

// DocumentPart is one file of a document (a payslip has "slip"; a contract
// has "pkwt" and "nda"). Name must match ^[a-z][a-z0-9_]*$.
type DocumentPart struct {
	Name     string
	Template docgen.TemplateID
	Payload  docgen.Payload
	Options  []docgen.Option
}

// PreparedDocument is what Prepare loads for one row. Claim is opaque to the
// worker and handed back to Complete / Fail so the handler can tell whether
// the row is still the one it claimed. ActorID (the user who generated the
// document) is used for the audit entry; empty skips it.
type PreparedDocument struct {
	Claim   string
	ActorID string
	Parts   []DocumentPart
}

// DocumentRenderHandler plugs a document kind into the worker. Prepare,
// Complete and Fail run with a tenant-scoped connection (repository.DB) and
// the tenant (tenant.FromContext) in ctx, so tenant-aware services such as
// CompanyProfileService.LogoPNG work from a handler. ListPending runs with
// the sweep's ctx.
type DocumentRenderHandler interface {
	// Kind is the queue key and the storage directory (e.g. "payslip").
	Kind() string
	// ListPending returns ids of rows with render_status 'pending', or
	// 'rendering' last claimed before staleBefore.
	ListPending(ctx context.Context, staleBefore time.Time) ([]string, error)
	// Prepare claims the row (sets 'rendering'), loads and decrypts its
	// snapshot and returns the parts to render. ErrDocumentJobSkipped when
	// there is nothing to do.
	Prepare(ctx context.Context, id string) (PreparedDocument, error)
	// Complete records the stored PDFs (paths + sha256) and 'ready' if the
	// row still matches claim, and returns the paths of the files it
	// replaced (deleted by the worker). ErrDocumentJobSuperseded when the
	// row moved on.
	Complete(ctx context.Context, id string, claim string, files []StoredDocument) (superseded []string, err error)
	// Fail records 'failed' with a user-facing reason if the row still
	// matches claim. claim is empty when Prepare itself failed.
	Fail(ctx context.Context, id string, claim string, reason string) error
}

// DocumentConverter is implemented by *docgen.Converter.
type DocumentConverter interface {
	Available() bool
	ConvertBatch(ctx context.Context, docs [][]byte) ([][]byte, error)
}

// documentStorage is implemented by *DocumentStore.
type documentStorage interface {
	Write(tenantID string, kind string, id string, part string, pdf []byte) (StoredDocument, error)
	Delete(tenantID string, rel string) error
}

// tenantConnector returns ctx carrying a tenant-scoped connection and the
// function that releases it.
type tenantConnector func(ctx context.Context, tenantID string) (context.Context, func(), error)

type documentAuditor func(ctx context.Context, entry auditservice.Entry)

type documentJobKey struct {
	tenantID string
	kind     string
	id       string
}

type DocumentWorker struct {
	converter DocumentConverter
	store     documentStorage
	connect   tenantConnector
	audit     documentAuditor
	now       func() time.Time
	render    func(part DocumentPart) ([]byte, error)
	gather    time.Duration

	queue chan DocumentJob

	mu       sync.Mutex
	handlers map[string]DocumentRenderHandler
	queued   map[documentJobKey]struct{}
}

// NewDocumentWorker wires the worker with the pool (tenant connections via
// AcquireTenantConn), the audit service, the PDF converter and the encrypted
// store. Handlers are added with Register before Run.
func NewDocumentWorker(pool *pgxpool.Pool, audit *auditservice.Service, converter DocumentConverter, store *DocumentStore) *DocumentWorker {
	worker := newDocumentWorker(converter, store, poolConnector(pool), nil)
	if audit != nil {
		worker.audit = audit.Log
	}
	return worker
}

func newDocumentWorker(converter DocumentConverter, store documentStorage, connect tenantConnector, audit documentAuditor) *DocumentWorker {
	return &DocumentWorker{
		converter: converter,
		store:     store,
		connect:   connect,
		audit:     audit,
		now:       time.Now,
		render:    renderDocumentPart,
		gather:    documentBatchGather,
		queue:     make(chan DocumentJob, documentQueueCapacity),
		handlers:  map[string]DocumentRenderHandler{},
		queued:    map[documentJobKey]struct{}{},
	}
}

func poolConnector(pool *pgxpool.Pool) tenantConnector {
	return func(ctx context.Context, tenantID string) (context.Context, func(), error) {
		if pool == nil {
			return nil, nil, errors.New("document worker: database pool is not configured")
		}
		conn, err := platformmiddleware.AcquireTenantConn(ctx, pool, tenantID)
		if err != nil {
			return nil, nil, err
		}
		return repository.WithConn(ctx, conn), func() { platformmiddleware.ReleaseTenantConn(conn) }, nil
	}
}

func renderDocumentPart(part DocumentPart) ([]byte, error) {
	tpl, err := docgen.Load(part.Template)
	if err != nil {
		return nil, err
	}
	return docgen.Render(tpl, part.Payload, part.Options...)
}

// Register adds the handler for a document kind (Phase 3: "payslip",
// Phase 4: "contract"). A kind can be registered once.
func (w *DocumentWorker) Register(handler DocumentRenderHandler) error {
	kind := handler.Kind()
	if !documentNamePattern.MatchString(kind) {
		return fmt.Errorf("document worker: invalid kind %q", kind)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, exists := w.handlers[kind]; exists {
		return fmt.Errorf("document worker: kind %q already registered", kind)
	}
	w.handlers[kind] = handler
	return nil
}

// PDFAvailable reports whether PDFs can be produced (a soffice binary was
// resolved: SOFFICE_BIN or auto-detected).
func (w *DocumentWorker) PDFAvailable() bool {
	return w.converter != nil && w.converter.Available()
}

// Enqueue queues a render without blocking. A job already waiting is not
// queued twice; when the queue is full the job is dropped and picked up by
// the next sweep (the row is still 'pending').
func (w *DocumentWorker) Enqueue(job DocumentJob) bool {
	key := documentJobKey{tenantID: job.Tenant.ID, kind: job.Kind, id: job.ID}
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, waiting := w.queued[key]; waiting {
		return true
	}
	select {
	case w.queue <- job:
		w.queued[key] = struct{}{}
		return true
	default:
		slog.Warn("document render queue is full; the sweep will retry", "kind", job.Kind, "id", job.ID, "tenant", job.Tenant.Slug)
		return false
	}
}

// Sweep enqueues every row of tenant t that still needs rendering. It runs
// with the tenant connection of the per-tenant ticker and never fails the
// ticker: errors are logged per kind.
func (w *DocumentWorker) Sweep(ctx context.Context, t tenant.Info, now time.Time) error {
	for _, handler := range w.sortedHandlers() {
		ids, err := handler.ListPending(ctx, now.Add(-documentRenderStaleAfter))
		if err != nil {
			slog.ErrorContext(ctx, "document render sweep failed", "kind", handler.Kind(), "tenant", t.Slug, "error", err)
			continue
		}
		for _, id := range ids {
			w.Enqueue(DocumentJob{Tenant: t, Kind: handler.Kind(), ID: id})
		}
	}
	return nil
}

// Run processes the queue until ctx is cancelled.
func (w *DocumentWorker) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-w.queue:
			batch := []DocumentJob{job}
			if w.gather > 0 {
				timer := time.NewTimer(w.gather)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
			}
		drain:
			for {
				select {
				case next := <-w.queue:
					batch = append(batch, next)
				default:
					break drain
				}
			}
			w.processQueued(ctx, batch)
		}
	}
}

// processQueued releases the dedupe keys (a job re-queued while it renders is
// handled by the claim) and processes each tenant's jobs as one batch.
func (w *DocumentWorker) processQueued(ctx context.Context, batch []DocumentJob) {
	w.mu.Lock()
	for _, job := range batch {
		delete(w.queued, documentJobKey{tenantID: job.Tenant.ID, kind: job.Kind, id: job.ID})
	}
	w.mu.Unlock()

	order := []string{}
	byTenant := map[string][]DocumentJob{}
	for _, job := range batch {
		if _, seen := byTenant[job.Tenant.ID]; !seen {
			order = append(order, job.Tenant.ID)
		}
		byTenant[job.Tenant.ID] = append(byTenant[job.Tenant.ID], job)
	}
	for _, tenantID := range order {
		jobs := byTenant[tenantID]
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.ErrorContext(ctx, "document render batch panicked", "tenant", jobs[0].Tenant.Slug, "panic", r, "stack", string(debug.Stack()))
				}
			}()
			w.ProcessTenantBatch(ctx, jobs[0].Tenant, jobs)
		}()
	}
}

type documentRenderState struct {
	job      DocumentJob
	handler  DocumentRenderHandler
	prepared PreparedDocument
	docx     [][]byte
	pdfs     [][]byte
	files    []StoredDocument
	failure  string
}

// ProcessTenantBatch renders jobs (all of tenant t) and records the outcome.
func (w *DocumentWorker) ProcessTenantBatch(ctx context.Context, t tenant.Info, jobs []DocumentJob) {
	states := w.prepareBatch(ctx, t, jobs)
	if len(states) == 0 {
		return
	}

	// No DB connection is held from here until the outcome is recorded.
	for _, state := range states {
		w.renderDocx(ctx, state)
	}
	w.convert(ctx, t, states)
	for _, state := range states {
		w.storePDFs(ctx, t, state)
	}

	w.recordOutcome(ctx, t, states)
}

func (w *DocumentWorker) prepareBatch(ctx context.Context, t tenant.Info, jobs []DocumentJob) []*documentRenderState {
	connCtx, release, err := w.connect(tenant.WithInfo(ctx, t), t.ID)
	if err != nil {
		// Rows stay 'pending'; the next sweep retries.
		slog.ErrorContext(ctx, "document worker: acquire tenant connection", "tenant", t.Slug, "error", err)
		return nil
	}
	defer release()

	seen := map[documentJobKey]bool{}
	states := make([]*documentRenderState, 0, len(jobs))
	for _, job := range jobs {
		key := documentJobKey{tenantID: t.ID, kind: job.Kind, id: job.ID}
		if seen[key] {
			continue
		}
		seen[key] = true

		handler := w.handler(job.Kind)
		if handler == nil {
			slog.WarnContext(ctx, "document worker: no handler for kind", "kind", job.Kind, "id", job.ID)
			continue
		}
		prepared, err := handler.Prepare(connCtx, job.ID)
		if errors.Is(err, ErrDocumentJobSkipped) {
			continue
		}
		if err != nil {
			slog.ErrorContext(ctx, "document worker: prepare failed", "kind", job.Kind, "id", job.ID, "error", err)
			if failErr := handler.Fail(connCtx, job.ID, "", documentRenderFailedPrepare); failErr != nil {
				slog.ErrorContext(ctx, "document worker: record failure", "kind", job.Kind, "id", job.ID, "error", failErr)
			}
			continue
		}
		state := &documentRenderState{job: job, handler: handler, prepared: prepared}
		if len(prepared.Parts) == 0 {
			state.failure = documentRenderFailedTemplate
		}
		for _, part := range prepared.Parts {
			if !documentNamePattern.MatchString(part.Name) {
				state.failure = documentRenderFailedTemplate
			}
		}
		states = append(states, state)
	}
	return states
}

func (w *DocumentWorker) renderDocx(ctx context.Context, state *documentRenderState) {
	if state.failure != "" {
		return
	}
	state.docx = make([][]byte, 0, len(state.prepared.Parts))
	for _, part := range state.prepared.Parts {
		docx, err := w.render(part)
		if err != nil {
			// Render errors name the missing variable, never its value.
			slog.ErrorContext(ctx, "document worker: render failed", "kind", state.job.Kind, "id", state.job.ID, "part", part.Name, "error", err)
			state.failure = documentRenderFailedTemplate
			state.docx = nil
			return
		}
		state.docx = append(state.docx, docx)
	}
}

// convert turns every rendered DOCX into a PDF, all documents of the tenant
// in one soffice call (chunked at maxDocumentsPerConversion). If a
// multi-document call fails, each document set is retried on its own so one
// bad document does not fail the others.
func (w *DocumentWorker) convert(ctx context.Context, t tenant.Info, states []*documentRenderState) {
	pending := make([]*documentRenderState, 0, len(states))
	for _, state := range states {
		if state.failure == "" {
			pending = append(pending, state)
		}
	}
	if len(pending) == 0 {
		return
	}
	if !w.PDFAvailable() {
		for _, state := range pending {
			state.failure = DocumentRenderFailedUnavailable
		}
		return
	}

	for len(pending) > 0 {
		chunk := []*documentRenderState{}
		docs := 0
		for len(pending) > 0 {
			next := pending[0]
			if len(chunk) > 0 && docs+len(next.docx) > maxDocumentsPerConversion {
				break
			}
			chunk = append(chunk, next)
			docs += len(next.docx)
			pending = pending[1:]
		}

		if err := w.convertChunk(ctx, chunk); err != nil {
			if len(chunk) == 1 || ctx.Err() != nil {
				w.failConversion(ctx, t, chunk, err)
				continue
			}
			slog.WarnContext(ctx, "document worker: batch conversion failed; converting one by one", "tenant", t.Slug, "documents", docs, "error", err)
			for _, state := range chunk {
				if err := w.convertChunk(ctx, []*documentRenderState{state}); err != nil {
					w.failConversion(ctx, t, []*documentRenderState{state}, err)
				}
			}
		}
	}
}

func (w *DocumentWorker) convertChunk(ctx context.Context, chunk []*documentRenderState) error {
	var docs [][]byte
	for _, state := range chunk {
		docs = append(docs, state.docx...)
	}
	pdfs, err := w.converter.ConvertBatch(ctx, docs)
	if err != nil {
		return err
	}
	if len(pdfs) != len(docs) {
		return fmt.Errorf("converter returned %d PDFs for %d documents", len(pdfs), len(docs))
	}
	offset := 0
	for _, state := range chunk {
		state.pdfs = pdfs[offset : offset+len(state.docx)]
		offset += len(state.docx)
	}
	return nil
}

func (w *DocumentWorker) failConversion(ctx context.Context, t tenant.Info, chunk []*documentRenderState, err error) {
	for _, state := range chunk {
		slog.ErrorContext(ctx, "document worker: PDF conversion failed", "tenant", t.Slug, "kind", state.job.Kind, "id", state.job.ID, "error", err)
		state.failure = documentRenderFailedConvert
	}
}

func (w *DocumentWorker) storePDFs(ctx context.Context, t tenant.Info, state *documentRenderState) {
	if state.failure != "" {
		return
	}
	for i, part := range state.prepared.Parts {
		file, err := w.store.Write(t.ID, state.job.Kind, state.job.ID, part.Name, state.pdfs[i])
		if err != nil {
			slog.ErrorContext(ctx, "document worker: store PDF failed", "kind", state.job.Kind, "id", state.job.ID, "part", part.Name, "error", err)
			w.deleteFiles(ctx, t, pathsOf(state.files))
			state.files = nil
			state.failure = documentRenderFailedStore
			return
		}
		file.Pages = docgen.PDFPageCount(state.pdfs[i])
		state.files = append(state.files, file)
	}
}

func (w *DocumentWorker) recordOutcome(ctx context.Context, t tenant.Info, states []*documentRenderState) {
	connCtx, release, err := w.connect(tenant.WithInfo(ctx, t), t.ID)
	if err != nil {
		// Rows stay 'rendering' and are retried by the stale sweep; the new
		// files are not referenced by anything.
		slog.ErrorContext(ctx, "document worker: acquire tenant connection to record outcome", "tenant", t.Slug, "error", err)
		for _, state := range states {
			w.deleteFiles(ctx, t, pathsOf(state.files))
		}
		return
	}

	// The connection is released before the superseded files are deleted,
	// and also when a handler or the audit hook panics (processQueued
	// recovers the panic; a leaked pooled connection would not come back).
	superseded := func() []string {
		defer release()
		var superseded []string
		for _, state := range states {
			job, handler, claim := state.job, state.handler, state.prepared.Claim
			if state.failure != "" {
				if err := handler.Fail(connCtx, job.ID, claim, state.failure); err != nil {
					slog.ErrorContext(ctx, "document worker: record failure", "kind", job.Kind, "id", job.ID, "error", err)
					continue
				}
				w.auditOutcome(connCtx, state, "failed")
				continue
			}

			replaced, err := handler.Complete(connCtx, job.ID, claim, state.files)
			if err != nil {
				if !errors.Is(err, ErrDocumentJobSuperseded) {
					slog.ErrorContext(ctx, "document worker: record result", "kind", job.Kind, "id", job.ID, "error", err)
				}
				w.deleteFiles(ctx, t, pathsOf(state.files))
				continue
			}
			superseded = append(superseded, replaced...)
			w.auditOutcome(connCtx, state, "ready")
		}
		return superseded
	}()

	w.deleteFiles(ctx, t, superseded)
}

// auditOutcome writes metadata only: kind, id, status, parts and format.
func (w *DocumentWorker) auditOutcome(ctx context.Context, state *documentRenderState, status string) {
	if w.audit == nil || state.prepared.ActorID == "" {
		return
	}
	parts := make([]string, 0, len(state.prepared.Parts))
	for _, part := range state.prepared.Parts {
		parts = append(parts, part.Name)
	}
	value := map[string]any{
		"status": status,
		"parts":  parts,
		"format": "pdf",
	}
	if state.failure != "" {
		value["reason"] = state.failure
	}
	w.audit(ctx, auditservice.Entry{
		UserID:     state.prepared.ActorID,
		Action:     "render",
		Module:     "hris",
		Resource:   state.job.Kind,
		ResourceID: state.job.ID,
		NewValue:   value,
	})
}

func (w *DocumentWorker) deleteFiles(ctx context.Context, t tenant.Info, paths []string) {
	for _, path := range paths {
		if path == "" {
			continue
		}
		if err := w.store.Delete(t.ID, path); err != nil {
			slog.WarnContext(ctx, "document worker: delete stored PDF failed", "tenant", t.Slug, "error", err)
		}
	}
}

func (w *DocumentWorker) handler(kind string) DocumentRenderHandler {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.handlers[kind]
}

func (w *DocumentWorker) sortedHandlers() []DocumentRenderHandler {
	w.mu.Lock()
	defer w.mu.Unlock()
	kinds := make([]string, 0, len(w.handlers))
	for kind := range w.handlers {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	handlers := make([]DocumentRenderHandler, 0, len(kinds))
	for _, kind := range kinds {
		handlers = append(handlers, w.handlers[kind])
	}
	return handlers
}

func pathsOf(files []StoredDocument) []string {
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.Path)
	}
	return paths
}
