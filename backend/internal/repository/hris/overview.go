package hris

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kana-consultant/kantor/backend/internal/model"
	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

type OverviewRepository struct {
	db *pgxpool.Pool
}

func NewOverviewRepository(db *pgxpool.Pool) *OverviewRepository {
	return &OverviewRepository{db: db}
}

func (r *OverviewRepository) GetOverview(ctx context.Context, now time.Time) (model.HrisOverview, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	overview := model.HrisOverview{}

	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM employees WHERE employment_status = 'active'`).Scan(&overview.TotalEmployees); err != nil {
		return model.HrisOverview{}, err
	}

	if err := r.db.QueryRow(ctx, `
		SELECT
			COUNT(*)::bigint,
			COALESCE(SUM(
				CASE billing_cycle
					WHEN 'monthly' THEN cost_amount
					WHEN 'quarterly' THEN cost_amount / 3
					WHEN 'yearly' THEN cost_amount / 12
					ELSE 0
				END
			), 0)::bigint
		FROM subscriptions
		WHERE status = 'active'
	`).Scan(&overview.ActiveSubscriptions, &overview.ActiveSubscriptionMonthlyCost); err != nil {
		return model.HrisOverview{}, err
	}

	financeSeries, monthlyNet, err := r.financeSeries(ctx, now)
	if err != nil {
		return model.HrisOverview{}, err
	}
	overview.IncomeVsOutcome = financeSeries
	overview.MonthlyNet = monthlyNet

	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM reimbursements WHERE status = 'submitted'`).Scan(&overview.PendingReimbursements); err != nil {
		return model.HrisOverview{}, err
	}

	upcomingRenewals, err := r.listUpcomingRenewals(ctx, now)
	if err != nil {
		return model.HrisOverview{}, err
	}
	overview.UpcomingRenewals = upcomingRenewals

	recentReimbursements, err := r.listRecentReimbursements(ctx)
	if err != nil {
		return model.HrisOverview{}, err
	}
	overview.RecentReimbursements = recentReimbursements

	return overview, nil
}

func (r *OverviewRepository) financeSeries(ctx context.Context, now time.Time) ([]model.FinanceOverviewPoint, int64, error) {
	startMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).AddDate(0, -5, 0)
	endMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).AddDate(0, 1, 0)

	rows, err := r.db.Query(ctx, `
		SELECT
			DATE_TRUNC('month', record_date)::date AS month_start,
			COALESCE(SUM(CASE WHEN type = 'income' AND approval_status = 'approved' THEN amount ELSE 0 END), 0)::bigint AS income,
			COALESCE(SUM(CASE WHEN type = 'outcome' AND approval_status = 'approved' THEN amount ELSE 0 END), 0)::bigint AS outcome
		FROM finance_records
		WHERE record_date >= $1::date
		  AND record_date < $2::date
		GROUP BY month_start
		ORDER BY month_start ASC
	`, startMonth, endMonth)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	monthlyMap := make(map[string]model.FinanceOverviewPoint, 6)
	for rows.Next() {
		var monthStart time.Time
		var income int64
		var outcome int64
		if err := rows.Scan(&monthStart, &income, &outcome); err != nil {
			return nil, 0, err
		}
		key := monthStart.Format("2006-01")
		monthlyMap[key] = model.FinanceOverviewPoint{
			Key:     key,
			Label:   monthStart.Format("Jan"),
			Income:  income,
			Outcome: outcome,
		}
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	series := make([]model.FinanceOverviewPoint, 0, 6)
	monthlyNet := int64(0)
	for month := 0; month < 6; month++ {
		current := startMonth.AddDate(0, month, 0)
		key := current.Format("2006-01")
		point, ok := monthlyMap[key]
		if !ok {
			point = model.FinanceOverviewPoint{
				Key:   key,
				Label: current.Format("Jan"),
			}
		}
		if current.Year() == now.Year() && current.Month() == now.Month() {
			monthlyNet = point.Income - point.Outcome
		}
		series = append(series, point)
	}

	return series, monthlyNet, nil
}

func (r *OverviewRepository) listUpcomingRenewals(ctx context.Context, now time.Time) ([]model.HrisUpcomingRenewal, error) {
	endDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, 30)
	rows, err := r.db.Query(ctx, `
		SELECT
			subscriptions.id::text,
			subscriptions.name,
			subscriptions.vendor,
			subscriptions.renewal_date,
			subscriptions.cost_amount,
			subscriptions.cost_currency,
			employees.full_name
		FROM subscriptions
		LEFT JOIN employees ON employees.id = subscriptions.pic_employee_id
		WHERE subscriptions.status = 'active'
		  AND subscriptions.renewal_date >= $1::date
		  AND subscriptions.renewal_date <= $2::date
		ORDER BY subscriptions.renewal_date ASC, subscriptions.name ASC
		LIMIT 5
	`, now, endDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.HrisUpcomingRenewal, 0, 5)
	for rows.Next() {
		var item model.HrisUpcomingRenewal
		if err := rows.Scan(
			&item.ID,
			&item.Name,
			&item.Vendor,
			&item.RenewalDate,
			&item.CostAmount,
			&item.CostCurrency,
			&item.PICEmployeeName,
		); err != nil {
			return nil, err
		}
		item.DaysRemaining = int(item.RenewalDate.Sub(time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())).Hours() / 24)
		items = append(items, item)
	}

	return items, rows.Err()
}

func (r *OverviewRepository) listRecentReimbursements(ctx context.Context) ([]model.Reimbursement, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			reimbursements.id::text,
			reimbursements.employee_id::text,
			employees.full_name,
			reimbursements.title,
			reimbursements.category,
			reimbursements.amount,
			reimbursements.transaction_date,
			reimbursements.description,
			reimbursements.status,
			reimbursements.attachments,
			reimbursements.submitted_by::text,
			reimbursements.manager_id::text,
			reimbursements.manager_action_at,
			reimbursements.manager_notes,
			reimbursements.finance_id::text,
			reimbursements.finance_action_at,
			reimbursements.finance_notes,
			reimbursements.paid_at,
			reimbursements.created_at,
			reimbursements.updated_at
		FROM reimbursements
		INNER JOIN employees ON employees.id = reimbursements.employee_id
		ORDER BY reimbursements.created_at DESC
		LIMIT 5
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.Reimbursement, 0, 5)
	for rows.Next() {
		item, err := scanReimbursement(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, rows.Err()
}
