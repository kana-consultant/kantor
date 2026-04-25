package operational

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"

	operationaldto "github.com/kana-consultant/kantor/backend/internal/dto/operational"
	"github.com/kana-consultant/kantor/backend/internal/httputil"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	"github.com/kana-consultant/kantor/backend/internal/response"
	operationalservice "github.com/kana-consultant/kantor/backend/internal/service/operational"
)

type TrackerHandler struct {
	service   *operationalservice.TrackerService
	validator *validator.Validate
}

func NewTrackerHandler(service *operationalservice.TrackerService) *TrackerHandler {
	return &TrackerHandler{
		service:   service,
		validator: validator.New(validator.WithRequiredStructEnabled()),
	}
}

func (h *TrackerHandler) RegisterRoutes(router chi.Router) {
	router.With(platformmiddleware.RequirePermission("operational:tracker:view")).Get("/consent", h.getConsent)
	router.With(platformmiddleware.RequirePermission("operational:tracker:view")).Post("/consent", h.giveConsent)
	router.With(platformmiddleware.RequirePermission("operational:tracker:view")).Delete("/consent", h.revokeConsent)

	router.With(platformmiddleware.RequirePermission("operational:tracker:view")).Post("/sessions/start", h.startSession)
	router.With(platformmiddleware.RequirePermission("operational:tracker:view")).Patch("/sessions/{sessionID}/end", h.endSession)
	router.With(platformmiddleware.RequirePermission("operational:tracker:view")).Post("/heartbeat", h.heartbeat)
	router.With(platformmiddleware.RequirePermission("operational:tracker:view")).Post("/entries/batch", h.batchEntries)

	router.With(platformmiddleware.RequirePermission("operational:tracker:view")).Get("/my-activity", h.getMyActivity)
	router.With(platformmiddleware.RequirePermission("operational:tracker:view")).Get("/extension/download", h.downloadExtension)
	router.With(platformmiddleware.RequirePermission("operational:tracker:view_team")).Get("/team-activity", h.getTeamActivity)
	router.With(platformmiddleware.RequirePermission("operational:tracker:view_team")).Get("/activity/{userID}", h.getUserActivity)
	router.With(platformmiddleware.RequirePermission("operational:tracker:view_team")).Get("/summary", h.getSummary)
	router.With(platformmiddleware.RequirePermission("operational:tracker:view_team")).Get("/consents", h.listConsentAudit)

	router.With(platformmiddleware.RequirePermission("operational:tracker:manage_domains")).Get("/domains", h.listDomains)
	router.With(platformmiddleware.RequirePermission("operational:tracker:manage_domains")).Get("/domains/observed", h.listObservedDomains)
	router.With(platformmiddleware.RequirePermission("operational:tracker:manage_domains")).Post("/domains", h.createDomain)
	router.With(platformmiddleware.RequirePermission("operational:tracker:manage_domains")).Post("/domains/observed/bulk-classify", h.bulkClassifyObservedDomains)
	router.With(platformmiddleware.RequirePermission("operational:tracker:manage_domains")).Put("/domains/{domainID}", h.updateDomain)
	router.With(platformmiddleware.RequirePermission("operational:tracker:manage_domains")).Delete("/domains/{domainID}", h.deleteDomain)
}

func (h *TrackerHandler) getConsent(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	consent, err := h.service.GetConsent(r.Context(), principal.UserID)
	if err != nil {
		response.WriteInternalError(r.Context(), w, err, "Failed to load tracker consent")
		return
	}
	response.WriteJSON(w, http.StatusOK, consent, nil)
}

func (h *TrackerHandler) giveConsent(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	consent, err := h.service.GiveConsent(r.Context(), principal.UserID, platformmiddleware.ClientIPFromContext(r.Context()), time.Now())
	if err != nil {
		platformmiddleware.LoggerFromContext(r.Context()).Error("tracker give consent failed", "error", err, "user_id", principal.UserID)
		response.WriteInternalError(r.Context(), w, err, "Failed to save tracker consent")
		return
	}
	platformmiddleware.AuditLog(r.Context(), "update", "operational", "tracker_consent", principal.UserID, nil, consent)
	response.WriteJSON(w, http.StatusOK, consent, nil)
}

func (h *TrackerHandler) revokeConsent(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	consent, err := h.service.RevokeConsent(r.Context(), principal.UserID, platformmiddleware.ClientIPFromContext(r.Context()), time.Now())
	if err != nil {
		platformmiddleware.LoggerFromContext(r.Context()).Error("tracker revoke consent failed", "error", err, "user_id", principal.UserID)
		response.WriteInternalError(r.Context(), w, err, "Failed to revoke tracker consent")
		return
	}
	platformmiddleware.AuditLog(r.Context(), "update", "operational", "tracker_consent", principal.UserID, nil, consent)
	response.WriteJSON(w, http.StatusOK, consent, nil)
}

