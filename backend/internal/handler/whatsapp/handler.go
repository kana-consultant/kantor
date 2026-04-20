package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"

	wadto "github.com/kana-consultant/kantor/backend/internal/dto/whatsapp"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	warepo "github.com/kana-consultant/kantor/backend/internal/repository/whatsapp"
	"github.com/kana-consultant/kantor/backend/internal/response"
	waservice "github.com/kana-consultant/kantor/backend/internal/service/whatsapp"
)

type Handler struct {
	service   *waservice.Service
	validator *validator.Validate
}

func New(service *waservice.Service) *Handler {
	return &Handler{
		service:   service,
		validator: validator.New(validator.WithRequiredStructEnabled()),
	}
}

func (h *Handler) RegisterRoutes(router chi.Router) {
	// Config (per-tenant)
	router.With(platformmiddleware.RequirePermission("operational:wa:manage")).Get("/config", h.getWAConfig)
	router.With(platformmiddleware.RequirePermission("operational:wa:manage")).Put("/config", h.updateWAConfig)

	// Connection
	router.With(platformmiddleware.RequirePermission("operational:wa:manage")).Get("/status", h.getStatus)
	router.With(platformmiddleware.RequirePermission("operational:wa:manage")).Get("/qr", h.getQR)
	router.With(platformmiddleware.RequirePermission("operational:wa:manage")).Post("/session/start", h.startSession)
	router.With(platformmiddleware.RequirePermission("operational:wa:manage")).Post("/session/stop", h.stopSession)
	router.With(platformmiddleware.RequirePermission("operational:wa:view")).Get("/stats", h.getStats)

	// Templates
	router.With(platformmiddleware.RequirePermission("operational:wa:view")).Get("/templates", h.listTemplates)
	router.With(platformmiddleware.RequirePermission("operational:wa:manage")).Post("/templates/generate-defaults", h.generateDefaultTemplates)
	router.With(platformmiddleware.RequirePermission("operational:wa:manage")).Post("/templates", h.createTemplate)
	router.With(platformmiddleware.RequirePermission("operational:wa:manage")).Put("/templates/{templateID}", h.updateTemplate)
	router.With(platformmiddleware.RequirePermission("operational:wa:manage")).Delete("/templates/{templateID}", h.deleteTemplate)
	router.With(platformmiddleware.RequirePermission("operational:wa:view")).Post("/templates/{templateID}/preview", h.previewTemplate)

	// Schedules
	router.With(platformmiddleware.RequirePermission("operational:wa:manage")).Get("/schedules", h.listSchedules)
	router.With(platformmiddleware.RequirePermission("operational:wa:manage")).Post("/schedules", h.createSchedule)
	router.With(platformmiddleware.RequirePermission("operational:wa:manage")).Put("/schedules/{scheduleID}", h.updateSchedule)
	router.With(platformmiddleware.RequirePermission("operational:wa:manage")).Delete("/schedules/{scheduleID}", h.deleteSchedule)
	router.With(platformmiddleware.RequirePermission("operational:wa:manage")).Post("/schedules/{scheduleID}/trigger", h.triggerSchedule)
	router.With(platformmiddleware.RequirePermission("operational:wa:manage")).Patch("/schedules/{scheduleID}/toggle", h.toggleSchedule)

	// Logs
	router.With(platformmiddleware.RequirePermission("operational:wa:view")).Get("/logs", h.listLogs)
	router.With(platformmiddleware.RequirePermission("operational:wa:view")).Get("/logs/summary", h.getLogSummary)

	// Quick Send
	router.With(platformmiddleware.RequirePermission("operational:wa:manage")).Post("/send", h.quickSend)

	// User phone
	router.Get("/phone", h.getUserPhone)
	router.Put("/phone", h.updateUserPhone)
}

// --------------- Config ---------------

