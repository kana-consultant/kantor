package hris

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"

	hrisdto "github.com/kana-consultant/kantor/backend/internal/dto/hris"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	"github.com/kana-consultant/kantor/backend/internal/rbac"
	"github.com/kana-consultant/kantor/backend/internal/response"
	hrisservice "github.com/kana-consultant/kantor/backend/internal/service/hris"
)

const (
	permissionEmployeeView     = "hris:employee:view"
	permissionEmployeeEdit     = "hris:employee:edit"
	permissionIdentityView     = "hris:employee_identity:view"
	permissionIdentityEdit     = "hris:employee_identity:edit"
	hrProfileAuditResourceName = "employee_hr_profile"
)

// HRProfilesHandler serves /hris/employees/{employeeID}/hr-profile.
type HRProfilesHandler struct {
	service   *hrisservice.HRProfilesService
	validator *validator.Validate
}

func NewHRProfilesHandler(service *hrisservice.HRProfilesService) *HRProfilesHandler {
	return &HRProfilesHandler{service: service, validator: newValidator()}
}

// RegisterRoutes mounts under /hris/employees/{employeeID}/hr-profile.
// GET needs hris:employee:view (identity only with employee_identity:view);
// PUT needs either edit permission and checks each field group itself.
func (h *HRProfilesHandler) RegisterRoutes(router chi.Router) {
	router.With(platformmiddleware.RequirePermission(permissionEmployeeView)).Get("/", h.get)
	router.With(platformmiddleware.RequireAnyPermission(permissionEmployeeEdit, permissionIdentityEdit)).Put("/", h.update)
}

func (h *HRProfilesHandler) get(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	result, err := h.service.Get(r.Context(), chi.URLParam(r, "employeeID"), hrProfileAccess(principal))
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func (h *HRProfilesHandler) update(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}

	var input hrisdto.UpdateHRProfileRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}

	employeeID := chi.URLParam(r, "employeeID")
	result, changed, err := h.service.Update(r.Context(), employeeID, input, hrProfileAccess(principal))
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}

	// Field names only: never the NIK, birth data or addresses.
	if len(changed) > 0 {
		platformmiddleware.AuditLog(r.Context(), "update", "hris", hrProfileAuditResourceName, employeeID, nil, map[string]any{
			"changed_fields": changed,
		})
	}
	response.WriteJSON(w, http.StatusOK, result, nil)
}

func hrProfileAccess(principal platformmiddleware.Principal) hrisservice.HRProfileAccess {
	has := func(permission string) bool {
		if principal.IsSuperAdmin {
			return true
		}
		if rbac.CanViewAll(principal.Cached, permission) {
			return true
		}
		for _, item := range principal.Permissions {
			if item == permission {
				return true
			}
		}
		return false
	}
	return hrisservice.HRProfileAccess{
		ActorID:         principal.UserID,
		CanViewIdentity: has(permissionIdentityView),
		CanEditJobTitle: has(permissionEmployeeEdit),
		CanEditIdentity: has(permissionIdentityEdit),
	}
}

func (h *HRProfilesHandler) writeError(ctx context.Context, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, hrisservice.ErrEmployeeNotFound):
		response.WriteError(w, http.StatusNotFound, "EMPLOYEE_NOT_FOUND", "Karyawan tidak ditemukan", nil)
	case errors.Is(err, hrisservice.ErrHRProfileJobTitleForbidden):
		response.WriteError(w, http.StatusForbidden, "FORBIDDEN", err.Error(), map[string]string{"job_title": "forbidden"})
	case errors.Is(err, hrisservice.ErrHRProfileIdentityForbidden):
		response.WriteError(w, http.StatusForbidden, "FORBIDDEN", err.Error(), map[string]string{"identity": "forbidden"})
	case errors.Is(err, hrisservice.ErrNIKInvalid):
		response.WriteError(w, http.StatusBadRequest, "NIK_INVALID", err.Error(), map[string]string{"identity.nik": "format"})
	case errors.Is(err, hrisservice.ErrNIKMismatch):
		response.WriteError(w, http.StatusBadRequest, "NIK_MISMATCH", err.Error(), map[string]string{"identity.nik": "mismatch"})
	case errors.Is(err, hrisservice.ErrNIKStoredMismatch):
		response.WriteError(w, http.StatusBadRequest, "NIK_MISMATCH", err.Error(), map[string]string{"identity.nik": "replace"})
	case errors.Is(err, hrisservice.ErrNIKRequired):
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), map[string]string{"identity.nik": "required"})
	case errors.Is(err, hrisservice.ErrNIKNeedsBirthData):
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), map[string]string{"identity.birth_date": "required", "identity.gender": "required"})
	case errors.Is(err, hrisservice.ErrBirthDateInvalid):
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), map[string]string{"identity.birth_date": "invalid"})
	case errors.Is(err, hrisservice.ErrGenderInvalid):
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), map[string]string{"identity.gender": "oneof"})
	case errors.Is(err, hrisservice.ErrPersonalEmailInvalid):
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), map[string]string{"personal_email": "email"})
	default:
		response.WriteInternalError(ctx, w, err, "Terjadi kesalahan saat memproses profil HR")
	}
}
