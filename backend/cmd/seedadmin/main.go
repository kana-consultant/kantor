// cmd/seedadmin creates ONE super admin per tenant from env vars, for production bootstrap.
// Unlike cmd/seed it creates no demo accounts and never uses a published password.
//
//	SEED_ADMIN_EMAIL=... SEED_ADMIN_PASSWORD=... [SEED_ADMIN_NAME=...] seedadmin
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	_ "time/tzdata"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kana-consultant/kantor/backend/internal/config"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	"github.com/kana-consultant/kantor/backend/internal/rbac"
	authrepo "github.com/kana-consultant/kantor/backend/internal/repository/auth"
	hrisrepo "github.com/kana-consultant/kantor/backend/internal/repository/hris"
	"github.com/kana-consultant/kantor/backend/internal/security"
	authservice "github.com/kana-consultant/kantor/backend/internal/service/auth"
	"github.com/kana-consultant/kantor/backend/internal/tenant"
)

func main() {
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	slog.SetDefault(slog.New(platformmiddleware.NewContextHandler(handler)))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		slog.Error("seedadmin failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	email := strings.TrimSpace(os.Getenv("SEED_ADMIN_EMAIL"))
	password := os.Getenv("SEED_ADMIN_PASSWORD")
	name := strings.TrimSpace(os.Getenv("SEED_ADMIN_NAME"))
	if name == "" {
		name = "Super Admin"
	}
	if email == "" || !strings.Contains(email, "@") {
		return errors.New("SEED_ADMIN_EMAIL is required (a real email address)")
	}
	if len(password) < 12 {
		return errors.New("SEED_ADMIN_PASSWORD is required (min 12 characters)")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping db: %w", err)
	}

	var previousKeys []string
	if cfg.DataEncryptionKeyPrevious != "" {
		previousKeys = append(previousKeys, cfg.DataEncryptionKeyPrevious)
	}
	encrypter, err := security.NewEncrypter(cfg.DataEncryptionKey, previousKeys...)
	if err != nil {
		return fmt.Errorf("encrypter: %w", err)
	}

	authService := authservice.New(
		authrepo.New(pool),
		hrisrepo.NewEmployeesRepository(pool, encrypter),
		cfg,
		rbac.NewPermissionCache(pool, 0),
		encrypter,
		nil,
	)

	done := 0
	if err := platformmiddleware.ForEachTenant(ctx, pool, func(tCtx context.Context, t tenant.Info) error {
		if err := authService.EnsureSeedSuperAdmin(tCtx, email, password, name); err != nil {
			return fmt.Errorf("seed super admin for %s: %w", t.Slug, err)
		}
		slog.InfoContext(tCtx, "super admin ensured", "tenant", t.Slug, "email", strings.ToLower(email))
		done++
		return nil
	}); err != nil {
		return err
	}
	if done == 0 {
		return errors.New("no tenants found — start the server once so TENANTS is seeded")
	}
	return nil
}
