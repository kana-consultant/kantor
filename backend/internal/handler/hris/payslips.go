package hris

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"

	"github.com/kana-consultant/kantor/backend/internal/clientvia"
	hrisdto "github.com/kana-consultant/kantor/backend/internal/dto/hris"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	"github.com/kana-consultant/kantor/backend/internal/rbac"
	"github.com/kana-consultant/kantor/backend/internal/response"
	hrisservice "github.com/kana-consultant/kantor/backend/internal/service/hris"
)

const (
	permissionSalaryView     = "hris:salary:view"
	permissionPayslipView    = "hris:payslip:view"
	permissionPayslipManage  = "hris:payslip:manage"
	permissionPayslipSend    = "hris:payslip:send"
	permissionContractView   = "hris:contract:view"
	payslipAuditResourceName = "payslip"
)

// PayslipsHandler serves /hris/payslips. Every route needs the payslip
// permission AND hris:salary:view; there is no employee (owner) route in v1.
type PayslipsHandler struct {
	service   *hrisservice.PayslipsService
	validator *validator.Validate
}

func NewPayslipsHandler(service *hrisservice.PayslipsService) *PayslipsHandler {
	return &PayslipsHandler{service: service, validator: newValidator()}
}

func payslipGate(permission string) func(http.Handler) http.Handler {
	return platformmiddleware.RequireAllPermissions(permission, permissionSalaryView)
}

func (h *PayslipsHandler) RegisterRoutes(router chi.Router) {
	router.With(payslipGate(permissionPayslipView)).Get("/", h.list)
	router.With(payslipGate(permissionPayslipManage)).Post("/generate", h.generate)
	router.With(payslipGate(permissionPayslipSend)).Post("/send-preview", h.sendPreview)
	router.With(
		payslipGate(permissionPayslipSend),
		platformmiddleware.NewUserRateLimit(10, time.Minute, "RATE_LIMITED", "Terlalu banyak permintaan kirim. Coba lagi sebentar."),
	).Post("/send-batch", h.sendBatch)
	router.With(payslipGate(permissionPayslipView)).Get("/employee/{employeeID}", h.history)
	router.With(payslipGate(permissionPayslipView)).Get("/{payslipID}", h.get)
	router.With(payslipGate(permissionPayslipManage)).Put("/{payslipID}", h.update)
	router.With(payslipGate(permissionPayslipView)).Get("/{payslipID}/pdf", h.pdf)
	router.With(payslipGate(permissionPayslipManage)).Get("/{payslipID}/docx", h.docx)
	router.With(
		payslipGate(permissionPayslipSend),
		platformmiddleware.NewUserRateLimit(60, time.Minute, "RATE_LIMITED", "Terlalu banyak permintaan. Coba lagi sebentar."),
	).Get("/{payslipID}/recipient", h.recipient)
	router.With(
		payslipGate(permissionPayslipSend),
		platformmiddleware.NewUserRateLimit(30, time.Minute, "RATE_LIMITED", "Terlalu banyak pengiriman. Coba lagi sebentar."),
	).Post("/{payslipID}/send", h.send)
	router.With(payslipGate(permissionPayslipManage)).Post("/{payslipID}/void-reissue", h.voidReissue)
}

// principalHas reports whether the principal holds permission (super admin,
// cached role permissions, view-all, or the token's list).
func principalHas(principal platformmiddleware.Principal, permission string) bool {
	if principal.IsSuperAdmin {
		return true
	}
	if principal.Cached != nil && principal.Cached.Permissions[permission] {
		return true
	}
	if rbac.CanViewAll(principal.Cached, permission) {
		return true
	}
	for _, item := range principal.Permissions {
		if item == permission {
			return true
		}
	}
	return false
}

// documentViewer: personal e-mail addresses are identity data, shown in
// full only with hris:employee_identity:view.
func documentViewer(r *http.Request, principal platformmiddleware.Principal) hrisservice.DocumentViewer {
	return hrisservice.DocumentViewer{
		ActorID:         principal.UserID,
		CanViewIdentity: principalHas(principal, permissionIdentityView),
		ViaMCP:          clientvia.IsMCP(r),
	}
}

