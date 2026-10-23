// Package legal serves the Dokumen Legal API under /operational:
// /legal-documents (freelance, subcontract, SOW, SLA, BAST) and
// /legal-parties (the freelancers, subcontractors and clients they are made
// for).
package legal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"

	legaldto "github.com/kana-consultant/kantor/backend/internal/dto/legal"
	"github.com/kana-consultant/kantor/backend/internal/httputil"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	legalrepo "github.com/kana-consultant/kantor/backend/internal/repository/legal"
	"github.com/kana-consultant/kantor/backend/internal/response"
	hrisservice "github.com/kana-consultant/kantor/backend/internal/service/hris"
	legalservice "github.com/kana-consultant/kantor/backend/internal/service/legal"
)

const (
	PermissionView   = "operational:legal_document:view"
	PermissionManage = "operational:legal_document:manage"
	PermissionSend   = "operational:legal_document:send"

	auditModule        = "operational"
	auditDocument      = "legal_document"
	auditParty         = "legal_party"
	multipartOverhead  = 1 << 20
	maxSearchCharacter = 100
)

type Handler struct {
	service   *legalservice.Service
	validator *validator.Validate
}

func NewHandler(service *legalservice.Service) *Handler {
	return &Handler{service: service, validator: httputil.NewValidator()}
}

// RegisterDocumentRoutes mounts /legal-documents.
func (h *Handler) RegisterDocumentRoutes(router chi.Router) {
	view := platformmiddleware.RequirePermission(PermissionView)
	manage := platformmiddleware.RequirePermission(PermissionManage)

	router.With(view).Get("/types", h.types)
	router.With(view).Get("/", h.listDocuments)
	router.With(manage).Post("/", h.createDocument)
	router.With(view).Get("/{documentID}", h.getDocument)
	router.With(manage).Put("/{documentID}", h.updateDocument)
	router.With(manage).Delete("/{documentID}", h.deleteDocument)
	router.With(manage).Post("/{documentID}/generate", h.generate)
	router.With(manage).Post("/{documentID}/cancel", h.cancel)
	router.With(manage).Post("/{documentID}/signed", h.markSigned)
	router.With(view).Get("/{documentID}/pdf", h.pdf)
	router.With(manage).Get("/{documentID}/docx", h.docx)
	router.With(view).Get("/{documentID}/signed-copy", h.signedCopy)
	router.With(
		platformmiddleware.RequirePermission(PermissionSend),
		platformmiddleware.NewUserRateLimit(30, time.Minute, "RATE_LIMITED", "Terlalu banyak pengiriman. Coba lagi sebentar."),
	).Post("/{documentID}/send", h.send)
}

// RegisterPartyRoutes mounts /legal-parties.
func (h *Handler) RegisterPartyRoutes(router chi.Router) {
	view := platformmiddleware.RequirePermission(PermissionView)
	manage := platformmiddleware.RequirePermission(PermissionManage)

	router.With(view).Get("/", h.listParties)
	router.With(manage).Post("/", h.createParty)
	router.With(view).Get("/{partyID}", h.getParty)
	router.With(manage).Put("/{partyID}", h.updateParty)
	router.With(manage).Delete("/{partyID}", h.deleteParty)
}

func principalOrUnauthorized(w http.ResponseWriter, r *http.Request) (platformmiddleware.Principal, bool) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
	}
	return principal, ok
}

