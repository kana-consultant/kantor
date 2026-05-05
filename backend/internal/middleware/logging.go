package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

type requestIDKey struct{}

// LoggingMiddleware injects the chi request ID into context so that
// ContextHandler (and LoggerFromContext) can attach it to every log line.
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := chimiddleware.GetReqID(r.Context())
		ctx := context.WithValue(r.Context(), requestIDKey{}, reqID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// accessLogSkipPaths lists endpoints that are excluded from access logging
// to keep the log volume manageable. Health probes are hit every few seconds
// by container orchestrators and would otherwise dominate the log output.
var accessLogSkipPaths = map[string]struct{}{
	"/healthz":       {},
	"/readyz":        {},
	"/api/v1/health": {},
}

// AccessLogger emits a structured log line for every HTTP request after the
// response is written. It captures method, path, status, latency, bytes,
// remote IP, request_id, and (when authenticated) user_id and tenant_id.
//
// Mount this OUTSIDE Recoverer (i.e. before Recoverer in the .Use chain) so
// that panics surfaced as 500s by Recoverer are still recorded.
func AccessLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, skip := accessLogSkipPaths[r.URL.Path]; skip {
			next.ServeHTTP(w, r)
			return
		}

		ww := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()

		defer func() {
			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}
			latency := time.Since(start)

			attrs := []any{
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", status),
				slog.Int("bytes", ww.BytesWritten()),
				slog.Int64("latency_ms", latency.Milliseconds()),
				slog.String("remote_ip", ClientIPFromRequest(r)),
			}
			if reqID := chimiddleware.GetReqID(r.Context()); reqID != "" {
				attrs = append(attrs, slog.String("request_id", reqID))
			}
			if principal, ok := PrincipalFromContext(r.Context()); ok {
				attrs = append(attrs, slog.String("user_id", principal.UserID))
				if principal.TenantID != "" {
					attrs = append(attrs, slog.String("tenant_id", principal.TenantID))
				}
			}

			level := slog.LevelInfo
			switch {
			case status >= 500:
				level = slog.LevelError
			case status >= 400:
				level = slog.LevelWarn
			}
			slog.Default().LogAttrs(r.Context(), level, "http_request", toSlogAttrs(attrs)...)
		}()

		next.ServeHTTP(ww, r)
	})
}

func toSlogAttrs(values []any) []slog.Attr {
	out := make([]slog.Attr, 0, len(values))
	for _, v := range values {
		if attr, ok := v.(slog.Attr); ok {
			out = append(out, attr)
		}
	}
	return out
}

// LoggerFromContext returns a logger enriched with the request_id
// from context. Use this in handlers for request-scoped logging.
func LoggerFromContext(ctx context.Context) *slog.Logger {
	reqID, _ := ctx.Value(requestIDKey{}).(string)
	if reqID != "" {
		return slog.Default().With("request_id", reqID)
	}
	return slog.Default()
}

// ContextHandler wraps an slog.Handler and automatically adds
// request_id from context to every log record. This allows plain
// slog.InfoContext / slog.ErrorContext calls to include request_id
// without explicitly calling LoggerFromContext.
type ContextHandler struct {
	inner slog.Handler
}

func NewContextHandler(inner slog.Handler) *ContextHandler {
	return &ContextHandler{inner: inner}
}

func (h *ContextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *ContextHandler) Handle(ctx context.Context, record slog.Record) error {
	if reqID, ok := ctx.Value(requestIDKey{}).(string); ok && reqID != "" {
		record.AddAttrs(slog.String("request_id", reqID))
	}
	return h.inner.Handle(ctx, record)
}

func (h *ContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ContextHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h *ContextHandler) WithGroup(name string) slog.Handler {
	return &ContextHandler{inner: h.inner.WithGroup(name)}
}
