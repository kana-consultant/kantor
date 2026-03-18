package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	fileshandler "github.com/kana-consultant/kantor/backend/internal/handler/files"
	hrishandler "github.com/kana-consultant/kantor/backend/internal/handler/hris"
	marketinghandler "github.com/kana-consultant/kantor/backend/internal/handler/marketing"
	notificationshandler "github.com/kana-consultant/kantor/backend/internal/handler/notifications"
	operationalhandler "github.com/kana-consultant/kantor/backend/internal/handler/operational"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	"github.com/kana-consultant/kantor/backend/internal/rbac"
	authrepo "github.com/kana-consultant/kantor/backend/internal/repository/auth"
	hrisrepo "github.com/kana-consultant/kantor/backend/internal/repository/hris"
	marketingrepo "github.com/kana-consultant/kantor/backend/internal/repository/marketing"
	notificationsrepo "github.com/kana-consultant/kantor/backend/internal/repository/notifications"
	operationalrepo "github.com/kana-consultant/kantor/backend/internal/repository/operational"
	"github.com/kana-consultant/kantor/backend/internal/response"
	"github.com/kana-consultant/kantor/backend/internal/security"
	authservice "github.com/kana-consultant/kantor/backend/internal/service/auth"
	filesservice "github.com/kana-consultant/kantor/backend/internal/service/files"
	hrisservice "github.com/kana-consultant/kantor/backend/internal/service/hris"
	marketingservice "github.com/kana-consultant/kantor/backend/internal/service/marketing"
	notificationsservice "github.com/kana-consultant/kantor/backend/internal/service/notifications"
	operationalservice "github.com/kana-consultant/kantor/backend/internal/service/operational"
)

