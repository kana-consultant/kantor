package operational

import (
	"context"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/model"
)

type overviewRepository interface {
	GetOverview(ctx context.Context, now time.Time) (model.OperationalOverview, error)
}

type OverviewService struct {
	repo overviewRepository
}

func NewOverviewService(repo overviewRepository) *OverviewService {
	return &OverviewService{repo: repo}
}

func (s *OverviewService) GetOverview(ctx context.Context) (model.OperationalOverview, error) {
	return s.repo.GetOverview(ctx, time.Now())
}
