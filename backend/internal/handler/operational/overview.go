package operational

import (
	"net/http"

	"github.com/kana-consultant/kantor/backend/internal/response"
	operationalservice "github.com/kana-consultant/kantor/backend/internal/service/operational"
)

type OverviewHandler struct {
	service *operationalservice.OverviewService
}

func NewOverviewHandler(service *operationalservice.OverviewService) *OverviewHandler {
	return &OverviewHandler{service: service}
}

func (h *OverviewHandler) Get(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.GetOverview(r.Context())
	if err != nil {
		response.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred", nil)
		return
	}

	response.WriteJSON(w, http.StatusOK, result, nil)
}
