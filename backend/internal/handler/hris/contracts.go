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

	hrisdto "github.com/kana-consultant/kantor/backend/internal/dto/hris"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	hrisrepo "github.com/kana-consultant/kantor/backend/internal/repository/hris"
	"github.com/kana-consultant/kantor/backend/internal/response"
	hrisservice "github.com/kana-consultant/kantor/backend/internal/service/hris"
)

const (
	permissionContractManage  = "hris:contract:manage"
	permissionContractSend    = "hris:contract:send"
	contractAuditResourceName = "contract"
)

// ContractsHandler serves /hris/contracts. Compensation is returned (and may
// be set) only with hris:salary:view; the PKWT/NDA PDF needs
// hris:contract:view AND hris:employee_identity:view (the PKWT prints the
// full NIK and account number), the DOCX hris:contract:manage AND
// hris:employee_identity:view; the PKWT part (PDF or DOCX) also needs
// hris:salary:view, since it prints the compensation. A send with CC needs
// what reading the documents needs (contract:view, employee_identity:view,
// salary:view): the CC recipients get both documents. There is no employee
// (owner) route in v1.
type ContractsHandler struct {
	service   *hrisservice.ContractsService
	validator *validator.Validate
}

func NewContractsHandler(service *hrisservice.ContractsService) *ContractsHandler {
	return &ContractsHandler{service: service, validator: newValidator()}
}

func (h *ContractsHandler) RegisterRoutes(router chi.Router) {
	view := platformmiddleware.RequirePermission(permissionContractView)
	manage := platformmiddleware.RequirePermission(permissionContractManage)

	router.With(view).Get("/", h.list)
	router.With(manage).Post("/", h.create)
	router.With(view).Get("/{contractID}", h.get)
	router.With(manage).Put("/{contractID}", h.update)
	router.With(manage).Get("/{contractID}/preflight", h.preflight)
	router.With(manage).Post("/{contractID}/generate", h.generate)
	router.With(manage).Post("/{contractID}/renew", h.renew)
	router.With(manage).Patch("/{contractID}/status", h.status)
	router.With(platformmiddleware.RequireAllPermissions(permissionContractView, permissionIdentityView)).Get("/{contractID}/files/{part}/pdf", h.pdf)
	router.With(platformmiddleware.RequireAllPermissions(permissionContractManage, permissionIdentityView)).Get("/{contractID}/files/{part}/docx", h.docx)
	router.With(
		platformmiddleware.RequirePermission(permissionContractSend),
		platformmiddleware.NewUserRateLimit(60, time.Minute, "RATE_LIMITED", "Terlalu banyak permintaan. Coba lagi sebentar."),
	).Get("/{contractID}/recipient", h.recipient)
	router.With(
		platformmiddleware.RequirePermission(permissionContractSend),
		platformmiddleware.NewUserRateLimit(30, time.Minute, "RATE_LIMITED", "Terlalu banyak pengiriman. Coba lagi sebentar."),
	).Post("/{contractID}/send", h.send)
}

func contractViewer(r *http.Request, principal platformmiddleware.Principal) hrisservice.ContractViewer {
	return hrisservice.ContractViewer{
		DocumentViewer:  documentViewer(r, principal),
		CanViewSalary:   principalHas(principal, permissionSalaryView),
		CanViewContract: principalHas(principal, permissionContractView),
	}
}

// contractPartAllowed refuses the PKWT (it prints the compensation) to a
// caller without hris:salary:view. The NDA prints no amounts.
func contractPartAllowed(w http.ResponseWriter, principal platformmiddleware.Principal, part string) bool {
	if strings.EqualFold(strings.TrimSpace(part), hrisservice.ContractPartPKWT) && !principalHas(principal, permissionSalaryView) {
		response.WriteError(w, http.StatusForbidden, "FORBIDDEN", "PKWT memuat gaji: perlu izin melihat gaji (hris:salary:view)", map[string]string{"part": "salary_forbidden"})
		return false
	}
	return true
}

func (h *ContractsHandler) list(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalOrUnauthorized(w, r)
	if !ok {
		return
	}
	query := r.URL.Query()
	filter := hrisrepo.ContractListFilter{
		EmployeeID:   strings.TrimSpace(query.Get("employee_id")),
		Status:       strings.TrimSpace(query.Get("status")),
		ContractType: strings.TrimSpace(query.Get("type")),
		Search:       strings.TrimSpace(query.Get("search")),
	}
	if len(filter.Search) > 100 {
		filter.Search = filter.Search[:100]
	}
	if raw := strings.TrimSpace(query.Get("limit")); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 500 {
			response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Query validation failed", map[string]string{"limit": "1-500"})
			return
		}
		filter.Limit = limit
	}
	result, err := h.service.List(r.Context(), contractViewer(r, principal), filter)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *ContractsHandler) get(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalOrUnauthorized(w, r)
	if !ok {
		return
	}
	result, err := h.service.Get(r.Context(), contractViewer(r, principal), chi.URLParam(r, "contractID"))
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *ContractsHandler) create(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalOrUnauthorized(w, r)
	if !ok {
		return
	}
	var input hrisdto.CreateContractRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}
	result, audit, err := h.service.Create(r.Context(), contractViewer(r, principal), input)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "create", "hris", contractAuditResourceName, audit.ContractID, nil, audit.Values)
	response.WriteJSON(w, http.StatusCreated, result, nil)
}

