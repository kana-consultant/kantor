package notifications

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	"github.com/kana-consultant/kantor/backend/internal/response"
	notificationsservice "github.com/kana-consultant/kantor/backend/internal/service/notifications"
)

type Handler struct {
	service *notificationsservice.Service
}

func New(service *notificationsservice.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/", h.List)
	r.Patch("/{notificationID}/read", h.MarkRead)
	r.Patch("/read-all", h.MarkAllRead)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}

	page, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("page")))
	perPage, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("per_page")))

	var read *bool
	if rawRead := strings.TrimSpace(r.URL.Query().Get("read")); rawRead != "" {
		parsed := rawRead == "true" || rawRead == "1"
		read = &parsed
	}

	items, total, err := h.service.List(r.Context(), notificationsservice.ListParams{
		UserID:  principal.UserID,
		Read:    read,
		Page:    page,
		PerPage: perPage,
	})
	if err != nil {
		response.WriteError(w, http.StatusInternalServerError, "NOTIFICATIONS_LIST_FAILED", "Unable to load notifications", nil)
		return
	}

	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}

	response.WriteJSON(w, http.StatusOK, items, map[string]int64{
		"page":     int64(page),
		"per_page": int64(perPage),
		"total":    total,
	})
}

func (h *Handler) MarkRead(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}

	if err := h.service.MarkRead(r.Context(), chi.URLParam(r, "notificationID"), principal.UserID); err != nil {
		if errors.Is(err, notificationsservice.ErrNotificationNotFound) {
			response.WriteError(w, http.StatusNotFound, "NOTIFICATION_NOT_FOUND", "Notification not found", nil)
			return
		}
		response.WriteError(w, http.StatusInternalServerError, "NOTIFICATION_READ_FAILED", "Unable to mark notification as read", nil)
		return
	}

	response.WriteJSON(w, http.StatusOK, map[string]bool{"marked": true}, nil)
}

func (h *Handler) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}

	if err := h.service.MarkAllRead(r.Context(), principal.UserID); err != nil {
		response.WriteError(w, http.StatusInternalServerError, "NOTIFICATION_READ_ALL_FAILED", "Unable to mark all notifications as read", nil)
		return
	}

	response.WriteJSON(w, http.StatusOK, map[string]bool{"marked_all": true}, nil)
}
