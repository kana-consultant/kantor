package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/app"
	"github.com/kana-consultant/kantor/backend/internal/config"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	application, err := app.New(ctx, cfg)
	if err != nil {
		slog.Error("failed to initialize application", "error", err)
		os.Exit(1)
	}
	defer application.Close()

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           application.Router(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("failed to shutdown server", "error", err)
		}
	}()

	slog.Info("starting server", "addr", server.Addr, "environment", cfg.AppEnv)

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server exited with error", "error", err)
		os.Exit(1)
	}
}
