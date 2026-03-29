package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
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
	adminhandler "github.com/kana-consultant/kantor/backend/internal/handler/admin"
	authhandler "github.com/kana-consultant/kantor/backend/internal/handler/auth"
	fileshandler "github.com/kana-consultant/kantor/backend/internal/handler/files"
	hrishandler "github.com/kana-consultant/kantor/backend/internal/handler/hris"
	marketinghandler "github.com/kana-consultant/kantor/backend/internal/handler/marketing"
	notificationshandler "github.com/kana-consultant/kantor/backend/internal/handler/notifications"
	operationalhandler "github.com/kana-consultant/kantor/backend/internal/handler/operational"
	wahandler "github.com/kana-consultant/kantor/backend/internal/handler/whatsapp"
	platformmiddleware "github.com/kana-consultant/kantor/backend/internal/middleware"
	"github.com/kana-consultant/kantor/backend/internal/rbac"
	auditrepo "github.com/kana-consultant/kantor/backend/internal/repository/audit"
	authrepo "github.com/kana-consultant/kantor/backend/internal/repository/auth"
	hrisrepo "github.com/kana-consultant/kantor/backend/internal/repository/hris"
	marketingrepo "github.com/kana-consultant/kantor/backend/internal/repository/marketing"
	notificationsrepo "github.com/kana-consultant/kantor/backend/internal/repository/notifications"
	operationalrepo "github.com/kana-consultant/kantor/backend/internal/repository/operational"
	warepo "github.com/kana-consultant/kantor/backend/internal/repository/whatsapp"
	"github.com/kana-consultant/kantor/backend/internal/response"
	"github.com/kana-consultant/kantor/backend/internal/security"
	auditservice "github.com/kana-consultant/kantor/backend/internal/service/audit"
	authservice "github.com/kana-consultant/kantor/backend/internal/service/auth"
	filesservice "github.com/kana-consultant/kantor/backend/internal/service/files"
	hrisservice "github.com/kana-consultant/kantor/backend/internal/service/hris"
	marketingservice "github.com/kana-consultant/kantor/backend/internal/service/marketing"
	notificationsservice "github.com/kana-consultant/kantor/backend/internal/service/notifications"
	operationalservice "github.com/kana-consultant/kantor/backend/internal/service/operational"
	waservice "github.com/kana-consultant/kantor/backend/internal/service/whatsapp"
	"github.com/kana-consultant/kantor/backend/internal/tenant"
)

