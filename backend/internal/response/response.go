package response

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
)

type ErrorBody struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

type Envelope struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *ErrorBody  `json:"error,omitempty"`
	Meta    interface{} `json:"meta,omitempty"`
}

func WriteJSON(w http.ResponseWriter, status int, data interface{}, meta interface{}) {
	write(w, status, Envelope{
		Success: true,
		Data:    data,
		Meta:    meta,
	})
}

func WriteError(w http.ResponseWriter, status int, code string, message string, details interface{}) {
	write(w, status, Envelope{
		Success: false,
		Error: &ErrorBody{
			Code:    code,
			Message: message,
			Details: details,
		},
	})
}

// WriteInternalError logs the underlying error with the request context and
// returns a 500 response with the standard INTERNAL_ERROR envelope. Handlers
// MUST use this for unexpected/unhandled failures so server-side observability
// captures every 500.
func WriteInternalError(ctx context.Context, w http.ResponseWriter, err error, message string) {
	slog.ErrorContext(ctx, "internal server error", "error", err, "user_message", message)
	WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", message, nil)
}

func write(w http.ResponseWriter, status int, payload Envelope) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(true)
	_ = encoder.Encode(payload)
}