func (h *ContractsHandler) update(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalOrUnauthorized(w, r)
	if !ok {
		return
	}
	var input hrisdto.UpdateContractRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}
	id := chi.URLParam(r, "contractID")
	result, changed, audit, err := h.service.Update(r.Context(), contractViewer(r, principal), id, input)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	// Field names only: never the terms or amounts.
	if len(changed) > 0 {
		action := "update"
		if revised, _ := audit["revised"].(bool); revised {
			action = "revise"
		}
		platformmiddleware.AuditLog(r.Context(), action, "hris", contractAuditResourceName, id, nil, audit)
	}
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *ContractsHandler) preflight(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalOrUnauthorized(w, r)
	if !ok {
		return
	}
	result, err := h.service.Preflight(r.Context(), contractViewer(r, principal), chi.URLParam(r, "contractID"))
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *ContractsHandler) generate(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalOrUnauthorized(w, r)
	if !ok {
		return
	}
	result, audit, err := h.service.Generate(r.Context(), contractViewer(r, principal), chi.URLParam(r, "contractID"))
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "generate", "hris", contractAuditResourceName, audit.ContractID, nil, audit.Values)
	response.WriteJSON(w, http.StatusAccepted, result, nil)
}

func (h *ContractsHandler) renew(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalOrUnauthorized(w, r)
	if !ok {
		return
	}
	result, audit, err := h.service.Renew(r.Context(), contractViewer(r, principal), chi.URLParam(r, "contractID"))
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "renew", "hris", contractAuditResourceName, audit.ContractID, nil, audit.Values)
	response.WriteJSON(w, http.StatusCreated, result, nil)
}

func (h *ContractsHandler) status(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalOrUnauthorized(w, r)
	if !ok {
		return
	}
	var input hrisdto.UpdateContractStatusRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}
	result, audit, err := h.service.SetStatus(r.Context(), contractViewer(r, principal), chi.URLParam(r, "contractID"), input)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "status_"+strings.ToLower(input.Status), "hris", contractAuditResourceName, audit.ContractID, nil, audit.Values)
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func writeContractFile(w http.ResponseWriter, file hrisservice.ContractFile, disposition string) {
	w.Header().Set("Content-Type", file.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("%s; filename=%q", disposition, file.Filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(file.Data)))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(file.Data)
}

func contractFileAudit(file hrisservice.ContractFile, format string) map[string]any {
	values := map[string]any{
		"part":     file.Part,
		"format":   format,
		"revision": file.Contract.Revision,
	}
	if file.Contract.DocNumber != nil {
		values["doc_number"] = *file.Contract.DocNumber
	}
	if file.Part == hrisservice.ContractPartNDA && file.Contract.NDADocNumber != nil {
		values["doc_number"] = *file.Contract.NDADocNumber
	}
	return values
}

func (h *ContractsHandler) pdf(w http.ResponseWriter, r *http.Request) {
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
	if !contractPartAllowed(w, principal, chi.URLParam(r, "part")) {
		return
	}
	id := chi.URLParam(r, "contractID")
	file, err := h.service.PDF(r.Context(), principal.UserID, id, chi.URLParam(r, "part"), disposition)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	action := "download"
	if disposition == "inline" {
		action = "preview"
	}
	platformmiddleware.AuditLog(r.Context(), action, "hris", contractAuditResourceName, id, nil, contractFileAudit(file, "pdf"))
	writeContractFile(w, file, disposition)
}

func (h *ContractsHandler) docx(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalOrUnauthorized(w, r)
	if !ok {
		return
	}
	if !contractPartAllowed(w, principal, chi.URLParam(r, "part")) {
		return
	}
	id := chi.URLParam(r, "contractID")
	file, err := h.service.Docx(r.Context(), principal.UserID, id, chi.URLParam(r, "part"))
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "download", "hris", contractAuditResourceName, id, nil, contractFileAudit(file, "docx"))
	writeContractFile(w, file, "attachment")
}

func (h *ContractsHandler) recipient(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalOrUnauthorized(w, r)
	if !ok {
		return
	}
	source := strings.TrimSpace(r.URL.Query().Get("source"))
	result, err := h.service.Recipient(r.Context(), contractViewer(r, principal), chi.URLParam(r, "contractID"), source)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *ContractsHandler) send(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalOrUnauthorized(w, r)
	if !ok {
		return
	}
	var input hrisdto.SendContractRequest
	if r.ContentLength != 0 {
		if !decodeAndValidate(h.validator, w, r, &input) {
			return
		}
	}
	// CC recipients receive both documents (full NIK, account number,
	// compensation): only a caller who may read them may add any.
	if len(input.Cc) > 0 && !(principalHas(principal, permissionContractView) && principalHas(principal, permissionIdentityView) && principalHas(principal, permissionSalaryView)) {
		response.WriteError(w, http.StatusForbidden, "FORBIDDEN", "CC memerlukan izin melihat kontrak, identitas karyawan, dan gaji", map[string]string{"cc": "forbidden"})
		return
	}
	id := chi.URLParam(r, "contractID")
	result, err := h.service.Send(r.Context(), contractViewer(r, principal), id, input)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	// Metadata only: numbers, revision, format, masked recipient and CC,
	// delivery.
	platformmiddleware.AuditLog(r.Context(), "send", "hris", contractAuditResourceName, id, nil, result.Audit)
	response.WriteJSON(w, http.StatusOK, result.Response, nil)
}

