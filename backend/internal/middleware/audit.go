package middleware

import (
	"context"
	"net"
	"net/http"

	auditservice "github.com/kana-consultant/kantor/backend/internal/service/audit"
)

const (
	auditServiceKey contextKey = "auditService"
	clientIPKey     contextKey = "clientIP"
)

func AuditMiddleware(svc *auditservice.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := extractIP(r)
			ctx := context.WithValue(r.Context(), auditServiceKey, svc)
			ctx = context.WithValue(ctx, clientIPKey, ip)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func AuditServiceFromContext(ctx context.Context) *auditservice.Service {
	svc, _ := ctx.Value(auditServiceKey).(*auditservice.Service)
	return svc
}

func ClientIPFromContext(ctx context.Context) string {
	ip, _ := ctx.Value(clientIPKey).(string)
	return ip
}

// AuditLog is a convenience helper for handlers to log an audit entry.
// It pulls the audit service, principal, and client IP from the request context.
func AuditLog(ctx context.Context, action, module, resource, resourceID string, oldValue, newValue interface{}) {
	svc := AuditServiceFromContext(ctx)
	if svc == nil {
		return
	}

	principal, _ := PrincipalFromContext(ctx)

	svc.Log(ctx, auditservice.Entry{
		UserID:     principal.UserID,
		Action:     action,
		Module:     module,
		Resource:   resource,
		ResourceID: resourceID,
		OldValue:   oldValue,
		NewValue:   newValue,
		IPAddress:  ClientIPFromContext(ctx),
	})
}

func extractIP(r *http.Request) string {
	// chi's RealIP middleware sets RemoteAddr from X-Forwarded-For / X-Real-IP,
	// so r.RemoteAddr already reflects the real client IP in most setups.
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