func (h *Handler) writeError(ctx context.Context, w http.ResponseWriter, err error) {
	var fieldsErr *legalservice.FieldsError
	var companyErr *legalservice.CompanyIncompleteError
	switch {
	case errors.As(err, &fieldsErr):
		response.WriteError(w, http.StatusUnprocessableEntity, "LEGAL_FIELDS_INVALID", fieldsErr.Message, fieldsErr.Fields)
	case errors.As(err, &companyErr):
		response.WriteError(w, http.StatusConflict, "COMPANY_PROFILE_INCOMPLETE", companyErr.Error(), map[string]any{"missing": companyErr.Missing})
	case errors.Is(err, legalservice.ErrDocumentNotFound):
		response.WriteError(w, http.StatusNotFound, "LEGAL_DOCUMENT_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, legalservice.ErrPartyNotFound):
		response.WriteError(w, http.StatusNotFound, "LEGAL_PARTY_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, legalservice.ErrPartyInUse):
		response.WriteError(w, http.StatusConflict, "LEGAL_PARTY_IN_USE", err.Error(), nil)
	case errors.Is(err, legalservice.ErrStateChanged), errors.Is(err, hrisservice.ErrDocumentSendInFlight):
		response.WriteError(w, http.StatusConflict, "LEGAL_DOCUMENT_STATE_CHANGED", err.Error(), nil)
	case errors.Is(err, legalservice.ErrNotEditable), errors.Is(err, legalservice.ErrNotGeneratable),
		errors.Is(err, legalservice.ErrNotDeletable), errors.Is(err, legalservice.ErrNotSendable):
		response.WriteError(w, http.StatusConflict, "LEGAL_DOCUMENT_STATUS", err.Error(), nil)
	case errors.Is(err, legalservice.ErrPDFNotReady), errors.Is(err, hrisservice.ErrDocumentPDFConverterMissing):
		response.WriteError(w, http.StatusConflict, "LEGAL_DOCUMENT_PDF_NOT_READY", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrDocumentMailNotReady):
		response.WriteError(w, http.StatusConflict, "DOCUMENT_MAIL_NOT_READY", err.Error(), nil)
	case errors.Is(err, legalservice.ErrNoSignedCopy):
		response.WriteError(w, http.StatusNotFound, "LEGAL_SIGNED_COPY_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, legalservice.ErrPartyTypeNotAllowed), errors.Is(err, legalservice.ErrRelatedInvalid),
		errors.Is(err, legalservice.ErrUnknownType), errors.Is(err, legalservice.ErrSignedCopyInvalid),
		errors.Is(err, legalservice.ErrSignedDateInvalid):
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
	default:
		response.WriteInternalError(ctx, w, err, "Gagal memproses dokumen legal")
	}
}

func writeFile(w http.ResponseWriter, file legalservice.File, disposition string) {
	w.Header().Set("Content-Type", file.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("%s; filename=%q", disposition, file.Filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(file.Data)))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(file.Data)
}

func fileAudit(file legalservice.File, format string) map[string]any {
	values := map[string]any{"doc_type": file.Document.DocType, "format": format}
	if file.Document.DocNumber != nil {
		values["doc_number"] = *file.Document.DocNumber
	}
	return values
}

// ---------------------------------------------------------------------------
// Documents

func (h *Handler) types(w http.ResponseWriter, r *http.Request) {
	response.WriteJSON(w, http.StatusOK, legalservice.Specs(), nil)
}

func (h *Handler) listDocuments(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	search := strings.TrimSpace(query.Get("search"))
	if len(search) > maxSearchCharacter {
		search = search[:maxSearchCharacter]
	}
	items, err := h.service.ListDocuments(r.Context(), legalrepo.DocumentFilter{
		DocType: strings.TrimSpace(query.Get("type")),
		Status:  strings.TrimSpace(query.Get("status")),
		PartyID: strings.TrimSpace(query.Get("party_id")),
		Search:  search,
	})
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, items, nil)
}

func (h *Handler) getDocument(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.GetDocument(r.Context(), chi.URLParam(r, "documentID"))
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *Handler) createDocument(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalOrUnauthorized(w, r)
	if !ok {
		return
	}
	var input legaldto.DocumentRequest
	if !httputil.DecodeAndValidate(h.validator, w, r, &input) {
		return
	}
	if input.DocType == "" {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Jenis dokumen wajib dipilih", map[string]string{"doc_type": "required"})
		return
	}
	result, err := h.service.CreateDocument(r.Context(), principal.UserID, input)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "create", auditModule, auditDocument, result.ID, nil, map[string]any{"doc_type": result.DocType, "party_id": result.PartyID})
	response.WriteJSON(w, http.StatusCreated, result, nil)
}

func (h *Handler) updateDocument(w http.ResponseWriter, r *http.Request) {
	var input legaldto.DocumentRequest
	if !httputil.DecodeAndValidate(h.validator, w, r, &input) {
		return
	}
	id := chi.URLParam(r, "documentID")
	result, err := h.service.UpdateDocument(r.Context(), id, input)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "update", auditModule, auditDocument, id, nil, map[string]any{"doc_type": result.DocType, "status": result.Status})
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *Handler) deleteDocument(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "documentID")
	if err := h.service.DeleteDocument(r.Context(), id); err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "delete", auditModule, auditDocument, id, nil, nil)
	response.WriteJSON(w, http.StatusOK, map[string]bool{"deleted": true}, nil)
}

func (h *Handler) generate(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalOrUnauthorized(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "documentID")
	result, audit, err := h.service.Generate(r.Context(), principal.UserID, id)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "generate", auditModule, auditDocument, id, nil, audit)
	response.WriteJSON(w, http.StatusAccepted, result, nil)
}

func (h *Handler) cancel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "documentID")
	result, err := h.service.CancelDocument(r.Context(), id)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "cancel", auditModule, auditDocument, id, nil, map[string]any{"doc_type": result.DocType})
	response.WriteJSON(w, http.StatusOK, result, nil)
}

