package hris

import (
	"net/http"

	"github.com/kana-consultant/kantor/backend/internal/response"
)

func writeBinaryAttachment(w http.ResponseWriter, contentType, filename string, payload []byte) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func responseUnsupportedFormat(w http.ResponseWriter) {
	response.WriteError(w, http.StatusBadRequest, "UNSUPPORTED_EXPORT_FORMAT", "Export format is not supported", map[string]string{"format": "must be pdf or xlsx"})
}

func responseUnauthorized(w http.ResponseWriter) {
	response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
}