func principalOrUnauthorized(w http.ResponseWriter, r *http.Request) (platformmiddleware.Principal, bool) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
	}
	return principal, ok
}

func (h *PayslipsHandler) list(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalOrUnauthorized(w, r)
	if !ok {
		return
	}
	year, errYear := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("year")))
	month, errMonth := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("month")))
	if errYear != nil || errMonth != nil {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Query validation failed", map[string]string{"year": "required", "month": "required"})
		return
	}
	result, err := h.service.List(r.Context(), documentViewer(r, principal), year, month)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *PayslipsHandler) history(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalOrUnauthorized(w, r)
	if !ok {
		return
	}
	limit := 12
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 120 {
			response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Query validation failed", map[string]string{"limit": "1-120"})
			return
		}
		limit = parsed
	}
	result, err := h.service.History(r.Context(), documentViewer(r, principal), chi.URLParam(r, "employeeID"), limit)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *PayslipsHandler) generate(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalOrUnauthorized(w, r)
	if !ok {
		return
	}
	var input hrisdto.GeneratePayslipsRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}
	result, audits, err := h.service.Generate(r.Context(), principal.UserID, input)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	for _, entry := range audits {
		platformmiddleware.AuditLog(r.Context(), "generate", "hris", payslipAuditResourceName, entry.PayslipID, nil, entry.Values)
	}
	response.WriteJSON(w, http.StatusAccepted, result, nil)
}

func (h *PayslipsHandler) get(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalOrUnauthorized(w, r)
	if !ok {
		return
	}
	result, err := h.service.Get(r.Context(), documentViewer(r, principal), chi.URLParam(r, "payslipID"))
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *PayslipsHandler) update(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalOrUnauthorized(w, r)
	if !ok {
		return
	}
	var input hrisdto.UpdatePayslipRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}
	id := chi.URLParam(r, "payslipID")
	result, changed, err := h.service.Update(r.Context(), documentViewer(r, principal), id, input)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	// Field names only: never the note text or amounts.
	if len(changed) > 0 {
		platformmiddleware.AuditLog(r.Context(), "update", "hris", payslipAuditResourceName, id, nil, map[string]any{
			"doc_number":     result.DocNumber,
			"period":         fmt.Sprintf("%04d-%02d", result.PeriodYear, result.PeriodMonth),
			"changed_fields": changed,
		})
	}
	response.WriteJSON(w, http.StatusOK, result, nil)
}

// writeDocument streams a generated document with the headers every
// document response carries (no-store, nosniff, Content-Disposition).
func writeDocument(w http.ResponseWriter, file hrisservice.PayslipFile, disposition string) {
	w.Header().Set("Content-Type", file.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("%s; filename=%q", disposition, file.Filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(file.Data)))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(file.Data)
}

func (h *PayslipsHandler) pdf(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalOrUnauthorized(w, r)
	if !ok {
		return
	}
	disposition := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("disposition")))
	switch disposition {
	case "":
		disposition = "attachment"
	case "inline", "attachment":
	default:
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "disposition harus inline atau attachment", map[string]string{"disposition": "oneof=inline attachment"})
		return
	}
	id := chi.URLParam(r, "payslipID")
	file, err := h.service.PDF(r.Context(), principal.UserID, id, disposition)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	action := "download"
	if disposition == "inline" {
		action = "preview"
	}
	platformmiddleware.AuditLog(r.Context(), action, "hris", payslipAuditResourceName, id, nil, map[string]any{
		"doc_number": file.Slip.DocNumber,
		"period":     fmt.Sprintf("%04d-%02d", file.Slip.PeriodYear, file.Slip.PeriodMonth),
		"format":     "pdf",
	})
	writeDocument(w, file, disposition)
}

