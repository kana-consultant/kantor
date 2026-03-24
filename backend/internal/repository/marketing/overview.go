package marketing

import (
	"context"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/model"
	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

type OverviewRepository struct {
	db repository.DBTX
}

func NewOverviewRepository(db repository.DBTX) *OverviewRepository {
	return &OverviewRepository{db: db}
}

func (r *OverviewRepository) GetOverview(ctx context.Context, now time.Time) (model.MarketingOverview, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	overview := model.MarketingOverview{}

	if err := repository.DB(ctx, r.db).QueryRow(ctx, `
		SELECT COUNT(*)
		FROM campaigns
		WHERE status IN ('ideation', 'planning', 'in_production', 'live')
	`).Scan(&overview.ActiveCampaigns); err != nil {
		return model.MarketingOverview{}, err
	}

	currentMonthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	nextMonthStart := currentMonthStart.AddDate(0, 1, 0)

	var currentRevenue int64
	if err := repository.DB(ctx, r.db).QueryRow(ctx, `
		SELECT
			COALESCE(SUM(amount_spent), 0)::bigint,
			COALESCE(SUM(revenue), 0)::bigint
		FROM ads_metrics
		WHERE period_end >= $1::date
		  AND period_start < $2::date
	`, currentMonthStart, nextMonthStart).Scan(&overview.TotalAdsSpent, &currentRevenue); err != nil {
		return model.MarketingOverview{}, err
	}

	if overview.TotalAdsSpent > 0 {
		roas := float64(currentRevenue) / float64(overview.TotalAdsSpent)
		overview.OverallROAS = &roas
	}

	var wonLeads int64
	if err := repository.DB(ctx, r.db).QueryRow(ctx, `
		SELECT
			COUNT(*)::bigint,
			COUNT(*) FILTER (WHERE pipeline_status = 'won')::bigint
		FROM leads
		WHERE created_at >= $1
		  AND created_at < $2
	`, currentMonthStart, nextMonthStart).Scan(&overview.TotalLeads, &wonLeads); err != nil {
		return model.MarketingOverview{}, err
	}
	if overview.TotalLeads > 0 {
		overview.ConversionRate = float64(wonLeads) / float64(overview.TotalLeads) * 100
	}

	roasTrend, err := r.listRoasTrend(ctx, currentMonthStart)
	if err != nil {
		return model.MarketingOverview{}, err
	}
	overview.ROASTrend = roasTrend

	leadsByStage, err := r.listLeadsByStage(ctx)
	if err != nil {
		return model.MarketingOverview{}, err
	}
	overview.LeadsByStage = leadsByStage

	topCampaigns, err := r.listTopCampaigns(ctx, currentMonthStart, nextMonthStart)
	if err != nil {
		return model.MarketingOverview{}, err
	}
	overview.TopCampaigns = topCampaigns

	return overview, nil
}

func (r *OverviewRepository) listRoasTrend(ctx context.Context, currentMonthStart time.Time) ([]model.MarketingROASTrendPoint, error) {
	startMonth := currentMonthStart.AddDate(0, -5, 0)
	endMonth := currentMonthStart.AddDate(0, 1, 0)

	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT
			DATE_TRUNC('month', period_start)::date AS month_start,
			COALESCE(SUM(amount_spent), 0)::bigint AS spent,
			COALESCE(SUM(revenue), 0)::bigint AS revenue
		FROM ads_metrics
		WHERE period_start >= $1::date
		  AND period_start < $2::date
		GROUP BY month_start
		ORDER BY month_start ASC
	`, startMonth, endMonth)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	monthlyMap := make(map[string]model.MarketingROASTrendPoint, 6)
	for rows.Next() {
		var monthStart time.Time
		var spent int64
		var revenue int64
		if err := rows.Scan(&monthStart, &spent, &revenue); err != nil {
			return nil, err
		}
		key := monthStart.Format("2006-01")
		point := model.MarketingROASTrendPoint{
			Key:     key,
			Label:   monthStart.Format("Jan"),
			Spent:   spent,
			Revenue: revenue,
		}
		if spent > 0 {
			roas := float64(revenue) / float64(spent)
			point.ROAS = &roas
		}
		monthlyMap[key] = point
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	series := make([]model.MarketingROASTrendPoint, 0, 6)
	for month := 0; month < 6; month++ {
		current := startMonth.AddDate(0, month, 0)
		key := current.Format("2006-01")
		point, ok := monthlyMap[key]
		if !ok {
			point = model.MarketingROASTrendPoint{
				Key:   key,
				Label: current.Format("Jan"),
			}
		}
		series = append(series, point)
	}

	return series, nil
}

func (r *OverviewRepository) listLeadsByStage(ctx context.Context) ([]model.LeadSummaryRow, error) {
	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT pipeline_status, COUNT(*)::bigint
		FROM leads
		GROUP BY pipeline_status
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byStatus := make(map[string]model.LeadSummaryRow, len(leadStatuses))
	for _, status := range leadStatuses {
		byStatus[status.Key] = model.LeadSummaryRow{
			Status: status.Key,
			Label:  status.Label,
		}
	}

	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		item := byStatus[status]
		item.LeadCount = count
		byStatus[status] = item
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	items := make([]model.LeadSummaryRow, 0, len(leadStatuses))
	for _, status := range leadStatuses {
		items = append(items, byStatus[status.Key])
	}

	return items, nil
}

func (r *OverviewRepository) listTopCampaigns(ctx context.Context, startDate time.Time, endDate time.Time) ([]model.MarketingTopCampaign, error) {
	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT
			campaigns.id::text,
			campaigns.name,
			campaigns.status,
			COALESCE(SUM(ads_metrics.amount_spent), 0)::bigint AS total_spent,
			COALESCE(SUM(ads_metrics.revenue), 0)::bigint AS total_revenue,
			CASE
				WHEN COALESCE(SUM(ads_metrics.amount_spent), 0) = 0 THEN NULL
				ELSE COALESCE(SUM(ads_metrics.revenue), 0)::float8 / COALESCE(SUM(ads_metrics.amount_spent), 0)::float8
			END AS roas
		FROM ads_metrics
		INNER JOIN campaigns ON campaigns.id = ads_metrics.campaign_id
		WHERE ads_metrics.period_end >= $1::date
		  AND ads_metrics.period_start < $2::date
		GROUP BY campaigns.id, campaigns.name, campaigns.status
		ORDER BY roas DESC NULLS LAST, total_revenue DESC, campaigns.name ASC
		LIMIT 5
	`, startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.MarketingTopCampaign, 0, 5)
	for rows.Next() {
		var item model.MarketingTopCampaign
		if err := rows.Scan(
			&item.CampaignID,
			&item.CampaignName,
			&item.Status,
			&item.TotalSpent,
			&item.TotalRevenue,
			&item.ROAS,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, rows.Err()
}
