package hris

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

var (
	ErrSalaryNotFound = errors.New("salary record not found")
	ErrBonusNotFound  = errors.New("bonus record not found")
)

type CompensationRepository struct {
	db *pgxpool.Pool
}

type SalaryRow struct {
	ID            string
	EmployeeID    string
	BaseSalary    string
	Allowances    string
	Deductions    string
	NetSalary     string
	EffectiveDate time.Time
	CreatedBy     string
	CreatedAt     time.Time
}

type BonusRow struct {
	ID             string
	EmployeeID     string
	Amount         string
	Reason         string
	PeriodMonth    int
	PeriodYear     int
	ApprovalStatus string
	ApprovedBy     *string
	ApprovedAt     *time.Time
	CreatedBy      string
	CreatedAt      time.Time
}

type CreateSalaryParams struct {
	EmployeeID    string
	BaseSalary    string
	Allowances    string
	Deductions    string
	NetSalary     string
	EffectiveDate time.Time
	CreatedBy     string
}

type CreateBonusParams struct {
	EmployeeID  string
	Amount      string
	Reason      string
	PeriodMonth int
	PeriodYear  int
	CreatedBy   string
}

type UpdateBonusParams struct {
	Amount      string
	Reason      string
	PeriodMonth int
	PeriodYear  int
}

func NewCompensationRepository(db *pgxpool.Pool) *CompensationRepository {
	return &CompensationRepository{db: db}
}

func (r *CompensationRepository) CreateSalary(ctx context.Context, params CreateSalaryParams) (SalaryRow, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	query := `
		INSERT INTO salaries (employee_id, base_salary, allowances, deductions, net_salary, effective_date, created_by)
		VALUES ($1::uuid, $2, $3, $4, $5, $6::date, $7::uuid)
		RETURNING id::text, employee_id::text, base_salary, allowances, deductions, net_salary, effective_date, created_by::text, created_at
	`

	var row SalaryRow
	err := r.db.QueryRow(ctx, query, params.EmployeeID, params.BaseSalary, params.Allowances, params.Deductions, params.NetSalary, params.EffectiveDate, params.CreatedBy).Scan(
		&row.ID,
		&row.EmployeeID,
		&row.BaseSalary,
		&row.Allowances,
		&row.Deductions,
		&row.NetSalary,
		&row.EffectiveDate,
		&row.CreatedBy,
		&row.CreatedAt,
	)
	return row, err
}

