package hris

import (
	"context"
	"database/sql"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/model"
	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

type CompensationPolicyRepository struct {
	db repository.DBTX
}

func NewCompensationPolicyRepository(db repository.DBTX) *CompensationPolicyRepository {
	return &CompensationPolicyRepository{db: db}
}

type UpdateCompensationPolicyParams struct {
	MonthlyBaseSalary int64
	MinHoursPerDay    float64
	MinHoursPerMonth  float64
	Timezone          string
	UpdatedBy         string
}

type EmployeeMonthlyHoursRow struct {
	EmployeeID    string
	UserID        *string
	FullName      string
	Position      string
	DateJoined    time.Time
	ActiveSeconds int64
	SessionCount  int64
}

type EmployeeDailyHoursRow struct {
	EmployeeID    string
	Date          time.Time
	ActiveSeconds int64
}

type EmployeeBaseSalaryRow struct {
	EmployeeID       string
	BaseSalaryCipher string
}

func (r *CompensationPolicyRepository) EnsureRow(ctx context.Context) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	_, err := repository.DB(ctx, r.db).Exec(ctx, `
		INSERT INTO compensation_policies (tenant_id)
		VALUES (current_setting('app.current_tenant')::uuid)
		ON CONFLICT (tenant_id) DO NOTHING
	`)
	return err
}

func (r *CompensationPolicyRepository) Get(ctx context.Context) (model.CompensationPolicy, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var policy model.CompensationPolicy
	err := repository.DB(ctx, r.db).QueryRow(ctx, `
		SELECT tenant_id::text, monthly_base_salary,
		       min_hours_per_day::double precision, min_hours_per_month::double precision,
		       timezone, updated_at
		FROM compensation_policies
		LIMIT 1
	`).Scan(
		&policy.TenantID,
		&policy.MonthlyBaseSalary,
		&policy.MinHoursPerDay,
		&policy.MinHoursPerMonth,
		&policy.Timezone,
		&policy.UpdatedAt,
	)
	return policy, err
}

func (r *CompensationPolicyRepository) Update(ctx context.Context, params UpdateCompensationPolicyParams) (model.CompensationPolicy, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var policy model.CompensationPolicy
	err := repository.DB(ctx, r.db).QueryRow(ctx, `
		UPDATE compensation_policies
		SET monthly_base_salary = $1,
		    min_hours_per_day = $2,
		    min_hours_per_month = $3,
		    timezone = $4,
		    updated_by = NULLIF($5, '')::uuid,
		    updated_at = NOW()
		RETURNING tenant_id::text, monthly_base_salary,
		          min_hours_per_day::double precision, min_hours_per_month::double precision,
		          timezone, updated_at
	`, params.MonthlyBaseSalary, params.MinHoursPerDay, params.MinHoursPerMonth, params.Timezone, params.UpdatedBy).Scan(
		&policy.TenantID,
		&policy.MonthlyBaseSalary,
		&policy.MinHoursPerDay,
		&policy.MinHoursPerMonth,
		&policy.Timezone,
		&policy.UpdatedAt,
	)
	return policy, err
}

// ListMonthlyActiveSeconds returns every active/probation employee who had
// joined by the end of the period, with their total tracked active seconds and
// session count between from and to (inclusive dates).
func (r *CompensationPolicyRepository) ListMonthlyActiveSeconds(ctx context.Context, from time.Time, to time.Time, employeeID *string) ([]EmployeeMonthlyHoursRow, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT e.id::text, e.user_id::text, e.full_name, e.position, e.date_joined,
		       COALESCE(SUM(s.total_active_seconds), 0)::bigint AS active_seconds,
		       COUNT(s.id)::bigint AS session_count
		FROM employees e
		LEFT JOIN activity_sessions s
		  ON s.user_id = e.user_id
		 AND s.date BETWEEN $1::date AND $2::date
		WHERE e.employment_status IN ('active', 'probation')
		  AND e.date_joined <= $2::date
		  AND ($3::uuid IS NULL OR e.id = $3::uuid)
		GROUP BY e.id, e.user_id, e.full_name, e.position, e.date_joined
		ORDER BY e.full_name
	`, from, to, employeeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]EmployeeMonthlyHoursRow, 0)
	for rows.Next() {
		var row EmployeeMonthlyHoursRow
		var userID sql.NullString
		if err := rows.Scan(&row.EmployeeID, &userID, &row.FullName, &row.Position, &row.DateJoined, &row.ActiveSeconds, &row.SessionCount); err != nil {
			return nil, err
		}
		if userID.Valid {
			row.UserID = &userID.String
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// ListDailyActiveSeconds returns the tracked active seconds per employee per
// day between from and to (inclusive). Days without any session are absent;
// the service derives short and absent weekdays from this map.
func (r *CompensationPolicyRepository) ListDailyActiveSeconds(ctx context.Context, from time.Time, to time.Time, employeeID *string) ([]EmployeeDailyHoursRow, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT e.id::text, s.date, SUM(s.total_active_seconds)::bigint AS active_seconds
		FROM employees e
		JOIN activity_sessions s ON s.user_id = e.user_id
		WHERE s.date BETWEEN $1::date AND $2::date
		  AND e.employment_status IN ('active', 'probation')
		  AND ($3::uuid IS NULL OR e.id = $3::uuid)
		GROUP BY e.id, s.date
		ORDER BY e.id, s.date
	`, from, to, employeeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]EmployeeDailyHoursRow, 0)
	for rows.Next() {
		var row EmployeeDailyHoursRow
		if err := rows.Scan(&row.EmployeeID, &row.Date, &row.ActiveSeconds); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// ListCurrentBaseSalaries returns each employee's newest salary row that is
// already effective on asOf (the period end), so a raise dated next month
// does not show up on this month's evaluation.
func (r *CompensationPolicyRepository) ListCurrentBaseSalaries(ctx context.Context, asOf time.Time, employeeID *string) ([]EmployeeBaseSalaryRow, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT DISTINCT ON (employee_id) employee_id::text, base_salary
		FROM salaries
		WHERE ($1::uuid IS NULL OR employee_id = $1::uuid)
		  AND effective_date <= $2::date
		ORDER BY employee_id, effective_date DESC, created_at DESC
	`, employeeID, asOf)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]EmployeeBaseSalaryRow, 0)
	for rows.Next() {
		var row EmployeeBaseSalaryRow
		if err := rows.Scan(&row.EmployeeID, &row.BaseSalaryCipher); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