// markSigned takes multipart form data: signed_at (YYYY-MM-DD) and an
// optional file (the scanned signed PDF).
func (h *Handler) markSigned(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "documentID")
	r.Body = http.MaxBytesReader(w, r.Body, legalservice.MaxSignedCopyBytes+multipartOverhead)
	if err := r.ParseMultipartForm(legalservice.MaxSignedCopyBytes + multipartOverhead); err != nil {
		if platformmiddleware.IsBodyTooLargeError(err) {
			response.WriteError(w, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", legalservice.ErrSignedCopyInvalid.Error(), nil)
			return
		}
		response.WriteError(w, http.StatusBadRequest, "INVALID_MULTIPART", "Kirim sebagai multipart form data", nil)
		return
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()

	var data []byte
	var fileHeader *multipart.FileHeader
	if files := r.MultipartForm.File["file"]; len(files) > 0 {
		fileHeader = files[0]
	}
	if fileHeader != nil {
		if fileHeader.Size > legalservice.MaxSignedCopyBytes {
			response.WriteError(w, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", legalservice.ErrSignedCopyInvalid.Error(), nil)
			return
		}
		file, err := fileHeader.Open()
		if err != nil {
			response.WriteError(w, http.StatusBadRequest, "INVALID_MULTIPART", "File tidak dapat dibaca", nil)
			return
		}
		data, err = io.ReadAll(io.LimitReader(file, legalservice.MaxSignedCopyBytes+1))
		_ = file.Close()
		if err != nil {
			response.WriteError(w, http.StatusBadRequest, "INVALID_MULTIPART", "File tidak dapat dibaca", nil)
			return
		}
	}
	result, err := h.service.MarkSigned(r.Context(), id, r.FormValue("signed_at"), data)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "mark_signed", auditModule, auditDocument, id, nil, map[string]any{"doc_type": result.DocType, "signed_at": result.SignedAt, "scan_uploaded": len(data) > 0})
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *Handler) pdf(w http.ResponseWriter, r *http.Request) {
	disposition := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("disposition")))
	switch disposition {
	case "":
		disposition = "attachment"
	case "inline", "attachment":
	default:
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "disposition harus inline atau attachment", nil)
		return
	}
	id := chi.URLParam(r, "documentID")
	file, err := h.service.PDF(r.Context(), id)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	action := "download"
	if disposition == "inline" {
		action = "preview"
	}
	platformmiddleware.AuditLog(r.Context(), action, auditModule, auditDocument, id, nil, fileAudit(file, "pdf"))
	writeFile(w, file, disposition)
}

func (h *Handler) docx(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "documentID")
	file, err := h.service.Docx(r.Context(), id)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "download", auditModule, auditDocument, id, nil, fileAudit(file, "docx"))
	writeFile(w, file, "attachment")
}

func (h *Handler) signedCopy(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "documentID")
	file, err := h.service.SignedCopy(r.Context(), id)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "download", auditModule, auditDocument, id, nil, fileAudit(file, "signed_pdf"))
	writeFile(w, file, "attachment")
}

func (h *Handler) send(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalOrUnauthorized(w, r)
	if !ok {
		return
	}
	var input legaldto.SendRequest
	if !httputil.DecodeAndValidate(h.validator, w, r, &input) {
		return
	}
	id := chi.URLParam(r, "documentID")
	result, err := h.service.Send(r.Context(), principal.UserID, id, input.Recipient)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "send", auditModule, auditDocument, id, nil, result.Audit)
	response.WriteJSON(w, http.StatusOK, result.Response, nil)
}

// ---------------------------------------------------------------------------
// Parties

func (h *Handler) listParties(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	search := strings.TrimSpace(query.Get("search"))
	if len(search) > maxSearchCharacter {
		search = search[:maxSearchCharacter]
	}
	items, err := h.service.ListParties(r.Context(), search, strings.TrimSpace(query.Get("type")))
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, items, nil)
}

func (h *Handler) getParty(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.GetParty(r.Context(), chi.URLParam(r, "partyID"))
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, result, nil)
}

// partyAudit names the fields set, never their values.
func partyAudit(input legaldto.PartyRequest) map[string]any {
	return map[string]any{
		"party_type":          input.PartyType,
		"id_number_set":       input.IDNumber != nil,
		"tax_id_set":          input.TaxID != nil,
		"bank_account_set":    input.BankAccountNumber != nil,
		"email_present":       strings.TrimSpace(input.Email) != "",
		"representative_name": strings.TrimSpace(input.RepresentativeName) != "",
	}
}

func (h *Handler) createParty(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalOrUnauthorized(w, r)
	if !ok {
		return
	}
	var input legaldto.PartyRequest
	if !httputil.DecodeAndValidate(h.validator, w, r, &input) {
		return
	}
	result, err := h.service.CreateParty(r.Context(), principal.UserID, input)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "create", auditModule, auditParty, result.ID, nil, partyAudit(input))
	response.WriteJSON(w, http.StatusCreated, result, nil)
}

func (h *Handler) updateParty(w http.ResponseWriter, r *http.Request) {
	var input legaldto.PartyRequest
	if !httputil.DecodeAndValidate(h.validator, w, r, &input) {
		return
	}
	id := chi.URLParam(r, "partyID")
	result, err := h.service.UpdateParty(r.Context(), id, input)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "update", auditModule, auditParty, id, nil, partyAudit(input))
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *Handler) deleteParty(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "partyID")
	if err := h.service.DeleteParty(r.Context(), id); err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "delete", auditModule, auditParty, id, nil, nil)
	response.WriteJSON(w, http.StatusOK, map[string]bool{"deleted": true}, nil)
}
