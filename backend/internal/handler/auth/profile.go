package auth

import (
	"net/http"

	"github.com/kana-consultant/kantor/backend/internal/dto"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	"github.com/kana-consultant/kantor/backend/internal/response"
)

func (h *Handler) GetProfile(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	employee, err := h.service.GetProfile(r.Context(), principal.UserID)
	if err != nil {
		response.WriteError(w, http.StatusNotFound, "PROFILE_NOT_FOUND", "Employee profile not found", nil)
		return
	}

	response.WriteJSON(w, http.StatusOK, employee, nil)
}

func (h *Handler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	var input dto.UpdateProfileRequest
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	employee, err := h.service.UpdateProfile(r.Context(), principal.UserID, input)
	if err != nil {
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update profile", nil)
		return
	}

	response.WriteJSON(w, http.StatusOK, employee, nil)
}
