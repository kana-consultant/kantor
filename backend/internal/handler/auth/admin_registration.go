package auth

import (
	"net/http"

	"github.com/kana-consultant/kantor/backend/internal/dto"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	"github.com/kana-consultant/kantor/backend/internal/response"
)

func (h *Handler) GetRegistrationSettings(w http.ResponseWriter, r *http.Request) {
	view, err := h.service.GetRegistrationSettings(r.Context())
	if err != nil {
		response.WriteInternalError(r.Context(), w, err, "Gagal memuat pengaturan registrasi")
		return
	}
	response.WriteJSON(w, http.StatusOK, view, nil)
}

func (h *Handler) UpdateRegistrationSettings(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi login tidak ditemukan", nil)
		return
	}

	previous, err := h.service.GetRegistrationSettings(r.Context())
	if err != nil {
		response.WriteInternalError(r.Context(), w, err, "Gagal memuat pengaturan registrasi")
		return
	}

	var input dto.UpdateRegistrationSettingsRequest
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	updated, err := h.service.UpdateRegistrationSettings(r.Context(), principal.UserID, input)
	if err != nil {
		response.WriteInternalError(r.Context(), w, err, "Gagal menyimpan pengaturan registrasi")
		return
	}

	platformmiddleware.AuditLog(r.Context(), "update", "admin", "system_setting", "registration", previous, updated)
	response.WriteJSON(w, http.StatusOK, updated, nil)
}

func (h *Handler) RollRegistrationCode(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Sesi login tidak ditemukan", nil)
		return
	}

	code, view, err := h.service.RollRegistrationCode(r.Context(), principal.UserID)
	if err != nil {
		response.WriteInternalError(r.Context(), w, err, "Gagal memutar kode registrasi")
		return
	}

	platformmiddleware.AuditLog(r.Context(), "roll", "admin", "system_setting", "registration_code", nil, map[string]any{
		"expires_at": view.CodeExpiresAt,
	})

	response.WriteJSON(w, http.StatusOK, dto.RollRegistrationCodeResponse{
		Code:     code,
		Settings: view,
	}, nil)
}
