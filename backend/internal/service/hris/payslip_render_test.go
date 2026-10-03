package hris

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/docgen"
	"github.com/kana-consultant/kantor/backend/internal/model"
	hrisrepo "github.com/kana-consultant/kantor/backend/internal/repository/hris"
	"github.com/kana-consultant/kantor/backend/internal/security"
)

type fakeRenderRepo struct {
	slip      model.Payslip
	claimable bool
	claim     string
	completed string
	previous  string
	pageCodes []string
	pageWarns []model.PayslipWarning
	failed    string
	stale     bool
}

func (r *fakeRenderRepo) ListRenderPending(context.Context, time.Time) ([]string, error) {
	return []string{r.slip.ID}, nil
}

func (r *fakeRenderRepo) ClaimRender(context.Context, string, time.Time) (string, model.Payslip, bool, error) {
	if !r.claimable {
		return "", model.Payslip{}, false, nil
	}
	r.claim = "claim-1"
	return r.claim, r.slip, true, nil
}

func (r *fakeRenderRepo) CompleteRender(_ context.Context, _ string, claim string, path string, _ string, version string, pageCodes []string, pageWarnings []model.PayslipWarning) (string, error) {
	if r.stale || claim != r.claim {
		return "", hrisrepo.ErrPayslipStateChanged
	}
	r.pageCodes, r.pageWarns = pageCodes, pageWarnings
	if version == "" {
		return "", errors.New("template version missing")
	}
	r.completed = path
	return r.previous, nil
}

func (r *fakeRenderRepo) FailRender(_ context.Context, _ string, _ string, reason string) error {
	r.failed = reason
	return nil
}

func TestPayslipRenderHandler(t *testing.T) {
	encrypter, err := security.NewEncrypter("render-test-secret-0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(docgen.Payload{"karyawan_nama": "Budi"})
	cipher, _ := encrypter.EncryptString(string(raw))
	actor := "actor-1"
	repo := &fakeRenderRepo{slip: model.Payslip{ID: "p1", PayloadEncrypted: cipher, GeneratedBy: &actor}, claimable: true, previous: "documents/t/payslip/old.pdf.enc"}
	handler := NewPayslipRenderHandler(repo, encrypter, fakePayslipCompany{})

	if handler.Kind() != "payslip" {
		t.Fatalf("kind = %s", handler.Kind())
	}
	prepared, err := handler.Prepare(context.Background(), "p1")
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Claim != "claim-1" || prepared.ActorID != actor || len(prepared.Parts) != 1 {
		t.Fatalf("prepared = %+v", prepared)
	}
	part := prepared.Parts[0]
	if part.Name != "slip" || part.Template != docgen.TemplateSlip || part.Payload["karyawan_nama"] != "Budi" || len(part.Options) != 1 {
		t.Fatalf("part = %+v", part)
	}

	superseded, err := handler.Complete(context.Background(), "p1", prepared.Claim, []StoredDocument{{Part: "slip", Path: "documents/t/payslip/new.pdf.enc", SHA256: "x", Pages: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if len(superseded) != 1 || superseded[0] != repo.previous || repo.completed != "documents/t/payslip/new.pdf.enc" {
		t.Fatalf("superseded = %v completed = %s", superseded, repo.completed)
	}
	// One page clears the estimate and records nothing.
	if len(repo.pageCodes) != 2 || len(repo.pageWarns) != 0 {
		t.Fatalf("one page: codes %v warnings %v", repo.pageCodes, repo.pageWarns)
	}
	// Two pages record a pdf_pages warning.
	if _, err := handler.Complete(context.Background(), "p1", prepared.Claim, []StoredDocument{{Part: "slip", Path: "p2", SHA256: "y", Pages: 2}}); err != nil {
		t.Fatal(err)
	}
	if len(repo.pageWarns) != 1 || repo.pageWarns[0].Code != PayslipWarnPDFPages || !strings.Contains(repo.pageWarns[0].Message, "2 halaman") {
		t.Fatalf("two pages: %v", repo.pageWarns)
	}
	// An unknown count keeps the estimate.
	if _, err := handler.Complete(context.Background(), "p1", prepared.Claim, []StoredDocument{{Part: "slip", Path: "p3", SHA256: "z"}}); err != nil {
		t.Fatal(err)
	}
	if len(repo.pageCodes) != 1 || repo.pageCodes[0] != PayslipWarnPDFPages || len(repo.pageWarns) != 0 {
		t.Fatalf("unknown pages: codes %v warnings %v", repo.pageCodes, repo.pageWarns)
	}

	repo.stale = true
	if _, err := handler.Complete(context.Background(), "p1", prepared.Claim, []StoredDocument{{Part: "slip", Path: "p"}}); !errors.Is(err, ErrDocumentJobSuperseded) {
		t.Fatalf("stale complete err = %v", err)
	}

	repo.claimable = false
	if _, err := handler.Prepare(context.Background(), "p1"); !errors.Is(err, ErrDocumentJobSkipped) {
		t.Fatalf("unclaimable prepare err = %v", err)
	}

	// A snapshot that cannot be decrypted fails the row under its claim.
	repo.claimable = true
	repo.slip.PayloadEncrypted = "garbage"
	if _, err := handler.Prepare(context.Background(), "p1"); !errors.Is(err, ErrDocumentJobSkipped) || repo.failed == "" {
		t.Fatalf("broken snapshot: err = %v failed = %q", err, repo.failed)
	}
}
