package hris

import (
	"context"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/model"
)

type hrisOverviewRepository interface {
	GetOverview(ctx context.Context, now time.Time) (model.HrisOverview, error)
}

type OverviewService struct {
	repo hrisOverviewRepository
}

func NewOverviewService(repo hrisOverviewRepository) *OverviewService {
	return &OverviewService{repo: repo}
}

func (s *OverviewService) GetOverview(ctx context.Context) (model.HrisOverview, error) {
	return s.repo.GetOverview(ctx, time.Now())
}
