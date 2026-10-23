package legal

import (
	"context"
	"errors"
	"fmt"
	"time"

	legalrepo "github.com/kana-consultant/kantor/backend/internal/repository/legal"
	hrisservice "github.com/kana-consultant/kantor/backend/internal/service/hris"
)

// renderStaleAfter matches the document worker's stale rendering timeout.
const renderStaleAfter = 5 * time.Minute

// RenderHandler plugs legal documents into the document worker (kind
// "legal", one part rendered from the type's template). The PDF is stored
// encrypted; the claim token makes a render that finished after the
// document changed lose.
type RenderHandler struct {
	service *Service
	now     func() time.Time
}

func NewRenderHandler(service *Service) *RenderHandler {
	return &RenderHandler{service: service, now: time.Now}
}

func (h *RenderHandler) Kind() string { return DocumentKind }

// AuditModule files the worker's render audit rows under Operasional.
func (h *RenderHandler) AuditModule() string { return "operational" }

func (h *RenderHandler) ListPending(ctx context.Context, staleBefore time.Time) ([]string, error) {
	return h.service.documents.ListRenderPending(ctx, staleBefore)
}

func (h *RenderHandler) Prepare(ctx context.Context, id string) (hrisservice.PreparedDocument, error) {
	claim, doc, ok, err := h.service.documents.ClaimRender(ctx, id, h.now().Add(-renderStaleAfter))
	if err != nil {
		return hrisservice.PreparedDocument{}, err
	}
	if !ok {
		return hrisservice.PreparedDocument{}, hrisservice.ErrDocumentJobSkipped
	}
	spec, found := SpecFor(doc.DocType)
	if !found || doc.PayloadEncrypted == nil {
		return h.failPrepare(ctx, id, claim, fmt.Errorf("legal document %s has no renderable snapshot", id))
	}
	payload, err := h.service.decryptPayload(doc)
	if err != nil {
		return h.failPrepare(ctx, id, claim, err)
	}
	actor := ""
	if doc.GeneratedBy != nil {
		actor = *doc.GeneratedBy
	}
	return hrisservice.PreparedDocument{
		Claim:   claim,
		ActorID: actor,
		Parts:   []hrisservice.DocumentPart{{Name: partMain, Template: spec.Template, Payload: payload}},
	}, nil
}

// failPrepare records the failure under the claim (so the worker's
// claim-less Fail cannot race a newer generate) and skips the job.
func (h *RenderHandler) failPrepare(ctx context.Context, id string, claim string, cause error) (hrisservice.PreparedDocument, error) {
	if failErr := h.service.documents.FailRender(ctx, id, claim, "Gagal memuat data dokumen"); failErr != nil {
		return hrisservice.PreparedDocument{}, fmt.Errorf("%w (record failure: %v)", cause, failErr)
	}
	return hrisservice.PreparedDocument{}, hrisservice.ErrDocumentJobSkipped
}

func (h *RenderHandler) Complete(ctx context.Context, id string, claim string, files []hrisservice.StoredDocument) ([]string, error) {
	var main *hrisservice.StoredDocument
	for index := range files {
		if files[index].Part == partMain {
			main = &files[index]
		}
	}
	if main == nil {
		return nil, errors.New("legal document render produced no file")
	}
	superseded, err := h.service.documents.CompleteRender(ctx, id, claim, main.Path, main.SHA256)
	if errors.Is(err, legalrepo.ErrDocumentStateChanged) {
		return nil, hrisservice.ErrDocumentJobSuperseded
	}
	return superseded, err
}

func (h *RenderHandler) Fail(ctx context.Context, id string, claim string, reason string) error {
	return h.service.documents.FailRender(ctx, id, claim, reason)
}

var _ hrisservice.DocumentRenderHandler = (*RenderHandler)(nil)
