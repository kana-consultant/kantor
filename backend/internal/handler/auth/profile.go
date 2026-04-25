package auth

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/dto"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	"github.com/kana-consultant/kantor/backend/internal/response"
	authservice "github.com/kana-consultant/kantor/backend/internal/service/auth"
	"github.com/kana-consultant/kantor/backend/internal/uploads"
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
		response.WriteInternalError(r.Context(), w, err, "Failed to update profile")
		return
	}

	response.WriteJSON(w, http.StatusOK, employee, nil)
}

func (h *Handler) ChangeEmail(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	var input dto.ChangeEmailRequest
	if !h.decodeAndValidate(w, r, &input) {
		return
	}

	err := h.service.ChangeEmail(r.Context(), principal.UserID, input.Email, input.Password)
	if err != nil {
		switch {
		case errors.Is(err, authservice.ErrInvalidCurrentPassword):
			response.WriteError(w, http.StatusBadRequest, "INVALID_PASSWORD", err.Error(), nil)
		case errors.Is(err, authservice.ErrEmailAlreadyExists):
			response.WriteError(w, http.StatusConflict, "EMAIL_ALREADY_EXISTS", err.Error(), nil)
		case errors.Is(err, authservice.ErrEmailUnchanged):
			response.WriteError(w, http.StatusBadRequest, "EMAIL_UNCHANGED", err.Error(), nil)
		default:
			response.WriteInternalError(r.Context(), w, err, "Failed to change email")
		}
		return
	}

	platformmiddleware.AuditLogWithUser(r.Context(), principal.UserID, "update", "admin", "email", principal.UserID, nil, map[string]any{
		"new_email": input.Email,
	})
	response.WriteJSON(w, http.StatusOK, map[string]string{"message": "Email berhasil diubah"}, nil)
}

func (h *Handler) UploadProfileAvatar(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	if err := r.ParseMultipartForm(5 << 20); err != nil {
		response.WriteError(w, http.StatusBadRequest, "INVALID_MULTIPART", "Upload harus menggunakan multipart form data", nil)
		return
	}

	var fileHeader *multipart.FileHeader
	if files := r.MultipartForm.File["avatar"]; len(files) > 0 {
		fileHeader = files[0]
	} else if files := r.MultipartForm.File["file"]; len(files) > 0 {
		fileHeader = files[0]
	}
	if fileHeader == nil {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "File avatar diperlukan", map[string]string{"avatar": "required"})
		return
	}

	avatarPath, err := saveProfileAvatar(h.uploadsDir, principal.UserID, fileHeader)
	if err != nil {
		response.WriteError(w, http.StatusBadRequest, "AVATAR_UPLOAD_FAILED", err.Error(), nil)
		return
	}

	if err := h.service.UpdateProfileAvatar(r.Context(), principal.UserID, avatarPath); err != nil {
		_ = os.Remove(filepath.Join(h.uploadsDir, filepath.FromSlash(avatarPath)))
		response.WriteInternalError(r.Context(), w, err, "Gagal menyimpan avatar")
		return
	}

	response.WriteJSON(w, http.StatusOK, map[string]string{"avatar_url": avatarPath}, nil)
}

var unsafeCharsRe = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

func saveProfileAvatar(baseDir string, userID string, file *multipart.FileHeader) (string, error) {
	if _, err := uploads.ValidateMultipartFile(uploads.KindAvatar, file); err != nil {
		return "", fmt.Errorf("avatar tidak valid: %w", err)
	}

	src, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("gagal membuka file: %w", err)
	}
	defer src.Close()

	dir := filepath.Join(baseDir, "profiles", userID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("gagal membuat direktori: %w", err)
	}

	sanitized := unsafeCharsRe.ReplaceAllString(filepath.Base(file.Filename), "_")
	filename := strconv.FormatInt(time.Now().UnixNano(), 10) + "-" + sanitized
	destPath := filepath.Join(dir, filename)

	dst, err := os.Create(destPath)
	if err != nil {
		return "", fmt.Errorf("gagal membuat file: %w", err)
	}

	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		_ = os.Remove(destPath)
		return "", fmt.Errorf("gagal menyimpan file: %w", err)
	}
	_ = dst.Close()

	return filepath.ToSlash(filepath.Join("profiles", userID, filename)), nil
}