type App struct {
	cfg              config.Config
	db               *pgxpool.Pool
	router           http.Handler
	backgroundCancel context.CancelFunc
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

	if err := ensureRuntimeDirectories(cfg); err != nil {
		pool.Close()
		return nil, fmt.Errorf("prepare runtime directories: %w", err)
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

	if cfg.SeedDemoUsers.Enabled {
		if err := authService.EnsureSeedUserWithRoles(ctx, authrepo.CreateUserParams{
			Email:      cfg.SeedDemoUsers.Staff.Email,
			FullName:   cfg.SeedDemoUsers.Staff.FullName,
			Department: stringPointer(cfg.SeedDemoUsers.Staff.Department),
			Skills:     cfg.SeedDemoUsers.Staff.Skills,
		}, []rbac.RoleKey{{Name: "staff", Module: "operational"}}, cfg.SeedDemoUsers.Staff.Password); err != nil {
			pool.Close()
			return nil, fmt.Errorf("seed staff demo user: %w", err)
		}

		if err := authService.EnsureSeedUserWithRoles(ctx, authrepo.CreateUserParams{
			Email:      cfg.SeedDemoUsers.Viewer.Email,
			FullName:   cfg.SeedDemoUsers.Viewer.FullName,
			Department: stringPointer(cfg.SeedDemoUsers.Viewer.Department),
			Skills:     cfg.SeedDemoUsers.Viewer.Skills,
		}, []rbac.RoleKey{{Name: "viewer", Module: "operational"}}, cfg.SeedDemoUsers.Viewer.Password); err != nil {
			pool.Close()
			return nil, fmt.Errorf("seed viewer demo user: %w", err)
		}

		if err := authService.EnsureSeedUserWithRoles(ctx, authrepo.CreateUserParams{
			Email:      cfg.SeedDemoUsers.MarketingStaff.Email,
			FullName:   cfg.SeedDemoUsers.MarketingStaff.FullName,
			Department: stringPointer(cfg.SeedDemoUsers.MarketingStaff.Department),
			Skills:     cfg.SeedDemoUsers.MarketingStaff.Skills,
		}, []rbac.RoleKey{{Name: "staff", Module: "marketing"}}, cfg.SeedDemoUsers.MarketingStaff.Password); err != nil {
			pool.Close()
			return nil, fmt.Errorf("seed marketing staff demo user: %w", err)
		}

		if err := authService.EnsureSeedUserWithRoles(ctx, authrepo.CreateUserParams{
			Email:      cfg.SeedDemoUsers.MarketingViewer.Email,
			FullName:   cfg.SeedDemoUsers.MarketingViewer.FullName,
			Department: stringPointer(cfg.SeedDemoUsers.MarketingViewer.Department),
			Skills:     cfg.SeedDemoUsers.MarketingViewer.Skills,
		}, []rbac.RoleKey{{Name: "viewer", Module: "marketing"}}, cfg.SeedDemoUsers.MarketingViewer.Password); err != nil {
			pool.Close()
			return nil, fmt.Errorf("seed marketing viewer demo user: %w", err)
		}
	}

	projectsRepository := operationalrepo.NewProjectsRepository(pool)
	kanbanRepository := operationalrepo.NewKanbanRepository(pool)
	assignmentRulesRepository := operationalrepo.NewAssignmentRulesRepository(pool)
	operationalOverviewRepository := operationalrepo.NewOverviewRepository(pool)
	employeesRepository := hrisrepo.NewEmployeesRepository(pool)
	departmentsRepository := hrisrepo.NewDepartmentsRepository(pool)
	compensationRepository := hrisrepo.NewCompensationRepository(pool)
	financeRepository := hrisrepo.NewFinanceRepository(pool)
	reimbursementsRepository := hrisrepo.NewReimbursementsRepository(pool)
	subscriptionsRepository := hrisrepo.NewSubscriptionsRepository(pool)
	hrisOverviewRepository := hrisrepo.NewOverviewRepository(pool)
	campaignsRepository := marketingrepo.NewCampaignsRepository(pool)
	adsMetricsRepository := marketingrepo.NewAdsMetricsRepository(pool)
	leadsRepository := marketingrepo.NewLeadsRepository(pool)
	marketingOverviewRepository := marketingrepo.NewOverviewRepository(pool)
	notificationsRepository := notificationsrepo.New(pool)
	var previousKeys []string
	if cfg.DataEncryptionKeyPrevious != "" {
		previousKeys = append(previousKeys, cfg.DataEncryptionKeyPrevious)
	}
	encrypter, err := security.NewEncrypter(cfg.DataEncryptionKey, previousKeys...)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("configure data encryption: %w", err)
	}

	projectsService := operationalservice.NewProjectsService(projectsRepository, kanbanRepository)
	kanbanService := operationalservice.NewKanbanService(kanbanRepository)
	assignmentRulesService := operationalservice.NewAssignmentRulesService(assignmentRulesRepository)
	operationalOverviewService := operationalservice.NewOverviewService(operationalOverviewRepository)
	employeesService := hrisservice.NewEmployeesService(employeesRepository)
	departmentsService := hrisservice.NewDepartmentsService(departmentsRepository, employeesRepository)
	compensationService := hrisservice.NewCompensationService(compensationRepository, employeesRepository, encrypter)
	financeService := hrisservice.NewFinanceService(financeRepository)
	notificationsService := notificationsservice.New(notificationsRepository)
	reimbursementsService := hrisservice.NewReimbursementsService(reimbursementsRepository, employeesRepository, authRepository, notificationsService)
	subscriptionsService := hrisservice.NewSubscriptionsService(subscriptionsRepository, employeesRepository, encrypter)
	hrisOverviewService := hrisservice.NewOverviewService(hrisOverviewRepository)
	campaignsService := marketingservice.NewCampaignsService(campaignsRepository, authRepository, notificationsService)
	adsMetricsService := marketingservice.NewAdsMetricsService(adsMetricsRepository)
	leadsService := marketingservice.NewLeadsService(leadsRepository, authRepository, notificationsService)
	marketingOverviewService := marketingservice.NewOverviewService(marketingOverviewRepository)
	filesService := filesservice.New(cfg.UploadsDir, reimbursementsRepository, campaignsRepository)

	application := &App{cfg: cfg, db: pool}
	application.router = application.buildRouter(
		authService,
		operationalhandler.NewOverviewHandler(operationalOverviewService),
		operationalhandler.NewProjectsHandler(projectsService),
		operationalhandler.NewKanbanHandler(kanbanService),
		operationalhandler.NewAssignmentRulesHandler(assignmentRulesService),
		hrishandler.NewOverviewHandler(hrisOverviewService),
		hrishandler.NewEmployeesHandler(employeesService),
		hrishandler.NewDepartmentsHandler(departmentsService),
		hrishandler.NewCompensationHandler(compensationService),
		hrishandler.NewFinanceHandler(financeService),
		hrishandler.NewReimbursementsHandler(reimbursementsService, cfg.UploadsDir),
		hrishandler.NewSubscriptionsHandler(subscriptionsService),
		marketinghandler.NewOverviewHandler(marketingOverviewService),
		marketinghandler.NewCampaignsHandler(campaignsService, cfg.UploadsDir),
		marketinghandler.NewAdsMetricsHandler(adsMetricsService),
		marketinghandler.NewLeadsHandler(leadsService),
		notificationshandler.New(notificationsService),
		fileshandler.New(filesService),
	)
	application.startBackgroundJobs(subscriptionsService)

	return application, nil
}

func (a *App) Router() http.Handler {
	return a.router
}

func (a *App) DB() *pgxpool.Pool {
	return a.db
}

func (a *App) Close() {
	if a.backgroundCancel != nil {
		a.backgroundCancel()
	}
	if a.db != nil {
		a.db.Close()
	}
}

