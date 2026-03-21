package middleware

import "net/http"

func RBACMiddleware(requiredPermission string) func(http.Handler) http.Handler {
	return RequirePermission(requiredPermission)
}

func SuperAdminMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, ok := PrincipalFromContext(r.Context())
			if !ok {
				writeUnauthorized(w)
				return
			}

			if principal.IsSuperAdmin {
				next.ServeHTTP(w, r)
				return
			}

			writeForbidden(w, "FORBIDDEN", "Super admin access is required")
		})
	}
}
