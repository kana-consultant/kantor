package marketing

import (
	"net/http"

	"github.com/kana-consultant/kantor/backend/internal/response"
	marketingservice "github.com/kana-consultant/kantor/backend/internal/service/marketing"
)

type OverviewHandler struct {
	service *marketingservice.OverviewService
}

func NewOverviewHandler(service *marketingservice.OverviewService) *OverviewHandler {
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
