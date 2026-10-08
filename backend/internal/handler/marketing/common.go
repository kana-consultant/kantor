package marketing

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"

	marketingdto "github.com/kana-consultant/kantor/backend/internal/dto/marketing"
	"github.com/kana-consultant/kantor/backend/internal/httputil"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	"github.com/kana-consultant/kantor/backend/internal/response"
)

// newValidator returns the validator of the marketing handlers. It is a
// fresh instance (httputil.NewValidator), so the two marketing-only settings
// below do not reach any other module:
//   - the channel/platform/stage tags backed by the shared lists in the dto
//     package, and
//   - validation details keyed by the JSON field name (end_date,
//     pic_employee_id) instead of the Go field name, so a client can attach
//     the message to the form field it sent.
func newValidator() *validator.Validate {
	v := httputil.NewValidator()
	marketingdto.RegisterValidations(v)
	v.RegisterTagNameFunc(func(field reflect.StructField) string {
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name == "" || name == "-" {
			return field.Name
		}
		return name
	})
	return v
}

// decodeAndValidate is httputil.DecodeAndValidate with the marketing
// validation details.
func decodeAndValidate(v *validator.Validate, w http.ResponseWriter, r *http.Request, target interface{}) bool {
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		response.WriteError(w, http.StatusBadRequest, "INVALID_JSON", "Request body must be valid JSON", nil)
		return false
	}

	if err := v.Struct(target); err != nil {
		response.WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Request validation failed", validationDetails(err))
		return false
	}

	return true
}

// validationDetails returns {json field name: failed rule}. The rule is the
// underlying validator tag, so the shared-list aliases report "oneof" like a
// literal list would.
func validationDetails(err error) map[string]string {
	details := map[string]string{}

	validationErrors, ok := err.(validator.ValidationErrors)
	if !ok {
		return details
	}

	for _, validationErr := range validationErrors {
		details[validationErr.Field()] = validationErr.ActualTag()
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
