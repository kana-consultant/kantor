package files

import (
	"errors"
	"net/http"

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
			response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred", nil)
		}
		return
	}

	if !hasPermission(principal, file.Permission) {
		response.WriteError(w, http.StatusForbidden, "FORBIDDEN", "You do not have permission to access this file", nil)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, file.Path)
}

func hasPermission(principal platformmiddleware.Principal, permission string) bool {
	if permission == "" {
		return true
	}

	if principal.IsSuperAdmin {
		return true
	}

	for _, item := range principal.Permissions {
		if item == permission {
			return true
		}
	}

	return false
}