func (h *Handler) getWAConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.service.GetWAConfig(r.Context())
	if err != nil {
		h.writeInternalError(r.Context(), w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"api_url":            cfg.APIURL,
		"api_key":            cfg.APIKey,
		"session_name":       cfg.SessionName,
		"enabled":            cfg.Enabled,
		"max_daily_messages": cfg.MaxDailyMessages,
		"min_delay_ms":       cfg.MinDelayMS,
		"max_delay_ms":       cfg.MaxDelayMS,
		"reminder_cron":      cfg.ReminderCron,
		"weekly_digest_cron": cfg.WeeklyDigestCron,
	}, nil)
}

func (h *Handler) updateWAConfig(w http.ResponseWriter, r *http.Request) {
	var input wadto.UpdateWAConfigRequest
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	cfg := warepo.WAConfig{
		APIURL:           input.APIURL,
		APIKey:           input.APIKey,
		SessionName:      input.SessionName,
		Enabled:          input.Enabled,
		MaxDailyMessages: input.MaxDailyMessages,
		MinDelayMS:       input.MinDelayMS,
		MaxDelayMS:       input.MaxDelayMS,
		ReminderCron:     input.ReminderCron,
		WeeklyDigestCron: input.WeeklyDigestCron,
	}

	if err := h.service.UpdateWAConfig(r.Context(), cfg); err != nil {
		h.writeInternalError(r.Context(), w, err)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "update", "operational", "wa_config", "", nil, cfg)
	response.WriteJSON(w, http.StatusOK, map[string]string{"message": "WhatsApp configuration updated"}, nil)
}

// --------------- Connection ---------------

func (h *Handler) getStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	status, err := h.service.GetStatus(ctx)
	if err != nil {
		response.WriteError(w, http.StatusBadGateway, "WAHA_ERROR", err.Error(), nil)
		return
	}
	response.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"enabled": h.service.IsEnabled(ctx),
		"session": status,
	}, nil)
}

func (h *Handler) getQR(w http.ResponseWriter, r *http.Request) {
	qr, err := h.service.GetQR(r.Context())
	if err != nil {
		if errors.Is(err, waservice.ErrWAHADisabled) {
			response.WriteError(w, http.StatusConflict, "WA_DISABLED", err.Error(), nil)
			return
		}
		response.WriteError(w, http.StatusBadGateway, "WAHA_ERROR", err.Error(), nil)
		return
	}
	response.WriteJSON(w, http.StatusOK, map[string]string{"qr": qr}, nil)
}

func (h *Handler) startSession(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	if err := h.service.StartSession(r.Context()); err != nil {
		if errors.Is(err, waservice.ErrWAHADisabled) {
			response.WriteError(w, http.StatusConflict, "WA_DISABLED", err.Error(), nil)
			return
		}
		response.WriteError(w, http.StatusBadGateway, "WAHA_ERROR", err.Error(), nil)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "update", "operational", "wa_session", principal.UserID, nil, map[string]any{"status": "started"})
	response.WriteJSON(w, http.StatusOK, map[string]string{"message": "Session started"}, nil)
}

func (h *Handler) stopSession(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	if err := h.service.StopSession(r.Context()); err != nil {
		if errors.Is(err, waservice.ErrWAHADisabled) {
			response.WriteError(w, http.StatusConflict, "WA_DISABLED", err.Error(), nil)
			return
		}
		response.WriteError(w, http.StatusBadGateway, "WAHA_ERROR", err.Error(), nil)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "update", "operational", "wa_session", principal.UserID, nil, map[string]any{"status": "stopped"})
	response.WriteJSON(w, http.StatusOK, map[string]string{"message": "Session stopped"}, nil)
}

func (h *Handler) getStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	stats := h.service.GetDailyStats(ctx)
	info, _ := h.service.GetAccountInfo(ctx)
	response.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"enabled":     h.service.IsEnabled(ctx),
		"daily_stats": stats,
		"account":     info,
	}, nil)
}

// --------------- Templates ---------------