type App struct {
	cfg              config.Config
	db               *pgxpool.Pool
	router           http.Handler
	backgroundCancel context.CancelFunc
	permissionCache  *rbac.PermissionCache
	tenantResolver   *tenant.Resolver
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

	tenantResolver := tenant.NewResolver(pool)

	// Seed all tenants from env (TENANTS=name|slug|domains;...).
	// Runs as superuser — no RLS needed for global tables.
	if err := seedTenants(ctx, pool, cfg.Tenants); err != nil {
		pool.Close()
		return nil, fmt.Errorf("seed tenants: %w", err)
	}

	// Seed RBAC defaults and finance categories per-tenant.
	financeRepositoryForSeed := hrisrepo.NewFinanceRepository(pool)
	if err := platformmiddleware.ForEachTenant(ctx, pool, func(tCtx context.Context, t tenant.Info) error {
		if err := rbac.SeedDefaults(tCtx, pool); err != nil {
			return fmt.Errorf("seed rbac defaults for tenant %s: %w", t.Slug, err)
		}
		if err := financeRepositoryForSeed.SeedDefaultCategories(tCtx); err != nil {
			return fmt.Errorf("seed finance categories for tenant %s: %w", t.Slug, err)
		}
		return nil
	}); err != nil {
		pool.Close()
		return nil, fmt.Errorf("seed per-tenant defaults: %w", err)
	}

	auditRepository := auditrepo.NewRepository(pool)
	auditService := auditservice.NewService(auditRepository)

	authRepository := authrepo.New(pool)
	permissionCache := rbac.NewPermissionCache(pool, 5*time.Minute)
	employeesRepository := hrisrepo.NewEmployeesRepository(pool) // used by both auth & hris
	authService := authservice.New(authRepository, employeesRepository, cfg, permissionCache)

	// Seed super admin and demo users per-tenant.
	if err := platformmiddleware.ForEachTenant(ctx, pool, func(tCtx context.Context, t tenant.Info) error {
		if cfg.SeedSuperAdmin.Enabled {
			if err := authService.EnsureSeedSuperAdmin(
				tCtx,
				cfg.SeedSuperAdmin.Email,
				cfg.SeedSuperAdmin.Password,
				cfg.SeedSuperAdmin.FullName,
			); err != nil {
				return fmt.Errorf("seed super admin for tenant %s: %w", t.Slug, err)
			}
		}

		if cfg.SeedDemoUsers.Enabled {
			if err := authService.EnsureSeedUserWithRoles(tCtx, authrepo.CreateUserParams{
				Email:      cfg.SeedDemoUsers.Staff.Email,
				FullName:   cfg.SeedDemoUsers.Staff.FullName,
				Department: stringPointer(cfg.SeedDemoUsers.Staff.Department),
				Skills:     cfg.SeedDemoUsers.Staff.Skills,
			}, []rbac.RoleKey{{Name: "staff", Module: "operational"}}, cfg.SeedDemoUsers.Staff.Password); err != nil {
				return fmt.Errorf("seed staff user for tenant %s: %w", t.Slug, err)
			}

			if err := authService.EnsureSeedUserWithRoles(tCtx, authrepo.CreateUserParams{
				Email:      cfg.SeedDemoUsers.Viewer.Email,
				FullName:   cfg.SeedDemoUsers.Viewer.FullName,
				Department: stringPointer(cfg.SeedDemoUsers.Viewer.Department),
				Skills:     cfg.SeedDemoUsers.Viewer.Skills,
			}, []rbac.RoleKey{{Name: "viewer", Module: "operational"}}, cfg.SeedDemoUsers.Viewer.Password); err != nil {
				return fmt.Errorf("seed viewer user for tenant %s: %w", t.Slug, err)
			}

			if err := authService.EnsureSeedUserWithRoles(tCtx, authrepo.CreateUserParams{
				Email:      cfg.SeedDemoUsers.MarketingStaff.Email,
				FullName:   cfg.SeedDemoUsers.MarketingStaff.FullName,
				Department: stringPointer(cfg.SeedDemoUsers.MarketingStaff.Department),
				Skills:     cfg.SeedDemoUsers.MarketingStaff.Skills,
			}, []rbac.RoleKey{{Name: "staff", Module: "marketing"}}, cfg.SeedDemoUsers.MarketingStaff.Password); err != nil {
				return fmt.Errorf("seed marketing staff user for tenant %s: %w", t.Slug, err)
			}

			if err := authService.EnsureSeedUserWithRoles(tCtx, authrepo.CreateUserParams{
				Email:      cfg.SeedDemoUsers.MarketingViewer.Email,
				FullName:   cfg.SeedDemoUsers.MarketingViewer.FullName,
				Department: stringPointer(cfg.SeedDemoUsers.MarketingViewer.Department),
				Skills:     cfg.SeedDemoUsers.MarketingViewer.Skills,
			}, []rbac.RoleKey{{Name: "viewer", Module: "marketing"}}, cfg.SeedDemoUsers.MarketingViewer.Password); err != nil {
				return fmt.Errorf("seed marketing viewer user for tenant %s: %w", t.Slug, err)
			}
		}

		return nil
	}); err != nil {
		pool.Close()
		return nil, fmt.Errorf("seed users: %w", err)
	}

	projectsRepository := operationalrepo.NewProjectsRepository(pool)
	kanbanRepository := operationalrepo.NewKanbanRepository(pool)
	operationalOverviewRepository := operationalrepo.NewOverviewRepository(pool)
	trackerRepository := operationalrepo.NewTrackerRepository(pool)
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
	kanbanService := operationalservice.NewKanbanService(kanbanRepository, projectsRepository)
	operationalOverviewService := operationalservice.NewOverviewService(operationalOverviewRepository)
	trackerService := operationalservice.NewTrackerService(trackerRepository, cfg.TrackerRetentionDays)
	employeesService := hrisservice.NewEmployeesService(employeesRepository)
	employeesService.SetAuthRepo(authRepository)
	departmentsService := hrisservice.NewDepartmentsService(departmentsRepository, employeesRepository)
	compensationService := hrisservice.NewCompensationService(compensationRepository, employeesRepository, encrypter)
	financeService := hrisservice.NewFinanceService(financeRepository)
	notificationsService := notificationsservice.New(notificationsRepository)
	reimbursementsService := hrisservice.NewReimbursementsService(reimbursementsRepository, employeesRepository, authRepository, notificationsService, financeService)
	subscriptionsService := hrisservice.NewSubscriptionsService(subscriptionsRepository, employeesRepository, encrypter, financeService)
	hrisOverviewService := hrisservice.NewOverviewService(hrisOverviewRepository, employeesRepository)
	campaignsService := marketingservice.NewCampaignsService(campaignsRepository, authRepository, notificationsService)
	adsMetricsService := marketingservice.NewAdsMetricsService(adsMetricsRepository)
	leadsService := marketingservice.NewLeadsService(leadsRepository, authRepository, notificationsService)
	marketingOverviewService := marketingservice.NewOverviewService(marketingOverviewRepository)
	filesService := filesservice.New(cfg.UploadsDir, reimbursementsRepository, campaignsRepository, employeesRepository)

	// WhatsApp Broadcast (per-tenant: client loaded from DB config on demand)
	waRepository := warepo.New(pool)
	whatsappService := waservice.NewService(waRepository, cfg, notificationsService)

	// Wire event triggers
	kanbanService.SetTaskAssignNotifier(whatsappService)
	reimbursementsService.SetWANotifier(whatsappService)

	application := &App{cfg: cfg, db: pool, permissionCache: permissionCache, tenantResolver: tenantResolver}
	application.router = application.buildRouter(
		auditService,
		authService,
		adminhandler.NewAuditLogsHandler(auditService),
		operationalhandler.NewOverviewHandler(operationalOverviewService),
		operationalhandler.NewProjectsHandler(projectsService, kanbanService, projectsRepository, authRepository),
		operationalhandler.NewKanbanHandler(kanbanService),
		operationalhandler.NewTrackerHandler(trackerService),
		hrishandler.NewOverviewHandler(hrisOverviewService),
		hrishandler.NewEmployeesHandler(employeesService, compensationService, cfg.UploadsDir, authRepository),
		hrishandler.NewDepartmentsHandler(departmentsService),
		hrishandler.NewCompensationHandler(compensationService),
		hrishandler.NewFinanceHandler(financeService, authRepository),
		hrishandler.NewReimbursementsHandler(reimbursementsService, cfg.UploadsDir, authRepository),
		hrishandler.NewSubscriptionsHandler(subscriptionsService, authRepository),
		marketinghandler.NewOverviewHandler(marketingOverviewService),
		marketinghandler.NewCampaignsHandler(campaignsService, cfg.UploadsDir, authRepository),
		marketinghandler.NewAdsMetricsHandler(adsMetricsService, authRepository),
		marketinghandler.NewLeadsHandler(leadsService, authRepository),
		notificationshandler.New(notificationsService),
		fileshandler.New(filesService),
		wahandler.New(whatsappService),
	)
	application.startBackgroundJobs(subscriptionsService, trackerService, whatsappService)

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
	auditService *auditservice.Service,
	authService *authservice.Service,
	auditLogsHandler *adminhandler.AuditLogsHandler,
	operationalOverviewHandler *operationalhandler.OverviewHandler,
	projectsHandler *operationalhandler.ProjectsHandler,
	kanbanHandler *operationalhandler.KanbanHandler,
	trackerHandler *operationalhandler.TrackerHandler,
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
	waHandler *wahandler.Handler,
) http.Handler {
	router := chi.NewRouter()
	authHandler := authhandler.New(authService, a.cfg)

	router.Use(chimiddleware.RequestID)
	router.Use(chimiddleware.RealIP)
	router.Use(chimiddleware.Recoverer)
	router.Use(platformmiddleware.AuditMiddleware(auditService))
	router.Use(platformmiddleware.MaxBodySize(1 << 20)) // 1 MB default for JSON endpoints
	router.Use(platformmiddleware.LoggingMiddleware)
	router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   a.cfg.CORSOrigins,
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Health checks are outside tenant middleware — no Host header required.
	router.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		response.WriteJSON(w, http.StatusOK, map[string]string{
			"status": "ok",
		}, nil)
	})
	router.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := a.db.Ping(ctx); err != nil {
			response.WriteError(w, http.StatusServiceUnavailable, "DB_UNHEALTHY", "Database is not reachable", nil)
			return
		}
		response.WriteJSON(w, http.StatusOK, map[string]string{
			"status": "ok",
		}, nil)
	})

	// All API routes require tenant resolution from the Host header.
	router.Group(func(tenanted chi.Router) {
		tenanted.Use(platformmiddleware.TenantMiddleware(a.db, a.tenantResolver))
		tenanted.Route("/api/v1", func(r chi.Router) {
			r.Route("/auth", authHandler.RegisterRoutes)
			r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
				response.WriteJSON(w, http.StatusOK, map[string]string{
					"status": "ok",
				}, nil)
			})

			r.Group(func(protected chi.Router) {
				protected.Use(platformmiddleware.AuthMiddleware(authService.ParseAccessToken, a.permissionCache.Load))
				protected.Get("/auth/me", authHandler.Me)
				protected.Get("/auth/profile", authHandler.GetProfile)
				protected.Put("/auth/profile", authHandler.UpdateProfile)
				protected.Put("/auth/profile/email", authHandler.ChangeEmail)
				protected.Post("/auth/profile/avatar", authHandler.UploadProfileAvatar)
				protected.Post("/auth/change-password", authHandler.ChangePassword)
				protected.Get("/files/{type}/{id}/{filename}", filesHandler.Serve)
				protected.With(platformmiddleware.RequireAnyPermission(
					"admin:roles:view",
					"admin:users:view",
					"admin:settings:view",
				)).Get("/modules", authHandler.ListModules)
				protected.Route("/notifications", notificationsHandler.RegisterRoutes)

				protected.Route("/admin", func(admin chi.Router) {
					admin.Use(platformmiddleware.RequireModuleAccess(rbac.ModuleAdmin))
					admin.Route("/audit-logs", auditLogsHandler.RegisterRoutes)
					admin.With(platformmiddleware.RequireAnyPermission(
						"admin:roles:view",
						"admin:users:view",
						"admin:settings:view",
					)).Get("/roles", authHandler.ListRoles)
					admin.With(platformmiddleware.RequireAnyPermission(
						"admin:roles:view",
						"admin:users:view",
					)).Get("/roles/{roleID}", authHandler.GetRole)
					admin.With(platformmiddleware.RequirePermission("admin:roles:manage")).Post("/roles", authHandler.CreateRole)
					admin.With(platformmiddleware.RequirePermission("admin:roles:manage")).Put("/roles/{roleID}", authHandler.UpdateRole)
					admin.With(platformmiddleware.RequirePermission("admin:roles:manage")).Delete("/roles/{roleID}", authHandler.DeleteRole)
					admin.With(platformmiddleware.RequirePermission("admin:roles:manage")).Patch("/roles/{roleID}/toggle", authHandler.ToggleRole)
					admin.With(platformmiddleware.RequirePermission("admin:roles:manage")).Post("/roles/{roleID}/duplicate", authHandler.DuplicateRole)
					admin.With(platformmiddleware.RequirePermission("admin:roles:view")).Get("/permissions", authHandler.ListPermissions)

					admin.With(platformmiddleware.RequirePermission("admin:users:view")).Get("/users", authHandler.ListUsers)
					admin.With(platformmiddleware.RequirePermission("admin:users:view")).Get("/users/{userID}", authHandler.GetUser)
					admin.With(platformmiddleware.RequirePermission("admin:users:manage")).Put("/users/{userID}/roles", authHandler.UpdateUserRoles)
					admin.With(platformmiddleware.RequirePermission("admin:users:manage")).Patch("/users/{userID}/active", authHandler.ToggleUserActive)
					admin.With(platformmiddleware.RequirePermission("admin:users:manage")).Post("/users/{userID}/ensure-employee-profile", authHandler.EnsureUserEmployeeProfile)
					admin.With(platformmiddleware.SuperAdminMiddleware()).Post("/users/{userID}/toggle-super-admin", authHandler.ToggleUserSuperAdmin)

					admin.With(platformmiddleware.RequirePermission("admin:settings:view")).Get("/settings", authHandler.GetSettings)
					admin.With(platformmiddleware.RequirePermission("admin:settings:view")).Get("/settings/departments", authHandler.ListSettingsDepartments)
					admin.With(platformmiddleware.RequirePermission("admin:settings:manage")).Put("/settings/default-roles", authHandler.UpdateDefaultRoles)
					admin.With(platformmiddleware.RequirePermission("admin:settings:manage")).Put("/settings/auto-create-employee", authHandler.UpdateAutoCreateEmployee)
				})

				protected.Route("/operational", func(module chi.Router) {
					module.Use(platformmiddleware.RequireModuleAccess(rbac.ModuleOperational))
					module.With(platformmiddleware.RequirePermission("operational:project:view")).Get("/overview", operationalOverviewHandler.Get)

					module.Route("/projects", projectsHandler.RegisterRoutes)
					module.Route("/projects/{projectID}/columns", kanbanHandler.RegisterColumnRoutes)
					module.Route("/projects/{projectID}/tasks", kanbanHandler.RegisterTaskRoutes)
				})

				protected.Route("/tracker", func(tracker chi.Router) {
					tracker.Use(platformmiddleware.RequireModuleAccess(rbac.ModuleOperational))
					trackerHandler.RegisterRoutes(tracker)
				})

				protected.Route("/hris", func(module chi.Router) {
					module.Use(platformmiddleware.RequireModuleAccess(rbac.ModuleHRIS))
					module.With(platformmiddleware.RequirePermission("hris:employee:view")).Get("/overview", hrisOverviewHandler.Get)

					module.Route("/employees", employeesHandler.RegisterRoutes)
					module.Route("/departments", departmentsHandler.RegisterRoutes)
					module.Route("/employees/{employeeID}/salaries", compensationHandler.RegisterSalaryRoutes)
					module.Route("/employees/{employeeID}/bonuses", compensationHandler.RegisterBonusRoutes)
					module.With(platformmiddleware.RequirePermission("hris:bonus:edit")).Put("/bonuses/{bonusID}", compensationHandler.UpdateBonus)
					module.With(platformmiddleware.RequirePermission("hris:bonus:delete")).Delete("/bonuses/{bonusID}", compensationHandler.DeleteBonus)
					module.With(platformmiddleware.RequirePermission("hris:bonus:approve")).Patch("/bonuses/{bonusID}/approve", compensationHandler.ApproveBonus)
					module.With(platformmiddleware.RequirePermission("hris:bonus:approve")).Patch("/bonuses/{bonusID}/reject", compensationHandler.RejectBonus)
					module.Route("/finance", financeHandler.RegisterRoutes)
					module.Route("/reimbursements", reimbursementsHandler.RegisterRoutes)
					module.Route("/subscriptions", subscriptionsHandler.RegisterRoutes)
				})

				protected.Route("/marketing", func(module chi.Router) {
					module.Use(platformmiddleware.RequireModuleAccess(rbac.ModuleMarketing))
					module.With(platformmiddleware.RequirePermission("marketing:campaign:view")).Get("/overview", marketingOverviewHandler.Get)

					module.Route("/campaigns", campaignsHandler.RegisterRoutes)
					module.Route("/ads-metrics", adsMetricsHandler.RegisterRoutes)
					module.Route("/leads", leadsHandler.RegisterRoutes)
					module.Route("/columns", campaignsHandler.RegisterColumnRoutes)
				})

				protected.Route("/wa", func(wa chi.Router) {
					wa.Use(platformmiddleware.RequireModuleAccess(rbac.ModuleOperational))
					waHandler.RegisterRoutes(wa)
				})
			})
		})
	})

	return router
}

