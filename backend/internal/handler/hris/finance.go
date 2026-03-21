package hris

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"

	hrisdto "github.com/kana-consultant/kantor/backend/internal/dto/hris"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	"github.com/kana-consultant/kantor/backend/internal/response"
	hrisservice "github.com/kana-consultant/kantor/backend/internal/service/hris"
)

type FinanceHandler struct {
	service   *hrisservice.FinanceService
	validator *validator.Validate
}

func NewFinanceHandler(service *hrisservice.FinanceService) *FinanceHandler {
	return &FinanceHandler{
		service:   service,
		validator: newValidator(),
	}
}

func (h *FinanceHandler) RegisterRoutes(router chi.Router) {
	router.With(platformmiddleware.RequirePermission("hris:finance:view")).Get("/categories", h.listCategories)
	router.With(platformmiddleware.RequirePermission("hris:finance:approve")).Post("/categories", h.createCategory)
	router.With(platformmiddleware.RequirePermission("hris:finance:approve")).Put("/categories/{categoryID}", h.updateCategory)
	router.With(platformmiddleware.RequirePermission("hris:finance:approve")).Delete("/categories/{categoryID}", h.deleteCategory)

	router.With(platformmiddleware.RequirePermission("hris:finance:create")).Post("/records", h.createRecord)
	router.With(platformmiddleware.RequirePermission("hris:finance:view")).Get("/records", h.listRecords)
	router.With(platformmiddleware.RequirePermission("hris:finance:view")).Get("/records/{recordID}", h.getRecord)
	router.With(platformmiddleware.RequirePermission("hris:finance:edit")).Put("/records/{recordID}", h.updateRecord)
	router.With(platformmiddleware.RequirePermission("hris:finance:delete")).Delete("/records/{recordID}", h.deleteRecord)
	router.With(platformmiddleware.RequirePermission("hris:finance:create")).Patch("/records/{recordID}/submit", h.submitRecord)
	router.With(platformmiddleware.RequirePermission("hris:finance:approve")).Patch("/records/{recordID}/review", h.reviewRecord)
	router.With(platformmiddleware.RequirePermission("hris:finance:view")).Get("/summary", h.summary)
	router.With(platformmiddleware.RequirePermission("hris:finance:view")).Get("/export", h.exportCSV)
}

func (h *FinanceHandler) createCategory(w http.ResponseWriter, r *http.Request) {
	if !requireFinanceAdmin(w, r) {
		return
	}
	var input hrisdto.CreateFinanceCategoryRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}
	result, err := h.service.CreateCategory(r.Context(), input)
	if err != nil {
		h.writeError(w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "create", "hris", "finance_category", result.ID, nil, input)
	response.WriteJSON(w, http.StatusCreated, result, nil)
}

func (h *FinanceHandler) listCategories(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListCategories(r.Context(), r.URL.Query().Get("type"))
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, items, nil)
}

func (h *FinanceHandler) updateCategory(w http.ResponseWriter, r *http.Request) {
	if !requireFinanceAdmin(w, r) {
		return
	}
	var input hrisdto.UpdateFinanceCategoryRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}
	categoryID := chi.URLParam(r, "categoryID")
	result, err := h.service.UpdateCategory(r.Context(), categoryID, input)
	if err != nil {
		h.writeError(w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "update", "hris", "finance_category", categoryID, nil, input)
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *FinanceHandler) deleteCategory(w http.ResponseWriter, r *http.Request) {
	if !requireFinanceAdmin(w, r) {
		return
	}
	categoryID := chi.URLParam(r, "categoryID")
	if err := h.service.DeleteCategory(r.Context(), categoryID); err != nil {
		h.writeError(w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "delete", "hris", "finance_category", categoryID, nil, nil)
	response.WriteJSON(w, http.StatusOK, map[string]string{"message": "Finance category deleted successfully"}, nil)
}

func requireFinanceAdmin(w http.ResponseWriter, r *http.Request) bool {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return false
	}
	if principal.IsSuperAdmin {
		return true
	}
	if principal.ModuleRoles != nil {
		if role, exists := principal.ModuleRoles["hris"]; exists && role.RoleSlug == "admin" {
			return true
		}
	}
	response.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Finance categories can only be managed by HRIS admin", nil)
	return false
}

func (h *FinanceHandler) createRecord(w http.ResponseWriter, r *http.Request) {
	var input hrisdto.CreateFinanceRecordRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}
	result, err := h.service.CreateRecord(r.Context(), input, principal.UserID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "create", "hris", "finance_record", result.ID, nil, input)
	response.WriteJSON(w, http.StatusCreated, result, nil)
}

func (h *FinanceHandler) listRecords(w http.ResponseWriter, r *http.Request) {
	query, ok := h.parseListQuery(w, r)
	if !ok {
		return
	}
	principal, principalOK := platformmiddleware.PrincipalFromContext(r.Context())
	if !principalOK {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}
	items, total, page, perPage, err := h.service.ListRecords(r.Context(), query, principal.UserID, principal.Cached)
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, items, map[string]int64{
		"page":     int64(page),
		"per_page": int64(perPage),
		"total":    total,
	})
}