func (h *TrackerHandler) startSession(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	var request operationaldto.TrackerStartSessionRequest
	if r.ContentLength > 0 {
		if !h.decodeAndValidate(w, r, &request) {
			return
		}
	}

	session, err := h.service.StartSession(r.Context(), principal.UserID, request, time.Now())
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	response.WriteJSON(w, http.StatusCreated, map[string]string{"session_id": session.ID}, nil)
}

func (h *TrackerHandler) endSession(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	var request operationaldto.TrackerEndSessionRequest
	if r.ContentLength > 0 {
		if !h.decodeAndValidate(w, r, &request) {
			return
		}
	}

	endedAt := time.Now()
	if request.Timestamp != nil {
		endedAt = *request.Timestamp
	}

	session, err := h.service.EndSession(r.Context(), principal.UserID, chi.URLParam(r, "sessionID"), endedAt)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, session, nil)
}

func (h *TrackerHandler) heartbeat(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	var request operationaldto.TrackerHeartbeatRequest
	if !h.decodeAndValidate(w, r, &request) {
		return
	}

	entry, session, err := h.service.RecordHeartbeat(r.Context(), principal.UserID, request)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}

	response.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"entry":   entry,
		"session": session,
	}, nil)
}

func (h *TrackerHandler) batchEntries(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	var request operationaldto.TrackerBatchEntriesRequest
	if !h.decodeAndValidate(w, r, &request) {
		return
	}

	result, err := h.service.RecordBatch(r.Context(), principal.UserID, request)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}

	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *TrackerHandler) getMyActivity(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	dateFrom, dateTo, ok := parseDateRange(w, r)
	if !ok {
		return
	}

	activity, err := h.service.GetMyActivity(r.Context(), principal.UserID, dateFrom, dateTo)
	if err != nil {
		slog.ErrorContext(r.Context(), "failed to load tracker activity", "error", err, "userID", principal.UserID)
		response.WriteInternalError(r.Context(), w, err, "Failed to load tracker activity")
		return
	}
	response.WriteJSON(w, http.StatusOK, activity, nil)
}

func (h *TrackerHandler) downloadExtension(w http.ResponseWriter, r *http.Request) {
	archiveBytes, filename, err := h.service.BuildExtensionArchive(r.Context())
	if err != nil {
		slog.ErrorContext(r.Context(), "failed to build extension archive", "error", err)
		response.WriteError(w, http.StatusInternalServerError, "TRACKER_EXTENSION_UNAVAILABLE", "Tracker extension package is not available right now", nil)
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(archiveBytes)
}

func (h *TrackerHandler) getTeamActivity(w http.ResponseWriter, r *http.Request) {
	dateFrom, dateTo, ok := parseDateRange(w, r)
	if !ok {
		return
	}

	var userID *string
	if raw := strings.TrimSpace(r.URL.Query().Get("user_id")); raw != "" {
		userID = &raw
	}

	activity, err := h.service.GetTeamActivity(r.Context(), dateFrom, dateTo, userID)
	if err != nil {
		response.WriteInternalError(r.Context(), w, err, "Failed to load team tracker activity")
		return
	}
	response.WriteJSON(w, http.StatusOK, activity, nil)
}

func (h *TrackerHandler) getUserActivity(w http.ResponseWriter, r *http.Request) {
	dateFrom, dateTo, ok := parseDateRange(w, r)
	if !ok {
		return
	}

	activity, err := h.service.GetUserActivity(r.Context(), chi.URLParam(r, "userID"), dateFrom, dateTo)
	if err != nil {
		response.WriteInternalError(r.Context(), w, err, "Failed to load user tracker activity")
		return
	}
	response.WriteJSON(w, http.StatusOK, activity, nil)
}

func (h *TrackerHandler) getSummary(w http.ResponseWriter, r *http.Request) {
	date := time.Now()
	if raw := strings.TrimSpace(r.URL.Query().Get("date")); raw != "" {
		parsed, err := time.Parse("2006-01-02", raw)
		if err != nil {
			response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "date must use YYYY-MM-DD format", map[string]string{"date": "invalid_date"})
			return
		}
		date = parsed
	}

	summary, err := h.service.GetDailySummary(r.Context(), date)
	if err != nil {
		response.WriteInternalError(r.Context(), w, err, "Failed to load tracker summary")
		return
	}
	response.WriteJSON(w, http.StatusOK, summary, nil)
}

