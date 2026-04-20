package hris

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"

	hrisdto "github.com/kana-consultant/kantor/backend/internal/dto/hris"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	"github.com/kana-consultant/kantor/backend/internal/response"
	hrisservice "github.com/kana-consultant/kantor/backend/internal/service/hris"
)

type CompensationHandler struct {
	service   *hrisservice.CompensationService
	validator *validator.Validate
}

func NewCompensationHandler(service *hrisservice.CompensationService) *CompensationHandler {
	return &CompensationHandler{
		service:   service,
		validator: newValidator(),
	}
}

func (h *CompensationHandler) RegisterSalaryRoutes(router chi.Router) {
	router.With(platformmiddleware.RequirePermission("hris:salary:create")).Post("/", h.createSalary)
	router.Get("/", h.listSalaries)
	router.Get("/current", h.getCurrentSalary)
}

func (h *CompensationHandler) RegisterBonusRoutes(router chi.Router) {
	router.With(platformmiddleware.RequirePermission("hris:bonus:create")).Post("/", h.createBonus)
	router.Get("/", h.listBonuses)
}

func (h *CompensationHandler) createSalary(w http.ResponseWriter, r *http.Request) {
	var input hrisdto.CreateSalaryRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}

	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	result, err := h.service.CreateSalary(r.Context(), chi.URLParam(r, "employeeID"), input, principal.UserID)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "create", "hris", "salary", result.ID, nil, input)
	response.WriteJSON(w, http.StatusCreated, result, nil)
}

func (h *CompensationHandler) listSalaries(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	result, err := h.service.ListSalaries(r.Context(), chi.URLParam(r, "employeeID"), principal.UserID, principal.Cached)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "view", "hris", "salary", chi.URLParam(r, "employeeID"), nil, map[string]any{
		"scope":         "history",
		"employee_id":   chi.URLParam(r, "employeeID"),
		"records_count": len(result),
	})
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *CompensationHandler) getCurrentSalary(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	result, err := h.service.GetCurrentSalary(r.Context(), chi.URLParam(r, "employeeID"), principal.UserID, principal.Cached)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "view", "hris", "salary", chi.URLParam(r, "employeeID"), nil, map[string]any{
		"scope":       "current",
		"employee_id": chi.URLParam(r, "employeeID"),
		"salary_id":   result.ID,
	})
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *CompensationHandler) createBonus(w http.ResponseWriter, r *http.Request) {
	var input hrisdto.CreateBonusRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}

	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	result, err := h.service.CreateBonus(r.Context(), chi.URLParam(r, "employeeID"), input, principal.UserID)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "create", "hris", "bonus", result.ID, nil, input)
	response.WriteJSON(w, http.StatusCreated, result, nil)
}

func (h *CompensationHandler) listBonuses(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}
	result, err := h.service.ListBonuses(r.Context(), chi.URLParam(r, "employeeID"), principal.UserID, principal.Cached)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *CompensationHandler) ApproveBonus(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}
	bonusID := chi.URLParam(r, "bonusID")
	result, err := h.service.ApproveBonus(r.Context(), bonusID, principal.UserID)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "approve", "hris", "bonus", bonusID, nil, result)
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *CompensationHandler) UpdateBonus(w http.ResponseWriter, r *http.Request) {
	var input hrisdto.UpdateBonusRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	bonusID := chi.URLParam(r, "bonusID")
	result, err := h.service.UpdateBonus(r.Context(), bonusID, input, principal.UserID, principal.Cached)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "update", "hris", "bonus", bonusID, nil, input)
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *CompensationHandler) RejectBonus(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}
	bonusID := chi.URLParam(r, "bonusID")
	result, err := h.service.RejectBonus(r.Context(), bonusID, principal.UserID)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "reject", "hris", "bonus", bonusID, nil, result)
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *CompensationHandler) DeleteBonus(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}
	bonusID := chi.URLParam(r, "bonusID")
	if err := h.service.DeleteBonus(r.Context(), bonusID, principal.UserID, principal.Cached); err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "delete", "hris", "bonus", bonusID, nil, nil)
	response.WriteJSON(w, http.StatusOK, map[string]string{"message": "Bonus deleted successfully"}, nil)
}

func (h *CompensationHandler) writeError(ctx context.Context, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, hrisservice.ErrEmployeeNotFound):
		response.WriteError(w, http.StatusNotFound, "EMPLOYEE_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrSalaryNotFound):
		response.WriteError(w, http.StatusNotFound, "SALARY_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrSalaryForbidden):
		response.WriteError(w, http.StatusForbidden, "SALARY_FORBIDDEN", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrBonusNotFound):
		response.WriteError(w, http.StatusNotFound, "BONUS_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrBonusNotPending):
		response.WriteError(w, http.StatusConflict, "BONUS_NOT_PENDING", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrBonusForbidden):
		response.WriteError(w, http.StatusForbidden, "BONUS_FORBIDDEN", err.Error(), nil)
	default:
		response.WriteInternalError(ctx, w, err, "An unexpected error occurred")
	}
}
