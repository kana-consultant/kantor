package auth

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/kana-consultant/kantor/backend/internal/dto"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	authrepo "github.com/kana-consultant/kantor/backend/internal/repository/auth"
	"github.com/kana-consultant/kantor/backend/internal/response"
)

func (h *Handler) ListRoles(w http.ResponseWriter, r *http.Request) {
	isSystem, err := parseOptionalBool(r.URL.Query().Get("is_system"))
	if err != nil {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "is_system must be true or false", map[string]string{"is_system": "invalid_boolean"})
		return
	}
	isActive, err := parseOptionalBool(r.URL.Query().Get("is_active"))
	if err != nil {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "is_active must be true or false", map[string]string{"is_active": "invalid_boolean"})
		return
	}

	items, err := h.service.ListRoles(r.Context(), authrepo.RoleListParams{
		Search:   strings.TrimSpace(r.URL.Query().Get("search")),
		IsSystem: isSystem,
		IsActive: isActive,
	})
	if err != nil {
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list roles", nil)
		return
	}

	response.WriteJSON(w, http.StatusOK, items, nil)
}

func (h *Handler) GetRole(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.GetRoleDetail(r.Context(), chi.URLParam(r, "roleID"))
	if err != nil {
		response.WriteError(w, http.StatusNotFound, "ROLE_NOT_FOUND", "Role not found", nil)
		return
	}
	response.WriteJSON(w, http.StatusOK, item, nil)
}

func (h *Handler) CreateRole(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	var input dto.UpsertRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Request body must be valid JSON", nil)
		return
	}
	if err := h.validator.Struct(input); err != nil {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Request validation failed", validationDetails(err))
		return
	}

	item, err := h.service.CreateRole(r.Context(), authrepo.UpsertRoleParams{
		Name:           strings.TrimSpace(input.Name),
		Slug:           strings.ToLower(strings.TrimSpace(input.Slug)),
		Description:    strings.TrimSpace(input.Description),
		HierarchyLevel: input.HierarchyLevel,
		PermissionIDs:  input.PermissionIDs,
	}, principal.UserID)
	if err != nil {
		h.writeRoleError(w, err)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "create", "admin", "role", item.ID, nil, item)
	response.WriteJSON(w, http.StatusCreated, item, nil)
}

func (h *Handler) UpdateRole(w http.ResponseWriter, r *http.Request) {
	roleID := chi.URLParam(r, "roleID")
	previous, previousErr := h.service.GetRoleDetail(r.Context(), roleID)
	if previousErr != nil {
		h.writeRoleError(w, previousErr)
		return
	}

	var input dto.UpsertRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Request body must be valid JSON", nil)
		return
	}
	if err := h.validator.Struct(input); err != nil {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Request validation failed", validationDetails(err))
		return
	}

	item, err := h.service.UpdateRole(r.Context(), roleID, authrepo.UpsertRoleParams{
		Name:           strings.TrimSpace(input.Name),
		Slug:           strings.ToLower(strings.TrimSpace(input.Slug)),
		Description:    strings.TrimSpace(input.Description),
		HierarchyLevel: input.HierarchyLevel,
		PermissionIDs:  input.PermissionIDs,
	})
	if err != nil {
		h.writeRoleError(w, err)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "update", "admin", "role", roleID, previous, item)
	response.WriteJSON(w, http.StatusOK, item, nil)
}

func (h *Handler) DeleteRole(w http.ResponseWriter, r *http.Request) {
	roleID := chi.URLParam(r, "roleID")
	previous, previousErr := h.service.GetRoleDetail(r.Context(), roleID)
	if previousErr != nil {
		h.writeRoleError(w, previousErr)
		return
	}

	if err := h.service.DeleteRole(r.Context(), roleID); err != nil {
		h.writeRoleError(w, err)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "delete", "admin", "role", roleID, previous, nil)
	response.WriteJSON(w, http.StatusOK, map[string]bool{"success": true}, nil)
}

func (h *Handler) ToggleRole(w http.ResponseWriter, r *http.Request) {
	roleID := chi.URLParam(r, "roleID")
	previous, previousErr := h.service.GetRoleDetail(r.Context(), roleID)
	if previousErr != nil {
		h.writeRoleError(w, previousErr)
		return
	}

	item, err := h.service.ToggleRole(r.Context(), roleID)
	if err != nil {
		h.writeRoleError(w, err)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "update", "admin", "role", roleID, previous, item)
	response.WriteJSON(w, http.StatusOK, item, nil)
}

func (h *Handler) DuplicateRole(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	item, err := h.service.DuplicateRole(r.Context(), chi.URLParam(r, "roleID"), principal.UserID)
	if err != nil {
		h.writeRoleError(w, err)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "create", "admin", "role", item.ID, nil, map[string]any{
		"duplicated_from": chi.URLParam(r, "roleID"),
		"role":            item,
	})
	response.WriteJSON(w, http.StatusCreated, item, nil)
}

func (h *Handler) ListPermissions(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListPermissionGroups(r.Context())
	if err != nil {
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list permissions", nil)
		return
	}

	response.WriteJSON(w, http.StatusOK, map[string]any{"modules": items}, nil)
}