func (h *TrackerHandler) listDomains(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListDomainCategories(r.Context())
	if err != nil {
		response.WriteInternalError(r.Context(), w, err, "Failed to list tracker domains")
		return
	}
	response.WriteJSON(w, http.StatusOK, items, nil)
}

func (h *TrackerHandler) listObservedDomains(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListObservedDomains(r.Context())
	if err != nil {
		response.WriteInternalError(r.Context(), w, err, "Failed to list tracked domains")
		return
	}
	response.WriteJSON(w, http.StatusOK, items, nil)
}

func (h *TrackerHandler) listConsentAudit(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListConsentAudit(r.Context())
	if err != nil {
		response.WriteInternalError(r.Context(), w, err, "Failed to load tracker consent audit")
		return
	}
	response.WriteJSON(w, http.StatusOK, items, nil)
}

func (h *TrackerHandler) createDomain(w http.ResponseWriter, r *http.Request) {
	var request operationaldto.DomainCategoryRequest
	if !h.decodeAndValidate(w, r, &request) {
		return
	}

	item, err := h.service.CreateDomainCategory(r.Context(), request)
	if err != nil {
		response.WriteInternalError(r.Context(), w, err, "Failed to create tracker domain")
		return
	}
	platformmiddleware.AuditLog(r.Context(), "create", "operational", "tracker_domain", item.ID, nil, item)
	response.WriteJSON(w, http.StatusCreated, item, nil)
}

func (h *TrackerHandler) bulkClassifyObservedDomains(w http.ResponseWriter, r *http.Request) {
	var request operationaldto.TrackerBulkClassifyDomainsRequest
	if !h.decodeAndValidate(w, r, &request) {
		return
	}

	result, err := h.service.BulkClassifyObservedDomains(r.Context(), request)
	if err != nil {
		response.WriteInternalError(r.Context(), w, err, "Failed to bulk classify tracker domains")
		return
	}
	platformmiddleware.AuditLog(r.Context(), "update", "operational", "tracker_observed_domains", strings.Join(result.Domains, ","), nil, result)
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *TrackerHandler) updateDomain(w http.ResponseWriter, r *http.Request) {
	var request operationaldto.DomainCategoryRequest
	if !h.decodeAndValidate(w, r, &request) {
		return
	}

	item, err := h.service.UpdateDomainCategory(r.Context(), chi.URLParam(r, "domainID"), request)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "update", "operational", "tracker_domain", item.ID, nil, item)
	response.WriteJSON(w, http.StatusOK, item, nil)
}

func (h *TrackerHandler) deleteDomain(w http.ResponseWriter, r *http.Request) {
	domainID := chi.URLParam(r, "domainID")
	if err := h.service.DeleteDomainCategory(r.Context(), domainID); err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "delete", "operational", "tracker_domain", domainID, nil, nil)
	response.WriteJSON(w, http.StatusOK, map[string]string{"message": "Domain category deleted successfully"}, nil)
}

func (h *TrackerHandler) decodeAndValidate(w http.ResponseWriter, r *http.Request, target interface{}) bool {
	return httputil.DecodeAndValidate(h.validator, w, r, target)
}

func (h *TrackerHandler) writeError(ctx context.Context, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, operationalservice.ErrConsentRequired):
		response.WriteError(w, http.StatusForbidden, "CONSENT_REQUIRED", err.Error(), nil)
	case errors.Is(err, operationalservice.ErrTrackerSessionNotFound):
		response.WriteError(w, http.StatusNotFound, "TRACKER_SESSION_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, operationalservice.ErrDomainCategoryNotFound):
		response.WriteError(w, http.StatusNotFound, "DOMAIN_CATEGORY_NOT_FOUND", err.Error(), nil)
	default:
		response.WriteInternalError(ctx, w, err, "An unexpected error occurred")
	}
}

func parseDateRange(w http.ResponseWriter, r *http.Request) (time.Time, time.Time, bool) {
	now := time.Now()
	dateFrom := now
	dateTo := now

	if raw := strings.TrimSpace(r.URL.Query().Get("date_from")); raw != "" {
		parsed, err := time.Parse("2006-01-02", raw)
		if err != nil {
			response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "date_from must use YYYY-MM-DD format", map[string]string{"date_from": "invalid_date"})
			return time.Time{}, time.Time{}, false
		}
		dateFrom = parsed
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("date_to")); raw != "" {
		parsed, err := time.Parse("2006-01-02", raw)
		if err != nil {
			response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "date_to must use YYYY-MM-DD format", map[string]string{"date_to": "invalid_date"})
			return time.Time{}, time.Time{}, false
		}
		dateTo = parsed
	}
	if dateTo.Before(dateFrom) {
		dateTo = dateFrom
	}
	return dateFrom, dateTo, true
}

