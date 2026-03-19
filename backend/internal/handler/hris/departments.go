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

type DepartmentsHandler struct {
	service   *hrisservice.DepartmentsService
	validator *validator.Validate
}

func NewDepartmentsHandler(service *hrisservice.DepartmentsService) *DepartmentsHandler {
	return &DepartmentsHandler{
		service:   service,
		validator: newValidator(),
	}
}

func (h *DepartmentsHandler) RegisterRoutes(router chi.Router) {
	router.With(platformmiddleware.RBACMiddleware("hris:department:create")).Post("/", h.createDepartment)
	router.With(platformmiddleware.RBACMiddleware("hris:department:view")).Get("/", h.listDepartments)
	router.With(platformmiddleware.RBACMiddleware("hris:department:view")).Get("/{departmentID}", h.getDepartment)
	router.With(platformmiddleware.RBACMiddleware("hris:department:edit")).Put("/{departmentID}", h.updateDepartment)
	router.With(platformmiddleware.RBACMiddleware("hris:department:delete")).Delete("/{departmentID}", h.deleteDepartment)
}

func (h *DepartmentsHandler) createDepartment(w http.ResponseWriter, r *http.Request) {
	var input hrisdto.CreateDepartmentRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}

	result, err := h.service.CreateDepartment(r.Context(), input)
	if err != nil {
		h.writeError(w, err)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "create", "hris", "department", result.ID, nil, input)
	response.WriteJSON(w, http.StatusCreated, result, nil)
}

func (h *DepartmentsHandler) listDepartments(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.ListDepartments(r.Context())
	if err != nil {
		h.writeError(w, err)
		return
	}

	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *DepartmentsHandler) getDepartment(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.GetDepartment(r.Context(), chi.URLParam(r, "departmentID"))
	if err != nil {
		h.writeError(w, err)
		return
	}

	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *DepartmentsHandler) updateDepartment(w http.ResponseWriter, r *http.Request) {
	var input hrisdto.UpdateDepartmentRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}

	departmentID := chi.URLParam(r, "departmentID")
	result, err := h.service.UpdateDepartment(r.Context(), departmentID, input)
	if err != nil {
		h.writeError(w, err)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "update", "hris", "department", departmentID, nil, input)
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *DepartmentsHandler) deleteDepartment(w http.ResponseWriter, r *http.Request) {
	departmentID := chi.URLParam(r, "departmentID")
	if err := h.service.DeleteDepartment(r.Context(), departmentID); err != nil {
		h.writeError(w, err)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "delete", "hris", "department", departmentID, nil, nil)
	response.WriteJSON(w, http.StatusOK, map[string]string{"message": "Department deleted successfully"}, nil)
}

func (h *DepartmentsHandler) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, hrisservice.ErrDepartmentNotFound):
		response.WriteError(w, http.StatusNotFound, "DEPARTMENT_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, hrisservice.ErrDepartmentNameExists):
		response.WriteError(w, http.StatusConflict, "DEPARTMENT_NAME_EXISTS", err.Error(), map[string]string{"name": "already exists"})
	case errors.Is(err, hrisservice.ErrDepartmentHeadMissing):
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), map[string]string{"head_id": "employee not found"})
	default:
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred", nil)
	}
}
