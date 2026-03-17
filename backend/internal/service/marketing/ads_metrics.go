package marketing

import (
	"context"
	"encoding/csv"
	"errors"
	"strconv"
	"strings"
	"time"

	marketingdto "github.com/kana-consultant/kantor/backend/internal/dto/marketing"
	"github.com/kana-consultant/kantor/backend/internal/model"
	marketingrepo "github.com/kana-consultant/kantor/backend/internal/repository/marketing"
)

var (
	ErrAdsMetricNotFound          = errors.New("ads metric not found")
	ErrAdsMetricCampaignNotFound  = errors.New("ads metric campaign not found")
	ErrAdsMetricInvalidPeriod     = errors.New("ads metric period end must be on or after period start")
	ErrAdsMetricUnsupportedExport = errors.New("ads metrics export format is not supported")
)

type AdsMetricsService struct {
	repo *marketingrepo.AdsMetricsRepository
}

func NewAdsMetricsService(repo *marketingrepo.AdsMetricsRepository) *AdsMetricsService {
	return &AdsMetricsService{repo: repo}
}

func (s *AdsMetricsService) CreateMetric(ctx context.Context, request marketingdto.CreateAdsMetricRequest, actorID string) (model.AdsMetric, error) {
	if err := validateAdsMetricPeriod(request.PeriodStart, request.PeriodEnd); err != nil {
		return model.AdsMetric{}, err
	}

	item, err := s.repo.CreateMetric(ctx, marketingrepo.UpsertAdsMetricParams{
		CampaignID:  strings.TrimSpace(request.CampaignID),
		Platform:    strings.TrimSpace(request.Platform),
		PeriodStart: request.PeriodStart,
		PeriodEnd:   request.PeriodEnd,
		AmountSpent: request.AmountSpent,
		Impressions: request.Impressions,
		Clicks:      request.Clicks,
		Conversions: request.Conversions,
		Revenue:     request.Revenue,
		Notes:       trimOptionalString(request.Notes),
		CreatedBy:   actorID,
	})
	return item, mapAdsMetricError(err)
}

func (s *AdsMetricsService) BatchCreateMetrics(ctx context.Context, request marketingdto.BatchCreateAdsMetricsRequest, actorID string) ([]model.AdsMetric, error) {
	params := make([]marketingrepo.UpsertAdsMetricParams, 0, len(request.Entries))
	for _, entry := range request.Entries {
		if err := validateAdsMetricPeriod(entry.PeriodStart, entry.PeriodEnd); err != nil {
			return nil, err
		}

		params = append(params, marketingrepo.UpsertAdsMetricParams{
			CampaignID:  strings.TrimSpace(entry.CampaignID),
			Platform:    strings.TrimSpace(entry.Platform),
			PeriodStart: entry.PeriodStart,
			PeriodEnd:   entry.PeriodEnd,
			AmountSpent: entry.AmountSpent,
			Impressions: entry.Impressions,
			Clicks:      entry.Clicks,
			Conversions: entry.Conversions,
			Revenue:     entry.Revenue,
			Notes:       trimOptionalString(entry.Notes),
			CreatedBy:   actorID,
		})
	}

	items, err := s.repo.BatchCreateMetrics(ctx, params)
	return items, mapAdsMetricError(err)
}

func (s *AdsMetricsService) ListMetrics(ctx context.Context, query marketingdto.ListAdsMetricsQuery) ([]model.AdsMetric, int64, int, int, error) {
	page := query.Page
	if page <= 0 {
		page = 1
	}

	perPage := query.PerPage
	if perPage <= 0 {
		perPage = 20
	}

	items, total, err := s.repo.ListMetrics(ctx, marketingrepo.ListAdsMetricsParams{
		Page:       page,
		PerPage:    perPage,
		CampaignID: strings.TrimSpace(query.CampaignID),
		Platform:   strings.TrimSpace(query.Platform),
		DateFrom:   strings.TrimSpace(query.DateFrom),
		DateTo:     strings.TrimSpace(query.DateTo),
	})
	if err != nil {
		return nil, 0, 0, 0, mapAdsMetricError(err)
	}

	return items, total, page, perPage, nil
}

func (s *AdsMetricsService) GetMetric(ctx context.Context, metricID string) (model.AdsMetric, error) {
	item, err := s.repo.GetMetricByID(ctx, strings.TrimSpace(metricID))
	return item, mapAdsMetricError(err)
}

