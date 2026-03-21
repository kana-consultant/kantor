package marketing

import (
	"encoding/json"
	"net/http"

	"github.com/go-playground/validator/v10"

	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	"github.com/kana-consultant/kantor/backend/internal/response"
)

func newValidator() *validator.Validate {
	return validator.New(validator.WithRequiredStructEnabled())
}

func decodeAndValidate(validator *validator.Validate, w http.ResponseWriter, r *http.Request, target interface{}) bool {
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		response.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Request body must be valid JSON", nil)
		return false
	}

	if err := validator.Struct(target); err != nil {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Request validation failed", validationDetails(err))
		return false
	}

	return true
}

func validationDetails(err error) map[string]string {
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

func requireMarketingAdmin(w http.ResponseWriter, r *http.Request) (platformmiddleware.Principal, bool) {
	principal, ok := platformmiddleware.PrincipalFromContext(r.Context())
	if !ok {
		response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", nil)
		return platformmiddleware.Principal{}, false
	}

	if principal.IsSuperAdmin {
		return principal, true
	}
	if principal.Cached != nil && principal.Cached.Permissions["marketing:campaign:manage_columns"] {
		return principal, true
	}

	response.WriteError(w, http.StatusForbidden, "FORBIDDEN", "This action requires marketing admin access", nil)
	return platformmiddleware.Principal{}, false
}
