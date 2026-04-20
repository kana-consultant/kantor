package operational

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	operationaldto "github.com/kana-consultant/kantor/backend/internal/dto/operational"
	"github.com/kana-consultant/kantor/backend/internal/exportutil"
	"github.com/kana-consultant/kantor/backend/internal/httputil"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	operationalrepo "github.com/kana-consultant/kantor/backend/internal/repository/operational"
	"github.com/kana-consultant/kantor/backend/internal/response"
	operationalservice "github.com/kana-consultant/kantor/backend/internal/service/operational"
)

type ProjectsHandler struct {
	service   *operationalservice.ProjectsService
	kanban    *operationalservice.KanbanService
	repo      *operationalrepo.ProjectsRepository
	users     exportutil.UserLookup
	validator *validator.Validate
}

func NewProjectsHandler(
	service *operationalservice.ProjectsService,
	kanban *operationalservice.KanbanService,
	repo *operationalrepo.ProjectsRepository,
	users exportutil.UserLookup,
) *ProjectsHandler {
	return &ProjectsHandler{
		service:   service,
		kanban:    kanban,
		repo:      repo,
		users:     users,
		validator: validator.New(validator.WithRequiredStructEnabled()),
	}
}

func (h *ProjectsHandler) RegisterRoutes(router chi.Router) {
	router.With(platformmiddleware.RequirePermission("operational:project:create")).Post("/", h.createProject)
	router.With(platformmiddleware.RequirePermission("operational:project:view")).Get("/", h.listProjects)
	router.With(platformmiddleware.RequirePermission("operational:project:view")).Get("/export", h.exportList)
	router.With(platformmiddleware.RequirePermission("operational:project:view")).Get("/available-users", h.listAvailableUsers)
	router.With(platformmiddleware.RequirePermission("operational:project:view")).Get("/{projectID}", h.getProject)
	router.With(platformmiddleware.RequirePermission("operational:project:view")).Get("/{projectID}/export", h.exportDetail)
	router.With(platformmiddleware.RequirePermission("operational:project:edit")).Put("/{projectID}", h.updateProject)
	router.With(platformmiddleware.RequirePermission("operational:project:delete")).Delete("/{projectID}", h.deleteProject)
	router.With(platformmiddleware.RequirePermission("operational:project:manage_members")).Post("/{projectID}/members", h.mutateMembers)
}

func (h *ProjectsHandler) createProject(w http.ResponseWriter, r *http.Request) {
	var input operationaldto.CreateProjectRequest
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	result, err := h.service.CreateProject(r.Context(), input, principal.UserID)
	if err != nil {
		h.writeError(w, err)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "create", "operational", "project", result.Project.ID, nil, input)
	response.WriteJSON(w, http.StatusCreated, result, nil)
}

func (h *ProjectsHandler) listProjects(w http.ResponseWriter, r *http.Request) {
	query, ok := h.parseListQuery(w, r)
	if !ok {
		return
	}

	projects, total, page, perPage, err := h.service.ListProjects(r.Context(), query)
	if err != nil {
		h.writeError(w, err)
		return
	}

	response.WriteJSON(w, http.StatusOK, projects, map[string]int64{
		"page":     int64(page),
		"per_page": int64(perPage),
		"total":    total,
	})
}

func (h *ProjectsHandler) getProject(w http.ResponseWriter, r *http.Request) {
	projectID, ok := validateProjectIDParam(w, chi.URLParam(r, "projectID"))
	if !ok {
		return
	}

	result, err := h.service.GetProject(r.Context(), projectID)
	if err != nil {
		h.writeError(w, err)
		return
	}

	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *ProjectsHandler) updateProject(w http.ResponseWriter, r *http.Request) {
	projectID, ok := validateProjectIDParam(w, chi.URLParam(r, "projectID"))
	if !ok {
		return
	}

	var input operationaldto.UpdateProjectRequest
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	result, err := h.service.UpdateProject(r.Context(), projectID, input)
	if err != nil {
		h.writeError(w, err)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "update", "operational", "project", projectID, nil, input)
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *ProjectsHandler) deleteProject(w http.ResponseWriter, r *http.Request) {
	projectID, ok := validateProjectIDParam(w, chi.URLParam(r, "projectID"))
	if !ok {
		return
	}

	if err := h.service.DeleteProject(r.Context(), projectID); err != nil {
		h.writeError(w, err)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "delete", "operational", "project", projectID, nil, nil)
	response.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "Project deleted successfully",
	}, nil)
}

func (h *ProjectsHandler) mutateMembers(w http.ResponseWriter, r *http.Request) {
	projectID, ok := validateProjectIDParam(w, chi.URLParam(r, "projectID"))
	if !ok {
		return
	}

	var input operationaldto.ProjectMembersMutationRequest
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	result, err := h.service.MutateProjectMember(r.Context(), projectID, input)
	if err != nil {
		h.writeError(w, err)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "update", "operational", "project_members", projectID, nil, input)
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *ProjectsHandler) listAvailableUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.repo.ListActiveUsers(r.Context())
	if err != nil {
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list available users", nil)
		return
	}

	response.WriteJSON(w, http.StatusOK, users, nil)
}

func (h *ProjectsHandler) decodeAndValidate(w http.ResponseWriter, r *http.Request, target interface{}) bool {
	return httputil.DecodeAndValidate(h.validator, w, r, target)
}

func (h *ProjectsHandler) parseListQuery(w http.ResponseWriter, r *http.Request) (operationaldto.ListProjectsQuery, bool) {
	query := operationaldto.ListProjectsQuery{
		Search:   r.URL.Query().Get("search"),
		Status:   r.URL.Query().Get("status"),
		Priority: r.URL.Query().Get("priority"),
	}

	if pageRaw := r.URL.Query().Get("page"); pageRaw != "" {
		page, err := strconv.Atoi(pageRaw)
		if err != nil {
			response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Query validation failed", map[string]string{"page": "must be a number"})
			return operationaldto.ListProjectsQuery{}, false
		}
		query.Page = page
	}

	if perPageRaw := r.URL.Query().Get("per_page"); perPageRaw != "" {
		perPage, err := strconv.Atoi(perPageRaw)
		if err != nil {
			response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Query validation failed", map[string]string{"per_page": "must be a number"})
			return operationaldto.ListProjectsQuery{}, false
		}
		query.PerPage = perPage
	}

	if err := h.validator.Struct(query); err != nil {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Query validation failed", validationDetails(err))
		return operationaldto.ListProjectsQuery{}, false
	}

	return query, true
}

func (h *ProjectsHandler) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, operationalservice.ErrProjectNotFound):
		response.WriteError(w, http.StatusNotFound, "PROJECT_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, operationalservice.ErrInvalidProjectMember):
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), map[string]string{"role_in_project": "required"})
	case errors.Is(err, operationalservice.ErrMissingProjectMember):
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), map[string]string{"user_id": "user_id or user_email is required"})
	case errors.Is(err, operationalservice.ErrProjectMemberNotFound):
		response.WriteError(w, http.StatusNotFound, "USER_NOT_FOUND", err.Error(), nil)
	default:
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred", nil)
	}
}

func validationDetails(err error) map[string]string {
	return httputil.ValidationDetails(err)
}

func validateProjectIDParam(w http.ResponseWriter, projectID string) (string, bool) {
	projectID = strings.TrimSpace(projectID)
	if _, err := uuid.Parse(projectID); err != nil {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Path validation failed", map[string]string{"projectID": "must be a valid UUID"})
		return "", false
	}
	return projectID, true
}