func (a *App) startBackgroundJobs(subscriptionsService *hrisservice.SubscriptionsService, trackerService *operationalservice.TrackerService, whatsappService *waservice.Service) {
	ctx, cancel := context.WithCancel(context.Background())
	a.backgroundCancel = cancel

	runPerTenant := func(name string, fn func(ctx context.Context, t tenant.Info) error) {
		if err := platformmiddleware.ForEachTenant(ctx, a.db, fn); err != nil {
			slog.Error("per-tenant background job failed", "job", name, "error", err)
		}
	}

	runBackground := func(name string, fn func()) {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("background job panicked", "job", name, "panic", r, "stack", string(debug.Stack()))
				}
			}()
			fn()
		}()
	}

	runBackground("background_scheduler", func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("background job panicked", "job", "background_scheduler", "panic", r, "stack", string(debug.Stack()))
			}
		}()

		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		runPerTenant("subscription_alerts", func(tCtx context.Context, t tenant.Info) error {
			return subscriptionsService.GenerateSubscriptionAlerts(tCtx, time.Now())
		})
		runPerTenant("tracker_retention", func(tCtx context.Context, t tenant.Info) error {
			_, err := trackerService.PurgeOldData(tCtx, time.Now())
			return err
		})

		for {
			select {
			case <-ctx.Done():
				return
			case tickAt := <-ticker.C:
				runPerTenant("subscription_alerts", func(tCtx context.Context, t tenant.Info) error {
					return subscriptionsService.GenerateSubscriptionAlerts(tCtx, tickAt)
				})
				runPerTenant("tracker_retention", func(tCtx context.Context, t tenant.Info) error {
					_, err := trackerService.PurgeOldData(tCtx, tickAt)
					return err
				})
			}
		}
	})

	runBackground("wa_scheduler", func() {
		runPerTenant("wa_scheduler", func(tCtx context.Context, t tenant.Info) error {
			return whatsappService.RunCronJobs(tCtx, time.Now())
		})

		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case tickAt := <-ticker.C:
				runPerTenant("wa_scheduler", func(tCtx context.Context, t tenant.Info) error {
					return whatsappService.RunCronJobs(tCtx, tickAt)
				})
			}
		}
	})
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