func (h *FinanceHandler) getRecord(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}
	result, err := h.service.GetRecord(r.Context(), chi.URLParam(r, "recordID"), principal.UserID, principal.Cached)
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *FinanceHandler) updateRecord(w http.ResponseWriter, r *http.Request) {
	var input hrisdto.UpdateFinanceRecordRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}
	recordID := chi.URLParam(r, "recordID")
	result, err := h.service.UpdateRecord(r.Context(), recordID, input, principal.UserID, principal.Cached)
	if err != nil {
		h.writeError(w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "update", "hris", "finance_record", recordID, nil, input)
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *FinanceHandler) deleteRecord(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}
	recordID := chi.URLParam(r, "recordID")
	if err := h.service.DeleteRecord(r.Context(), recordID, principal.UserID, principal.Cached); err != nil {
		h.writeError(w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "delete", "hris", "finance_record", recordID, nil, nil)
	response.WriteJSON(w, http.StatusOK, map[string]string{"message": "Finance record deleted successfully"}, nil)
}

func (h *FinanceHandler) submitRecord(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}
	recordID := chi.URLParam(r, "recordID")
	result, err := h.service.SubmitRecord(r.Context(), recordID, principal.UserID, principal.Cached)
	if err != nil {
		h.writeError(w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "submit", "hris", "finance_record", recordID, nil, result)
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *FinanceHandler) reviewRecord(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}
	var input hrisdto.ReviewFinanceRecordRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}
	recordID := chi.URLParam(r, "recordID")
	result, err := h.service.ReviewRecord(r.Context(), recordID, input.Decision, principal.UserID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "review", "hris", "finance_record", recordID, nil, input)
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *FinanceHandler) summary(w http.ResponseWriter, r *http.Request) {
	year, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("year")))
	if err != nil || year == 0 {
		year = time.Now().Year()
	}
	result, err := h.service.Summary(r.Context(), year)
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *FinanceHandler) exportCSV(w http.ResponseWriter, r *http.Request) {
	year, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("year")))
	if err != nil || year == 0 {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Query validation failed", map[string]string{"year": "must be a number"})
		return
	}
	month, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("month")))
	payload, err := h.service.ExportCSV(r.Context(), year, month)
	if err != nil {
		h.writeError(w, err)
		return
	}
	filename := "finance-" + strconv.Itoa(year)
	if month > 0 {
		filename += "-" + strconv.Itoa(month)
	}
	filename += ".csv"
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (h *FinanceHandler) parseListQuery(w http.ResponseWriter, r *http.Request) (hrisdto.ListFinanceRecordsQuery, bool) {
	query := hrisdto.ListFinanceRecordsQuery{
		Type:       r.URL.Query().Get("type"),
		CategoryID: r.URL.Query().Get("category"),
		Status:     r.URL.Query().Get("status"),
	}
	if value := strings.TrimSpace(r.URL.Query().Get("page")); value != "" {
		page, err := strconv.Atoi(value)
		if err != nil {
			response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Query validation failed", map[string]string{"page": "must be a number"})
			return hrisdto.ListFinanceRecordsQuery{}, false
		}
		query.Page = page
	}
	if value := strings.TrimSpace(r.URL.Query().Get("per_page")); value != "" {
		perPage, err := strconv.Atoi(value)
		if err != nil {
			response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Query validation failed", map[string]string{"per_page": "must be a number"})
			return hrisdto.ListFinanceRecordsQuery{}, false
		}
		query.PerPage = perPage
	}
	if value := strings.TrimSpace(r.URL.Query().Get("month")); value != "" {
		month, err := strconv.Atoi(value)
		if err != nil {
			response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Query validation failed", map[string]string{"month": "must be a number"})
			return hrisdto.ListFinanceRecordsQuery{}, false
		}
		query.Month = month
	}
	if value := strings.TrimSpace(r.URL.Query().Get("year")); value != "" {
		year, err := strconv.Atoi(value)
		if err != nil {
			response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Query validation failed", map[string]string{"year": "must be a number"})
			return hrisdto.ListFinanceRecordsQuery{}, false
		}
		query.Year = year
	}
	if err := h.validator.Struct(query); err != nil {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Query validation failed", validationDetails(err))
		return hrisdto.ListFinanceRecordsQuery{}, false
	}
	return query, true
}

func (h *FinanceHandler) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, hrisservice.ErrFinanceCategoryNotFound):
		response.WriteError(w, http.StatusNotFound, "FINANCE_CATEGORY_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrFinanceRecordNotFound):
		response.WriteError(w, http.StatusNotFound, "FINANCE_RECORD_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrFinanceCategoryExists):
		response.WriteError(w, http.StatusConflict, "FINANCE_CATEGORY_EXISTS", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrFinanceForbidden):
		response.WriteError(w, http.StatusForbidden, "FINANCE_FORBIDDEN", err.Error(), nil)
	default:
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred", nil)
	}
}