func (a *App) buildRouter(
	authService *authservice.Service,
	operationalOverviewHandler *operationalhandler.OverviewHandler,
	projectsHandler *operationalhandler.ProjectsHandler,
	kanbanHandler *operationalhandler.KanbanHandler,
	assignmentRulesHandler *operationalhandler.AssignmentRulesHandler,
	hrisOverviewHandler *hrishandler.OverviewHandler,
	employeesHandler *hrishandler.EmployeesHandler,
	departmentsHandler *hrishandler.DepartmentsHandler,
	compensationHandler *hrishandler.CompensationHandler,
	financeHandler *hrishandler.FinanceHandler,
	reimbursementsHandler *hrishandler.ReimbursementsHandler,
	subscriptionsHandler *hrishandler.SubscriptionsHandler,
	marketingOverviewHandler *marketinghandler.OverviewHandler,
	campaignsHandler *marketinghandler.CampaignsHandler,
	adsMetricsHandler *marketinghandler.AdsMetricsHandler,
	leadsHandler *marketinghandler.LeadsHandler,
	notificationsHandler *notificationshandler.Handler,
	filesHandler *fileshandler.Handler,
) http.Handler {
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
			protected.Get("/files/{type}/{id}/{filename}", filesHandler.Serve)
			protected.Route("/notifications", notificationsHandler.RegisterRoutes)

			protected.Route("/operational", func(module chi.Router) {
				module.With(platformmiddleware.RBACMiddleware("operational:project:view")).Get("/overview", operationalOverviewHandler.Get)

				module.Route("/projects", projectsHandler.RegisterRoutes)
				module.Route("/projects/{projectID}/columns", kanbanHandler.RegisterColumnRoutes)
				module.Route("/projects/{projectID}/tasks", kanbanHandler.RegisterTaskRoutes)
				module.Route("/projects/{projectID}/assignment-rules", assignmentRulesHandler.RegisterRuleRoutes)
				module.With(platformmiddleware.RBACMiddleware("operational:assignment:edit")).Post("/projects/{projectID}/tasks/{taskID}/auto-assign", assignmentRulesHandler.AutoAssignTask)
			})

			protected.Route("/hris", func(module chi.Router) {
				module.With(platformmiddleware.RBACMiddleware("hris:employee:view")).Get("/overview", hrisOverviewHandler.Get)

				module.Route("/employees", employeesHandler.RegisterRoutes)
				module.Route("/departments", departmentsHandler.RegisterRoutes)
				module.Route("/employees/{employeeID}/salaries", compensationHandler.RegisterSalaryRoutes)
				module.Route("/employees/{employeeID}/bonuses", compensationHandler.RegisterBonusRoutes)
				module.With(platformmiddleware.RBACMiddleware("hris:bonus:edit")).Put("/bonuses/{bonusID}", compensationHandler.UpdateBonus)
				module.With(platformmiddleware.RBACMiddleware("hris:bonus:edit")).Delete("/bonuses/{bonusID}", compensationHandler.DeleteBonus)
				module.With(platformmiddleware.RBACMiddleware("hris:bonus:approve")).Patch("/bonuses/{bonusID}/approve", compensationHandler.ApproveBonus)
				module.With(platformmiddleware.RBACMiddleware("hris:bonus:approve")).Patch("/bonuses/{bonusID}/reject", compensationHandler.RejectBonus)
				module.Route("/finance", financeHandler.RegisterRoutes)
				module.Route("/reimbursements", reimbursementsHandler.RegisterRoutes)
				module.Route("/subscriptions", subscriptionsHandler.RegisterRoutes)
			})

			protected.Route("/marketing", func(module chi.Router) {
				module.With(platformmiddleware.RBACMiddleware("marketing:campaign:view")).Get("/overview", marketingOverviewHandler.Get)

				module.Route("/campaigns", campaignsHandler.RegisterRoutes)
				module.Route("/ads-metrics", adsMetricsHandler.RegisterRoutes)
				module.Route("/leads", leadsHandler.RegisterRoutes)
				module.Route("/columns", campaignsHandler.RegisterColumnRoutes)
			})
		})
	})

	return router
}

func (a *App) startBackgroundJobs(subscriptionsService *hrisservice.SubscriptionsService) {
	ctx, cancel := context.WithCancel(context.Background())
	a.backgroundCancel = cancel

	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		_ = subscriptionsService.GenerateSubscriptionAlerts(ctx, time.Now())
		for {
			select {
			case <-ctx.Done():
				return
			case tickAt := <-ticker.C:
				_ = subscriptionsService.GenerateSubscriptionAlerts(ctx, tickAt)
			}
		}
	}()
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

func ensureRuntimeDirectories(cfg config.Config) error {
	if err := os.MkdirAll(cfg.UploadsDir, 0o755); err != nil {
		return fmt.Errorf("create uploads dir: %w", err)
	}

	return nil
}

func stringPointer(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
