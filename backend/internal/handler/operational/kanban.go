package operational

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"

	operationaldto "github.com/kana-consultant/kantor/backend/internal/dto/operational"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	"github.com/kana-consultant/kantor/backend/internal/response"
	operationalservice "github.com/kana-consultant/kantor/backend/internal/service/operational"
)

type KanbanHandler struct {
	service   *operationalservice.KanbanService
	validator *validator.Validate
}

func NewKanbanHandler(service *operationalservice.KanbanService) *KanbanHandler {
	return &KanbanHandler{
		service:   service,
		validator: validator.New(validator.WithRequiredStructEnabled()),
	}
}

func (h *KanbanHandler) RegisterColumnRoutes(router chi.Router) {
	router.With(platformmiddleware.RBACMiddleware("operational:kanban:create")).Post("/", h.createColumn)
	router.With(platformmiddleware.RBACMiddleware("operational:kanban:view")).Get("/", h.listColumns)
	router.With(platformmiddleware.RBACMiddleware("operational:kanban:edit")).Put("/{columnID}", h.updateColumn)
	router.With(platformmiddleware.RBACMiddleware("operational:kanban:delete")).Delete("/{columnID}", h.deleteColumn)
	router.With(platformmiddleware.RBACMiddleware("operational:kanban:edit")).Patch("/reorder", h.reorderColumns)
}

func (h *KanbanHandler) RegisterTaskRoutes(router chi.Router) {
	router.With(platformmiddleware.RBACMiddleware("operational:kanban:create")).Post("/", h.createTask)
	router.With(platformmiddleware.RBACMiddleware("operational:kanban:view")).Get("/", h.listTasks)
	router.With(platformmiddleware.RBACMiddleware("operational:kanban:edit")).Put("/{taskID}", h.updateTask)
	router.With(platformmiddleware.RBACMiddleware("operational:kanban:delete")).Delete("/{taskID}", h.deleteTask)
	router.With(platformmiddleware.RBACMiddleware("operational:kanban:edit")).Patch("/{taskID}/move", h.moveTask)
}

func (h *KanbanHandler) createColumn(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")

	var input operationaldto.CreateKanbanColumnRequest
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	result, err := h.service.CreateColumn(r.Context(), projectID, input)
	if err != nil {
		h.writeError(w, err)
		return
	}

	response.WriteJSON(w, http.StatusCreated, result, nil)
}

func (h *KanbanHandler) listColumns(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")

	result, err := h.service.ListColumns(r.Context(), projectID)
	if err != nil {
		h.writeError(w, err)
		return
	}

	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *KanbanHandler) updateColumn(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	columnID := chi.URLParam(r, "columnID")

	var input operationaldto.UpdateKanbanColumnRequest
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	result, err := h.service.UpdateColumn(r.Context(), projectID, columnID, input)
	if err != nil {
		h.writeError(w, err)
		return
	}

	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *KanbanHandler) deleteColumn(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	columnID := chi.URLParam(r, "columnID")

	if err := h.service.DeleteColumn(r.Context(), projectID, columnID); err != nil {
		h.writeError(w, err)
		return
	}

	response.WriteJSON(w, http.StatusOK, map[string]string{"message": "Column deleted successfully"}, nil)
}

func (h *KanbanHandler) reorderColumns(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")

	var input operationaldto.ReorderKanbanColumnsRequest
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	if err := h.service.ReorderColumns(r.Context(), projectID, input); err != nil {
		h.writeError(w, err)
		return
	}

	response.WriteJSON(w, http.StatusOK, map[string]string{"message": "Columns reordered successfully"}, nil)
}

func (h *KanbanHandler) createTask(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")

	var input operationaldto.CreateKanbanTaskRequest
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	result, err := h.service.CreateTask(r.Context(), projectID, input, principal.UserID)
	if err != nil {
		h.writeError(w, err)
		return
	}

	response.WriteJSON(w, http.StatusCreated, result, nil)
}

func (h *KanbanHandler) listTasks(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")

	result, err := h.service.ListTasks(r.Context(), projectID)
	if err != nil {
		h.writeError(w, err)
		return
	}

	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *KanbanHandler) updateTask(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	taskID := chi.URLParam(r, "taskID")

	var input operationaldto.UpdateKanbanTaskRequest
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	result, err := h.service.UpdateTask(r.Context(), projectID, taskID, input, principal.UserID)
	if err != nil {
		h.writeError(w, err)
		return
	}

	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *KanbanHandler) deleteTask(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	taskID := chi.URLParam(r, "taskID")

	if err := h.service.DeleteTask(r.Context(), projectID, taskID); err != nil {
		h.writeError(w, err)
		return
	}

	response.WriteJSON(w, http.StatusOK, map[string]string{"message": "Task deleted successfully"}, nil)
}

func (h *KanbanHandler) moveTask(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	taskID := chi.URLParam(r, "taskID")

	var input operationaldto.MoveKanbanTaskRequest
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	if err := h.service.MoveTask(r.Context(), projectID, taskID, input); err != nil {
		h.writeError(w, err)
		return
	}

	response.WriteJSON(w, http.StatusOK, map[string]string{"message": "Task moved successfully"}, nil)
}

func (h *KanbanHandler) decodeAndValidate(w http.ResponseWriter, r *http.Request, target interface{}) bool {
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

func (h *KanbanHandler) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, operationalservice.ErrKanbanColumnNotFound):
		response.WriteError(w, http.StatusNotFound, "KANBAN_COLUMN_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, operationalservice.ErrKanbanTaskNotFound):
		response.WriteError(w, http.StatusNotFound, "KANBAN_TASK_NOT_FOUND", err.Error(), nil)
	default:
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred", nil)
	}
}
