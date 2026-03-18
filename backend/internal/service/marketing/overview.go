package marketing

import (
	"context"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/model"
	marketingrepo "github.com/kana-consultant/kantor/backend/internal/repository/marketing"
)

type OverviewService struct {
	repo *marketingrepo.OverviewRepository
}

func NewOverviewService(repo *marketingrepo.OverviewRepository) *OverviewService {
	return &OverviewService{repo: repo}
}

func (s *OverviewService) GetOverview(ctx context.Context) (model.MarketingOverview, error) {
	return s.repo.GetOverview(ctx, time.Now())
}
