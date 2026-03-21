package marketing

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"

	marketingdto "github.com/kana-consultant/kantor/backend/internal/dto/marketing"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	"github.com/kana-consultant/kantor/backend/internal/response"
	marketingservice "github.com/kana-consultant/kantor/backend/internal/service/marketing"
)

type AdsMetricsHandler struct {
	service   *marketingservice.AdsMetricsService
	validator *validator.Validate
}

func NewAdsMetricsHandler(service *marketingservice.AdsMetricsService) *AdsMetricsHandler {
	return &AdsMetricsHandler{
		service:   service,
		validator: newValidator(),
	}
}

func (h *AdsMetricsHandler) RegisterRoutes(router chi.Router) {
	router.With(platformmiddleware.RequirePermission("marketing:ads_metrics:create")).Post("/", h.createMetric)
	router.With(platformmiddleware.RequirePermission("marketing:ads_metrics:create")).Post("/batch", h.batchCreateMetrics)
	router.With(platformmiddleware.RequirePermission("marketing:ads_metrics:view")).Get("/", h.listMetrics)
	router.With(platformmiddleware.RequirePermission("marketing:ads_metrics:view")).Get("/summary", h.summary)
	router.With(platformmiddleware.RequirePermission("marketing:ads_metrics:view")).Get("/export", h.exportCSV)
	router.With(platformmiddleware.RequirePermission("marketing:ads_metrics:view")).Get("/{metricID}", h.getMetric)
	router.With(platformmiddleware.RequirePermission("marketing:ads_metrics:edit")).Put("/{metricID}", h.updateMetric)
	router.With(platformmiddleware.RequirePermission("marketing:ads_metrics:delete")).Delete("/{metricID}", h.deleteMetric)
}

func (h *AdsMetricsHandler) createMetric(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}

	var input marketingdto.CreateAdsMetricRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}

	item, err := h.service.CreateMetric(r.Context(), input, principal.UserID)
	if err != nil {
		h.writeError(w, err)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "create", "marketing", "ads_metric", item.ID, nil, input)
	response.WriteJSON(w, http.StatusCreated, item, nil)
}

func (h *AdsMetricsHandler) batchCreateMetrics(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}

	var input marketingdto.BatchCreateAdsMetricsRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}

	items, err := h.service.BatchCreateMetrics(r.Context(), input, principal.UserID)
	if err != nil {
		h.writeError(w, err)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "batch_create", "marketing", "ads_metric", "bulk", nil, input)
	response.WriteJSON(w, http.StatusCreated, items, nil)
}

func (h *AdsMetricsHandler) listMetrics(w http.ResponseWriter, r *http.Request) {
	query, ok := h.parseListQuery(w, r)
	if !ok {
		return
	}

	items, total, page, perPage, err := h.service.ListMetrics(r.Context(), query)
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

func (h *AdsMetricsHandler) getMetric(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.GetMetric(r.Context(), chi.URLParam(r, "metricID"))
	if err != nil {
		h.writeError(w, err)
		return
	}

	response.WriteJSON(w, http.StatusOK, item, nil)
}

func (h *AdsMetricsHandler) updateMetric(w http.ResponseWriter, r *http.Request) {
	var input marketingdto.UpdateAdsMetricRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}

	metricID := chi.URLParam(r, "metricID")
	item, err := h.service.UpdateMetric(r.Context(), metricID, input)
	if err != nil {
		h.writeError(w, err)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "update", "marketing", "ads_metric", metricID, nil, input)
	response.WriteJSON(w, http.StatusOK, item, nil)
}

func (h *AdsMetricsHandler) deleteMetric(w http.ResponseWriter, r *http.Request) {
	metricID := chi.URLParam(r, "metricID")
	if err := h.service.DeleteMetric(r.Context(), metricID); err != nil {
		h.writeError(w, err)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "delete", "marketing", "ads_metric", metricID, nil, nil)
	response.WriteJSON(w, http.StatusOK, map[string]string{"message": "Ads metric deleted successfully"}, nil)
}

func (h *AdsMetricsHandler) summary(w http.ResponseWriter, r *http.Request) {
	query := marketingdto.AdsMetricsSummaryQuery{
		GroupBy:  defaultString(strings.TrimSpace(r.URL.Query().Get("group_by")), "month"),
		DateFrom: strings.TrimSpace(r.URL.Query().Get("date_from")),
		DateTo:   strings.TrimSpace(r.URL.Query().Get("date_to")),
	}
	if err := h.validator.Struct(query); err != nil {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Query validation failed", validationDetails(err))
		return
	}

	item, err := h.service.Summary(r.Context(), query)
	if err != nil {
		h.writeError(w, err)
		return
	}

	response.WriteJSON(w, http.StatusOK, item, nil)
}

func (h *AdsMetricsHandler) exportCSV(w http.ResponseWriter, r *http.Request) {
	format := strings.TrimSpace(r.URL.Query().Get("format"))
	if format == "" {
		format = "csv"
	}

	payload, err := h.service.ExportCSV(
		r.Context(),
		format,
		strings.TrimSpace(r.URL.Query().Get("date_from")),
		strings.TrimSpace(r.URL.Query().Get("date_to")),
	)
	if err != nil {
		h.writeError(w, err)
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=ads-metrics.csv")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (h *AdsMetricsHandler) parseListQuery(w http.ResponseWriter, r *http.Request) (marketingdto.ListAdsMetricsQuery, bool) {
	query := marketingdto.ListAdsMetricsQuery{
		CampaignID: strings.TrimSpace(r.URL.Query().Get("campaign_id")),
		Platform:   strings.TrimSpace(r.URL.Query().Get("platform")),
		DateFrom:   strings.TrimSpace(r.URL.Query().Get("date_from")),
		DateTo:     strings.TrimSpace(r.URL.Query().Get("date_to")),
	}

	if value := strings.TrimSpace(r.URL.Query().Get("page")); value != "" {
		page, err := strconv.Atoi(value)
		if err != nil {
			response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Query validation failed", map[string]string{"page": "must be a number"})
			return marketingdto.ListAdsMetricsQuery{}, false
		}
		query.Page = page
	}

	if value := strings.TrimSpace(r.URL.Query().Get("per_page")); value != "" {
		perPage, err := strconv.Atoi(value)
		if err != nil {
			response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Query validation failed", map[string]string{"per_page": "must be a number"})
			return marketingdto.ListAdsMetricsQuery{}, false
		}
		query.PerPage = perPage
	}

	if err := h.validator.Struct(query); err != nil {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Query validation failed", validationDetails(err))
		return marketingdto.ListAdsMetricsQuery{}, false
	}

	return query, true
}

func (h *AdsMetricsHandler) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, marketingservice.ErrAdsMetricNotFound):
		response.WriteError(w, http.StatusNotFound, "ADS_METRIC_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, marketingservice.ErrAdsMetricCampaignNotFound):
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), map[string]string{"campaign_id": "not found"})
	case errors.Is(err, marketingservice.ErrAdsMetricInvalidPeriod):
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), map[string]string{"period_end": "must be on or after period_start"})
	case errors.Is(err, marketingservice.ErrAdsMetricUnsupportedExport):
		response.WriteError(w, http.StatusBadRequest, "UNSUPPORTED_EXPORT_FORMAT", err.Error(), map[string]string{"format": "must be csv"})
	default:
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred", nil)
	}
}

func defaultString(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
