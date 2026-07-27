package operational

import (
	"errors"
	"net/http"
	"strings"
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

type DiscordReminderHandler struct {
	service   *operationalservice.DiscordReminderService
	validator *validator.Validate
}

func NewDiscordReminderHandler(service *operationalservice.DiscordReminderService) *DiscordReminderHandler {
	return &DiscordReminderHandler{
		service:   service,
		validator: validator.New(validator.WithRequiredStructEnabled()),
	}
}

// Config is stored in the DB and set only via super-admin API — prod env is
// managed by an external nix-config we can't touch.
func (h *DiscordReminderHandler) RegisterRoutes(router chi.Router) {
	router.With(platformmiddleware.SuperAdminMiddleware()).Get("/config", h.getConfig)
	router.With(platformmiddleware.SuperAdminMiddleware()).Put("/config", h.updateConfig)
	router.With(platformmiddleware.SuperAdminMiddleware()).Post("/config/test", h.sendTest)
}

func (h *DiscordReminderHandler) getConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.service.GetConfig(r.Context())
	if err != nil {
		platformmiddleware.LoggerFromContext(r.Context()).Error("discord reminder get config failed", "error", err)
		response.WriteInternalError(r.Context(), w, err, "Failed to load discord reminder config")
		return
	}
	response.WriteJSON(w, http.StatusOK, toDiscordReminderResponse(cfg), nil)
}

func (h *DiscordReminderHandler) updateConfig(w http.ResponseWriter, r *http.Request) {
	var req operationaldto.UpdateDiscordReminderConfigRequest
	if !httputil.DecodeAndValidate(h.validator, w, r, &req) {
		return
	}

	before, err := h.service.GetConfig(r.Context())
	if err != nil {
		platformmiddleware.LoggerFromContext(r.Context()).Error("discord reminder get config failed", "error", err)
		response.WriteInternalError(r.Context(), w, err, "Failed to load discord reminder config")
		return
	}

	// Keep the existing stored secret when the request omits it — never
	// overwrite with blank.
	sharedSecret := strings.TrimSpace(req.SharedSecret)
	if sharedSecret == "" {
		sharedSecret = before.SharedSecret
	}

	cfg, err := h.service.UpdateConfig(r.Context(), operationalrepo.UpdateDiscordReminderConfigParams{
		Enabled:      req.Enabled,
		WebhookURL:   strings.TrimSpace(req.WebhookURL),
		SharedSecret: sharedSecret,
		SendHour:     req.SendHour,
		WeekdaysOnly: req.WeekdaysOnly,
		Timezone:     strings.TrimSpace(req.Timezone),
	})
	if err != nil {
		platformmiddleware.LoggerFromContext(r.Context()).Error("discord reminder update config failed", "error", err)
		response.WriteInternalError(r.Context(), w, err, "Failed to update discord reminder config")
		return
	}

	// Audit trail must never carry the plaintext secret either.
	auditBefore, auditAfter := before, cfg
	auditBefore.SharedSecret, auditAfter.SharedSecret = "", ""
	platformmiddleware.AuditLog(r.Context(), "update", "operational", "discord_reminder_config", cfg.TenantID, auditBefore, auditAfter)

	response.WriteJSON(w, http.StatusOK, toDiscordReminderResponse(cfg), nil)
}

func (h *DiscordReminderHandler) sendTest(w http.ResponseWriter, r *http.Request) {
	sent, err := h.service.SendTestDigest(r.Context(), time.Now())
	if err != nil {
		if errors.Is(err, operationalservice.ErrDiscordReminderDisabled) {
			response.WriteError(w, http.StatusBadRequest, "DISCORD_REMINDER_DISABLED", "Discord reminder push is disabled for this tenant", nil)
			return
		}
		platformmiddleware.LoggerFromContext(r.Context()).Error("discord reminder test push failed", "error", err)
		response.WriteInternalError(r.Context(), w, err, "Failed to send discord reminder digest")
		return
	}
	response.WriteJSON(w, http.StatusOK, operationaldto.DiscordReminderTestResponse{Sent: sent}, nil)
}

func toDiscordReminderResponse(cfg model.DiscordReminderConfig) operationaldto.DiscordReminderConfigResponse {
	return operationaldto.DiscordReminderConfigResponse{
		TenantID:     cfg.TenantID,
		Enabled:      cfg.Enabled,
		WebhookURL:   cfg.WebhookURL,
		HasSecret:    strings.TrimSpace(cfg.SharedSecret) != "",
		SendHour:     cfg.SendHour,
		WeekdaysOnly: cfg.WeekdaysOnly,
		Timezone:     cfg.Timezone,
		UpdatedAt:    cfg.UpdatedAt.Format(time.RFC3339),
	}
}
