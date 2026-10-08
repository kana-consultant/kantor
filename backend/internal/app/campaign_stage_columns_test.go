package app

import (
	"context"
	"errors"
	"testing"

	marketingrepo "github.com/kana-consultant/kantor/backend/internal/repository/marketing"
	"github.com/kana-consultant/kantor/backend/internal/tenant"
)

type stageEnsurerFunc func(ctx context.Context) (marketingrepo.EnsureStageColumnsResult, error)

func (f stageEnsurerFunc) EnsureStageColumns(ctx context.Context) (marketingrepo.EnsureStageColumnsResult, error) {
	return f(ctx)
}

// The boot-time stage guarantee is best effort: whatever the repository does
// (error, partial result with an error, panic), the call returns normally so
// the per-tenant seed loop, and with it the server start, goes on.
func TestEnsureCampaignStageColumnsNeverStopsStartup(t *testing.T) {
	info := tenant.Info{ID: "00000000-0000-0000-0000-000000000001", Slug: "default", Name: "Default"}
	cases := map[string]stageEnsurerFunc{
		"nothing to do": func(context.Context) (marketingrepo.EnsureStageColumnsResult, error) {
			return marketingrepo.EnsureStageColumnsResult{}, nil
		},
		"repaired": func(context.Context) (marketingrepo.EnsureStageColumnsResult, error) {
			return marketingrepo.EnsureStageColumnsResult{Adopted: []string{"live"}, Created: []string{"archived"}}, nil
		},
		"error": func(context.Context) (marketingrepo.EnsureStageColumnsResult, error) {
			return marketingrepo.EnsureStageColumnsResult{}, errors.New("permission denied for table campaign_columns")
		},
		"partial result with error": func(context.Context) (marketingrepo.EnsureStageColumnsResult, error) {
			return marketingrepo.EnsureStageColumnsResult{Created: []string{"live"}}, errors.New("append column for stage archived: duplicate key")
		},
		"panic": func(context.Context) (marketingrepo.EnsureStageColumnsResult, error) {
			panic("unexpected board")
		},
	}
	for name, ensurer := range cases {
		t.Run(name, func(t *testing.T) {
			called := false
			wrapped := stageEnsurerFunc(func(ctx context.Context) (marketingrepo.EnsureStageColumnsResult, error) {
				called = true
				return ensurer(ctx)
			})
			// Must return (no panic escapes, nothing to propagate).
			ensureCampaignStageColumns(context.Background(), wrapped, info)
			if !called {
				t.Error("the repository was not asked")
			}
		})
	}
}
