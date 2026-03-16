package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/go-chi/chi/v5"

	"github.com/kana-consultant/kantor/backend/internal/config"
	authhandler "github.com/kana-consultant/kantor/backend/internal/handler/auth"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	"github.com/kana-consultant/kantor/backend/internal/rbac"
	authrepo "github.com/kana-consultant/kantor/backend/internal/repository/auth"
	"github.com/kana-consultant/kantor/backend/internal/response"
	authservice "github.com/kana-consultant/kantor/backend/internal/service/auth"
)

type App struct {
	cfg    config.Config
	db     *pgxpool.Pool
	router http.Handler
}

func New(ctx context.Context, cfg config.Config) (*App, error) {
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("create pgx pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	if err := runMigrations(cfg.DatabaseURL); err != nil {
		pool.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	if err := rbac.SeedDefaults(ctx, pool); err != nil {
		pool.Close()
		return nil, fmt.Errorf("seed rbac defaults: %w", err)
	}

	authRepository := authrepo.New(pool)
	authService := authservice.New(authRepository, cfg)

	if cfg.SeedSuperAdmin.Enabled {
		if err := authService.EnsureSeedSuperAdmin(
			ctx,
			cfg.SeedSuperAdmin.Email,
			cfg.SeedSuperAdmin.Password,
			cfg.SeedSuperAdmin.FullName,
		); err != nil {
			pool.Close()
			return nil, fmt.Errorf("seed initial super admin: %w", err)
		}
	}

	application := &App{
		cfg: cfg,
		db:  pool,
	}
	application.router = application.buildRouter(authService)

	return application, nil
}

func (a *App) Router() http.Handler {
	return a.router
}

func (a *App) DB() *pgxpool.Pool {
	return a.db
}

func (a *App) Close() {
	if a.db != nil {
		a.db.Close()
	}
}

func (a *App) buildRouter(authService *authservice.Service) http.Handler {
	router := chi.NewRouter()
	authHandler := authhandler.New(authService)

	router.Use(chimiddleware.RequestID)
	router.Use(chimiddleware.RealIP)
	router.Use(chimiddleware.Recoverer)
	router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   a.cfg.CORSOrigins,
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	router.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		response.WriteJSON(w, http.StatusOK, map[string]string{
			"status": "ok",
		}, nil)
	})

	router.Route("/api/v1", func(r chi.Router) {
		r.Route("/auth", authHandler.RegisterRoutes)
		r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
			response.WriteJSON(w, http.StatusOK, map[string]string{
				"status": "ok",
			}, nil)
		})

		r.Group(func(protected chi.Router) {
			protected.Use(platformmiddleware.AuthMiddleware(authService.ParseAccessToken))
			protected.Get("/auth/me", authHandler.Me)

			protected.Route("/operational", func(module chi.Router) {
				module.With(platformmiddleware.RBACMiddleware("operational:project:view")).Get("/overview", func(w http.ResponseWriter, r *http.Request) {
					response.WriteJSON(w, http.StatusOK, map[string]string{
						"module":  "operational",
						"message": "Operational overview is protected by RBAC middleware",
					}, nil)
				})
			})

			protected.Route("/hris", func(module chi.Router) {
				module.With(platformmiddleware.RBACMiddleware("hris:employee:view")).Get("/overview", func(w http.ResponseWriter, r *http.Request) {
					response.WriteJSON(w, http.StatusOK, map[string]string{
						"module":  "hris",
						"message": "HRIS overview is protected by RBAC middleware",
					}, nil)
				})
			})

			protected.Route("/marketing", func(module chi.Router) {
				module.With(platformmiddleware.RBACMiddleware("marketing:campaign:view")).Get("/overview", func(w http.ResponseWriter, r *http.Request) {
					response.WriteJSON(w, http.StatusOK, map[string]string{
						"module":  "marketing",
						"message": "Marketing overview is protected by RBAC middleware",
					}, nil)
				})
			})
		})
	})

	return router
}

func runMigrations(databaseURL string) error {
	migrationsPath, err := resolveMigrationsPath()
	if err != nil {
		return err
	}

	sqlDB, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("open database for migrations: %w", err)
	}
	defer sqlDB.Close()

	driver, err := postgres.WithInstance(sqlDB, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("create migration driver: %w", err)
	}

	instance, err := migrate.NewWithDatabaseInstance(
		"file://"+filepath.ToSlash(migrationsPath),
		"postgres",
		driver,
	)
	if err != nil {
		return fmt.Errorf("create migration instance: %w", err)
	}
	defer instance.Close()

	if err := instance.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}

	return nil
}

func resolveMigrationsPath() (string, error) {
	candidates := []string{
		"migrations",
		filepath.Join("backend", "migrations"),
		filepath.Join("..", "migrations"),
		filepath.Join("..", "..", "backend", "migrations"),
	}

	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			absolutePath, absErr := filepath.Abs(candidate)
			if absErr != nil {
				return "", fmt.Errorf("resolve migrations path: %w", absErr)
			}

			return absolutePath, nil
		}
	}

	return "", errors.New("migrations directory not found")
}
