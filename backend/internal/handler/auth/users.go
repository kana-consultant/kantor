package auth

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/kana-consultant/kantor/backend/internal/dto"
	"github.com/kana-consultant/kantor/backend/internal/rbac"
	authrepo "github.com/kana-consultant/kantor/backend/internal/repository/auth"
	"github.com/kana-consultant/kantor/backend/internal/response"
)

func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	search := r.URL.Query().Get("search")

	users, total, err := h.service.ListUsers(r.Context(), authrepo.ListUsersParams{
		Page:    page,
		PerPage: perPage,
		Search:  search,
	})
	if err != nil {
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list users", nil)
		return
	}

	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 20
	}
	totalPages := int(total) / perPage
	if int(total)%perPage > 0 {
		totalPages++
	}

	response.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"items": users,
		"meta": map[string]interface{}{
			"total":       total,
			"page":        page,
			"per_page":    perPage,
			"total_pages": totalPages,
		},
	}, nil)
}

func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userID")

	result, err := h.service.GetUserWithRoles(r.Context(), userID)
	if err != nil {
		response.WriteError(w, http.StatusNotFound, "USER_NOT_FOUND", "User not found", nil)
		return
	}

	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *Handler) UpdateUserRoles(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userID")

	var input dto.UpdateUserRolesRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Request body must be valid JSON", nil)
		return
	}

	if err := h.validator.Struct(input); err != nil {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Request validation failed", nil)
		return
	}

	roles := make([]rbac.RoleKey, len(input.Roles))
	for i, r := range input.Roles {
		roles[i] = rbac.RoleKey{Name: r.Name, Module: r.Module}
	}

	if err := h.service.UpdateUserRoles(r.Context(), userID, roles); err != nil {
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update roles", nil)
		return
	}

	// Return updated user
	result, err := h.service.GetUserWithRoles(r.Context(), userID)
	if err != nil {
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Roles updated but failed to fetch user", nil)
		return
	}

	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *Handler) ToggleUserActive(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userID")

	var input dto.ToggleActiveRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		response.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Request body must be valid JSON", nil)
		return
	}

	if err := h.service.SetUserActive(r.Context(), userID, input.Active); err != nil {
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update user status", nil)
		return
	}

	response.WriteJSON(w, http.StatusOK, map[string]bool{"success": true}, nil)
}
