package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/kana-consultant/kantor/backend/internal/response"
)

// MaxBodySize limits the request body to maxBytes for non-multipart requests.
// Multipart/form-data requests are exempt because upload handlers set their
// own limits via ParseMultipartForm.
// Returns 413 if the body exceeds the limit.
func MaxBodySize(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ct := r.Header.Get("Content-Type")
			if !strings.HasPrefix(ct, "multipart/form-data") {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// WriteBodyTooLargeError writes a 413 error response for max body size violations.
// Handlers that decode JSON should check for http.MaxBytesError and call this.
func WriteBodyTooLargeError(w http.ResponseWriter) {
	response.WriteError(w, http.StatusRequestEntityTooLarge, "BODY_TOO_LARGE", "Request body exceeds the maximum allowed size", nil)
}

// IsBodyTooLargeError detects body-size violations produced by http.MaxBytesReader.
func IsBodyTooLargeError(err error) bool {
	if err == nil {
		return false
	}

	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		return true
	}

	return strings.Contains(strings.ToLower(err.Error()), "request body too large")
}
