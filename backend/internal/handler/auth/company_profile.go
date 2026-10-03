package auth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"

	"github.com/kana-consultant/kantor/backend/internal/dto"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	"github.com/kana-consultant/kantor/backend/internal/response"
	authservice "github.com/kana-consultant/kantor/backend/internal/service/auth"
)

// companyLogoMultipartOverhead leaves room for the multipart envelope around
// a logo of exactly MaxCompanyLogoBytes.
const companyLogoMultipartOverhead = 64 << 10

// SetCompanyProfileService wires /admin/settings/company-profile (+ /logo).
func (h *Handler) SetCompanyProfileService(service *authservice.CompanyProfileService) {
	h.companyProfile = service
}

func (h *Handler) GetCompanyProfile(w http.ResponseWriter, r *http.Request) {
	if !h.requireCompanyProfile(w) {
		return
	}
	profile, err := h.companyProfile.Get(r.Context())
	if err != nil {
		h.writeCompanyProfileError(r.Context(), w, err, "Gagal memuat profil perusahaan")
		return
	}
	response.WriteJSON(w, http.StatusOK, profile, nil)
}

func (h *Handler) UpdateCompanyProfile(w http.ResponseWriter, r *http.Request) {
	if !h.requireCompanyProfile(w) {
		return
	}
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	var input dto.UpdateCompanyProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		if platformmiddleware.IsBodyTooLargeError(err) {
			platformmiddleware.WriteBodyTooLargeError(w)
			return
		}
		response.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Request body must be valid JSON", nil)
		return
	}
	if err := h.validator.Struct(input); err != nil {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Request validation failed", validationDetails(err))
		return
	}

	profile, change, err := h.companyProfile.Update(r.Context(), principal.UserID, input)
	if err != nil {
		h.writeCompanyProfileError(r.Context(), w, err, "Gagal menyimpan profil perusahaan")
		return
	}

	// The company profile holds business data only (no personal data or
	// credentials), so the audit keeps a diff of the changed fields.
	if len(change.Fields) > 0 {
		platformmiddleware.AuditLog(r.Context(), "update", "admin", "system_setting", "company_profile", change.Old, map[string]any{
			"changed_fields": change.Fields,
			"values":         change.New,
		})
	}
	response.WriteJSON(w, http.StatusOK, profile, nil)
}

func (h *Handler) GetCompanyLogo(w http.ResponseWriter, r *http.Request) {
	if !h.requireCompanyProfile(w) {
		return
	}
	data, err := h.companyProfile.LogoPNG(r.Context())
	if err != nil {
		h.writeCompanyProfileError(r.Context(), w, err, "Gagal memuat logo perusahaan")
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", `inline; filename="logo.png"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (h *Handler) UploadCompanyLogo(w http.ResponseWriter, r *http.Request) {
	if !h.requireCompanyProfile(w) {
		return
	}
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, authservice.MaxCompanyLogoBytes+companyLogoMultipartOverhead)
	if err := r.ParseMultipartForm(authservice.MaxCompanyLogoBytes + companyLogoMultipartOverhead); err != nil {
		if platformmiddleware.IsBodyTooLargeError(err) {
			response.WriteError(w, http.StatusRequestEntityTooLarge, "LOGO_TOO_LARGE", authservice.ErrCompanyLogoTooLarge.Error(), nil)
			return
		}
		response.WriteError(w, http.StatusBadRequest, "INVALID_MULTIPART", "Upload harus menggunakan multipart form data", nil)
		return
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()

	var fileHeader *multipart.FileHeader
	if files := r.MultipartForm.File["file"]; len(files) > 0 {
		fileHeader = files[0]
	}
	if fileHeader == nil {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "File logo wajib diunggah", map[string]string{"file": "required"})
		return
	}
	if fileHeader.Size > authservice.MaxCompanyLogoBytes {
		response.WriteError(w, http.StatusRequestEntityTooLarge, "LOGO_TOO_LARGE", authservice.ErrCompanyLogoTooLarge.Error(), nil)
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		response.WriteError(w, http.StatusBadRequest, "INVALID_MULTIPART", "File logo tidak dapat dibaca", nil)
		return
	}
	data, err := io.ReadAll(io.LimitReader(file, authservice.MaxCompanyLogoBytes+1))
	_ = file.Close()
	if err != nil {
		response.WriteError(w, http.StatusBadRequest, "INVALID_MULTIPART", "File logo tidak dapat dibaca", nil)
		return
	}

	profile, err := h.companyProfile.UploadLogo(r.Context(), principal.UserID, data)
	if err != nil {
		h.writeCompanyProfileError(r.Context(), w, err, "Gagal menyimpan logo perusahaan")
		return
	}

	// Field names only.
	platformmiddleware.AuditLog(r.Context(), "update_logo", "admin", "system_setting", "company_profile", nil, map[string]any{
		"changed_fields": []string{"logo"},
	})
	response.WriteJSON(w, http.StatusOK, profile, nil)
}

func (h *Handler) DeleteCompanyLogo(w http.ResponseWriter, r *http.Request) {
	if !h.requireCompanyProfile(w) {
		return
	}
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	profile, removed, err := h.companyProfile.DeleteLogo(r.Context(), principal.UserID)
	if err != nil {
		h.writeCompanyProfileError(r.Context(), w, err, "Gagal menghapus logo perusahaan")
		return
	}
	if removed {
		platformmiddleware.AuditLog(r.Context(), "delete_logo", "admin", "system_setting", "company_profile", nil, map[string]any{
			"changed_fields": []string{"logo"},
		})
	}
	response.WriteJSON(w, http.StatusOK, profile, nil)
}

func (h *Handler) requireCompanyProfile(w http.ResponseWriter) bool {
	if h.companyProfile == nil {
		response.WriteError(w, http.StatusServiceUnavailable, "COMPANY_PROFILE_UNAVAILABLE", "Profil perusahaan tidak tersedia", nil)
		return false
	}
	return true
}

func (h *Handler) writeCompanyProfileError(ctx context.Context, w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, authservice.ErrCompanyDocCodeInvalid):
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), map[string]string{"doc_code": "format"})
	case errors.Is(err, authservice.ErrCompanyLogoTooLarge):
		response.WriteError(w, http.StatusRequestEntityTooLarge, "LOGO_TOO_LARGE", err.Error(), nil)
	case errors.Is(err, authservice.ErrCompanyLogoInvalid):
		response.WriteError(w, http.StatusBadRequest, "INVALID_LOGO", err.Error(), map[string]string{"file": "image"})
	case errors.Is(err, authservice.ErrCompanyLogoNotFound):
		response.WriteError(w, http.StatusNotFound, "LOGO_NOT_FOUND", err.Error(), nil)
	default:
		response.WriteInternalError(ctx, w, err, fallback)
	}
}