const defaultTenantID = "00000000-0000-0000-0000-000000000001"

// seedTenants ensures every tenant defined in the TENANTS env var exists in the
// database with the correct name, slug, domains, and a WA config row.
// The first tenant reuses the well-known UUID created by the migration
// (existing data is backfilled to it). Additional tenants are upserted by slug.
// Runs as superuser — no RLS.
func seedTenants(ctx context.Context, pool *pgxpool.Pool, tenants []config.TenantConfig) error {
	for i, tc := range tenants {
		var tenantID string

		if i == 0 {
			// First tenant: update the migration placeholder.
			tenantID = defaultTenantID
			_, err := pool.Exec(ctx,
				`UPDATE tenants SET name = $1, slug = $2, updated_at = NOW()
				 WHERE id = $3`,
				tc.Name, tc.Slug, tenantID)
			if err != nil {
				return fmt.Errorf("update default tenant: %w", err)
			}
		} else {
			// Additional tenants: insert or update by slug.
			err := pool.QueryRow(ctx,
				`INSERT INTO tenants (name, slug)
				 VALUES ($1, $2)
				 ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name, updated_at = NOW()
				 RETURNING id::text`,
				tc.Name, tc.Slug).Scan(&tenantID)
			if err != nil {
				return fmt.Errorf("upsert tenant %q: %w", tc.Slug, err)
			}
		}

		// Upsert domains (first = primary).
		for j, domain := range tc.Domains {
			domain = strings.TrimSpace(domain)
			if domain == "" {
				continue
			}
			_, err := pool.Exec(ctx,
				`INSERT INTO tenant_domains (tenant_id, domain, is_primary)
				 VALUES ($1, $2, $3)
				 ON CONFLICT (domain) DO UPDATE
				   SET is_primary = EXCLUDED.is_primary,
				       tenant_id  = EXCLUDED.tenant_id`,
				tenantID, domain, j == 0)
			if err != nil {
				return fmt.Errorf("upsert domain %q for tenant %q: %w", domain, tc.Slug, err)
			}
		}

		// Ensure WA config row exists (disabled by default, admin enables via UI).
		// tenant_wa_configs has RLS, so we must SET the GUC on a dedicated connection.
		waConn, err := pool.Acquire(ctx)
		if err != nil {
			return fmt.Errorf("acquire conn for wa config seed: %w", err)
		}
		_, err = waConn.Exec(ctx, fmt.Sprintf("SET app.current_tenant = '%s'", tenantID))
		if err != nil {
			waConn.Release()
			return fmt.Errorf("set tenant guc for wa config seed: %w", err)
		}
		_, err = waConn.Exec(ctx,
			`INSERT INTO tenant_wa_configs (tenant_id)
			 VALUES ($1::uuid)
			 ON CONFLICT (tenant_id) DO NOTHING`,
			tenantID)
		_, _ = waConn.Exec(ctx, "RESET ALL")
		waConn.Release()
		if err != nil {
			return fmt.Errorf("seed wa config for tenant %q: %w", tc.Slug, err)
		}

		slog.Info("tenant seeded", "name", tc.Name, "slug", tc.Slug, "domains", tc.Domains)
	}
	return nil
}