func (h *Handler) listTemplates(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")
	triggerType := r.URL.Query().Get("trigger_type")

	templates, err := h.service.ListTemplates(r.Context(), category, triggerType)
	if err != nil {
		h.writeInternalError(r.Context(), w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, templates, nil)
}

func (h *Handler) createTemplate(w http.ResponseWriter, r *http.Request) {
	var input wadto.CreateTemplateRequest
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	result, err := h.service.CreateTemplate(r.Context(), warepo.CreateTemplateParams{
		Name:               input.Name,
		Slug:               input.Slug,
		Category:           input.Category,
		TriggerType:        input.TriggerType,
		BodyTemplate:       input.BodyTemplate,
		Description:        input.Description,
		AvailableVariables: input.AvailableVariables,
		IsActive:           input.IsActive,
		CreatedBy:          &principal.UserID,
	})
	if err != nil {
		h.writeInternalError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "create", "operational", "wa_template", result.ID, nil, result)
	response.WriteJSON(w, http.StatusCreated, result, nil)
}

func (h *Handler) generateDefaultTemplates(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.EnsureDefaultTemplates(r.Context())
	if err != nil {
		h.writeInternalError(r.Context(), w, err)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "generate_defaults", "operational", "wa_template", "", nil, result)
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *Handler) updateTemplate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "templateID")
	previous, previousErr := h.service.GetTemplate(r.Context(), id)
	if previousErr != nil {
		if errors.Is(previousErr, waservice.ErrTemplateNotFound) {
			response.WriteError(w, http.StatusNotFound, "TEMPLATE_NOT_FOUND", "Template not found", nil)
			return
		}
		h.writeInternalError(r.Context(), w, previousErr)
		return
	}

	var input wadto.UpdateTemplateRequest
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	result, err := h.service.UpdateTemplate(r.Context(), id, warepo.UpdateTemplateParams{
		Name:               input.Name,
		Category:           input.Category,
		TriggerType:        input.TriggerType,
		BodyTemplate:       input.BodyTemplate,
		Description:        input.Description,
		AvailableVariables: input.AvailableVariables,
		IsActive:           input.IsActive,
	})
	if errors.Is(err, waservice.ErrTemplateNotFound) {
		response.WriteError(w, http.StatusNotFound, "TEMPLATE_NOT_FOUND", "Template not found", nil)
		return
	}
	if err != nil {
		h.writeInternalError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "update", "operational", "wa_template", id, previous, result)
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *Handler) deleteTemplate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "templateID")
	previous, previousErr := h.service.GetTemplate(r.Context(), id)
	if previousErr != nil && !errors.Is(previousErr, waservice.ErrTemplateNotFound) {
		h.writeInternalError(r.Context(), w, previousErr)
		return
	}
	err := h.service.DeleteTemplate(r.Context(), id)
	switch {
	case errors.Is(err, waservice.ErrTemplateNotFound):
		response.WriteError(w, http.StatusNotFound, "TEMPLATE_NOT_FOUND", "Template not found", nil)
	case errors.Is(err, waservice.ErrSystemTemplate):
		response.WriteError(w, http.StatusForbidden, "SYSTEM_TEMPLATE", "Cannot delete system template", nil)
	case err != nil:
		h.writeInternalError(r.Context(), w, err)
	default:
		platformmiddleware.AuditLog(r.Context(), "delete", "operational", "wa_template", id, previous, nil)
		response.WriteJSON(w, http.StatusOK, map[string]string{"message": "Template deleted"}, nil)
	}
}

func (h *Handler) previewTemplate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "templateID")
	preview, err := h.service.PreviewTemplate(r.Context(), id)
	if errors.Is(err, waservice.ErrTemplateNotFound) {
		response.WriteError(w, http.StatusNotFound, "TEMPLATE_NOT_FOUND", "Template not found", nil)
		return
	}
	if err != nil {
		h.writeInternalError(r.Context(), w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, map[string]string{"preview": preview}, nil)
}

// --------------- Schedules ---------------

