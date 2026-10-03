package hris

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/docgen"
	"github.com/kana-consultant/kantor/backend/internal/model"
	hrisrepo "github.com/kana-consultant/kantor/backend/internal/repository/hris"
	"github.com/kana-consultant/kantor/backend/internal/security"
)

// ContractRenderHandler plugs employment contracts into the document worker
// (kind "contract", parts "pkwt" from 01_PKWT and "nda" from 02_NDA). Both
// parts of every pending contract of a tenant go into the worker's single
// soffice batch; the PDFs are stored encrypted and the PDFs of the previous
// render (an earlier revision) are deleted once the row points at the new
// ones. The claim token makes a render that finished after the contract
// changed lose.
type ContractRenderHandler struct {
	repo      contractRenderRepository
	encrypter *security.Encrypter
	now       func() time.Time
}

type contractRenderRepository interface {
	ListRenderPending(ctx context.Context, staleBefore time.Time) ([]string, error)
	ClaimRender(ctx context.Context, id string, staleBefore time.Time) (string, model.EmploymentContract, bool, error)
	CompleteRender(ctx context.Context, id string, claim string, pkwtPath string, ndaPath string, digests []string, templateVersion string) ([]string, error)
	FailRender(ctx context.Context, id string, claim string, reason string) error
}

func NewContractRenderHandler(repo contractRenderRepository, encrypter *security.Encrypter) *ContractRenderHandler {
	return &ContractRenderHandler{repo: repo, encrypter: encrypter, now: time.Now}
}

func (h *ContractRenderHandler) Kind() string {
	return ContractDocumentKind
}

func (h *ContractRenderHandler) ListPending(ctx context.Context, staleBefore time.Time) ([]string, error) {
	return h.repo.ListRenderPending(ctx, staleBefore)
}

func (h *ContractRenderHandler) Prepare(ctx context.Context, id string) (PreparedDocument, error) {
	claim, contract, ok, err := h.repo.ClaimRender(ctx, id, h.now().Add(-documentRenderStaleAfter))
	if err != nil {
		return PreparedDocument{}, err
	}
	if !ok {
		return PreparedDocument{}, ErrDocumentJobSkipped
	}
	payloads, err := decryptContractPayloads(h.encrypter, contract)
	if err != nil {
		// Recorded under the claim so the worker's claim-less Fail cannot
		// race a newer generation.
		if failErr := h.repo.FailRender(ctx, id, claim, documentRenderFailedPrepare); failErr != nil {
			return PreparedDocument{}, fmt.Errorf("%w (record failure: %v)", err, failErr)
		}
		return PreparedDocument{}, ErrDocumentJobSkipped
	}
	actor := ""
	if contract.GeneratedBy != nil {
		actor = *contract.GeneratedBy
	}
	return PreparedDocument{
		Claim:   claim,
		ActorID: actor,
		Parts: []DocumentPart{
			{Name: ContractPartPKWT, Template: docgen.TemplatePKWT, Payload: payloads.PKWT},
			{Name: ContractPartNDA, Template: docgen.TemplateNDA, Payload: payloads.NDA},
		},
	}, nil
}

func (h *ContractRenderHandler) Complete(ctx context.Context, id string, claim string, files []StoredDocument) ([]string, error) {
	var pkwt, nda *StoredDocument
	for index := range files {
		switch files[index].Part {
		case ContractPartPKWT:
			pkwt = &files[index]
		case ContractPartNDA:
			nda = &files[index]
		}
	}
	if pkwt == nil || nda == nil {
		return nil, errors.New("contract render did not produce both the PKWT and the NDA")
	}
	if pkwt.Pages != contractExpectedPages || nda.Pages != contractExpectedPages {
		// Not a failure (a long benefit list or prior works can add a page),
		// but worth noticing in the logs.
		slog.InfoContext(ctx, "contract PDF page count", "contract_id", id, "pkwt_pages", pkwt.Pages, "nda_pages", nda.Pages)
	}
	superseded, err := h.repo.CompleteRender(ctx, id, claim, pkwt.Path, nda.Path, []string{pkwt.SHA256, nda.SHA256}, contractTemplateVersion())
	if errors.Is(err, hrisrepo.ErrContractStateChanged) {
		return nil, ErrDocumentJobSuperseded
	}
	if err != nil {
		return nil, err
	}
	return superseded, nil
}

func (h *ContractRenderHandler) Fail(ctx context.Context, id string, claim string, reason string) error {
	return h.repo.FailRender(ctx, id, claim, reason)
}
