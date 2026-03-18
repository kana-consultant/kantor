package hris

import (
	"context"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/model"
	hrisrepo "github.com/kana-consultant/kantor/backend/internal/repository/hris"
)

type OverviewService struct {
	repo *hrisrepo.OverviewRepository
}

func NewOverviewService(repo *hrisrepo.OverviewRepository) *OverviewService {
	return &OverviewService{repo: repo}
}

func (s *OverviewService) GetOverview(ctx context.Context) (model.HrisOverview, error) {
	return s.repo.GetOverview(ctx, time.Now())
}
