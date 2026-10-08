package app

import (
	"context"
	"fmt"
	"log/slog"

	marketingrepo "github.com/kana-consultant/kantor/backend/internal/repository/marketing"
	"github.com/kana-consultant/kantor/backend/internal/tenant"
)

type campaignStageColumnsEnsurer interface {
	EnsureStageColumns(ctx context.Context) (marketingrepo.EnsureStageColumnsResult, error)
}

// ensureCampaignStageColumns makes sure the tenant's campaign board has a
// column for every campaign stage (see CampaignsRepository.EnsureStageColumns
// for what that may and may not change).
//
// It is best effort by design and returns nothing: a board that cannot be
// repaired here is repaired on the next campaign create or update, which
// runs the same routine, so it must never keep the server from starting.
// Any failure, a panic included, becomes a WARN line with the tenant id.
func ensureCampaignStageColumns(ctx context.Context, repo campaignStageColumnsEnsurer, t tenant.Info) {
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.WarnContext(ctx, "campaign stage columns not ensured; continuing startup",
				"tenant_id", t.ID, "tenant", t.Slug, "error", fmt.Sprintf("panic: %v", recovered))
		}
	}()

	result, err := repo.EnsureStageColumns(ctx)
	if result.Changed() {
		slog.InfoContext(ctx, "ensured campaign stage columns",
			"tenant_id", t.ID, "tenant", t.Slug, "adopted", result.Adopted, "created", result.Created)
	}
	if err != nil {
		slog.WarnContext(ctx, "campaign stage columns not ensured; continuing startup",
			"tenant_id", t.ID, "tenant", t.Slug, "error", err)
	}
}
