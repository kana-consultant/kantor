package hris

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/docgen"
	"github.com/kana-consultant/kantor/backend/internal/model"
	hrisrepo "github.com/kana-consultant/kantor/backend/internal/repository/hris"
	"github.com/kana-consultant/kantor/backend/internal/security"
)

// PayslipRenderHandler plugs payslips into the document worker (kind
// "payslip", one part "slip" rendered from 08_Slip_Gaji with the stored
// processed tenant logo). The claim token in payslips.render_claim makes a
// render that finished after the slip changed (edit, regeneration) lose:
// its file is discarded and the newer render wins.
type PayslipRenderHandler struct {
	repo      payslipRenderRepository
	encrypter *security.Encrypter
	company   payslipLogoSource
	now       func() time.Time
}

type payslipRenderRepository interface {
	ListRenderPending(ctx context.Context, staleBefore time.Time) ([]string, error)
	ClaimRender(ctx context.Context, id string, staleBefore time.Time) (string, model.Payslip, bool, error)
	CompleteRender(ctx context.Context, id string, claim string, pdfPath string, pdfSHA256 string, templateVersion string, pageCodes []string, pageWarnings []model.PayslipWarning) (string, error)
	FailRender(ctx context.Context, id string, claim string, reason string) error
}

type payslipLogoSource interface {
	LogoPNG(ctx context.Context) ([]byte, error)
}

func NewPayslipRenderHandler(repo payslipRenderRepository, encrypter *security.Encrypter, company payslipLogoSource) *PayslipRenderHandler {
	return &PayslipRenderHandler{repo: repo, encrypter: encrypter, company: company, now: time.Now}
}

func (h *PayslipRenderHandler) Kind() string {
	return PayslipDocumentKind
}

func (h *PayslipRenderHandler) ListPending(ctx context.Context, staleBefore time.Time) ([]string, error) {
	return h.repo.ListRenderPending(ctx, staleBefore)
}

func (h *PayslipRenderHandler) Prepare(ctx context.Context, id string) (PreparedDocument, error) {
	claim, slip, ok, err := h.repo.ClaimRender(ctx, id, h.now().Add(-documentRenderStaleAfter))
	if err != nil {
		return PreparedDocument{}, err
	}
	if !ok {
		return PreparedDocument{}, ErrDocumentJobSkipped
	}

	fail := func(cause error) (PreparedDocument, error) {
		// The row is claimed: record the failure under the claim so the
		// worker's claim-less Fail cannot race a newer generation.
		if failErr := h.repo.FailRender(ctx, id, claim, documentRenderFailedPrepare); failErr != nil {
			return PreparedDocument{}, fmt.Errorf("%w (record failure: %v)", cause, failErr)
		}
		return PreparedDocument{}, ErrDocumentJobSkipped
	}

	payload, err := decryptPayslipPayload(h.encrypter, slip)
	if err != nil {
		return fail(err)
	}
	logo, err := tenantLogo(ctx, h.company)
	if err != nil {
		return fail(err)
	}
	actor := ""
	if slip.GeneratedBy != nil {
		actor = *slip.GeneratedBy
	}
	return PreparedDocument{
		Claim:   claim,
		ActorID: actor,
		Parts: []DocumentPart{{
			Name:     payslipDocumentPart,
			Template: docgen.TemplateSlip,
			Payload:  payload,
			Options:  []docgen.Option{docgen.WithLogo(logo)},
		}},
	}, nil
}

func (h *PayslipRenderHandler) Complete(ctx context.Context, id string, claim string, files []StoredDocument) ([]string, error) {
	var slipFile *StoredDocument
	for index := range files {
		if files[index].Part == payslipDocumentPart {
			slipFile = &files[index]
		}
	}
	if slipFile == nil {
		return nil, errors.New("payslip render produced no slip part")
	}
	pageCodes, pageWarnings := payslipPageCheck(slipFile.Pages)
	previous, err := h.repo.CompleteRender(ctx, id, claim, slipFile.Path, slipFile.SHA256, templateVersion(), pageCodes, pageWarnings)
	if errors.Is(err, hrisrepo.ErrPayslipStateChanged) {
		return nil, ErrDocumentJobSuperseded
	}
	if err != nil {
		return nil, err
	}
	if previous == "" {
		return nil, nil
	}
	// The superseded draft PDF is deleted by the worker.
	return []string{previous}, nil
}

// payslipPageCheck is what the page count of the rendered PDF settles: the
// warning codes it replaces (the pre-render many_rows estimate and the page
// count of the previous PDF) and the warning it adds (none for one page).
// An unknown count (0) keeps the estimate.
func payslipPageCheck(pages int) ([]string, []model.PayslipWarning) {
	switch {
	case pages == 1:
		return []string{PayslipWarnManyRows, PayslipWarnPDFPages}, nil
	case pages > 1:
		return []string{PayslipWarnManyRows, PayslipWarnPDFPages}, []model.PayslipWarning{
			warning(PayslipWarnPDFPages, false, "PDF slip %d halaman: kurangi baris penyesuaian atau catatan agar muat 1 halaman", pages),
		}
	default:
		return []string{PayslipWarnPDFPages}, nil
	}
}

func (h *PayslipRenderHandler) Fail(ctx context.Context, id string, claim string, reason string) error {
	return h.repo.FailRender(ctx, id, claim, reason)
}
