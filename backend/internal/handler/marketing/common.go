package marketing

import (
	"net/http"

	"github.com/go-playground/validator/v10"

	"github.com/kana-consultant/kantor/backend/internal/httputil"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	"github.com/kana-consultant/kantor/backend/internal/response"
)

func newValidator() *validator.Validate {
	return httputil.NewValidator()
}

func decodeAndValidate(v *validator.Validate, w http.ResponseWriter, r *http.Request, target interface{}) bool {
	return httputil.DecodeAndValidate(v, w, r, target)
}

func validationDetails(err error) map[string]string {
	return httputil.ValidationDetails(err)
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
