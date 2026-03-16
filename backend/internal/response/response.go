package response

import (
	"encoding/json"
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

func write(w http.ResponseWriter, status int, payload Envelope) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(true)
	_ = encoder.Encode(payload)
}
