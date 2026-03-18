package middleware

import (
	"net/http"

	"github.com/kana-consultant/kantor/backend/internal/response"
)

// MaxBodySize limits the request body to maxBytes.
// Returns 413 if the body exceeds the limit.
func MaxBodySize(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}

// WriteBodyTooLargeError writes a 413 error response for max body size violations.
// Handlers that decode JSON should check for http.MaxBytesError and call this.
func WriteBodyTooLargeError(w http.ResponseWriter) {
	response.WriteError(w, http.StatusRequestEntityTooLarge, "BODY_TOO_LARGE", "Request body exceeds the maximum allowed size", nil)
}
