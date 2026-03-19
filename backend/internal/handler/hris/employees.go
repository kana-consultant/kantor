package hris

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"

	hrisdto "github.com/kana-consultant/kantor/backend/internal/dto/hris"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	"github.com/kana-consultant/kantor/backend/internal/response"
	hrisservice "github.com/kana-consultant/kantor/backend/internal/service/hris"
)

type EmployeesHandler struct {
	service   *hrisservice.EmployeesService
	validator *validator.Validate
}

func NewEmployeesHandler(service *hrisservice.EmployeesService) *EmployeesHandler {
	return &EmployeesHandler{
		service:   service,
		validator: newValidator(),
	}
}

func (h *EmployeesHandler) RegisterRoutes(router chi.Router) {
	router.With(platformmiddleware.RBACMiddleware("hris:employee:create")).Post("/", h.createEmployee)
	router.With(platformmiddleware.RBACMiddleware("hris:employee:view")).Get("/", h.listEmployees)
	router.With(platformmiddleware.RBACMiddleware("hris:employee:view")).Get("/{employeeID}", h.getEmployee)
	router.With(platformmiddleware.RBACMiddleware("hris:employee:edit")).Put("/{employeeID}", h.updateEmployee)
	router.With(platformmiddleware.RBACMiddleware("hris:employee:delete")).Delete("/{employeeID}", h.deleteEmployee)
}

func (h *EmployeesHandler) createEmployee(w http.ResponseWriter, r *http.Request) {
	var input hrisdto.CreateEmployeeRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}

	result, err := h.service.CreateEmployee(r.Context(), input)
	if err != nil {
		h.writeError(w, err)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "create", "hris", "employee", result.ID, nil, input)
	response.WriteJSON(w, http.StatusCreated, result, nil)
}

func (h *EmployeesHandler) listEmployees(w http.ResponseWriter, r *http.Request) {
	query, ok := h.parseListQuery(w, r)
	if !ok {
		return
	}

	result, total, page, perPage, err := h.service.ListEmployees(r.Context(), query)
	if err != nil {
		h.writeError(w, err)
		return
	}

	response.WriteJSON(w, http.StatusOK, result, map[string]int64{
		"page":     int64(page),
		"per_page": int64(perPage),
		"total":    total,
	})
}

func (h *EmployeesHandler) getEmployee(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.GetEmployee(r.Context(), chi.URLParam(r, "employeeID"))
	if err != nil {
		h.writeError(w, err)
		return
	}

	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *EmployeesHandler) updateEmployee(w http.ResponseWriter, r *http.Request) {
	var input hrisdto.UpdateEmployeeRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}

	employeeID := chi.URLParam(r, "employeeID")
	result, err := h.service.UpdateEmployee(r.Context(), employeeID, input)
	if err != nil {
		h.writeError(w, err)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "update", "hris", "employee", employeeID, nil, input)
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *EmployeesHandler) deleteEmployee(w http.ResponseWriter, r *http.Request) {
	employeeID := chi.URLParam(r, "employeeID")
	if err := h.service.DeleteEmployee(r.Context(), employeeID); err != nil {
		h.writeError(w, err)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "delete", "hris", "employee", employeeID, nil, nil)
	response.WriteJSON(w, http.StatusOK, map[string]string{"message": "Employee deleted successfully"}, nil)
}

func (h *EmployeesHandler) parseListQuery(w http.ResponseWriter, r *http.Request) (hrisdto.ListEmployeesQuery, bool) {
	query := hrisdto.ListEmployeesQuery{
		Search:           r.URL.Query().Get("search"),
		Department:       r.URL.Query().Get("department"),
		EmploymentStatus: r.URL.Query().Get("status"),
	}

	if pageRaw := r.URL.Query().Get("page"); pageRaw != "" {
		page, err := strconv.Atoi(pageRaw)
		if err != nil {
			response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Query validation failed", map[string]string{"page": "must be a number"})
			return hrisdto.ListEmployeesQuery{}, false
		}
		query.Page = page
	}

	if perPageRaw := r.URL.Query().Get("per_page"); perPageRaw != "" {
		perPage, err := strconv.Atoi(perPageRaw)
		if err != nil {
			response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Query validation failed", map[string]string{"per_page": "must be a number"})
			return hrisdto.ListEmployeesQuery{}, false
		}
		query.PerPage = perPage
	}

	if err := h.validator.Struct(query); err != nil {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Query validation failed", validationDetails(err))
		return hrisdto.ListEmployeesQuery{}, false
	}

	return query, true
}

func (h *EmployeesHandler) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, hrisservice.ErrEmployeeNotFound):
		response.WriteError(w, http.StatusNotFound, "EMPLOYEE_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrEmployeeEmailExists):
		response.WriteError(w, http.StatusConflict, "EMPLOYEE_EMAIL_EXISTS", err.Error(), map[string]string{"email": "already exists"})
	case errors.Is(err, hrisservice.ErrEmployeeUserLinkedTwice):
		response.WriteError(w, http.StatusConflict, "EMPLOYEE_USER_ALREADY_LINKED", err.Error(), map[string]string{"user_id": "already linked"})
	default:
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred", nil)
	}
}
