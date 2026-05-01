package operational

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	operationaldto "github.com/kana-consultant/kantor/backend/internal/dto/operational"
	"github.com/kana-consultant/kantor/backend/internal/httputil"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	operationalrepo "github.com/kana-consultant/kantor/backend/internal/repository/operational"
	"github.com/kana-consultant/kantor/backend/internal/response"
	operationalservice "github.com/kana-consultant/kantor/backend/internal/service/operational"
)

// VPSHandler exposes the operational VPS monitoring endpoints. All routes are
// gated behind operational:vps:* permissions which the default seed only
// grants to super_admin + admin.
type VPSHandler struct {
	service   *operationalservice.VPSService
	validator *validator.Validate
}

func NewVPSHandler(service *operationalservice.VPSService) *VPSHandler {
	return &VPSHandler{
		service:   service,
		validator: validator.New(validator.WithRequiredStructEnabled()),
	}
}

func (h *VPSHandler) RegisterRoutes(router chi.Router) {
	router.With(platformmiddleware.RequirePermission("operational:vps:view")).Get("/", h.listVPS)
	router.With(platformmiddleware.RequirePermission("operational:vps:create")).Post("/", h.createVPS)
	router.With(platformmiddleware.RequirePermission("operational:vps:view")).Get("/{vpsID}", h.getVPS)
	router.With(platformmiddleware.RequirePermission("operational:vps:edit")).Put("/{vpsID}", h.updateVPS)
	router.With(platformmiddleware.RequirePermission("operational:vps:delete")).Delete("/{vpsID}", h.deleteVPS)

	router.With(platformmiddleware.RequirePermission("operational:vps:edit")).Post("/{vpsID}/checks", h.createCheck)
	router.With(platformmiddleware.RequirePermission("operational:vps:edit")).Put("/{vpsID}/checks/{checkID}", h.updateCheck)
	router.With(platformmiddleware.RequirePermission("operational:vps:edit")).Delete("/{vpsID}/checks/{checkID}", h.deleteCheck)

	router.With(platformmiddleware.RequirePermission("operational:vps:edit")).Post("/{vpsID}/apps", h.createApp)
	router.With(platformmiddleware.RequirePermission("operational:vps:edit")).Put("/{vpsID}/apps/{appID}", h.updateApp)
	router.With(platformmiddleware.RequirePermission("operational:vps:edit")).Delete("/{vpsID}/apps/{appID}", h.deleteApp)
}

func (h *VPSHandler) listVPS(w http.ResponseWriter, r *http.Request) {
	q := operationalrepo.ListVPSParams{
		Status:   strings.TrimSpace(r.URL.Query().Get("status")),
		Provider: strings.TrimSpace(r.URL.Query().Get("provider")),
		Tag:      strings.TrimSpace(r.URL.Query().Get("tag")),
		Search:   strings.TrimSpace(r.URL.Query().Get("search")),
	}
	servers, err := h.service.ListVPS(r.Context(), q)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, servers, nil)
}

func (h *VPSHandler) createVPS(w http.ResponseWriter, r *http.Request) {
	var input operationaldto.CreateVPSRequest
	if !httputil.DecodeAndValidate(h.validator, w, r, &input) {
		return
	}
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}
	v, err := h.service.CreateVPS(r.Context(), input, principal.UserID)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "create", "operational", "vps", v.ID, nil, input)
	response.WriteJSON(w, http.StatusCreated, v, nil)
}

func (h *VPSHandler) getVPS(w http.ResponseWriter, r *http.Request) {
	vpsID, ok := validateVPSIDParam(w, chi.URLParam(r, "vpsID"))
	if !ok {
		return
	}
	detail, err := h.service.GetVPSDetail(r.Context(), vpsID)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, detail, nil)
}

func (h *VPSHandler) updateVPS(w http.ResponseWriter, r *http.Request) {
	vpsID, ok := validateVPSIDParam(w, chi.URLParam(r, "vpsID"))
	if !ok {
		return
	}
	var input operationaldto.UpdateVPSRequest
	if !httputil.DecodeAndValidate(h.validator, w, r, &input) {
		return
	}
	v, err := h.service.UpdateVPS(r.Context(), vpsID, input)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "update", "operational", "vps", vpsID, nil, input)
	response.WriteJSON(w, http.StatusOK, v, nil)
}

func (h *VPSHandler) deleteVPS(w http.ResponseWriter, r *http.Request) {
	vpsID, ok := validateVPSIDParam(w, chi.URLParam(r, "vpsID"))
	if !ok {
		return
	}
	if err := h.service.DeleteVPS(r.Context(), vpsID); err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "delete", "operational", "vps", vpsID, nil, nil)
	response.WriteJSON(w, http.StatusOK, map[string]string{"message": "VPS deleted"}, nil)
}

