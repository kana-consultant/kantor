package app

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/kana-consultant/kantor/backend/internal/model"
	marketingrepo "github.com/kana-consultant/kantor/backend/internal/repository/marketing"
	"github.com/kana-consultant/kantor/backend/internal/tenant"
)

type fakeFinanceCategories struct {
	calls *[]string
	err   error
}

func (f fakeFinanceCategories) SeedDefaultCategories(context.Context) error {
	*f.calls = append(*f.calls, "finance")
	return f.err
}

type fakeWATemplates struct{ calls *[]string }

func (f fakeWATemplates) EnsureDefaultTemplates(context.Context) (model.WADefaultTemplatesSeedResult, error) {
	*f.calls = append(*f.calls, "wa")
	return model.WADefaultTemplatesSeedResult{}, nil
}

func newFakeSeeder(calls *[]string, financeErr error, stagesErr error) tenantDefaultsSeeder {
	return tenantDefaultsSeeder{
		seedRBAC: func(context.Context) error {
			*calls = append(*calls, "rbac")
			return nil
		},
		financeCategories: fakeFinanceCategories{calls: calls, err: financeErr},
		waTemplates:       fakeWATemplates{calls: calls},
		campaignStages: stageEnsurerFunc(func(context.Context) (marketingrepo.EnsureStageColumnsResult, error) {
			*calls = append(*calls, "campaign stages")
			return marketingrepo.EnsureStageColumnsResult{}, stagesErr
		}),
	}
}

// The per-tenant boot seed (what New hands to ForEachTenant) guarantees the
// campaign stage columns, last and without ever failing the boot.
func TestTenantDefaultsSeedEnsuresCampaignStageColumns(t *testing.T) {
	info := tenant.Info{ID: "00000000-0000-0000-0000-000000000002", Slug: "acme", Name: "Acme"}

	var calls []string
	if err := newFakeSeeder(&calls, nil, nil).seed(context.Background(), info); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if want := []string{"rbac", "finance", "wa", "campaign stages"}; !reflect.DeepEqual(calls, want) {
		t.Errorf("seed calls = %v, want %v", calls, want)
	}

	calls = nil
	if err := newFakeSeeder(&calls, nil, errors.New("permission denied")).seed(context.Background(), info); err != nil {
		t.Errorf("a failed stage guarantee stopped the boot: %v", err)
	}

	// The seeds that always were fatal still are, and stop before the rest.
	calls = nil
	if err := newFakeSeeder(&calls, errors.New("boom"), nil).seed(context.Background(), info); err == nil {
		t.Error("a failed finance seed was swallowed")
	}
	if want := []string{"rbac", "finance"}; !reflect.DeepEqual(calls, want) {
		t.Errorf("seed calls after a finance failure = %v, want %v", calls, want)
	}
}

// The seeder New builds is wired to the real campaigns repository.
func TestNewTenantDefaultsSeederWiresTheCampaignsRepository(t *testing.T) {
	seeder := newTenantDefaultsSeeder(nil)
	if _, ok := seeder.campaignStages.(*marketingrepo.CampaignsRepository); !ok {
		t.Errorf("campaignStages = %T, want *marketing.CampaignsRepository", seeder.campaignStages)
	}
	if seeder.seedRBAC == nil || seeder.financeCategories == nil || seeder.waTemplates == nil {
		t.Error("a seed is not wired")
	}
}
