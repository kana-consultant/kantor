package hris

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/kana-consultant/kantor/backend/internal/response"
	hrisservice "github.com/kana-consultant/kantor/backend/internal/service/hris"
)

// EmailDeliveriesHandler serves GET /hris/email-deliveries: the delivery
// history ('Riwayat Pengiriman') of one payslip or contract. The required
// permission follows the document kind (hris:payslip:view or
// hris:contract:view), checked against the stored rows; admin test e-mails
// are never listed.
type EmailDeliveriesHandler struct {
	mailer *hrisservice.DocumentMailer
}

func NewEmailDeliveriesHandler(mailer *hrisservice.DocumentMailer) *EmailDeliveriesHandler {
	return &EmailDeliveriesHandler{mailer: mailer}
}

func (h *EmailDeliveriesHandler) RegisterRoutes(router chi.Router) {
	router.Get("/", h.list)
}

func (h *EmailDeliveriesHandler) list(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalOrUnauthorized(w, r)
	if !ok {
		return
	}
	referenceType := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("reference_type")))
	referenceID := strings.TrimSpace(r.URL.Query().Get("reference_id"))
	if referenceType == "" || referenceID == "" {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "reference_type dan reference_id wajib diisi", map[string]string{"reference_type": "required", "reference_id": "required"})
		return
	}
	result, err := h.mailer.ListDeliveries(r.Context(), referenceType, referenceID, documentPermissionChecker(principal), documentViewer(principal))
	if err != nil {
		writeDocumentError(r.Context(), w, err, "Terjadi kesalahan saat memuat riwayat pengiriman")
		return
	}
	response.WriteJSON(w, http.StatusOK, result, nil)
}