func (h *Handler) listSchedules(w http.ResponseWriter, r *http.Request) {
	schedules, err := h.service.ListSchedules(r.Context())
	if err != nil {
		h.writeInternalError(r.Context(), w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, schedules, nil)
}

func (h *Handler) createSchedule(w http.ResponseWriter, r *http.Request) {
	var input wadto.CreateScheduleRequest
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	result, err := h.service.CreateSchedule(r.Context(), warepo.CreateScheduleParams{
		Name:           input.Name,
		TemplateID:     input.TemplateID,
		ScheduleType:   input.ScheduleType,
		CronExpression: input.CronExpression,
		TargetType:     input.TargetType,
		TargetConfig:   input.TargetConfig,
		IsActive:       input.IsActive,
		CreatedBy:      &principal.UserID,
	})
	if err != nil {
		h.writeInternalError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "create", "operational", "wa_schedule", result.ID, nil, result)
	response.WriteJSON(w, http.StatusCreated, result, nil)
}

func (h *Handler) updateSchedule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "scheduleID")
	previous, previousErr := h.service.GetSchedule(r.Context(), id)
	if previousErr != nil {
		if errors.Is(previousErr, waservice.ErrScheduleNotFound) {
			response.WriteError(w, http.StatusNotFound, "SCHEDULE_NOT_FOUND", "Schedule not found", nil)
			return
		}
		h.writeInternalError(r.Context(), w, previousErr)
		return
	}

	var input wadto.UpdateScheduleRequest
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	result, err := h.service.UpdateSchedule(r.Context(), id, warepo.UpdateScheduleParams{
		Name:           input.Name,
		TemplateID:     input.TemplateID,
		ScheduleType:   input.ScheduleType,
		CronExpression: input.CronExpression,
		TargetType:     input.TargetType,
		TargetConfig:   input.TargetConfig,
		IsActive:       input.IsActive,
	})
	if errors.Is(err, waservice.ErrScheduleNotFound) {
		response.WriteError(w, http.StatusNotFound, "SCHEDULE_NOT_FOUND", "Schedule not found", nil)
		return
	}
	if err != nil {
		h.writeInternalError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "update", "operational", "wa_schedule", id, previous, result)
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *Handler) deleteSchedule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "scheduleID")
	previous, previousErr := h.service.GetSchedule(r.Context(), id)
	if previousErr != nil && !errors.Is(previousErr, waservice.ErrScheduleNotFound) {
		h.writeInternalError(r.Context(), w, previousErr)
		return
	}
	err := h.service.DeleteSchedule(r.Context(), id)
	if errors.Is(err, waservice.ErrScheduleNotFound) {
		response.WriteError(w, http.StatusNotFound, "SCHEDULE_NOT_FOUND", "Schedule not found", nil)
		return
	}
	if err != nil {
		h.writeInternalError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "delete", "operational", "wa_schedule", id, previous, nil)
	response.WriteJSON(w, http.StatusOK, map[string]string{"message": "Schedule deleted"}, nil)
}

func (h *Handler) triggerSchedule(w http.ResponseWriter, r *http.Request) {
	scheduleID := chi.URLParam(r, "scheduleID")
	if err := h.service.TriggerSchedule(r.Context(), scheduleID); err != nil {
		switch {
		case errors.Is(err, waservice.ErrScheduleNotFound):
			response.WriteError(w, http.StatusNotFound, "SCHEDULE_NOT_FOUND", "Schedule not found", nil)
		case errors.Is(err, waservice.ErrWAHADisabled):
			response.WriteError(w, http.StatusConflict, "WA_DISABLED", err.Error(), nil)
		default:
			response.WriteError(w, http.StatusBadRequest, "SCHEDULE_TRIGGER_FAILED", err.Error(), nil)
		}
		return
	}
	platformmiddleware.AuditLog(r.Context(), "trigger", "operational", "wa_schedule", scheduleID, nil, map[string]any{"trigger": "manual"})
	response.WriteJSON(w, http.StatusOK, map[string]string{"message": "Schedule triggered"}, nil)
}

