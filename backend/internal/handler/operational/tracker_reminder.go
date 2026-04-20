package operational

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"

	operationaldto "github.com/kana-consultant/kantor/backend/internal/dto/operational"
	"github.com/kana-consultant/kantor/backend/internal/httputil"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	"github.com/kana-consultant/kantor/backend/internal/model"
	operationalrepo "github.com/kana-consultant/kantor/backend/internal/repository/operational"
	"github.com/kana-consultant/kantor/backend/internal/response"
	operationalservice "github.com/kana-consultant/kantor/backend/internal/service/operational"
)

type TrackerReminderHandler struct {
	service   *operationalservice.TrackerReminderService
	validator *validator.Validate
}

func NewTrackerReminderHandler(service *operationalservice.TrackerReminderService) *TrackerReminderHandler {
	return &TrackerReminderHandler{
		service:   service,
		validator: validator.New(validator.WithRequiredStructEnabled()),
	}
}

func (h *TrackerReminderHandler) RegisterRoutes(router chi.Router) {
	router.With(platformmiddleware.RequirePermission("operational:tracker-reminder:manage")).Get("/reminder-config", h.getConfig)
	router.With(platformmiddleware.RequirePermission("operational:tracker-reminder:manage")).Put("/reminder-config", h.updateConfig)
	router.With(platformmiddleware.RequirePermission("operational:tracker-reminder:manage")).Post("/reminder-config/test", h.sendTest)
}

func (h *TrackerReminderHandler) getConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.service.GetConfig(r.Context())
	if err != nil {
		platformmiddleware.LoggerFromContext(r.Context()).Error("tracker reminder get config failed", "error", err)
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to load reminder config", nil)
		return
	}
	response.WriteJSON(w, http.StatusOK, h.toResponse(cfg), nil)
}

func (h *TrackerReminderHandler) updateConfig(w http.ResponseWriter, r *http.Request) {
	var req operationaldto.UpdateTrackerReminderConfigRequest
	if !h.decodeAndValidate(w, r, &req) {
		return
	}
	before, _ := h.service.GetConfig(r.Context())
	cfg, err := h.service.UpdateConfig(r.Context(), operationalrepo.UpdateTrackerReminderConfigParams{
		Enabled:               req.Enabled,
		StartHour:             req.StartHour,
		EndHour:               req.EndHour,
		WeekdaysOnly:          req.WeekdaysOnly,
		Timezone:              req.Timezone,
		HeartbeatStaleMinutes: req.HeartbeatStaleMinutes,
		NotifyInApp:           req.NotifyInApp,
		NotifyWhatsapp:        req.NotifyWhatsapp,
	})
	if err != nil {
		if errors.Is(err, operationalservice.ErrTrackerReminderConfigInvalid) {
			response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
			return
		}
		platformmiddleware.LoggerFromContext(r.Context()).Error("tracker reminder update config failed", "error", err)
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update reminder config", nil)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "update", "operational", "tracker_reminder_config", cfg.TenantID, before, cfg)
	response.WriteJSON(w, http.StatusOK, h.toResponse(cfg), nil)
}

func (h *TrackerReminderHandler) sendTest(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}
	inApp, wa, waErr, err := h.service.SendTestReminder(r.Context(), principal.UserID)
	if err != nil {
		platformmiddleware.LoggerFromContext(r.Context()).Error("tracker reminder test dispatch failed", "error", err, "user_id", principal.UserID)
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to send test reminder", nil)
		return
	}
	res := operationaldto.TrackerReminderTestResponse{
		DeliveredInApp:    inApp,
		DeliveredWhatsapp: wa,
	}
	if waErr != nil {
		msg := waErr.Error()
		res.WhatsappError = &msg
	}
	response.WriteJSON(w, http.StatusOK, res, nil)
}

func (h *TrackerReminderHandler) toResponse(cfg model.TrackerReminderConfig) operationaldto.TrackerReminderConfigResponse {
	res := operationaldto.TrackerReminderConfigResponse{
		TenantID:              cfg.TenantID,
		Enabled:               cfg.Enabled,
		StartHour:             cfg.StartHour,
		EndHour:               cfg.EndHour,
		WeekdaysOnly:          cfg.WeekdaysOnly,
		Timezone:              cfg.Timezone,
		HeartbeatStaleMinutes: cfg.HeartbeatStaleMinutes,
		NotifyInApp:           cfg.NotifyInApp,
		NotifyWhatsapp:        cfg.NotifyWhatsapp,
		UpdatedAt:             cfg.UpdatedAt.Format(time.RFC3339),
	}
	if next := h.service.NextReminderAt(cfg, time.Now()); next != nil {
		formatted := next.Format(time.RFC3339)
		res.NextReminderAt = &formatted
	}
	return res
}

func (h *TrackerReminderHandler) decodeAndValidate(w http.ResponseWriter, r *http.Request, target interface{}) bool {
	return httputil.DecodeAndValidate(h.validator, w, r, target)
}
