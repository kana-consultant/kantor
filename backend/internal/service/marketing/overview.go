package marketing

import (
	"context"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/model"
)

type marketingOverviewRepository interface {
	GetOverview(ctx context.Context, now time.Time) (model.MarketingOverview, error)
}

type OverviewService struct {
	repo marketingOverviewRepository
}

func NewOverviewService(repo marketingOverviewRepository) *OverviewService {
	return &OverviewService{repo: repo}
}

func (s *OverviewService) GetOverview(ctx context.Context) (model.MarketingOverview, error) {
	return s.repo.GetOverview(ctx, time.Now())
}
