package hris

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/kana-consultant/kantor/backend/internal/model"
	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

type OverviewRepository struct {
	db repository.DBTX
}

func NewOverviewRepository(db repository.DBTX) *OverviewRepository {
	return &OverviewRepository{db: db}
}

func (r *OverviewRepository) GetOverview(ctx context.Context, now time.Time, employeeFilter string) (model.HrisOverview, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	overview := model.HrisOverview{}

	if err := repository.DB(ctx, r.db).QueryRow(ctx, `SELECT COUNT(*) FROM employees WHERE employment_status = 'active'`).Scan(&overview.TotalEmployees); err != nil {
		return model.HrisOverview{}, err
	}

	if err := repository.DB(ctx, r.db).QueryRow(ctx, `
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

	pendingCount, err := r.countPendingReimbursements(ctx, employeeFilter)
	if err != nil {
		return model.HrisOverview{}, err
	}
	overview.PendingReimbursements = pendingCount

	monthlyReimbursementTotal, err := r.sumMonthlyReimbursements(ctx, now, employeeFilter)
	if err != nil {
		return model.HrisOverview{}, err
	}
	overview.MonthlyReimbursementTotal = monthlyReimbursementTotal

	upcomingRenewals, err := r.listUpcomingRenewals(ctx, now)
	if err != nil {
		return model.HrisOverview{}, err
	}
	overview.UpcomingRenewals = upcomingRenewals

	recentReimbursements, err := r.listRecentReimbursements(ctx, employeeFilter)
	if err != nil {
		return model.HrisOverview{}, err
	}
	overview.RecentReimbursements = recentReimbursements

	return overview, nil
}

func (r *OverviewRepository) ListLatestActivePayrollCiphertexts(ctx context.Context) ([]string, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT latest.net_salary
		FROM (
			SELECT DISTINCT ON (s.employee_id) s.net_salary
			FROM salaries s
			INNER JOIN employees e ON e.id = s.employee_id
			WHERE e.employment_status = 'active'
			ORDER BY s.employee_id, s.effective_date DESC, s.created_at DESC
		) latest
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ciphertexts := make([]string, 0)
	for rows.Next() {
		var ciphertext string
		if err := rows.Scan(&ciphertext); err != nil {
			return nil, err
		}
		ciphertexts = append(ciphertexts, ciphertext)
	}

	return ciphertexts, rows.Err()
}

func (r *OverviewRepository) ListActivePayrollHistoryRows(ctx context.Context) ([]model.HrisOverviewSalaryHistoryRow, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT s.employee_id::text, s.effective_date, s.net_salary
		FROM salaries s
		INNER JOIN employees e ON e.id = s.employee_id
		WHERE e.employment_status = 'active'
		ORDER BY s.employee_id ASC, s.effective_date ASC, s.created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.HrisOverviewSalaryHistoryRow, 0)
	for rows.Next() {
		var item model.HrisOverviewSalaryHistoryRow
		if err := rows.Scan(&item.EmployeeID, &item.EffectiveDate, &item.NetSalaryEncrypted); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, rows.Err()
}

func (r *OverviewRepository) ListActiveSubscriptionsForOverview(ctx context.Context) ([]model.HrisOverviewSubscriptionRow, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT start_date, billing_cycle, cost_amount
		FROM subscriptions
		WHERE status = 'active'
		ORDER BY start_date ASC, created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.HrisOverviewSubscriptionRow, 0)
	for rows.Next() {
		var item model.HrisOverviewSubscriptionRow
		if err := rows.Scan(&item.StartDate, &item.BillingCycle, &item.CostAmount); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, rows.Err()
}

func (r *OverviewRepository) financeSeries(ctx context.Context, now time.Time) ([]model.FinanceOverviewPoint, int64, error) {
	startMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).AddDate(0, -5, 0)
	endMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).AddDate(0, 1, 0)

	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT
			DATE_TRUNC('month', finance_records.record_date)::date AS month_start,
			COALESCE(SUM(
				CASE
					WHEN finance_records.type = 'income' AND finance_records.approval_status = 'approved'
						THEN finance_records.amount
					ELSE 0
				END
			), 0)::bigint AS income,
			COALESCE(SUM(
				CASE
					WHEN finance_records.type = 'outcome'
						AND finance_records.approval_status = 'approved'
						AND LOWER(COALESCE(finance_categories.name, '')) NOT IN ('subscription', 'gaji')
						THEN finance_records.amount
					ELSE 0
				END
			), 0)::bigint AS outcome
		FROM finance_records
		INNER JOIN finance_categories ON finance_categories.id = finance_records.category_id
		WHERE finance_records.record_date >= $1::date
		  AND finance_records.record_date < $2::date
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
	rows, err := repository.DB(ctx, r.db).Query(ctx, `
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

func (r *OverviewRepository) sumMonthlyReimbursements(ctx context.Context, now time.Time, employeeFilter string) (int64, error) {
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	monthEnd := monthStart.AddDate(0, 1, 0)

	var total int64
	var err error
	if employeeFilter != "" {
		err = repository.DB(ctx, r.db).QueryRow(ctx, `
			SELECT COALESCE(SUM(amount), 0)::bigint
			FROM reimbursements
			WHERE transaction_date >= $1::date
			  AND transaction_date < $2::date
			  AND employee_id = $3::uuid
		`, monthStart, monthEnd, employeeFilter).Scan(&total)
	} else {
		err = repository.DB(ctx, r.db).QueryRow(ctx, `
			SELECT COALESCE(SUM(amount), 0)::bigint
			FROM reimbursements
			WHERE transaction_date >= $1::date
			  AND transaction_date < $2::date
		`, monthStart, monthEnd).Scan(&total)
	}
	return total, err
}

func (r *OverviewRepository) countPendingReimbursements(ctx context.Context, employeeFilter string) (int64, error) {
	var count int64
	var err error
	if employeeFilter != "" {
		err = repository.DB(ctx, r.db).QueryRow(ctx,
			`SELECT COUNT(*) FROM reimbursements WHERE status = 'submitted' AND employee_id = $1::uuid`,
			employeeFilter,
		).Scan(&count)
	} else {
		err = repository.DB(ctx, r.db).QueryRow(ctx,
			`SELECT COUNT(*) FROM reimbursements WHERE status = 'submitted'`,
		).Scan(&count)
	}
	return count, err
}

func (r *OverviewRepository) listRecentReimbursements(ctx context.Context, employeeFilter string) ([]model.Reimbursement, error) {
	var (
		rows pgx.Rows
		err  error
	)
	if employeeFilter != "" {
		rows, err = repository.DB(ctx, r.db).Query(ctx, `
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
			WHERE reimbursements.employee_id = $1::uuid
			ORDER BY reimbursements.created_at DESC
			LIMIT 5
		`, employeeFilter)
	} else {
		rows, err = repository.DB(ctx, r.db).Query(ctx, `
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
	}
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
