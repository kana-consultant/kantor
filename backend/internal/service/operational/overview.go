package operational

import (
	"context"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/model"
	operationalrepo "github.com/kana-consultant/kantor/backend/internal/repository/operational"
)

type OverviewService struct {
	repo *operationalrepo.OverviewRepository
}

func NewOverviewService(repo *operationalrepo.OverviewRepository) *OverviewService {
	return &OverviewService{repo: repo}
}

func (s *OverviewService) GetOverview(ctx context.Context) (model.OperationalOverview, error) {
	return s.repo.GetOverview(ctx, time.Now())
}