func (h *Handler) toggleSchedule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "scheduleID")
	var input wadto.ToggleRequest
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	result, err := h.service.ToggleSchedule(r.Context(), id, input.IsActive)
	if errors.Is(err, waservice.ErrScheduleNotFound) {
		response.WriteError(w, http.StatusNotFound, "SCHEDULE_NOT_FOUND", "Schedule not found", nil)
		return
	}
	if err != nil {
		h.writeInternalError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "update", "operational", "wa_schedule", id, nil, result)
	response.WriteJSON(w, http.StatusOK, result, nil)
}

// --------------- Logs ---------------

func (h *Handler) listLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	perPage, _ := strconv.Atoi(q.Get("per_page"))

	logs, total, err := h.service.ListLogs(r.Context(), warepo.ListLogsParams{
		Page:         page,
		PerPage:      perPage,
		ScheduleID:   q.Get("schedule_id"),
		TriggerType:  q.Get("trigger_type"),
		TemplateSlug: q.Get("template_slug"),
		Status:       q.Get("status"),
		DateFrom:     q.Get("date_from"),
		DateTo:       q.Get("date_to"),
		Search:       q.Get("search"),
	})
	if err != nil {
		h.writeInternalError(r.Context(), w, err)
		return
	}

	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}

	response.WriteJSON(w, http.StatusOK, logs, map[string]interface{}{
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

func (h *Handler) getLogSummary(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	summary, err := h.service.GetLogSummary(r.Context(), date)
	if err != nil {
		h.writeInternalError(r.Context(), w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, summary, nil)
}

// --------------- Quick Send ---------------

func (h *Handler) quickSend(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	var input wadto.QuickSendRequest
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	if err := h.service.QuickSend(r.Context(), input.Phone, input.Message); err != nil {
		if errors.Is(err, waservice.ErrInvalidPhone) {
			response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), map[string]string{"phone": "must use 08xx, 8xx, 628xx, or +628xx format"})
			return
		}
		if errors.Is(err, waservice.ErrWAHADisabled) {
			response.WriteError(w, http.StatusConflict, "WA_DISABLED", err.Error(), nil)
			return
		}
		response.WriteError(w, http.StatusBadGateway, "SEND_FAILED", err.Error(), nil)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "send", "operational", "wa_broadcast", principal.UserID, nil, map[string]any{
		"phone":          input.Phone,
		"message_length": len([]rune(input.Message)),
	})
	response.WriteJSON(w, http.StatusOK, map[string]string{"message": "Message sent"}, nil)
}

// --------------- User Phone ---------------

func (h *Handler) getUserPhone(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	phone, err := h.service.GetUserPhone(r.Context(), principal.UserID)
	if err != nil {
		h.writeInternalError(r.Context(), w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, map[string]interface{}{"phone": phone}, nil)
}

func (h *Handler) updateUserPhone(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	var input wadto.UpdatePhoneRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Request body must be valid JSON", nil)
		return
	}

	if err := h.service.UpdateUserPhone(r.Context(), principal.UserID, input.Phone); err != nil {
		if errors.Is(err, waservice.ErrInvalidPhone) {
			response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), map[string]string{"phone": "must use 08xx, 8xx, 628xx, or +628xx format"})
			return
		}
		h.writeInternalError(r.Context(), w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, map[string]string{"message": "Phone updated"}, nil)
}

// --------------- Helpers ---------------

func (h *Handler) decodeAndValidate(w http.ResponseWriter, r *http.Request, target interface{}) bool {
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		response.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Request body must be valid JSON", nil)
		return false
	}
	if err := h.validator.Struct(target); err != nil {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Request validation failed", validationDetails(err))
		return false
	}
	return true
}

func (h *Handler) writeInternalError(ctx context.Context, w http.ResponseWriter, err error) {
	response.WriteInternalError(ctx, w, err, "An unexpected error occurred")
}

func validationDetails(err error) map[string]string {
	details := make(map[string]string)
	var validationErrors validator.ValidationErrors
	if errors.As(err, &validationErrors) {
		for _, fe := range validationErrors {
			details[fe.Field()] = fe.Tag()
		}
	}
	return details
}
