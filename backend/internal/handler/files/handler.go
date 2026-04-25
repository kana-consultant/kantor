package files

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	"github.com/kana-consultant/kantor/backend/internal/response"
	filesservice "github.com/kana-consultant/kantor/backend/internal/service/files"
)

type Handler struct {
	service *filesservice.Service
}

func New(service *filesservice.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Serve(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}

	fileType := chi.URLParam(r, "type")
	resourceID := chi.URLParam(r, "id")
	filename := chi.URLParam(r, "filename")

	file, err := h.service.Resolve(r.Context(), fileType, resourceID, filename)
	if err != nil {
		switch {
		case errors.Is(err, filesservice.ErrUnsupportedType):
			response.WriteError(w, http.StatusNotFound, "FILE_TYPE_NOT_FOUND", "Requested file type is not supported", nil)
		case errors.Is(err, filesservice.ErrFileNotFound):
			response.WriteError(w, http.StatusNotFound, "FILE_NOT_FOUND", "Requested file was not found", nil)
		default:
			response.WriteInternalError(r.Context(), w, err, "An unexpected error occurred")
		}
		return
	}

	if !hasPermission(principal, file.Permission, file.OwnerUserID) {
		response.WriteError(w, http.StatusForbidden, "FORBIDDEN", "You do not have permission to access this file", nil)
		return
	}

	// Force browsers to download instead of render uploaded files. Combined
	// with X-Content-Type-Options: nosniff this neutralises polyglot files
	// that pass our magic-byte check but also embed executable HTML/JS.
	disposition := "inline"
	if shouldForceDownload(filename) {
		disposition = "attachment"
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("%s; filename=%q", disposition, filepath.Base(filename)))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, file.Path)
}

// shouldForceDownload returns true for filenames whose extension is not a
// safe-to-render image. Documents, archives, scripts, and unknown types are
// forced into a download rather than rendered inline.
func shouldForceDownload(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp":
		return false
	default:
		return true
	}
}

func hasPermission(principal platformmiddleware.Principal, permission string, ownerUserID *string) bool {
	if principal.IsSuperAdmin {
		return true
	}

	if ownerUserID != nil && strings.TrimSpace(*ownerUserID) != "" && principal.UserID == strings.TrimSpace(*ownerUserID) {
		return true
	}

	if strings.TrimSpace(permission) == "" {
		return false
	}

	for _, item := range principal.Permissions {
		if item == permission {
			return true
		}
	}

	return false
}
