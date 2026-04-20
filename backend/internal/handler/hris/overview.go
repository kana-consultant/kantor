package hris

import (
	"net/http"

	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	"github.com/kana-consultant/kantor/backend/internal/response"
	hrisservice "github.com/kana-consultant/kantor/backend/internal/service/hris"
)

type OverviewHandler struct {
	service *hrisservice.OverviewService
}

func NewOverviewHandler(service *hrisservice.OverviewService) *OverviewHandler {
	return &OverviewHandler{service: service}
}

func (h *OverviewHandler) Get(w http.ResponseWriter, r *http.Request) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return
	}

	result, err := h.service.GetOverview(r.Context(), principal.UserID, principal.Cached)
	if err != nil {
		response.WriteInternalError(r.Context(), w, err, "An unexpected error occurred")
		return
	}

	response.WriteJSON(w, http.StatusOK, result, nil)
}
