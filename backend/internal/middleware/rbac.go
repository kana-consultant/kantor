package middleware

import (
	"net/http"
	"strings"

	"github.com/kana-consultant/kantor/backend/internal/response"
)

func RBACMiddleware(requiredPermission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, ok := PrincipalFromContext(r.Context())
			if !ok {
				response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
				return
			}

			if requiredPermission == "" || hasPermission(principal, requiredPermission) {
				next.ServeHTTP(w, r)
				return
			}

			response.WriteError(w, http.StatusForbidden, "FORBIDDEN", "You do not have permission to access this resource", map[string]string{
				"permission": requiredPermission,
			})
		})
	}
}

func hasPermission(principal Principal, requiredPermission string) bool {
	for _, role := range principal.Roles {
		if role == "super_admin" || strings.HasPrefix(role, "super_admin:") {
			return true
		}
	}

	for _, permission := range principal.Permissions {
		if permission == requiredPermission {
			return true
		}
	}

	return false
}