func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.service.GetSettings(r.Context())
	if err != nil {
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to load settings", nil)
		return
	}

	response.WriteJSON(w, http.StatusOK, settings, nil)
}

func (h *Handler) ListSettingsDepartments(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListSettingsDepartments(r.Context())
	if err != nil {
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to load departments", nil)
		return
	}

	response.WriteJSON(w, http.StatusOK, items, nil)
}

func (h *Handler) UpdateDefaultRoles(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	previous, err := h.service.GetSettings(r.Context())
	if err != nil {
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to load current settings", nil)
		return
	}

	var input dto.UpdateDefaultRolesRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Request body must be valid JSON", nil)
		return
	}
	if err := h.validator.Struct(input); err != nil {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Request validation failed", validationDetails(err))
		return
	}

	if err := h.service.UpdateDefaultRoles(r.Context(), principal.UserID, input.DefaultRoles); err != nil {
		if err == authrepo.ErrInvalidModuleRole {
			response.WriteError(w, http.StatusBadRequest, "INVALID_MODULE_ROLE", "Default role mapping contains an invalid role", nil)
			return
		}
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update default roles", nil)
		return
	}

	settings, err := h.service.GetSettings(r.Context())
	if err != nil {
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Default roles updated but failed to fetch settings", nil)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "update", "admin", "system_setting", "default_roles", previous.DefaultRoles, settings.DefaultRoles)
	response.WriteJSON(w, http.StatusOK, settings, nil)
}

func (h *Handler) UpdateAutoCreateEmployee(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	previous, err := h.service.GetSettings(r.Context())
	if err != nil {
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to load current settings", nil)
		return
	}

	var input dto.UpdateAutoCreateEmployeeRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Request body must be valid JSON", nil)
		return
	}

	if err := h.service.UpdateAutoCreateEmployee(r.Context(), principal.UserID, authrepo.AutoCreateEmployeeSetting{
		Enabled:             input.Enabled,
		DefaultDepartmentID: input.DefaultDepartmentID,
	}); err != nil {
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update auto-create employee setting", nil)
		return
	}

	settings, err := h.service.GetSettings(r.Context())
	if err != nil {
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Setting updated but failed to fetch settings", nil)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "update", "admin", "system_setting", "auto_create_employee", previous.AutoCreateEmployee, settings.AutoCreateEmployee)
	response.WriteJSON(w, http.StatusOK, settings, nil)
}

func (h *Handler) UpdateMailDelivery(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	previous, err := h.service.GetSettings(r.Context())
	if err != nil {
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to load current settings", nil)
		return
	}

	var input dto.UpdateMailDeliveryRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Request body must be valid JSON", nil)
		return
	}
	if err := h.validator.Struct(input); err != nil {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Request validation failed", validationDetails(err))
		return
	}

	if err := h.service.UpdateMailDelivery(r.Context(), principal.UserID, input); err != nil {
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update mail delivery setting", nil)
		return
	}

	settings, err := h.service.GetSettings(r.Context())
	if err != nil {
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Setting updated but failed to fetch settings", nil)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "update", "admin", "system_setting", "mail_delivery", previous.MailDelivery, settings.MailDelivery)
	response.WriteJSON(w, http.StatusOK, settings, nil)
}

func (h *Handler) UpdateReimbursementReminder(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	previous, err := h.service.GetSettings(r.Context())
	if err != nil {
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to load current settings", nil)
		return
	}

	var input dto.UpdateReimbursementReminderRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Request body must be valid JSON", nil)
		return
	}
	if err := h.validator.Struct(input); err != nil {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Request validation failed", validationDetails(err))
		return
	}

	if err := h.service.UpdateReimbursementReminder(r.Context(), principal.UserID, input); err != nil {
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update reimbursement reminder setting", nil)
		return
	}

	settings, err := h.service.GetSettings(r.Context())
	if err != nil {
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Setting updated but failed to fetch settings", nil)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "update", "admin", "system_setting", "reimbursement_reminder", previous.ReimbursementReminder, settings.ReimbursementReminder)
	response.WriteJSON(w, http.StatusOK, settings, nil)
}

func (h *Handler) ListModules(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListModules(r.Context())
	if err != nil {
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list modules", nil)
		return
	}
	response.WriteJSON(w, http.StatusOK, items, nil)
}

func (h *Handler) writeRoleError(w http.ResponseWriter, err error) {
	switch err {
	case authrepo.ErrRoleNotFound:
		response.WriteError(w, http.StatusNotFound, "ROLE_NOT_FOUND", err.Error(), nil)
	case authrepo.ErrRoleSlugExists:
		response.WriteError(w, http.StatusConflict, "ROLE_SLUG_EXISTS", err.Error(), nil)
	case authrepo.ErrReservedRoleSlug:
		response.WriteError(w, http.StatusBadRequest, "ROLE_SLUG_RESERVED", err.Error(), nil)
	case authrepo.ErrSystemRoleImmutable:
		response.WriteError(w, http.StatusConflict, "SYSTEM_ROLE_IMMUTABLE", err.Error(), nil)
	case authrepo.ErrRoleHasAssignments:
		response.WriteError(w, http.StatusConflict, "ROLE_HAS_ASSIGNMENTS", err.Error(), nil)
	default:
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred", nil)
	}
}
