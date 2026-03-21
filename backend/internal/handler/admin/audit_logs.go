package admin

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	auditrepo "github.com/kana-consultant/kantor/backend/internal/repository/audit"
	"github.com/kana-consultant/kantor/backend/internal/response"
	auditservice "github.com/kana-consultant/kantor/backend/internal/service/audit"
)

type AuditLogsHandler struct {
	service *auditservice.Service
}

func NewAuditLogsHandler(service *auditservice.Service) *AuditLogsHandler {
	return &AuditLogsHandler{service: service}
}

func (h *AuditLogsHandler) RegisterRoutes(router chi.Router) {
	router.With(platformmiddleware.RequirePermission("admin:audit_log:view")).Get("/", h.list)
	router.With(platformmiddleware.RequirePermission("admin:audit_log:view")).Get("/summary", h.summary)
	router.With(platformmiddleware.RequirePermission("admin:audit_log:view")).Get("/users", h.listUsers)
	router.With(platformmiddleware.RequirePermission("admin:audit_log:export")).Get("/export", h.exportCSV)
}

func (h *AuditLogsHandler) list(w http.ResponseWriter, r *http.Request) {
	params, ok := parseListParams(w, r, true)
	if !ok {
		return
	}

	items, total, err := h.service.ListLogs(r.Context(), params)
	if err != nil {
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list audit logs", nil)
		return
	}

	page := params.Page
	if page <= 0 {
		page = 1
	}
	perPage := params.PerPage
	if perPage <= 0 {
		perPage = 20
	}

	response.WriteJSON(w, http.StatusOK, items, map[string]any{
		"page":     page,
		"per_page": perPage,
		"total":    total,
	})
}

func (h *AuditLogsHandler) summary(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.GetSummary(r.Context())
	if err != nil {
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to load audit log summary", nil)
		return
	}

	response.WriteJSON(w, http.StatusOK, item, nil)
}

func (h *AuditLogsHandler) listUsers(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListActors(r.Context(), strings.TrimSpace(r.URL.Query().Get("search")))
	if err != nil {
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to load audit log users", nil)
		return
	}

	response.WriteJSON(w, http.StatusOK, items, nil)
}

func (h *AuditLogsHandler) exportCSV(w http.ResponseWriter, r *http.Request) {
	params, ok := parseListParams(w, r, false)
	if !ok {
		return
	}

	payload, err := h.service.ExportCSV(r.Context(), params)
	if err != nil {
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to export audit logs", nil)
		return
	}

	filename := "audit-logs-" + time.Now().Format("20060102-150405") + ".csv"
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func parseListParams(w http.ResponseWriter, r *http.Request, includePagination bool) (auditrepo.ListParams, bool) {
	params := auditrepo.ListParams{
		Module:     strings.TrimSpace(r.URL.Query().Get("module")),
		Action:     strings.TrimSpace(r.URL.Query().Get("action")),
		UserID:     strings.TrimSpace(r.URL.Query().Get("user_id")),
		Resource:   strings.TrimSpace(r.URL.Query().Get("resource")),
		ResourceID: strings.TrimSpace(r.URL.Query().Get("resource_id")),
		Search:     strings.TrimSpace(r.URL.Query().Get("search")),
	}

	dateFrom, ok := parseDate(w, "date_from", strings.TrimSpace(r.URL.Query().Get("date_from")))
	if !ok {
		return auditrepo.ListParams{}, false
	}
	if dateFrom != nil {
		params.DateFrom = dateFrom
	}

	dateTo, ok := parseDate(w, "date_to", strings.TrimSpace(r.URL.Query().Get("date_to")))
	if !ok {
		return auditrepo.ListParams{}, false
	}
	if dateTo != nil {
		nextDay := dateTo.Add(24 * time.Hour)
		params.DateTo = &nextDay
	}

	if includePagination {
		if value := strings.TrimSpace(r.URL.Query().Get("page")); value != "" {
			page, err := strconv.Atoi(value)
			if err != nil {
				response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "page must be a number", map[string]string{"page": "invalid_number"})
				return auditrepo.ListParams{}, false
			}
			params.Page = page
		}
		if value := strings.TrimSpace(r.URL.Query().Get("per_page")); value != "" {
			perPage, err := strconv.Atoi(value)
			if err != nil {
				response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "per_page must be a number", map[string]string{"per_page": "invalid_number"})
				return auditrepo.ListParams{}, false
			}
			params.PerPage = perPage
		}
	}

	return params, true
}

func parseDate(w http.ResponseWriter, field string, raw string) (*time.Time, bool) {
	if raw == "" {
		return nil, true
	}

	value, err := time.Parse("2006-01-02", raw)
	if err != nil {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", field+" must use YYYY-MM-DD format", map[string]string{field: "invalid_date"})
		return nil, false
	}

	return &value, true
}
