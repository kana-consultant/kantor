package middleware

import (
	"context"
	"log/slog"
	"net/http"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

type loggerContextKey string

const slogContextKey loggerContextKey = "slog_logger"

func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := chimiddleware.GetReqID(r.Context())
		logger := slog.Default().With("request_id", reqID)
		ctx := context.WithValue(r.Context(), slogContextKey, logger)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func LoggerFromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(slogContextKey).(*slog.Logger); ok {
		return logger
	}
	return slog.Default()
}
