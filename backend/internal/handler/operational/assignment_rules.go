package operational

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"

	operationaldto "github.com/kana-consultant/kantor/backend/internal/dto/operational"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	"github.com/kana-consultant/kantor/backend/internal/response"
	operationalservice "github.com/kana-consultant/kantor/backend/internal/service/operational"
)

type AssignmentRulesHandler struct {
	service   *operationalservice.AssignmentRulesService
	validator *validator.Validate
}

func NewAssignmentRulesHandler(service *operationalservice.AssignmentRulesService) *AssignmentRulesHandler {
	return &AssignmentRulesHandler{
		service:   service,
		validator: validator.New(validator.WithRequiredStructEnabled()),
	}
}

func (h *AssignmentRulesHandler) RegisterRuleRoutes(router chi.Router) {
	router.With(platformmiddleware.RBACMiddleware("operational:assignment:create")).Post("/", h.createRule)
	router.With(platformmiddleware.RBACMiddleware("operational:assignment:view")).Get("/", h.listRules)
	router.With(platformmiddleware.RBACMiddleware("operational:assignment:edit")).Put("/{ruleID}", h.updateRule)
	router.With(platformmiddleware.RBACMiddleware("operational:assignment:delete")).Delete("/{ruleID}", h.deleteRule)
}

func (h *AssignmentRulesHandler) AutoAssignTask(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	taskID := chi.URLParam(r, "taskID")

	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	result, err := h.service.AutoAssignTask(r.Context(), projectID, taskID, principal.UserID, requestIPAddress(r))
	if err != nil {
		h.writeError(w, err)
		return
	}

	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *AssignmentRulesHandler) createRule(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")

	var input operationaldto.CreateAssignmentRuleRequest
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	result, err := h.service.CreateRule(r.Context(), projectID, input, principal.UserID)
	if err != nil {
		h.writeError(w, err)
		return
	}

	response.WriteJSON(w, http.StatusCreated, result, nil)
}

func (h *AssignmentRulesHandler) listRules(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")

	result, err := h.service.ListRules(r.Context(), projectID)
	if err != nil {
		h.writeError(w, err)
		return
	}

	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *AssignmentRulesHandler) updateRule(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	ruleID := chi.URLParam(r, "ruleID")

	var input operationaldto.UpdateAssignmentRuleRequest
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	result, err := h.service.UpdateRule(r.Context(), projectID, ruleID, input)
	if err != nil {
		h.writeError(w, err)
		return
	}

	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *AssignmentRulesHandler) deleteRule(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "projectID")
	ruleID := chi.URLParam(r, "ruleID")

	if err := h.service.DeleteRule(r.Context(), projectID, ruleID); err != nil {
		h.writeError(w, err)
		return
	}

	response.WriteJSON(w, http.StatusOK, map[string]string{"message": "Assignment rule deleted successfully"}, nil)
}

func (h *AssignmentRulesHandler) decodeAndValidate(w http.ResponseWriter, r *http.Request, target interface{}) bool {
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

func (h *AssignmentRulesHandler) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, operationalservice.ErrAssignmentRuleNotFound):
		response.WriteError(w, http.StatusNotFound, "ASSIGNMENT_RULE_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, operationalservice.ErrInvalidAssignmentRuleConfig):
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), map[string]string{"rule_config": "invalid"})
	case errors.Is(err, operationalservice.ErrAutoAssignNoMatch):
		response.WriteError(w, http.StatusUnprocessableEntity, "AUTO_ASSIGN_NO_MATCH", err.Error(), nil)
	case errors.Is(err, operationalservice.ErrKanbanTaskNotFound):
		response.WriteError(w, http.StatusNotFound, "KANBAN_TASK_NOT_FOUND", err.Error(), nil)
	default:
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred", nil)
	}
}

func requestIPAddress(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
