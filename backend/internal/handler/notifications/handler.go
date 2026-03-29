package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

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
	r.Get("/stream", h.Stream)
	r.Get("/unread-count", h.UnreadCount)
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

func (h *Handler) UnreadCount(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}

	count, err := h.service.CountUnread(r.Context(), principal.UserID)
	if err != nil {
		response.WriteError(w, http.StatusInternalServerError, "NOTIFICATION_COUNT_FAILED", "Unable to load unread notification count", nil)
		return
	}

	response.WriteJSON(w, http.StatusOK, map[string]int64{"unread_count": count}, nil)
}

func (h *Handler) Stream(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		response.WriteError(w, http.StatusInternalServerError, "STREAM_UNSUPPORTED", "Streaming is not supported", nil)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	lastSignature := ""
	if err := h.writeNotificationSnapshot(r.Context(), w, principal.UserID, &lastSignature); err != nil {
		return
	}
	flusher.Flush()

	pollTicker := time.NewTicker(10 * time.Second)
	defer pollTicker.Stop()

	heartbeatTicker := time.NewTicker(25 * time.Second)
	defer heartbeatTicker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-pollTicker.C:
			if err := h.writeNotificationSnapshot(r.Context(), w, principal.UserID, &lastSignature); err != nil {
				return
			}
			flusher.Flush()
		case <-heartbeatTicker.C:
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (h *Handler) writeNotificationSnapshot(ctx context.Context, w http.ResponseWriter, userID string, lastSignature *string) error {
	state, err := platformmiddleware.WithScopedTenantConn(ctx, func(scopedCtx context.Context) (notificationsservice.StreamState, error) {
		return h.service.GetStreamState(scopedCtx, userID)
	})
	if err != nil {
		return err
	}

	signature := fmt.Sprintf("%d:%s", state.UnreadCount, state.LatestID)
	if signature == *lastSignature {
		return nil
	}
	*lastSignature = signature

	payload, err := json.Marshal(map[string]interface{}{
		"unread_count": state.UnreadCount,
		"latest_id":    state.LatestID,
	})
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(w, "event: notifications_updated\ndata: %s\n\n", payload)
	return err
}