func (h *PayslipsHandler) docx(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalOrUnauthorized(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "payslipID")
	file, err := h.service.Docx(r.Context(), principal.UserID, id)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "download", "hris", payslipAuditResourceName, id, nil, map[string]any{
		"doc_number": file.Slip.DocNumber,
		"period":     fmt.Sprintf("%04d-%02d", file.Slip.PeriodYear, file.Slip.PeriodMonth),
		"format":     "docx",
	})
	writeDocument(w, file, "attachment")
}

func (h *PayslipsHandler) recipient(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalOrUnauthorized(w, r)
	if !ok {
		return
	}
	source := strings.TrimSpace(r.URL.Query().Get("source"))
	result, err := h.service.Recipient(r.Context(), documentViewer(r, principal), chi.URLParam(r, "payslipID"), source)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *PayslipsHandler) sendPreview(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalOrUnauthorized(w, r)
	if !ok {
		return
	}
	var input hrisdto.PayslipSendPreviewRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}
	result, err := h.service.SendPreview(r.Context(), documentViewer(r, principal), input)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *PayslipsHandler) send(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalOrUnauthorized(w, r)
	if !ok {
		return
	}
	var input hrisdto.SendPayslipRequest
	if r.ContentLength != 0 {
		if !decodeAndValidate(h.validator, w, r, &input) {
			return
		}
	}
	id := chi.URLParam(r, "payslipID")
	result, err := h.service.Send(r.Context(), documentViewer(r, principal), id, input)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	// Metadata only: doc number, period, format, masked recipient, delivery.
	platformmiddleware.AuditLog(r.Context(), "send", "hris", payslipAuditResourceName, id, nil, result.Audit)
	response.WriteJSON(w, http.StatusOK, result.Response, nil)
}

func (h *PayslipsHandler) sendBatch(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalOrUnauthorized(w, r)
	if !ok {
		return
	}
	var input hrisdto.SendPayslipBatchRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}
	prepared, err := h.service.PrepareBatch(r.Context(), documentViewer(r, principal), input)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	// One 'send_requested' row per document, written before anything is
	// sent; the outcome rows come from the background sender.
	for _, entry := range prepared.Requested {
		platformmiddleware.AuditLog(r.Context(), "send_requested", "hris", payslipAuditResourceName, entry.PayslipID, nil, entry.Values)
	}
	h.service.StartBatch(r.Context(), principal.UserID, prepared)
	response.WriteJSON(w, http.StatusAccepted, prepared.Response, nil)
}

func (h *PayslipsHandler) voidReissue(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalOrUnauthorized(w, r)
	if !ok {
		return
	}
	var input hrisdto.VoidReissuePayslipRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}
	result, audits, err := h.service.VoidReissue(r.Context(), documentViewer(r, principal), chi.URLParam(r, "payslipID"), input.Reason)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	actions := []string{"void", "reissue"}
	for index, entry := range audits {
		action := "reissue"
		if index < len(actions) {
			action = actions[index]
		}
		platformmiddleware.AuditLog(r.Context(), action, "hris", payslipAuditResourceName, entry.PayslipID, nil, entry.Values)
	}
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *PayslipsHandler) writeError(ctx context.Context, w http.ResponseWriter, err error) {
	writeDocumentError(ctx, w, err, "Terjadi kesalahan saat memproses slip gaji")
}

