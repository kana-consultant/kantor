package operational

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"

	operationaldto "github.com/kana-consultant/kantor/backend/internal/dto/operational"
	"github.com/kana-consultant/kantor/backend/internal/httputil"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	operationalrepo "github.com/kana-consultant/kantor/backend/internal/repository/operational"
	"github.com/kana-consultant/kantor/backend/internal/response"
	operationalservice "github.com/kana-consultant/kantor/backend/internal/service/operational"
)

// DomainHandler exposes the operational Domain monitoring endpoints. All
// routes gated behind operational:domain:* permissions.
type DomainHandler struct {
	service   *operationalservice.DomainService
	validator *validator.Validate
}

func NewDomainHandler(service *operationalservice.DomainService) *DomainHandler {
	return &DomainHandler{
		service:   service,
		validator: validator.New(validator.WithRequiredStructEnabled()),
	}
}

func (h *DomainHandler) RegisterRoutes(router chi.Router) {
	router.With(platformmiddleware.RequirePermission("operational:domain:view")).Get("/", h.listDomains)
	router.With(platformmiddleware.RequirePermission("operational:domain:create")).Post("/", h.createDomain)
	router.With(platformmiddleware.RequirePermission("operational:domain:view")).Get("/{domainID}", h.getDomain)
	router.With(platformmiddleware.RequirePermission("operational:domain:edit")).Put("/{domainID}", h.updateDomain)
	router.With(platformmiddleware.RequirePermission("operational:domain:delete")).Delete("/{domainID}", h.deleteDomain)
}

func (h *DomainHandler) listDomains(w http.ResponseWriter, r *http.Request) {
	q := operationalrepo.ListDomainParams{
		Status:    strings.TrimSpace(r.URL.Query().Get("status")),
		Registrar: strings.TrimSpace(r.URL.Query().Get("registrar")),
		Tag:       strings.TrimSpace(r.URL.Query().Get("tag")),
		Search:    strings.TrimSpace(r.URL.Query().Get("search")),
	}
	domains, err := h.service.ListDomains(r.Context(), q)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, domains, nil)
}

func (h *DomainHandler) createDomain(w http.ResponseWriter, r *http.Request) {
	var input operationaldto.CreateDomainRequest
	if !httputil.DecodeAndValidate(h.validator, w, r, &input) {
		return
	}
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
		return
	}
	d, err := h.service.CreateDomain(r.Context(), input, principal.UserID)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "create", "operational", "domain", d.ID, nil, input)
	response.WriteJSON(w, http.StatusCreated, d, nil)
}

func (h *DomainHandler) getDomain(w http.ResponseWriter, r *http.Request) {
	domainID, ok := validateDomainIDParam(w, chi.URLParam(r, "domainID"))
	if !ok {
		return
	}
	detail, err := h.service.GetDomainDetail(r.Context(), domainID)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, detail, nil)
}

func (h *DomainHandler) updateDomain(w http.ResponseWriter, r *http.Request) {
	domainID, ok := validateDomainIDParam(w, chi.URLParam(r, "domainID"))
	if !ok {
		return
	}
	var input operationaldto.UpdateDomainRequest
	if !httputil.DecodeAndValidate(h.validator, w, r, &input) {
		return
	}
	d, err := h.service.UpdateDomain(r.Context(), domainID, input)
	if err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "update", "operational", "domain", domainID, nil, input)
	response.WriteJSON(w, http.StatusOK, d, nil)
}

func (h *DomainHandler) deleteDomain(w http.ResponseWriter, r *http.Request) {
	domainID, ok := validateDomainIDParam(w, chi.URLParam(r, "domainID"))
	if !ok {
		return
	}
	if err := h.service.DeleteDomain(r.Context(), domainID); err != nil {
		h.writeError(r.Context(), w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "delete", "operational", "domain", domainID, nil, nil)
	response.WriteJSON(w, http.StatusOK, map[string]string{"message": "Domain deleted"}, nil)
}

func (h *DomainHandler) writeError(ctx context.Context, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, operationalservice.ErrDomainNotFound):
		response.WriteError(w, http.StatusNotFound, "DOMAIN_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, operationalservice.ErrInvalidDomain):
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), map[string]string{"name": "invalid domain"})
	default:
		response.WriteInternalError(ctx, w, err, "An unexpected error occurred")
	}
}

func validateDomainIDParam(w http.ResponseWriter, domainID string) (string, bool) {
	return validateUUIDParam(w, "domainID", domainID)
}