func (h *ContractsHandler) writeError(ctx context.Context, w http.ResponseWriter, err error) {
	var incomplete *hrisservice.ContractIncompleteError
	switch {
	case errors.As(err, &incomplete):
		details := make(map[string]string, len(incomplete.Missing))
		for _, item := range incomplete.Missing {
			details[item.Scope+"."+item.Field] = "missing"
		}
		response.WriteError(w, http.StatusConflict, "CONTRACT_INCOMPLETE", err.Error(), details)
	case errors.Is(err, hrisservice.ErrContractNotFound):
		response.WriteError(w, http.StatusNotFound, "CONTRACT_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrContractEndDateRequired):
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), map[string]string{"end_date": "required"})
	case errors.Is(err, hrisservice.ErrContractDatesInvalid):
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), map[string]string{"end_date": "gtefield=start_date"})
	case errors.Is(err, hrisservice.ErrContractDocumentDateInvalid):
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), map[string]string{"document_date": "datetime"})
	case errors.Is(err, hrisservice.ErrContractJobTitleRequired):
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), map[string]string{"job_title": "required"})
	case errors.Is(err, hrisservice.ErrContractTypeInvalid):
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), map[string]string{"contract_type": "oneof"})
	case errors.Is(err, hrisservice.ErrContractPartInvalid):
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), map[string]string{"part": "oneof=pkwt nda"})
	case errors.Is(err, hrisservice.ErrContractStatusDateInvalid):
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), map[string]string{"date": "invalid"})
	case errors.Is(err, hrisservice.ErrContractCcInvalid):
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), map[string]string{"cc": "email"})
	case errors.Is(err, hrisservice.ErrContractCcDomain):
		response.WriteError(w, http.StatusBadRequest, "CC_DOMAIN_NOT_ALLOWED", err.Error(), map[string]string{"cc": "domain"})
	case errors.Is(err, hrisservice.ErrContractCompensationForbidden):
		response.WriteError(w, http.StatusForbidden, "FORBIDDEN", err.Error(), map[string]string{"compensation": "forbidden"})
	case errors.Is(err, hrisservice.ErrContractTypeLocked):
		response.WriteError(w, http.StatusConflict, "CONTRACT_NUMBER_LOCKED", err.Error(), map[string]string{"contract_type": "locked"})
	case errors.Is(err, hrisservice.ErrContractDocumentDateLocked):
		response.WriteError(w, http.StatusConflict, "CONTRACT_NUMBER_LOCKED", err.Error(), map[string]string{"document_date": "locked"})
	case errors.Is(err, hrisservice.ErrContractNotEditable):
		response.WriteError(w, http.StatusConflict, "CONTRACT_NOT_EDITABLE", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrContractRecordOnly):
		response.WriteError(w, http.StatusConflict, "CONTRACT_RECORD_ONLY", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrContractNotGeneratable):
		response.WriteError(w, http.StatusConflict, "CONTRACT_NOT_GENERATABLE", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrContractNotSendable), errors.Is(err, hrisservice.ErrContractNoDocument), errors.Is(err, hrisservice.ErrContractDocumentNotCurrent):
		response.WriteError(w, http.StatusConflict, "CONTRACT_NOT_GENERATED", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrContractStatusTransition):
		response.WriteError(w, http.StatusConflict, "CONTRACT_STATUS_TRANSITION", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrContractNotRenewable):
		response.WriteError(w, http.StatusConflict, "CONTRACT_NOT_RENEWABLE", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrContractAlreadyRenewed):
		response.WriteError(w, http.StatusConflict, "CONTRACT_ALREADY_RENEWED", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrContractStateChanged):
		response.WriteError(w, http.StatusConflict, "CONTRACT_STATE_CHANGED", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrContractNumberTaken):
		response.WriteError(w, http.StatusConflict, "CONTRACT_NUMBER_TAKEN", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrContractPDFNotReady), errors.Is(err, hrisservice.ErrContractSignBeforePDF):
		response.WriteError(w, http.StatusConflict, "PDF_NOT_READY", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrDocumentNotFound), errors.Is(err, hrisservice.ErrDocumentHashMismatch):
		response.WriteError(w, http.StatusConflict, "PDF_MISSING", "File PDF tidak ditemukan atau tidak cocok; generate ulang kontrak", nil)
	default:
		writeDocumentError(ctx, w, err, "Terjadi kesalahan saat memproses kontrak kerja")
	}
}
