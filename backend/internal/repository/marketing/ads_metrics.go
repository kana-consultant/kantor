package marketing

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v5"

	"github.com/kana-consultant/kantor/backend/internal/model"
	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

var (
	ErrAdsMetricNotFound        = errors.New("ads metric not found")
	ErrAdsMetricCampaignMissing = errors.New("ads metric campaign not found")
)

type AdsMetricsRepository struct {
	db repository.DBTX
}

type UpsertAdsMetricParams struct {
	CampaignID  string
	Platform    string
	PeriodStart time.Time
	PeriodEnd   time.Time
	AmountSpent int64
	Impressions int64
	Clicks      int64
	Conversions int64
	Revenue     int64
	Notes       *string
	CreatedBy   string
}

type ListAdsMetricsParams struct {
	Page       int
	PerPage    int
	CampaignID string
	Platform   string
	DateFrom   string
	DateTo     string
}

type AdsMetricsSummaryParams struct {
	GroupBy  string
	DateFrom string
	DateTo   string
}

type AdsMetricsExportParams struct {
	DateFrom string
	DateTo   string
}

func NewAdsMetricsRepository(db repository.DBTX) *AdsMetricsRepository {
	return &AdsMetricsRepository{db: db}
}

func (r *AdsMetricsRepository) CreateMetric(ctx context.Context, params UpsertAdsMetricParams) (model.AdsMetric, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	row := repository.DB(ctx, r.db).QueryRow(
		ctx,
		`
			INSERT INTO ads_metrics (
				campaign_id, platform, period_start, period_end, amount_spent, impressions, clicks, conversions, revenue, notes, created_by
			)
			VALUES (
				$1::uuid, $2, $3::date, $4::date, $5, $6, $7, $8, $9, NULLIF($10, ''), $11::uuid
			)
			RETURNING id::text, campaign_id::text, platform, period_start, period_end, amount_spent, impressions, clicks, conversions, revenue, notes, created_by::text, created_at, updated_at
		`,
		params.CampaignID,
		params.Platform,
		params.PeriodStart,
		params.PeriodEnd,
		params.AmountSpent,
		params.Impressions,
		params.Clicks,
		params.Conversions,
		params.Revenue,
		nullableText(params.Notes),
		params.CreatedBy,
	)

	item, err := r.scanMetricRow(row)
	if err != nil {
		return model.AdsMetric{}, mapAdsMetricError(err)
	}

	return r.hydrateCampaignName(ctx, item)
}

func (r *AdsMetricsRepository) BatchCreateMetrics(ctx context.Context, params []UpsertAdsMetricParams) ([]model.AdsMetric, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	tx, err := repository.DB(ctx, r.db).Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	items := make([]model.AdsMetric, 0, len(params))
	for _, param := range params {
		row := tx.QueryRow(
			ctx,
			`
				INSERT INTO ads_metrics (
					campaign_id, platform, period_start, period_end, amount_spent, impressions, clicks, conversions, revenue, notes, created_by
				)
				VALUES (
					$1::uuid, $2, $3::date, $4::date, $5, $6, $7, $8, $9, NULLIF($10, ''), $11::uuid
				)
				RETURNING id::text, campaign_id::text, platform, period_start, period_end, amount_spent, impressions, clicks, conversions, revenue, notes, created_by::text, created_at, updated_at
			`,
			param.CampaignID,
			param.Platform,
			param.PeriodStart,
			param.PeriodEnd,
			param.AmountSpent,
			param.Impressions,
			param.Clicks,
			param.Conversions,
			param.Revenue,
			nullableText(param.Notes),
			param.CreatedBy,
		)

		item, scanErr := r.scanMetricRow(row)
		if scanErr != nil {
			return nil, mapAdsMetricError(scanErr)
		}
		items = append(items, item)
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}

	for index, item := range items {
		hydrated, hydrateErr := r.hydrateCampaignName(ctx, item)
		if hydrateErr != nil {
			return nil, hydrateErr
		}
		items[index] = hydrated
	}

	return items, nil
}

