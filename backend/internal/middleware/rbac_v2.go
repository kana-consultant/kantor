package middleware

import (
	"net/http"

	"github.com/kana-consultant/kantor/backend/internal/response"
)

func RequirePermission(permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, ok := PrincipalFromContext(r.Context())
			if !ok {
				writeUnauthorized(w)
				return
			}

			if principal.IsSuperAdmin || hasPermission(principal, permission) {
				next.ServeHTTP(w, r)
				return
			}

			writeForbidden(w, "FORBIDDEN", "You do not have permission to access this resource")
		})
	}
}

func RequireModuleAccess(moduleID string) func(http.Handler) http.Handler {
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

			if _, exists := principal.ModuleRoles[moduleID]; exists {
				next.ServeHTTP(w, r)
				return
			}

			writeForbidden(w, "FORBIDDEN", "You do not have access to this module")
		})
	}
}

func RequireAnyPermission(permissions ...string) func(http.Handler) http.Handler {
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

			for _, permission := range permissions {
				if hasPermission(principal, permission) {
					next.ServeHTTP(w, r)
					return
				}
			}

			writeForbidden(w, "FORBIDDEN", "You do not have permission to access this resource")
		})
	}
}

func RequireAllPermissions(permissions ...string) func(http.Handler) http.Handler {
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

			for _, permission := range permissions {
				if !hasPermission(principal, permission) {
					writeForbidden(w, "FORBIDDEN", "You do not have permission to access this resource")
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

func hasPermission(principal Principal, permission string) bool {
	if principal.Cached != nil && principal.Cached.Permissions[permission] {
		return true
	}

	for _, item := range principal.Permissions {
		if item == permission {
			return true
		}
	}

	return false
}

func writeUnauthorized(w http.ResponseWriter) {
	response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authenticated principal is missing", nil)
}

func writeForbidden(w http.ResponseWriter, code string, message string) {
	response.WriteError(w, http.StatusForbidden, code, message, nil)
}