// writeDocumentError maps the payslip / document mailer errors (shared with
// the e-mail delivery history handler).
func writeDocumentError(ctx context.Context, w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, hrisservice.ErrPayslipNotFound):
		response.WriteError(w, http.StatusNotFound, "PAYSLIP_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrEmployeeNotFound):
		response.WriteError(w, http.StatusNotFound, "EMPLOYEE_NOT_FOUND", "Karyawan tidak ditemukan", nil)
	case errors.Is(err, hrisservice.ErrPayslipPeriodInvalid):
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), map[string]string{"period": "invalid"})
	case errors.Is(err, hrisservice.ErrPayslipPayDateInvalid):
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), map[string]string{"pay_date": "range"})
	case errors.Is(err, hrisservice.ErrPayslipNoteInvalid):
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), map[string]string{"note": "max=120"})
	case errors.Is(err, hrisservice.ErrPayslipManualLineInvalid):
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), map[string]string{"manual_lines": "invalid"})
	case errors.Is(err, hrisservice.ErrPayslipVoidReasonInvalid):
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), map[string]string{"reason": "required"})
	case errors.Is(err, hrisservice.ErrDocumentRecipientSourceBad):
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), map[string]string{"source": "oneof"})
	case errors.Is(err, hrisservice.ErrDocumentDeliveryReferenceType):
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), map[string]string{"reference_type": "oneof"})
	case errors.Is(err, hrisservice.ErrDocumentDeliveryForbidden):
		response.WriteError(w, http.StatusForbidden, "FORBIDDEN", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrPayslipNotDraft):
		response.WriteError(w, http.StatusConflict, "PAYSLIP_NOT_DRAFT", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrPayslipNotSent):
		response.WriteError(w, http.StatusConflict, "PAYSLIP_NOT_SENT", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrPayslipVoided):
		response.WriteError(w, http.StatusConflict, "PAYSLIP_VOID", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrPayslipStateChanged):
		response.WriteError(w, http.StatusConflict, "PAYSLIP_STATE_CHANGED", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrPayslipBlocked):
		response.WriteError(w, http.StatusConflict, "PAYSLIP_BLOCKED", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrPayslipItemsTaken):
		response.WriteError(w, http.StatusConflict, "PAYSLIP_ITEMS_TAKEN", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrPayslipItemsChanged):
		response.WriteError(w, http.StatusConflict, "PAYSLIP_ITEMS_CHANGED", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrDocumentPDFConverterMissing):
		response.WriteError(w, http.StatusConflict, "PDF_CONVERTER_UNAVAILABLE", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrPayslipPDFNotReady):
		response.WriteError(w, http.StatusConflict, "PDF_NOT_READY", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrDocumentNotFound), errors.Is(err, hrisservice.ErrDocumentHashMismatch):
		response.WriteError(w, http.StatusConflict, "PDF_MISSING", "File PDF tidak ditemukan atau tidak cocok; generate ulang slip", nil)
	case errors.Is(err, hrisservice.ErrDocumentSendInFlight):
		response.WriteError(w, http.StatusConflict, "SEND_IN_PROGRESS", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrDocumentMailNotReady):
		response.WriteError(w, http.StatusConflict, "DOCUMENT_MAIL_NOT_READY", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrDocumentRecipientUnavailable):
		response.WriteError(w, http.StatusConflict, "RECIPIENT_UNAVAILABLE", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrDocumentExpectedRecipientRequired):
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), map[string]string{"expected_recipient": "required"})
	case errors.Is(err, hrisservice.ErrDocumentExpectedRecipientsRequired):
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), map[string]string{"expected_recipients": "required"})
	case errors.Is(err, hrisservice.ErrDocumentRecipientMismatch):
		response.WriteError(w, http.StatusConflict, "RECIPIENT_MISMATCH", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrDocumentRecipientRestricted):
		response.WriteError(w, http.StatusConflict, "MCP_RECIPIENT_NOT_ALLOWED", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrDocumentCcRestricted):
		response.WriteError(w, http.StatusConflict, "MCP_CC_NOT_ALLOWED", err.Error(), map[string]string{"cc": "mcp"})
	default:
		response.WriteInternalError(ctx, w, err, fallback)
	}
}

// documentPermissionChecker answers "may the caller see documents of this
// kind" for the delivery history.
func documentPermissionChecker(principal platformmiddleware.Principal) func(kind string) bool {
	return func(kind string) bool {
		switch kind {
		case "payslip":
			return principalHas(principal, permissionPayslipView)
		case "contract":
			return principalHas(principal, permissionContractView)
		default:
			return false
		}
	}
}
