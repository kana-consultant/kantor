// cmd/seed creates the demo super admin and demo users for every tenant
// configured in TENANTS. Intended for local development and demo setups —
// running it in production would expose accounts with publicly known
// passwords and is therefore a deliberate, explicit step.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	_ "time/tzdata"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kana-consultant/kantor/backend/internal/config"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	"github.com/kana-consultant/kantor/backend/internal/rbac"
	authrepo "github.com/kana-consultant/kantor/backend/internal/repository/auth"
	hrisrepo "github.com/kana-consultant/kantor/backend/internal/repository/hris"
	"github.com/kana-consultant/kantor/backend/internal/seed"
	"github.com/kana-consultant/kantor/backend/internal/security"
	authservice "github.com/kana-consultant/kantor/backend/internal/service/auth"
	"github.com/kana-consultant/kantor/backend/internal/tenant"
)

func main() {
	configureLogger()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		slog.Error("seed failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
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

	authRepository := authrepo.New(pool)
	employeesRepository := hrisrepo.NewEmployeesRepository(pool)
	permissionCache := rbac.NewPermissionCache(pool, 0)
	authService := authservice.New(authRepository, employeesRepository, cfg, permissionCache, encrypter, nil)

	tenantsSeeded := 0
	if err := platformmiddleware.ForEachTenant(ctx, pool, func(tCtx context.Context, t tenant.Info) error {
		slog.InfoContext(tCtx, "seeding demo accounts", "tenant", t.Slug)

		if err := authService.EnsureSeedSuperAdmin(
			tCtx,
			seed.DemoSuperAdmin.Email,
			seed.DemoSuperAdmin.Password,
			seed.DemoSuperAdmin.FullName,
		); err != nil {
			return fmt.Errorf("seed super admin for %s: %w", t.Slug, err)
		}

		for _, user := range seed.DemoUsers {
			department := user.Department
			params := authrepo.CreateUserParams{
				Email:      user.Email,
				FullName:   user.FullName,
				Department: stringPtr(department),
				Skills:     append([]string(nil), user.Skills...),
			}
			if err := authService.EnsureSeedUserWithRoles(tCtx, params, user.Roles, user.Password); err != nil {
				return fmt.Errorf("seed user %s for %s: %w", user.Email, t.Slug, err)
			}
		}

		tenantsSeeded++
		return nil
	}); err != nil {
		return err
	}

	if tenantsSeeded == 0 {
		return errors.New("no tenants found — run the server once or seed tenants via TENANTS env first")
	}

	slog.Info("seed completed", "tenants", tenantsSeeded, "users_per_tenant", len(seed.DemoUsers)+1)
	return nil
}

func configureLogger() {
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	slog.SetDefault(slog.New(platformmiddleware.NewContextHandler(handler)))
}

func stringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
