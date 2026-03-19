package marketing

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"

	marketingdto "github.com/kana-consultant/kantor/backend/internal/dto/marketing"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	"github.com/kana-consultant/kantor/backend/internal/response"
	marketingservice "github.com/kana-consultant/kantor/backend/internal/service/marketing"
)

type LeadsHandler struct {
	service   *marketingservice.LeadsService
	validator *validator.Validate
}

func NewLeadsHandler(service *marketingservice.LeadsService) *LeadsHandler {
	return &LeadsHandler{
		service:   service,
		validator: newValidator(),
	}
}

func (h *LeadsHandler) RegisterRoutes(router chi.Router) {
	router.With(platformmiddleware.RBACMiddleware("marketing:leads:create")).Post("/", h.createLead)
	router.With(platformmiddleware.RBACMiddleware("marketing:leads:view")).Get("/", h.listLeads)
	router.With(platformmiddleware.RBACMiddleware("marketing:leads:view")).Get("/summary", h.summary)
	router.With(platformmiddleware.RBACMiddleware("marketing:leads:view")).Get("/pipeline", h.pipeline)
	router.With(platformmiddleware.RBACMiddleware("marketing:leads:create")).Post("/import", h.importCSV)
	router.With(platformmiddleware.RBACMiddleware("marketing:leads:view")).Get("/{leadID}", h.getLead)
	router.With(platformmiddleware.RBACMiddleware("marketing:leads:edit")).Put("/{leadID}", h.updateLead)
	router.With(platformmiddleware.RBACMiddleware("marketing:leads:delete")).Delete("/{leadID}", h.deleteLead)
	router.With(platformmiddleware.RBACMiddleware("marketing:leads:edit")).Patch("/{leadID}/status", h.moveStatus)
	router.With(platformmiddleware.RBACMiddleware("marketing:leads:view")).Get("/{leadID}/activities", h.listActivities)
	router.With(platformmiddleware.RBACMiddleware("marketing:leads:edit")).Post("/{leadID}/activities", h.createActivity)
}

func (h *LeadsHandler) createLead(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}

	var input marketingdto.CreateLeadRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}

	item, err := h.service.CreateLead(r.Context(), input, principal.UserID)
	if err != nil {
		h.writeError(w, err)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "create", "marketing", "lead", item.ID, nil, input)
	response.WriteJSON(w, http.StatusCreated, item, nil)
}

func (h *LeadsHandler) listLeads(w http.ResponseWriter, r *http.Request) {
	query, ok := h.parseListQuery(w, r)
	if !ok {
		return
	}

	items, total, page, perPage, err := h.service.ListLeads(r.Context(), query)
	if err != nil {
		h.writeError(w, err)
		return
	}

	response.WriteJSON(w, http.StatusOK, items, map[string]int64{
		"page":     int64(page),
		"per_page": int64(perPage),
		"total":    total,
	})
}

func (h *LeadsHandler) getLead(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.GetLead(r.Context(), chi.URLParam(r, "leadID"))
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, item, nil)
}

func (h *LeadsHandler) updateLead(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}

	var input marketingdto.UpdateLeadRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}

	leadID := chi.URLParam(r, "leadID")
	item, err := h.service.UpdateLead(r.Context(), leadID, input, principal.UserID)
	if err != nil {
		h.writeError(w, err)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "update", "marketing", "lead", leadID, nil, input)
	response.WriteJSON(w, http.StatusOK, item, nil)
}

func (h *LeadsHandler) deleteLead(w http.ResponseWriter, r *http.Request) {
	leadID := chi.URLParam(r, "leadID")
	if err := h.service.DeleteLead(r.Context(), leadID); err != nil {
		h.writeError(w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "delete", "marketing", "lead", leadID, nil, nil)
	response.WriteJSON(w, http.StatusOK, map[string]string{"message": "Lead deleted successfully"}, nil)
}

func (h *LeadsHandler) pipeline(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.Pipeline(r.Context())
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, items, nil)
}

