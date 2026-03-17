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
	router.With(platformmiddleware.RBACMiddleware("hris:salary:create")).Post("/", h.createSalary)
	router.With(platformmiddleware.RBACMiddleware("hris:salary:view")).Get("/", h.listSalaries)
	router.With(platformmiddleware.RBACMiddleware("hris:salary:view")).Get("/current", h.getCurrentSalary)
}

func (h *CompensationHandler) RegisterBonusRoutes(router chi.Router) {
	router.With(platformmiddleware.RBACMiddleware("hris:bonus:create")).Post("/", h.createBonus)
	router.With(platformmiddleware.RBACMiddleware("hris:bonus:view")).Get("/", h.listBonuses)
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
		h.writeError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusCreated, result, nil)
}

func (h *CompensationHandler) listSalaries(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	result, err := h.service.ListSalaries(r.Context(), chi.URLParam(r, "employeeID"), principal.UserID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *CompensationHandler) getCurrentSalary(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	result, err := h.service.GetCurrentSalary(r.Context(), chi.URLParam(r, "employeeID"), principal.UserID)
	if err != nil {
		h.writeError(w, err)
		return
	}
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
		h.writeError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusCreated, result, nil)
}

func (h *CompensationHandler) listBonuses(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.ListBonuses(r.Context(), chi.URLParam(r, "employeeID"))
	if err != nil {
		h.writeError(w, err)
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
	result, err := h.service.ApproveBonus(r.Context(), chi.URLParam(r, "bonusID"), principal.UserID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *CompensationHandler) RejectBonus(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}
	result, err := h.service.RejectBonus(r.Context(), chi.URLParam(r, "bonusID"), principal.UserID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *CompensationHandler) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, hrisservice.ErrEmployeeNotFound):
		response.WriteError(w, http.StatusNotFound, "EMPLOYEE_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrSalaryNotFound):
		response.WriteError(w, http.StatusNotFound, "SALARY_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrBonusNotFound):
		response.WriteError(w, http.StatusNotFound, "BONUS_NOT_FOUND", err.Error(), nil)
	default:
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred", nil)
	}
}
