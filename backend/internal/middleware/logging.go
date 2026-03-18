package middleware

import (
	"context"
	"log/slog"
	"net/http"

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
