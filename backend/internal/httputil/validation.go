// Package httputil contains small HTTP helpers shared by every handler
// package. Keeping them here avoids the drift that appears when each
// package defines its own copy of the same helper.
package httputil

import (
	"encoding/json"
	"net/http"

	"github.com/go-playground/validator/v10"

	"github.com/kana-consultant/kantor/backend/internal/response"
)

// NewValidator returns a validator.Validate with the project-wide settings.
// All handler packages must construct their validator via this helper so
// validation behaviour stays consistent across modules.
func NewValidator() *validator.Validate {
	return validator.New(validator.WithRequiredStructEnabled())
}

// DecodeAndValidate reads a JSON body into target and validates it using v.
// On any failure it writes a 400 error response and returns false; the caller
// should simply return when false is returned.
//
// This is the single source of truth for request decoding + validation —
// every handler package used to define its own identical copy.
func DecodeAndValidate(v *validator.Validate, w http.ResponseWriter, r *http.Request, target interface{}) bool {
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		response.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Request body must be valid JSON", nil)
		return false
	}

	if err := v.Struct(target); err != nil {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Request validation failed", ValidationDetails(err))
		return false
	}

	return true
}

// ValidationDetails converts a validator.ValidationErrors value into the
// {field: tag} map that the API returns under the "details" key.
func ValidationDetails(err error) map[string]string {
	details := map[string]string{}

	validationErrors, ok := err.(validator.ValidationErrors)
	if !ok {
		return details
	}

	for _, validationErr := range validationErrors {
		details[validationErr.Field()] = validationErr.Tag()
	}

	return details
}
