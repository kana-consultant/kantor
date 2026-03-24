package auth

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/kana-consultant/kantor/backend/internal/dto"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	authrepo "github.com/kana-consultant/kantor/backend/internal/repository/auth"
	"github.com/kana-consultant/kantor/backend/internal/response"
)

func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	superAdmin, err := parseOptionalBool(r.URL.Query().Get("super_admin"))
	if err != nil {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "super_admin must be true or false", map[string]string{"super_admin": "invalid_boolean"})
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))

	result, total, err := h.service.ListAdminUsers(r.Context(), dto.ListUsersQuery{
		Page:       page,
		PerPage:    perPage,
		Search:     strings.TrimSpace(r.URL.Query().Get("search")),
		ModuleID:   strings.TrimSpace(r.URL.Query().Get("module")),
		RoleID:     strings.TrimSpace(r.URL.Query().Get("role")),
		SuperAdmin: superAdmin,
	})
	if err != nil {
		slog.Error("failed to list users", "error", err)
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list users", nil)
		return
	}

	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 20
	}

	response.WriteJSON(w, http.StatusOK, result, map[string]interface{}{
		"total":       total,
		"page":        page,
		"per_page":    perPage,
		"total_pages": totalPages(total, perPage),
	})
}

func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userID")

	result, err := h.service.GetAdminUserDetail(r.Context(), userID)
	if err != nil {
		response.WriteError(w, http.StatusNotFound, "USER_NOT_FOUND", "User not found", nil)
		return
	}

	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *Handler) UpdateUserRoles(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userID")
	previous, previousErr := h.service.GetAdminUserDetail(r.Context(), userID)
	if previousErr != nil {
		response.WriteError(w, http.StatusNotFound, "USER_NOT_FOUND", "User not found", nil)
		return
	}

	var input dto.UpdateUserModuleRolesRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Request body must be valid JSON", nil)
		return
	}

	if err := h.validator.Struct(input); err != nil {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Request validation failed", validationDetails(err))
		return
	}

	if err := h.service.UpdateUserModuleRoles(r.Context(), userID, input.ModuleRoles); err != nil {
		switch err {
		case authrepo.ErrInvalidModuleRole:
			response.WriteError(w, http.StatusBadRequest, "INVALID_MODULE_ROLE", "Role assignment is invalid", nil)
		default:
			response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update module roles", nil)
		}
		return
	}

	result, err := h.service.GetAdminUserDetail(r.Context(), userID)
	if err != nil {
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Roles updated but failed to fetch user", nil)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "update", "admin", "user_module_roles", userID, previous, result)
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *Handler) ToggleUserActive(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userID")
	previous, previousErr := h.service.GetAdminUserDetail(r.Context(), userID)
	if previousErr != nil {
		response.WriteError(w, http.StatusNotFound, "USER_NOT_FOUND", "User not found", nil)
		return
	}

	var input dto.ToggleActiveRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Request body must be valid JSON", nil)
		return
	}

	if err := h.service.SetUserActive(r.Context(), userID, input.Active); err != nil {
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update user status", nil)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "update", "admin", "user", userID, map[string]any{
		"is_active": previous.User.IsActive,
	}, map[string]any{
		"is_active": input.Active,
	})
	response.WriteJSON(w, http.StatusOK, map[string]bool{"success": true}, nil)
}

func (h *Handler) ToggleUserSuperAdmin(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}
	if !principal.IsSuperAdmin {
		response.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Only super admin can change super admin status", nil)
		return
	}

	userID := chi.URLParam(r, "userID")
	previous, previousErr := h.service.GetAdminUserDetail(r.Context(), userID)
	if previousErr != nil {
		response.WriteError(w, http.StatusNotFound, "USER_NOT_FOUND", "User not found", nil)
		return
	}

	var input dto.ToggleSuperAdminRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Request body must be valid JSON", nil)
		return
	}

	if err := h.service.ToggleUserSuperAdmin(r.Context(), principal.UserID, userID, input.Enabled); err != nil {
		switch err {
		case authrepo.ErrCannotToggleSelf:
			response.WriteError(w, http.StatusBadRequest, "CANNOT_TOGGLE_SELF", err.Error(), nil)
		default:
			response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update super admin status", nil)
		}
		return
	}

	result, err := h.service.GetAdminUserDetail(r.Context(), userID)
	if err != nil {
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Super admin updated but failed to fetch user", nil)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "update", "admin", "user", userID, map[string]any{
		"is_super_admin": previous.IsSuperAdmin,
	}, map[string]any{
		"is_super_admin": result.IsSuperAdmin,
	})
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func parseOptionalBool(raw string) (*bool, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, nil
	}

	switch strings.ToLower(value) {
	case "true", "1", "yes":
		result := true
		return &result, nil
	case "false", "0", "no":
		result := false
		return &result, nil
	default:
		return nil, errors.New("invalid boolean")
	}
}

func totalPages(total int64, perPage int) int {
	if perPage <= 0 {
		perPage = 20
	}
	totalPages := int(total) / perPage
	if int(total)%perPage > 0 {
		totalPages++
	}
	if totalPages == 0 {
		return 1
	}
	return totalPages
}
