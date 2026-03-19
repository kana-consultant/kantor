package hris

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"

	hrisdto "github.com/kana-consultant/kantor/backend/internal/dto/hris"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	"github.com/kana-consultant/kantor/backend/internal/response"
	hrisservice "github.com/kana-consultant/kantor/backend/internal/service/hris"
)

type SubscriptionsHandler struct {
	service   *hrisservice.SubscriptionsService
	validator *validator.Validate
}

func NewSubscriptionsHandler(service *hrisservice.SubscriptionsService) *SubscriptionsHandler {
	return &SubscriptionsHandler{
		service:   service,
		validator: newValidator(),
	}
}

func (h *SubscriptionsHandler) RegisterRoutes(router chi.Router) {
	router.With(platformmiddleware.RBACMiddleware("hris:subscription:create")).Post("/", h.createSubscription)
	router.With(platformmiddleware.RBACMiddleware("hris:subscription:view")).Get("/", h.listSubscriptions)
	router.With(platformmiddleware.RBACMiddleware("hris:subscription:view")).Get("/summary", h.summary)
	router.With(platformmiddleware.RBACMiddleware("hris:subscription:view")).Get("/alerts", h.listAlerts)
	router.With(platformmiddleware.RBACMiddleware("hris:subscription:view")).Get("/{subscriptionID}", h.getSubscription)
	router.With(platformmiddleware.RBACMiddleware("hris:subscription:edit")).Put("/{subscriptionID}", h.updateSubscription)
	router.With(platformmiddleware.RBACMiddleware("hris:subscription:delete")).Delete("/{subscriptionID}", h.deleteSubscription)
	router.With(platformmiddleware.RBACMiddleware("hris:subscription:view")).Patch("/alerts/{alertID}/read", h.markAlertRead)
}

func (h *SubscriptionsHandler) createSubscription(w http.ResponseWriter, r *http.Request) {
	var input hrisdto.CreateSubscriptionRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}
	result, err := h.service.CreateSubscription(r.Context(), input, principal.UserID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "create", "hris", "subscription", result.ID, nil, input)
	response.WriteJSON(w, http.StatusCreated, result, nil)
}

func (h *SubscriptionsHandler) listSubscriptions(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.ListSubscriptions(r.Context())
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *SubscriptionsHandler) getSubscription(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.GetSubscription(r.Context(), chi.URLParam(r, "subscriptionID"))
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *SubscriptionsHandler) updateSubscription(w http.ResponseWriter, r *http.Request) {
	var input hrisdto.UpdateSubscriptionRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}
	subscriptionID := chi.URLParam(r, "subscriptionID")
	result, err := h.service.UpdateSubscription(r.Context(), subscriptionID, input, principal.UserID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "update", "hris", "subscription", subscriptionID, nil, input)
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *SubscriptionsHandler) deleteSubscription(w http.ResponseWriter, r *http.Request) {
	subscriptionID := chi.URLParam(r, "subscriptionID")
	if err := h.service.DeleteSubscription(r.Context(), subscriptionID); err != nil {
		h.writeError(w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "delete", "hris", "subscription", subscriptionID, nil, nil)
	response.WriteJSON(w, http.StatusOK, map[string]string{"message": "Subscription deleted successfully"}, nil)
}

func (h *SubscriptionsHandler) summary(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.Summary(r.Context())
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *SubscriptionsHandler) listAlerts(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.ListAlerts(r.Context())
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *SubscriptionsHandler) markAlertRead(w http.ResponseWriter, r *http.Request) {
	if err := h.service.MarkAlertRead(r.Context(), chi.URLParam(r, "alertID")); err != nil {
		h.writeError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, map[string]string{"message": "Alert marked as read"}, nil)
}

func (h *SubscriptionsHandler) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, hrisservice.ErrSubscriptionNotFound):
		response.WriteError(w, http.StatusNotFound, "SUBSCRIPTION_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrSubscriptionAlertNotFound):
		response.WriteError(w, http.StatusNotFound, "SUBSCRIPTION_ALERT_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrEmployeeNotFound):
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), map[string]string{"pic_employee_id": "employee not found"})
	default:
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred", nil)
	}
}