func (r *AdsMetricsRepository) ListMetrics(ctx context.Context, params ListAdsMetricsParams) ([]model.AdsMetric, int64, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	filters := []string{"1=1"}
	args := make([]interface{}, 0)
	index := 1

	if campaignID := strings.TrimSpace(params.CampaignID); campaignID != "" {
		filters = append(filters, fmt.Sprintf("ads_metrics.campaign_id = $%d::uuid", index))
		args = append(args, campaignID)
		index++
	}
	if platform := strings.TrimSpace(params.Platform); platform != "" {
		filters = append(filters, fmt.Sprintf("ads_metrics.platform = $%d", index))
		args = append(args, platform)
		index++
	}
	if dateFrom := strings.TrimSpace(params.DateFrom); dateFrom != "" {
		filters = append(filters, fmt.Sprintf("ads_metrics.period_end >= $%d::date", index))
		args = append(args, dateFrom)
		index++
	}
	if dateTo := strings.TrimSpace(params.DateTo); dateTo != "" {
		filters = append(filters, fmt.Sprintf("ads_metrics.period_start <= $%d::date", index))
		args = append(args, dateTo)
		index++
	}

	whereClause := strings.Join(filters, " AND ")

	var total int64
	if err := repository.DB(ctx, r.db).QueryRow(ctx, `SELECT COUNT(*) FROM ads_metrics WHERE `+whereClause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (params.Page - 1) * params.PerPage
	query := fmt.Sprintf(`
		SELECT
			ads_metrics.id::text,
			ads_metrics.campaign_id::text,
			campaigns.name,
			ads_metrics.platform,
			ads_metrics.period_start,
			ads_metrics.period_end,
			ads_metrics.amount_spent,
			ads_metrics.impressions,
			ads_metrics.clicks,
			ads_metrics.conversions,
			ads_metrics.revenue,
			ads_metrics.notes,
			ads_metrics.created_by::text,
			ads_metrics.created_at,
			ads_metrics.updated_at
		FROM ads_metrics
		INNER JOIN campaigns ON campaigns.id = ads_metrics.campaign_id
		WHERE %s
		ORDER BY ads_metrics.period_start DESC, ads_metrics.created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, index, index+1)
	args = append(args, params.PerPage, offset)

	rows, err := repository.DB(ctx, r.db).Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := make([]model.AdsMetric, 0)
	for rows.Next() {
		var item model.AdsMetric
		var campaignName string
		if err := rows.Scan(
			&item.ID,
			&item.CampaignID,
			&campaignName,
			&item.Platform,
			&item.PeriodStart,
			&item.PeriodEnd,
			&item.AmountSpent,
			&item.Impressions,
			&item.Clicks,
			&item.Conversions,
			&item.Revenue,
			&item.Notes,
			&item.CreatedBy,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		item.CampaignName = &campaignName
		applyDerivedMetrics(&item)
		items = append(items, item)
	}

	return items, total, rows.Err()
}

func (r *AdsMetricsRepository) GetMetricByID(ctx context.Context, metricID string) (model.AdsMetric, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	row := repository.DB(ctx, r.db).QueryRow(
		ctx,
		`
			SELECT
				ads_metrics.id::text,
				ads_metrics.campaign_id::text,
				campaigns.name,
				ads_metrics.platform,
				ads_metrics.period_start,
				ads_metrics.period_end,
				ads_metrics.amount_spent,
				ads_metrics.impressions,
				ads_metrics.clicks,
				ads_metrics.conversions,
				ads_metrics.revenue,
				ads_metrics.notes,
				ads_metrics.created_by::text,
				ads_metrics.created_at,
				ads_metrics.updated_at
			FROM ads_metrics
			INNER JOIN campaigns ON campaigns.id = ads_metrics.campaign_id
			WHERE ads_metrics.id = $1::uuid
		`,
		metricID,
	)

	var item model.AdsMetric
	var campaignName string
	if err := row.Scan(
		&item.ID,
		&item.CampaignID,
		&campaignName,
		&item.Platform,
		&item.PeriodStart,
		&item.PeriodEnd,
		&item.AmountSpent,
		&item.Impressions,
		&item.Clicks,
		&item.Conversions,
		&item.Revenue,
		&item.Notes,
		&item.CreatedBy,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.AdsMetric{}, ErrAdsMetricNotFound
		}
		return model.AdsMetric{}, err
	}

	item.CampaignName = &campaignName
	applyDerivedMetrics(&item)
	return item, nil
}

func (r *AdsMetricsRepository) UpdateMetric(ctx context.Context, metricID string, params UpsertAdsMetricParams) (model.AdsMetric, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	row := repository.DB(ctx, r.db).QueryRow(
		ctx,
		`
			UPDATE ads_metrics
			SET
				campaign_id = $2::uuid,
				platform = $3,
				period_start = $4::date,
				period_end = $5::date,
				amount_spent = $6,
				impressions = $7,
				clicks = $8,
				conversions = $9,
				revenue = $10,
				notes = NULLIF($11, ''),
				updated_at = NOW()
			WHERE id = $1::uuid
			RETURNING id::text, campaign_id::text, platform, period_start, period_end, amount_spent, impressions, clicks, conversions, revenue, notes, created_by::text, created_at, updated_at
		`,
		metricID,
		params.CampaignID,
		params.Platform,
		params.PeriodStart,
		params.PeriodEnd,
		params.AmountSpent,
		params.Impressions,
		params.Clicks,
		params.Conversions,
		params.Revenue,
		nullableText(params.Notes),
	)

	item, err := r.scanMetricRow(row)
	if err != nil {
		return model.AdsMetric{}, mapAdsMetricError(err)
	}

	return r.hydrateCampaignName(ctx, item)
}

func (r *AdsMetricsRepository) DeleteMetric(ctx context.Context, metricID string) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	tag, err := repository.DB(ctx, r.db).Exec(ctx, `DELETE FROM ads_metrics WHERE id = $1::uuid`, metricID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrAdsMetricNotFound
	}
	return nil
}

func (r *AdsMetricsRepository) Summary(ctx context.Context, params AdsMetricsSummaryParams) (model.AdsMetricsSummary, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	filters := []string{"1=1"}
	args := make([]interface{}, 0)
	index := 1

	if dateFrom := strings.TrimSpace(params.DateFrom); dateFrom != "" {
		filters = append(filters, fmt.Sprintf("ads_metrics.period_end >= $%d::date", index))
		args = append(args, dateFrom)
		index++
	}
	if dateTo := strings.TrimSpace(params.DateTo); dateTo != "" {
		filters = append(filters, fmt.Sprintf("ads_metrics.period_start <= $%d::date", index))
		args = append(args, dateTo)
		index++
	}

	selectGroup := ""
	groupBy := ""
	orderBy := ""
	joinClause := ""

	switch params.GroupBy {
	case "campaign":
		joinClause = "INNER JOIN campaigns ON campaigns.id = ads_metrics.campaign_id"
		selectGroup = "campaigns.id::text AS group_key, campaigns.name AS group_label"
		groupBy = "campaigns.id, campaigns.name"
		orderBy = "campaigns.name ASC"
	case "platform":
		selectGroup = "ads_metrics.platform AS group_key, ads_metrics.platform AS group_label"
		groupBy = "ads_metrics.platform"
		orderBy = "ads_metrics.platform ASC"
	default:
		selectGroup = "TO_CHAR(DATE_TRUNC('month', ads_metrics.period_start), 'YYYY-MM') AS group_key, TO_CHAR(DATE_TRUNC('month', ads_metrics.period_start), 'YYYY-MM') AS group_label"
		groupBy = "DATE_TRUNC('month', ads_metrics.period_start)"
		orderBy = "DATE_TRUNC('month', ads_metrics.period_start) ASC"
	}

	query := `
		SELECT
			` + selectGroup + `,
			COALESCE(SUM(ads_metrics.amount_spent), 0) AS total_spent,
			COALESCE(SUM(ads_metrics.impressions), 0) AS total_impressions,
			COALESCE(SUM(ads_metrics.clicks), 0) AS total_clicks,
			COALESCE(SUM(ads_metrics.conversions), 0) AS total_conversions,
			COALESCE(SUM(ads_metrics.revenue), 0) AS total_revenue
		FROM ads_metrics
		` + joinClause + `
		WHERE ` + strings.Join(filters, " AND ") + `
		GROUP BY ` + groupBy + `
		ORDER BY ` + orderBy

	rows, err := repository.DB(ctx, r.db).Query(ctx, query, args...)
	if err != nil {
		return model.AdsMetricsSummary{}, err
	}
	defer rows.Close()

	items := make([]model.AdsMetricsSummaryRow, 0)
	for rows.Next() {
		var item model.AdsMetricsSummaryRow
		if err := rows.Scan(
			&item.GroupKey,
			&item.GroupLabel,
			&item.TotalSpent,
			&item.TotalImpressions,
			&item.TotalClicks,
			&item.TotalConversions,
			&item.TotalRevenue,
		); err != nil {
			return model.AdsMetricsSummary{}, err
		}
		applySummaryDerivedMetrics(&item)
		item.GroupLabel = formatSummaryLabel(params.GroupBy, item.GroupLabel)
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return model.AdsMetricsSummary{}, err
	}

	return model.AdsMetricsSummary{
		GroupBy: params.GroupBy,
		Items:   items,
	}, nil
}

func (r *AdsMetricsRepository) ListForExport(ctx context.Context, params AdsMetricsExportParams) ([]model.AdsMetric, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	filters := []string{"1=1"}
	args := make([]interface{}, 0)
	index := 1

	if dateFrom := strings.TrimSpace(params.DateFrom); dateFrom != "" {
		filters = append(filters, fmt.Sprintf("ads_metrics.period_end >= $%d::date", index))
		args = append(args, dateFrom)
		index++
	}
	if dateTo := strings.TrimSpace(params.DateTo); dateTo != "" {
		filters = append(filters, fmt.Sprintf("ads_metrics.period_start <= $%d::date", index))
		args = append(args, dateTo)
		index++
	}

	query := `
		SELECT
			ads_metrics.id::text,
			ads_metrics.campaign_id::text,
			campaigns.name,
			ads_metrics.platform,
			ads_metrics.period_start,
			ads_metrics.period_end,
			ads_metrics.amount_spent,
			ads_metrics.impressions,
			ads_metrics.clicks,
			ads_metrics.conversions,
			ads_metrics.revenue,
			ads_metrics.notes,
			ads_metrics.created_by::text,
			ads_metrics.created_at,
			ads_metrics.updated_at
		FROM ads_metrics
		INNER JOIN campaigns ON campaigns.id = ads_metrics.campaign_id
		WHERE ` + strings.Join(filters, " AND ") + `
		ORDER BY ads_metrics.period_start DESC, ads_metrics.created_at DESC
	`

	rows, err := repository.DB(ctx, r.db).Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.AdsMetric, 0)
	for rows.Next() {
		var item model.AdsMetric
		var campaignName string
		if err := rows.Scan(
			&item.ID,
			&item.CampaignID,
			&campaignName,
			&item.Platform,
			&item.PeriodStart,
			&item.PeriodEnd,
			&item.AmountSpent,
			&item.Impressions,
			&item.Clicks,
			&item.Conversions,
			&item.Revenue,
			&item.Notes,
			&item.CreatedBy,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.CampaignName = &campaignName
		applyDerivedMetrics(&item)
		items = append(items, item)
	}

	return items, rows.Err()
}

func (r *AdsMetricsRepository) scanMetricRow(row pgx.Row) (model.AdsMetric, error) {
	var item model.AdsMetric
	err := row.Scan(
		&item.ID,
		&item.CampaignID,
		&item.Platform,
		&item.PeriodStart,
		&item.PeriodEnd,
		&item.AmountSpent,
		&item.Impressions,
		&item.Clicks,
		&item.Conversions,
		&item.Revenue,
		&item.Notes,
		&item.CreatedBy,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.AdsMetric{}, ErrAdsMetricNotFound
		}
		return model.AdsMetric{}, err
	}
	applyDerivedMetrics(&item)
	return item, nil
}

func (r *AdsMetricsRepository) hydrateCampaignName(ctx context.Context, item model.AdsMetric) (model.AdsMetric, error) {
	var campaignName string
	if err := repository.DB(ctx, r.db).QueryRow(ctx, `SELECT name FROM campaigns WHERE id = $1::uuid`, item.CampaignID).Scan(&campaignName); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.AdsMetric{}, ErrAdsMetricCampaignMissing
		}
		return model.AdsMetric{}, err
	}
	item.CampaignName = &campaignName
	return item, nil
}

func applyDerivedMetrics(item *model.AdsMetric) {
	item.CPR = calculateRatio(float64(item.AmountSpent), float64(item.Conversions), 1)
	item.ROAS = calculateRatio(float64(item.Revenue), float64(item.AmountSpent), 1)
	item.CTR = calculateRatio(float64(item.Clicks), float64(item.Impressions), 100)
	item.CPC = calculateRatio(float64(item.AmountSpent), float64(item.Clicks), 1)
	item.CPM = calculateRatio(float64(item.AmountSpent), float64(item.Impressions), 1000)
}

func applySummaryDerivedMetrics(item *model.AdsMetricsSummaryRow) {
	item.CPR = calculateRatio(float64(item.TotalSpent), float64(item.TotalConversions), 1)
	item.ROAS = calculateRatio(float64(item.TotalRevenue), float64(item.TotalSpent), 1)
	item.CTR = calculateRatio(float64(item.TotalClicks), float64(item.TotalImpressions), 100)
	item.CPC = calculateRatio(float64(item.TotalSpent), float64(item.TotalClicks), 1)
	item.CPM = calculateRatio(float64(item.TotalSpent), float64(item.TotalImpressions), 1000)
}

func calculateRatio(numerator float64, denominator float64, multiplier float64) *float64 {
	if denominator == 0 {
		return nil
	}
	value := (numerator / denominator) * multiplier
	return &value
}

func formatSummaryLabel(groupBy string, value string) string {
	if groupBy != "platform" {
		return value
	}
	parts := strings.Split(strings.ReplaceAll(value, "_", " "), " ")
	for index, part := range parts {
		if part == "" {
			continue
		}
		parts[index] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
	}
	return strings.Join(parts, " ")
}

func mapAdsMetricError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.ConstraintName == "ads_metrics_campaign_id_fkey" {
		return ErrAdsMetricCampaignMissing
	}
	return err
}

func nullableText(value *string) interface{} {
	if value == nil {
		return nil
	}

	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}

	return trimmed
}