func (h *VPSHandler) createCheck(w http.ResponseWriter, r *http.Request) {
	vpsID, ok := validateVPSIDParam(w, chi.URLParam(r, "vpsID"))
	if !ok {
		return
	}
	var input operationaldto.CreateVPSCheckRequest
	if !httputil.DecodeAndValidate(h.validator, w, r, &input) {
		return
	}
	c, err := h.service.CreateCheck(r.Context(), vpsID, input)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "create", "operational", "vps_check", c.ID, nil, input)
	response.WriteJSON(w, http.StatusCreated, c, nil)
}

func (h *VPSHandler) updateCheck(w http.ResponseWriter, r *http.Request) {
	if _, ok := validateVPSIDParam(w, chi.URLParam(r, "vpsID")); !ok {
		return
	}
	checkID, ok := validateUUIDParam(w, "checkID", chi.URLParam(r, "checkID"))
	if !ok {
		return
	}
	var input operationaldto.UpdateVPSCheckRequest
	if !httputil.DecodeAndValidate(h.validator, w, r, &input) {
		return
	}
	c, err := h.service.UpdateCheck(r.Context(), checkID, input)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "update", "operational", "vps_check", checkID, nil, input)
	response.WriteJSON(w, http.StatusOK, c, nil)
}

func (h *VPSHandler) deleteCheck(w http.ResponseWriter, r *http.Request) {
	if _, ok := validateVPSIDParam(w, chi.URLParam(r, "vpsID")); !ok {
		return
	}
	checkID, ok := validateUUIDParam(w, "checkID", chi.URLParam(r, "checkID"))
	if !ok {
		return
	}
	if err := h.service.DeleteCheck(r.Context(), checkID); err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "delete", "operational", "vps_check", checkID, nil, nil)
	response.WriteJSON(w, http.StatusOK, map[string]string{"message": "Check deleted"}, nil)
}

func (h *VPSHandler) createApp(w http.ResponseWriter, r *http.Request) {
	vpsID, ok := validateVPSIDParam(w, chi.URLParam(r, "vpsID"))
	if !ok {
		return
	}
	var input operationaldto.CreateVPSAppRequest
	if !httputil.DecodeAndValidate(h.validator, w, r, &input) {
		return
	}
	a, err := h.service.CreateApp(r.Context(), vpsID, input)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "create", "operational", "vps_app", a.ID, nil, input)
	response.WriteJSON(w, http.StatusCreated, a, nil)
}

func (h *VPSHandler) updateApp(w http.ResponseWriter, r *http.Request) {
	vpsID, ok := validateVPSIDParam(w, chi.URLParam(r, "vpsID"))
	if !ok {
		return
	}
	appID, ok := validateUUIDParam(w, "appID", chi.URLParam(r, "appID"))
	if !ok {
		return
	}
	var input operationaldto.UpdateVPSAppRequest
	if !httputil.DecodeAndValidate(h.validator, w, r, &input) {
		return
	}
	a, err := h.service.UpdateApp(r.Context(), vpsID, appID, input)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "update", "operational", "vps_app", appID, nil, input)
	response.WriteJSON(w, http.StatusOK, a, nil)
}

func (h *VPSHandler) deleteApp(w http.ResponseWriter, r *http.Request) {
	if _, ok := validateVPSIDParam(w, chi.URLParam(r, "vpsID")); !ok {
		return
	}
	appID, ok := validateUUIDParam(w, "appID", chi.URLParam(r, "appID"))
	if !ok {
		return
	}
	if err := h.service.DeleteApp(r.Context(), appID); err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "delete", "operational", "vps_app", appID, nil, nil)
	response.WriteJSON(w, http.StatusOK, map[string]string{"message": "App deleted"}, nil)
}

func (h *VPSHandler) writeError(ctx context.Context, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, operationalservice.ErrVPSNotFound):
		response.WriteError(w, http.StatusNotFound, "VPS_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, operationalservice.ErrVPSCheckNotFound):
		response.WriteError(w, http.StatusNotFound, "VPS_CHECK_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, operationalservice.ErrVPSAppNotFound):
		response.WriteError(w, http.StatusNotFound, "VPS_APP_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, operationalservice.ErrVPSCheckOnOther):
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), map[string]string{"check_id": "must belong to the same vps"})
	case errors.Is(err, operationalservice.ErrInvalidVPSCheck):
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), map[string]string{"target": "invalid for selected type"})
	default:
		response.WriteInternalError(ctx, w, err, "An unexpected error occurred")
	}
}

func validateVPSIDParam(w http.ResponseWriter, vpsID string) (string, bool) {
	return validateUUIDParam(w, "vpsID", vpsID)
}

func validateUUIDParam(w http.ResponseWriter, name, value string) (string, bool) {
	value = strings.TrimSpace(value)
	if _, err := uuid.Parse(value); err != nil {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Path validation failed", map[string]string{name: "must be a valid UUID"})
		return "", false
	}
	return value, true
}