func (r *CompensationRepository) ListSalaries(ctx context.Context, employeeID string) ([]SalaryRow, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	rows, err := r.db.Query(ctx, `
		SELECT id::text, employee_id::text, base_salary, allowances, deductions, net_salary, effective_date, created_by::text, created_at
		FROM salaries
		WHERE employee_id = $1::uuid
		ORDER BY effective_date DESC, created_at DESC
	`, employeeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]SalaryRow, 0)
	for rows.Next() {
		var row SalaryRow
		if err := rows.Scan(
			&row.ID,
			&row.EmployeeID,
			&row.BaseSalary,
			&row.Allowances,
			&row.Deductions,
			&row.NetSalary,
			&row.EffectiveDate,
			&row.CreatedBy,
			&row.CreatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (r *CompensationRepository) GetCurrentSalary(ctx context.Context, employeeID string) (SalaryRow, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	var row SalaryRow
	err := r.db.QueryRow(ctx, `
		SELECT id::text, employee_id::text, base_salary, allowances, deductions, net_salary, effective_date, created_by::text, created_at
		FROM salaries
		WHERE employee_id = $1::uuid
		ORDER BY effective_date DESC, created_at DESC
		LIMIT 1
	`, employeeID).Scan(
		&row.ID,
		&row.EmployeeID,
		&row.BaseSalary,
		&row.Allowances,
		&row.Deductions,
		&row.NetSalary,
		&row.EffectiveDate,
		&row.CreatedBy,
		&row.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return SalaryRow{}, ErrSalaryNotFound
	}
	return row, err
}

func (r *CompensationRepository) CreateBonus(ctx context.Context, params CreateBonusParams) (BonusRow, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	query := `
		INSERT INTO bonuses (employee_id, amount, reason, period_month, period_year, created_by)
		VALUES ($1::uuid, $2, $3, $4, $5, $6::uuid)
		RETURNING id::text, employee_id::text, amount, reason, period_month, period_year, approval_status, approved_by::text, approved_at, created_by::text, created_at
	`

	var row BonusRow
	err := r.db.QueryRow(ctx, query, params.EmployeeID, params.Amount, params.Reason, params.PeriodMonth, params.PeriodYear, params.CreatedBy).Scan(
		&row.ID,
		&row.EmployeeID,
		&row.Amount,
		&row.Reason,
		&row.PeriodMonth,
		&row.PeriodYear,
		&row.ApprovalStatus,
		&row.ApprovedBy,
		&row.ApprovedAt,
		&row.CreatedBy,
		&row.CreatedAt,
	)
	return row, err
}

func (r *CompensationRepository) ListBonuses(ctx context.Context, employeeID string) ([]BonusRow, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	rows, err := r.db.Query(ctx, `
		SELECT id::text, employee_id::text, amount, reason, period_month, period_year, approval_status, approved_by::text, approved_at, created_by::text, created_at
		FROM bonuses
		WHERE employee_id = $1::uuid
		ORDER BY period_year DESC, period_month DESC, created_at DESC
	`, employeeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]BonusRow, 0)
	for rows.Next() {
		var row BonusRow
		if err := rows.Scan(
			&row.ID,
			&row.EmployeeID,
			&row.Amount,
			&row.Reason,
			&row.PeriodMonth,
			&row.PeriodYear,
			&row.ApprovalStatus,
			&row.ApprovedBy,
			&row.ApprovedAt,
			&row.CreatedBy,
			&row.CreatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (r *CompensationRepository) GetBonusByID(ctx context.Context, bonusID string) (BonusRow, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	var row BonusRow
	err := r.db.QueryRow(ctx, `
		SELECT id::text, employee_id::text, amount, reason, period_month, period_year, approval_status, approved_by::text, approved_at, created_by::text, created_at
		FROM bonuses
		WHERE id = $1::uuid
	`, bonusID).Scan(
		&row.ID,
		&row.EmployeeID,
		&row.Amount,
		&row.Reason,
		&row.PeriodMonth,
		&row.PeriodYear,
		&row.ApprovalStatus,
		&row.ApprovedBy,
		&row.ApprovedAt,
		&row.CreatedBy,
		&row.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return BonusRow{}, ErrBonusNotFound
	}
	return row, err
}

func (r *CompensationRepository) UpdateBonus(ctx context.Context, bonusID string, params UpdateBonusParams) (BonusRow, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	var row BonusRow
	err := r.db.QueryRow(ctx, `
		UPDATE bonuses
		SET amount = $2, reason = $3, period_month = $4, period_year = $5
		WHERE id = $1::uuid
		RETURNING id::text, employee_id::text, amount, reason, period_month, period_year, approval_status, approved_by::text, approved_at, created_by::text, created_at
	`, bonusID, params.Amount, params.Reason, params.PeriodMonth, params.PeriodYear).Scan(
		&row.ID,
		&row.EmployeeID,
		&row.Amount,
		&row.Reason,
		&row.PeriodMonth,
		&row.PeriodYear,
		&row.ApprovalStatus,
		&row.ApprovedBy,
		&row.ApprovedAt,
		&row.CreatedBy,
		&row.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return BonusRow{}, ErrBonusNotFound
	}
	return row, err
}

func (r *CompensationRepository) UpdateBonusApprovalStatus(ctx context.Context, bonusID string, status string, approverID string) (BonusRow, error) {
	query := `
		UPDATE bonuses
		SET approval_status = $2, approved_by = $3::uuid, approved_at = NOW()
		WHERE id = $1::uuid
		RETURNING id::text, employee_id::text, amount, reason, period_month, period_year, approval_status, approved_by::text, approved_at, created_by::text, created_at
	`

	var row BonusRow
	err := r.db.QueryRow(ctx, query, bonusID, status, approverID).Scan(
		&row.ID,
		&row.EmployeeID,
		&row.Amount,
		&row.Reason,
		&row.PeriodMonth,
		&row.PeriodYear,
		&row.ApprovalStatus,
		&row.ApprovedBy,
		&row.ApprovedAt,
		&row.CreatedBy,
		&row.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return BonusRow{}, ErrBonusNotFound
	}
	return row, err
}

func (r *CompensationRepository) DeleteBonus(ctx context.Context, bonusID string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM bonuses WHERE id = $1::uuid`, bonusID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrBonusNotFound
	}
	return nil
}

func (r *CompensationRepository) LogSalaryAccess(ctx context.Context, userID string, employeeID string, action string) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO audit_logs (user_id, action, module, resource, resource_id, created_at)
		VALUES ($1::uuid, $2, 'hris', 'salary', $3, NOW())
	`, userID, action, employeeID)
	return err
}
