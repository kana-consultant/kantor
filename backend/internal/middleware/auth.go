package middleware

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	backendauth "github.com/kana-consultant/kantor/backend/internal/auth"
	"github.com/kana-consultant/kantor/backend/internal/rbac"
	"github.com/kana-consultant/kantor/backend/internal/response"
)

type Principal struct {
	UserID       string
	TenantID     string
	IsSuperAdmin bool
	Roles        []string
	Permissions  []string
	ModuleRoles  map[string]rbac.ModuleRole
	Cached       *rbac.CachedPermissions
}

type contextKey string

const principalContextKey contextKey = "principal"

type patIdentity struct {
	userID   string
	tenantID string
	scope    *string
}

type patScopeDef struct {
	permissionPrefixes []string
	modules            []string
}

// patScopeDefs maps a PAT scope to the permissions and modules it may exercise.
// A scoped token can ONLY reach endpoints under these prefixes/modules and never
// receives the super-admin bypass — limiting the blast radius if it leaks.
var patScopeDefs = map[string]patScopeDef{
	"tracker": {
		permissionPrefixes: []string{"operational:tracker:"},
		modules:            []string{rbac.ModuleOperational},
	},
}

func AuthMiddleware(
	parseToken func(string) (*backendauth.AccessClaims, error),
	loadPermissions func(context.Context, string) (*rbac.CachedPermissions, error),
	blacklist *backendauth.AccessTokenBlacklist,
	authenticatePAT func(context.Context, string) (string, string, *string, error),
	patRateLimiter *PATAuthRateLimiter,
) func(http.Handler) http.Handler {
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

			rawToken := parts[1]

			var userID, tenantID string
			var patScope *string
			if backendauth.IsPersonalAccessToken(rawToken) {
				if authenticatePAT == nil {
					response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Access token is invalid or expired", nil)
					return
				}
				// Rate limit PAT authentication attempts BEFORE hitting database
				if patRateLimiter != nil {
					retryAfter := patRateLimiter.checkLimit(clientIPFromRequest(r), rawToken)
					if retryAfter > 0 {
						retryAfterSeconds := int(retryAfter.Seconds())
						if retryAfterSeconds < 1 {
							retryAfterSeconds = 1
						}
						response.WriteError(w, http.StatusTooManyRequests, "PAT_RATE_LIMIT_EXCEEDED", 
							"Too many PAT authentication attempts. Please try again later.", 
							map[string]int{"retry_after_seconds": retryAfterSeconds})
						return
					}
				}
				identity, err := WithScopedTenantConn(r.Context(), func(ctx context.Context) (patIdentity, error) {
					uid, tid, scope, authErr := authenticatePAT(ctx, rawToken)
					return patIdentity{userID: uid, tenantID: tid, scope: scope}, authErr
				})
				if err != nil {
					response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Access token is invalid or expired", nil)
					return
				}
				userID, tenantID, patScope = identity.userID, identity.tenantID, identity.scope
			} else {
				claims, err := parseToken(rawToken)
				if err != nil {
					response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Access token is invalid or expired", nil)
					return
				}
				if blacklist != nil && blacklist.IsRevoked(claims.ID) {
					response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Access token has been revoked", nil)
					return
				}
				userID, tenantID = claims.Subject, claims.TenantID
			}

			cachedPermissions, err := WithScopedTenantConn(r.Context(), func(ctx context.Context) (*rbac.CachedPermissions, error) {
				return loadPermissions(ctx, userID)
			})
			if err != nil {
				response.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Failed to resolve user permissions", nil)
				return
			}
			if !cachedPermissions.IsActive {
				response.WriteError(w, http.StatusForbidden, "INACTIVE_USER", "akun pengguna sedang tidak aktif", nil)
				return
			}

			if patScope != nil {
				def, ok := patScopeDefs[*patScope]
				if !ok {
					response.WriteError(w, http.StatusForbidden, "FORBIDDEN", "token scope is not permitted", nil)
					return
				}
				cachedPermissions = restrictCachedPermissions(cachedPermissions, def)
			}

			roles := make([]string, 0, len(cachedPermissions.ModuleRoles)+1)
			if cachedPermissions.IsSuperAdmin {
				roles = append(roles, "super_admin")
			}
			for moduleID, role := range cachedPermissions.ModuleRoles {
				roles = append(roles, role.RoleSlug+":"+moduleID)
			}

			principal := Principal{
				UserID:       userID,
				TenantID:     tenantID,
				IsSuperAdmin: cachedPermissions.IsSuperAdmin,
				Roles:        roles,
				Permissions:  cachedPermissions.PermissionList(),
				ModuleRoles:  cachedPermissions.ModuleRoles,
				Cached:       cachedPermissions,
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

// restrictCachedPermissions returns a scoped COPY of the loaded permissions:
// only permissions under the scope's prefixes survive, the super-admin bypass is
// dropped, and for a super admin (who normally carries no explicit permissions)
// the scope's full permission + module set is synthesized so the token still
// works. The source is never mutated — it is shared from the permission cache.
func restrictCachedPermissions(src *rbac.CachedPermissions, def patScopeDef) *rbac.CachedPermissions {
	filtered := make(map[string]bool)
	if src.IsSuperAdmin {
		for _, permission := range rbac.DefaultPermissions() {
			if hasAnyPrefix(permission.ID, def.permissionPrefixes) {
				filtered[permission.ID] = true
			}
		}
	} else {
		for permission := range src.Permissions {
			if hasAnyPrefix(permission, def.permissionPrefixes) {
				filtered[permission] = true
			}
		}
	}

	moduleRoles := src.ModuleRoles
	if src.IsSuperAdmin {
		moduleRoles = make(map[string]rbac.ModuleRole, len(def.modules))
		for _, moduleID := range def.modules {
			moduleRoles[moduleID] = rbac.ModuleRole{}
		}
	}

	return &rbac.CachedPermissions{
		IsActive:     src.IsActive,
		IsSuperAdmin: false,
		ModuleRoles:  moduleRoles,
		Permissions:  filtered,
		CachedAt:     src.CachedAt,
		TTL:          src.TTL,
	}
}

func hasAnyPrefix(value string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

// PATAuthRateLimiter implements rate limiting for PAT authentication attempts.
// It tracks attempts by both IP address and token hash to prevent:
// 1. Brute force attacks (limiting attempts per IP)
// 2. Leaked token abuse (limiting attempts per token)
type PATAuthRateLimiter struct {
	maxAttemptsPerIP    int
	maxAttemptsPerToken int
	window              time.Duration

	mu      sync.Mutex
	entries map[string]rateLimitEntry
}

// NewPATAuthRateLimiter creates a rate limiter for PAT authentication.
// maxAttemptsPerIP: maximum auth attempts from a single IP in the window
// maxAttemptsPerToken: maximum auth attempts for a single token in the window
// window: time window for rate limiting (e.g., 1 minute)
func NewPATAuthRateLimiter(maxAttemptsPerIP, maxAttemptsPerToken int, window time.Duration) *PATAuthRateLimiter {
	return &PATAuthRateLimiter{
		maxAttemptsPerIP:    maxAttemptsPerIP,
		maxAttemptsPerToken: maxAttemptsPerToken,
		window:              window,
		entries:             make(map[string]rateLimitEntry),
	}
}

// checkLimit checks if the request should be rate limited.
// Returns retry-after duration if limited, 0 if allowed.
func (l *PATAuthRateLimiter) checkLimit(clientIP, token string) time.Duration {
	now := time.Now().UTC()
	
	// Check IP-based limit
	ipKey := "ip:" + clientIP
	if retryAfter := l.checkKey(ipKey, l.maxAttemptsPerIP, now); retryAfter > 0 {
		return retryAfter
	}
	
	// Check token-based limit (hash token to avoid storing actual token)
	// Use first 16 chars of token as identifier (enough to distinguish tokens)
	tokenKey := "token:" + tokenPrefix(token)
	if retryAfter := l.checkKey(tokenKey, l.maxAttemptsPerToken, now); retryAfter > 0 {
		return retryAfter
	}
	
	return 0
}

func (l *PATAuthRateLimiter) checkKey(key string, maxAttempts int, now time.Time) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	// Cleanup old entries periodically
	if len(l.entries) > 1024 {
		for candidate, entry := range l.entries {
			if now.After(entry.resetAt) {
				delete(l.entries, candidate)
			}
		}
		// Hard cap to prevent OOM under attack
		if len(l.entries) > 10000 {
			clear(l.entries)
		}
	}
	
	entry, exists := l.entries[key]
	if !exists || now.After(entry.resetAt) {
		l.entries[key] = rateLimitEntry{
			count:   1,
			resetAt: now.Add(l.window),
		}
		return 0
	}
	
	if entry.count >= maxAttempts {
		return entry.resetAt.Sub(now)
	}
	
	entry.count++
	l.entries[key] = entry
	return 0
}

// tokenPrefix returns first 16 characters of token for rate limit keying.
// This is enough to distinguish tokens without storing full token values.
func tokenPrefix(token string) string {
	if len(token) <= 16 {
		return token
	}
	return token[:16]
}
