package operational

import (
	"net/http"
)

func writeBinaryAttachment(w http.ResponseWriter, contentType, filename string, payload []byte) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}
