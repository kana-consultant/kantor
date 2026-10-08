package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kana-consultant/kantor/backend/internal/model"
	"github.com/kana-consultant/kantor/backend/internal/rbac"
	hrisrepo "github.com/kana-consultant/kantor/backend/internal/repository/hris"
	marketingrepo "github.com/kana-consultant/kantor/backend/internal/repository/marketing"
	warepo "github.com/kana-consultant/kantor/backend/internal/repository/whatsapp"
	"github.com/kana-consultant/kantor/backend/internal/tenant"
)

type financeCategorySeeder interface {
	SeedDefaultCategories(ctx context.Context) error
}

type waTemplateSeeder interface {
	EnsureDefaultTemplates(ctx context.Context) (model.WADefaultTemplatesSeedResult, error)
}

// tenantDefaultsSeeder writes the defaults every tenant needs, once per tenant
// at boot (seed is the ForEachTenant callback in New).
type tenantDefaultsSeeder struct {
	seedRBAC          func(ctx context.Context) error
	financeCategories financeCategorySeeder
	waTemplates       waTemplateSeeder
	campaignStages    campaignStageColumnsEnsurer
}

func newTenantDefaultsSeeder(pool *pgxpool.Pool) tenantDefaultsSeeder {
	return tenantDefaultsSeeder{
		seedRBAC:          func(ctx context.Context) error { return rbac.SeedDefaults(ctx, pool) },
		financeCategories: hrisrepo.NewFinanceRepository(pool),
		waTemplates:       warepo.New(pool),
		campaignStages:    marketingrepo.NewCampaignsRepository(pool),
	}
}

// seed runs the tenant's seeds. An RBAC, finance or WA template failure stops
// the server from starting (as it always has); the campaign stage guarantee
// is best effort and never does.
func (s tenantDefaultsSeeder) seed(ctx context.Context, t tenant.Info) error {
	if err := s.seedRBAC(ctx); err != nil {
		return fmt.Errorf("seed rbac defaults for tenant %s: %w", t.Slug, err)
	}
	if err := s.financeCategories.SeedDefaultCategories(ctx); err != nil {
		return fmt.Errorf("seed finance categories for tenant %s: %w", t.Slug, err)
	}
	if result, err := s.waTemplates.EnsureDefaultTemplates(ctx); err != nil {
		return fmt.Errorf("seed wa templates for tenant %s: %w", t.Slug, err)
	} else if result.InsertedCount > 0 {
		slog.InfoContext(ctx, "seeded wa templates", "tenant", t.Slug, "inserted", result.InsertedCount, "slugs", result.InsertedSlugs)
	}
	// Last, so that whatever happens to it cannot affect the seeds above.
	ensureCampaignStageColumns(ctx, s.campaignStages, t)
	return nil
}
