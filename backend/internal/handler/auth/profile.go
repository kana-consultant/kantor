package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/dto"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	"github.com/kana-consultant/kantor/backend/internal/model"
	"github.com/kana-consultant/kantor/backend/internal/response"
	authservice "github.com/kana-consultant/kantor/backend/internal/service/auth"
	"github.com/kana-consultant/kantor/backend/internal/uploads"
)

const maxProfileAvatarMultipartMaxBytes = 6 << 20

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

	// The previous record tells which bank fields this save changes; a
	// missing profile simply has nothing to compare.
	previous, previousErr := h.service.GetProfile(r.Context(), principal.UserID)

	employee, err := h.service.UpdateProfile(r.Context(), principal.UserID, input)
	if err != nil {
		response.WriteInternalError(r.Context(), w, err, "Failed to update profile")
		return
	}

	// Bank changes made by the employee are audited by field name only.
	if previousErr == nil {
		if fields := changedBankFields(previous, employee); len(fields) > 0 {
			platformmiddleware.AuditLog(r.Context(), "update", "hris", "employee", employee.ID, nil, map[string]any{
				"source":         "profile",
				"changed_fields": fields,
			})
		}
	}

	response.WriteJSON(w, http.StatusOK, employee, nil)
}

// changedBankFields lists the bank fields that differ between two versions
// of an employee record (names only, never values).
func changedBankFields(previous model.Employee, next model.Employee) []string {
	fields := make([]string, 0, 2)
	if optionalValue(previous.BankAccountNumber) != optionalValue(next.BankAccountNumber) {
		fields = append(fields, "bank_account_number")
	}
	if optionalValue(previous.BankName) != optionalValue(next.BankName) {
		fields = append(fields, "bank_name")
	}
	return fields
}

func optionalValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
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

	r.Body = http.MaxBytesReader(w, r.Body, maxProfileAvatarMultipartMaxBytes)
	if err := r.ParseMultipartForm(5 << 20); err != nil {
		if platformmiddleware.IsBodyTooLargeError(err) {
			platformmiddleware.WriteBodyTooLargeError(w)
			return
		}
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

	avatarPath, err := h.saveProfileAvatar(r.Context(), principal.UserID, fileHeader)
	if err != nil {
		response.WriteError(w, http.StatusBadRequest, "AVATAR_UPLOAD_FAILED", err.Error(), nil)
		return
	}

	if err := h.service.UpdateProfileAvatar(r.Context(), principal.UserID, avatarPath); err != nil {
		h.removeProfileAvatar(r.Context(), avatarPath)
		response.WriteInternalError(r.Context(), w, err, "Gagal menyimpan avatar")
		return
	}

	response.WriteJSON(w, http.StatusOK, map[string]string{"avatar_url": avatarPath}, nil)
}

var unsafeCharsRe = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

func (h *Handler) saveProfileAvatar(ctx context.Context, userID string, file *multipart.FileHeader) (string, error) {
	validation, err := uploads.ValidateMultipartFile(uploads.KindAvatar, file)
	if err != nil {
		return "", fmt.Errorf("avatar tidak valid: %w", err)
	}

	src, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("gagal membuka file: %w", err)
	}
	defer src.Close()

	sanitized := unsafeCharsRe.ReplaceAllString(filepath.Base(file.Filename), "_")
	filename := strconv.FormatInt(time.Now().UnixNano(), 10) + "-" + sanitized
	relKey := filepath.ToSlash(filepath.Join("profiles", userID, filename))

	if h.storage != nil {
		if err := h.storage.Put(ctx, relKey, src, file.Size, validation.ContentType); err != nil {
			return "", fmt.Errorf("gagal menyimpan file: %w", err)
		}
		return relKey, nil
	}

	dir := filepath.Join(h.uploadsDir, "profiles", userID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("gagal membuat direktori: %w", err)
	}

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

	return relKey, nil
}

func (h *Handler) removeProfileAvatar(ctx context.Context, path string) {
	if h.storage != nil {
		_ = h.storage.Delete(ctx, filepath.ToSlash(path))
	} else {
		_ = os.Remove(filepath.Join(h.uploadsDir, filepath.FromSlash(path)))
	}
}
