package middleware

import (
	"context"
	"net/http"
	"strings"

	backendauth "github.com/kana-consultant/kantor/backend/internal/auth"
	"github.com/kana-consultant/kantor/backend/internal/response"
)

type Principal struct {
	UserID      string
	Roles       []string
	Permissions []string
}

type contextKey string

const principalContextKey contextKey = "principal"

func AuthMiddleware(parseToken func(string) (*backendauth.AccessClaims, error)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := strings.TrimSpace(r.Header.Get("Authorization"))
			if header == "" {
				response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authorization header is required", nil)
				return
			}

			parts := strings.SplitN(header, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authorization header must use Bearer token", nil)
				return
			}

			claims, err := parseToken(parts[1])
			if err != nil {
				response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Access token is invalid or expired", nil)
				return
			}

			principal := Principal{
				UserID:      claims.Subject,
				Roles:       claims.Roles,
				Permissions: claims.Permissions,
			}

			ctx := context.WithValue(r.Context(), principalContextKey, principal)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalContextKey).(Principal)
	return principal, ok
}