func (h *LeadsHandler) moveStatus(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}

	var input marketingdto.MoveLeadStatusRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}

	leadID := chi.URLParam(r, "leadID")
	item, err := h.service.MoveStatus(r.Context(), leadID, input, principal.UserID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "move_status", "marketing", "lead", leadID, nil, input)
	response.WriteJSON(w, http.StatusOK, item, nil)
}

func (h *LeadsHandler) listActivities(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.ListActivities(r.Context(), chi.URLParam(r, "leadID"))
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, items, nil)
}

func (h *LeadsHandler) createActivity(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}

	var input marketingdto.CreateLeadActivityRequest
	if !decodeAndValidate(h.validator, w, r, &input) {
		return
	}

	leadID := chi.URLParam(r, "leadID")
	item, err := h.service.CreateActivity(r.Context(), leadID, input, principal.UserID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	platformmiddleware.AuditLog(r.Context(), "create", "marketing", "lead_activity", item.ID, nil, input)
	response.WriteJSON(w, http.StatusCreated, item, nil)
}

func (h *LeadsHandler) importCSV(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		response.WriteError(w, http.StatusBadRequest, "INVALID_MULTIPART", "Lead import must use multipart form data", nil)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "CSV file is required", map[string]string{"file": "required"})
		return
	}
	defer file.Close()

	summary, err := h.service.ImportCSV(r.Context(), file, principal.UserID)
	if err != nil {
		h.writeError(w, err)
		return
	}

	platformmiddleware.AuditLog(r.Context(), "import_csv", "marketing", "lead", "bulk", nil, summary)
	response.WriteJSON(w, http.StatusOK, summary, nil)
}

func (h *LeadsHandler) summary(w http.ResponseWriter, r *http.Request) {
	item, err := h.service.Summary(r.Context())
	if err != nil {
		h.writeError(w, err)
		return
	}
	response.WriteJSON(w, http.StatusOK, item, nil)
}

func (h *LeadsHandler) parseListQuery(w http.ResponseWriter, r *http.Request) (marketingdto.ListLeadsQuery, bool) {
	query := marketingdto.ListLeadsQuery{
		PipelineStatus: strings.TrimSpace(r.URL.Query().Get("pipeline_status")),
		SourceChannel:  strings.TrimSpace(r.URL.Query().Get("source_channel")),
		CampaignID:     strings.TrimSpace(r.URL.Query().Get("campaign_id")),
		AssignedTo:     strings.TrimSpace(r.URL.Query().Get("assigned_to")),
		DateFrom:       strings.TrimSpace(r.URL.Query().Get("date_from")),
		DateTo:         strings.TrimSpace(r.URL.Query().Get("date_to")),
		Search:         strings.TrimSpace(r.URL.Query().Get("search")),
	}

	if value := strings.TrimSpace(r.URL.Query().Get("page")); value != "" {
		page, err := strconv.Atoi(value)
		if err != nil {
			response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Query validation failed", map[string]string{"page": "must be a number"})
			return marketingdto.ListLeadsQuery{}, false
		}
		query.Page = page
	}
	if value := strings.TrimSpace(r.URL.Query().Get("per_page")); value != "" {
		perPage, err := strconv.Atoi(value)
		if err != nil {
			response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Query validation failed", map[string]string{"per_page": "must be a number"})
			return marketingdto.ListLeadsQuery{}, false
		}
		query.PerPage = perPage
	}

	if err := h.validator.Struct(query); err != nil {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Query validation failed", validationDetails(err))
		return marketingdto.ListLeadsQuery{}, false
	}

	return query, true
}

func (h *LeadsHandler) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, marketingservice.ErrLeadNotFound):
		response.WriteError(w, http.StatusNotFound, "LEAD_NOT_FOUND", err.Error(), nil)
	case errors.Is(err, marketingservice.ErrLeadAssignedUserNotFound):
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), map[string]string{"assigned_to": "not found"})
	case errors.Is(err, marketingservice.ErrLeadCampaignNotFound):
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), map[string]string{"campaign_id": "not found"})
	case errors.Is(err, marketingservice.ErrLeadContactRequired):
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), map[string]string{"phone": "phone or email required", "email": "phone or email required"})
	default:
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred", nil)
	}
}