func (s *AdsMetricsService) UpdateMetric(ctx context.Context, metricID string, request marketingdto.UpdateAdsMetricRequest) (model.AdsMetric, error) {
	if err := validateAdsMetricPeriod(request.PeriodStart, request.PeriodEnd); err != nil {
		return model.AdsMetric{}, err
	}

	item, err := s.repo.UpdateMetric(ctx, strings.TrimSpace(metricID), marketingrepo.UpsertAdsMetricParams{
		CampaignID:  strings.TrimSpace(request.CampaignID),
		Platform:    strings.TrimSpace(request.Platform),
		PeriodStart: request.PeriodStart,
		PeriodEnd:   request.PeriodEnd,
		AmountSpent: request.AmountSpent,
		Impressions: request.Impressions,
		Clicks:      request.Clicks,
		Conversions: request.Conversions,
		Revenue:     request.Revenue,
		Notes:       trimOptionalString(request.Notes),
	})
	return item, mapAdsMetricError(err)
}

func (s *AdsMetricsService) DeleteMetric(ctx context.Context, metricID string) error {
	return mapAdsMetricError(s.repo.DeleteMetric(ctx, strings.TrimSpace(metricID)))
}

func (s *AdsMetricsService) Summary(ctx context.Context, query marketingdto.AdsMetricsSummaryQuery) (model.AdsMetricsSummary, error) {
	groupBy := strings.TrimSpace(query.GroupBy)
	if groupBy == "" {
		groupBy = "month"
	}

	item, err := s.repo.Summary(ctx, marketingrepo.AdsMetricsSummaryParams{
		GroupBy:  groupBy,
		DateFrom: strings.TrimSpace(query.DateFrom),
		DateTo:   strings.TrimSpace(query.DateTo),
	})
	return item, mapAdsMetricError(err)
}

func (s *AdsMetricsService) ExportCSV(ctx context.Context, format string, dateFrom string, dateTo string) ([]byte, error) {
	if strings.ToLower(strings.TrimSpace(format)) != "csv" {
		return nil, ErrAdsMetricUnsupportedExport
	}

	items, err := s.repo.ListForExport(ctx, marketingrepo.AdsMetricsExportParams{
		DateFrom: strings.TrimSpace(dateFrom),
		DateTo:   strings.TrimSpace(dateTo),
	})
	if err != nil {
		return nil, mapAdsMetricError(err)
	}

	builder := &strings.Builder{}
	writer := csv.NewWriter(builder)
	if err := writer.Write([]string{
		"id", "campaign_name", "platform", "period_start", "period_end",
		"amount_spent", "impressions", "clicks", "conversions", "revenue",
		"cpr", "roas", "ctr", "cpc", "cpm", "notes",
	}); err != nil {
		return nil, err
	}

	for _, item := range items {
		if err := writer.Write([]string{
			item.ID,
			derefString(item.CampaignName),
			item.Platform,
			item.PeriodStart.Format("2006-01-02"),
			item.PeriodEnd.Format("2006-01-02"),
			strconv.FormatInt(item.AmountSpent, 10),
			strconv.FormatInt(item.Impressions, 10),
			strconv.FormatInt(item.Clicks, 10),
			strconv.FormatInt(item.Conversions, 10),
			strconv.FormatInt(item.Revenue, 10),
			formatOptionalFloat(item.CPR),
			formatOptionalFloat(item.ROAS),
			formatOptionalFloat(item.CTR),
			formatOptionalFloat(item.CPC),
			formatOptionalFloat(item.CPM),
			derefString(item.Notes),
		}); err != nil {
			return nil, err
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}

	return []byte(builder.String()), nil
}

func validateAdsMetricPeriod(startDate time.Time, endDate time.Time) error {
	if endDate.Before(startDate) {
		return ErrAdsMetricInvalidPeriod
	}
	return nil
}

func mapAdsMetricError(err error) error {
	switch {
	case errors.Is(err, marketingrepo.ErrAdsMetricNotFound):
		return ErrAdsMetricNotFound
	case errors.Is(err, marketingrepo.ErrAdsMetricCampaignMissing):
		return ErrAdsMetricCampaignNotFound
	default:
		return err
	}
}

func formatOptionalFloat(value *float64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatFloat(*value, 'f', 4, 64)
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
