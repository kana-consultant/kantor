package hris

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
	ErrFinanceCategoryNotFound = errors.New("finance category not found")
	ErrFinanceRecordNotFound   = errors.New("finance record not found")
	ErrFinanceCategoryExists   = errors.New("finance category already exists")
)

type FinanceRepository struct {
	db repository.DBTX
}

type UpsertFinanceCategoryParams struct {
	Name string
	Type string
}

type UpsertFinanceRecordParams struct {
	CategoryID  string
	Type        string
	Amount      int64
	Description string
	RecordDate  time.Time
	SubmittedBy string
}

type ListFinanceRecordsParams struct {
	Page        int
	PerPage     int
	Type        string
	CategoryID  string
	Month       int
	Year        int
	Status      string
	SubmittedBy string
}

type ListFinanceExportParams struct {
	Year  int
	Month int
}

func NewFinanceRepository(db repository.DBTX) *FinanceRepository {
	return &FinanceRepository{db: db}
}

func (r *FinanceRepository) CreateCategory(ctx context.Context, params UpsertFinanceCategoryParams) (model.FinanceCategory, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	query := `
		INSERT INTO finance_categories (name, type)
		VALUES ($1, $2)
		RETURNING id::text, name, type, is_default, created_at
	`
	var item model.FinanceCategory
	err := repository.DB(ctx, r.db).QueryRow(ctx, query, strings.TrimSpace(params.Name), strings.TrimSpace(params.Type)).Scan(
		&item.ID,
		&item.Name,
		&item.Type,
		&item.IsDefault,
		&item.CreatedAt,
	)
	if err != nil {
		return model.FinanceCategory{}, mapFinanceError(err)
	}
	return item, nil
}

func (r *FinanceRepository) ListCategories(ctx context.Context, recordType string) ([]model.FinanceCategory, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	query := `
		SELECT id::text, name, type, is_default, created_at
		FROM finance_categories
		WHERE ($1 = '' OR type = $1)
		ORDER BY type ASC, name ASC
	`
	rows, err := repository.DB(ctx, r.db).Query(ctx, query, strings.TrimSpace(recordType))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.FinanceCategory, 0)
	for rows.Next() {
		var item model.FinanceCategory
		if err := rows.Scan(&item.ID, &item.Name, &item.Type, &item.IsDefault, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *FinanceRepository) UpdateCategory(ctx context.Context, categoryID string, params UpsertFinanceCategoryParams) (model.FinanceCategory, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	query := `
		UPDATE finance_categories
		SET name = $2, type = $3
		WHERE id = $1::uuid
		RETURNING id::text, name, type, is_default, created_at
	`
	var item model.FinanceCategory
	err := repository.DB(ctx, r.db).QueryRow(ctx, query, categoryID, strings.TrimSpace(params.Name), strings.TrimSpace(params.Type)).Scan(
		&item.ID,
		&item.Name,
		&item.Type,
		&item.IsDefault,
		&item.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.FinanceCategory{}, ErrFinanceCategoryNotFound
		}
		return model.FinanceCategory{}, mapFinanceError(err)
	}
	return item, nil
}

func (r *FinanceRepository) DeleteCategory(ctx context.Context, categoryID string) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	tag, err := repository.DB(ctx, r.db).Exec(ctx, `DELETE FROM finance_categories WHERE id = $1::uuid AND is_default = FALSE`, categoryID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrFinanceCategoryNotFound
	}
	return nil
}

func (r *FinanceRepository) CreateRecord(ctx context.Context, params UpsertFinanceRecordParams) (model.FinanceRecord, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	query := `
		INSERT INTO finance_records (category_id, type, amount, description, record_date, record_month, record_year, submitted_by)
		VALUES ($1::uuid, $2, $3, $4, $5::date, EXTRACT(MONTH FROM $5::date)::int, EXTRACT(YEAR FROM $5::date)::int, NULLIF($6, '')::uuid)
		RETURNING id::text, category_id::text, type, amount, description, record_date, record_month, record_year, approval_status, submitted_by::text, reviewed_by::text, reviewed_at, approved_by::text, approved_at, created_at, updated_at
	`
	record, err := r.scanRecordRow(ctx, repository.DB(ctx, r.db).QueryRow(ctx, query, params.CategoryID, params.Type, params.Amount, params.Description, params.RecordDate, params.SubmittedBy))
	if err != nil {
		return model.FinanceRecord{}, err
	}
	return r.hydrateCategoryName(ctx, record)
}

func (r *FinanceRepository) ListRecords(ctx context.Context, params ListFinanceRecordsParams) ([]model.FinanceRecord, int64, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	filters := []string{"1=1"}
	args := make([]interface{}, 0)
	index := 1

	if params.Type != "" {
		filters = append(filters, fmt.Sprintf("finance_records.type = $%d", index))
		args = append(args, params.Type)
		index++
	}
	if params.CategoryID != "" {
		filters = append(filters, fmt.Sprintf("finance_records.category_id = $%d::uuid", index))
		args = append(args, params.CategoryID)
		index++
	}
	if params.Month > 0 {
		filters = append(filters, fmt.Sprintf("finance_records.record_month = $%d", index))
		args = append(args, params.Month)
		index++
	}
	if params.Year > 0 {
		filters = append(filters, fmt.Sprintf("finance_records.record_year = $%d", index))
		args = append(args, params.Year)
		index++
	}
	if params.Status != "" {
		filters = append(filters, fmt.Sprintf("finance_records.approval_status = $%d", index))
		args = append(args, params.Status)
		index++
	}
	if params.SubmittedBy != "" {
		filters = append(filters, fmt.Sprintf("finance_records.submitted_by = $%d::uuid", index))
		args = append(args, params.SubmittedBy)
		index++
	}

	whereClause := strings.Join(filters, " AND ")
	var total int64
	if err := repository.DB(ctx, r.db).QueryRow(ctx, `SELECT COUNT(*) FROM finance_records WHERE `+whereClause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (params.Page - 1) * params.PerPage
	query := fmt.Sprintf(`
		SELECT
			finance_records.id::text,
			finance_records.category_id::text,
			finance_categories.name,
			finance_records.type,
			finance_records.amount,
			finance_records.description,
			finance_records.record_date,
			finance_records.record_month,
			finance_records.record_year,
			finance_records.approval_status,
			finance_records.submitted_by::text,
			finance_records.reviewed_by::text,
			finance_records.reviewed_at,
			finance_records.approved_by::text,
			finance_records.approved_at,
			finance_records.created_at,
			finance_records.updated_at
		FROM finance_records
		INNER JOIN finance_categories ON finance_categories.id = finance_records.category_id
		WHERE %s
		ORDER BY finance_records.record_date DESC, finance_records.created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, index, index+1)
	args = append(args, params.PerPage, offset)

	rows, err := repository.DB(ctx, r.db).Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := make([]model.FinanceRecord, 0)
	for rows.Next() {
		var item model.FinanceRecord
		if err := rows.Scan(
			&item.ID,
			&item.CategoryID,
			&item.CategoryName,
			&item.Type,
			&item.Amount,
			&item.Description,
			&item.RecordDate,
			&item.RecordMonth,
			&item.RecordYear,
			&item.ApprovalStatus,
			&item.SubmittedBy,
			&item.ReviewedBy,
			&item.ReviewedAt,
			&item.ApprovedBy,
			&item.ApprovedAt,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (r *FinanceRepository) GetRecordByID(ctx context.Context, recordID string) (model.FinanceRecord, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	query := `
		SELECT
			finance_records.id::text,
			finance_records.category_id::text,
			finance_categories.name,
			finance_records.type,
			finance_records.amount,
			finance_records.description,
			finance_records.record_date,
			finance_records.record_month,
			finance_records.record_year,
			finance_records.approval_status,
			finance_records.submitted_by::text,
			finance_records.reviewed_by::text,
			finance_records.reviewed_at,
			finance_records.approved_by::text,
			finance_records.approved_at,
			finance_records.created_at,
			finance_records.updated_at
		FROM finance_records
		INNER JOIN finance_categories ON finance_categories.id = finance_records.category_id
		WHERE finance_records.id = $1::uuid
	`
	var item model.FinanceRecord
	err := repository.DB(ctx, r.db).QueryRow(ctx, query, recordID).Scan(
		&item.ID,
		&item.CategoryID,
		&item.CategoryName,
		&item.Type,
		&item.Amount,
		&item.Description,
		&item.RecordDate,
		&item.RecordMonth,
		&item.RecordYear,
		&item.ApprovalStatus,
		&item.SubmittedBy,
		&item.ReviewedBy,
		&item.ReviewedAt,
		&item.ApprovedBy,
		&item.ApprovedAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.FinanceRecord{}, ErrFinanceRecordNotFound
		}
		return model.FinanceRecord{}, err
	}
	return item, nil
}

func (r *FinanceRepository) UpdateRecord(ctx context.Context, recordID string, params UpsertFinanceRecordParams) (model.FinanceRecord, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	query := `
		UPDATE finance_records
		SET category_id = $2::uuid,
			type = $3,
			amount = $4,
			description = $5,
			record_date = $6::date,
			record_month = EXTRACT(MONTH FROM $6::date)::int,
			record_year = EXTRACT(YEAR FROM $6::date)::int,
			updated_at = NOW()
		WHERE id = $1::uuid AND approval_status IN ('draft', 'rejected')
		RETURNING id::text, category_id::text, type, amount, description, record_date, record_month, record_year, approval_status, submitted_by::text, reviewed_by::text, reviewed_at, approved_by::text, approved_at, created_at, updated_at
	`
	record, err := r.scanRecordRow(ctx, repository.DB(ctx, r.db).QueryRow(ctx, query, recordID, params.CategoryID, params.Type, params.Amount, params.Description, params.RecordDate))
	if err != nil {
		return model.FinanceRecord{}, err
	}
	return r.hydrateCategoryName(ctx, record)
}

func (r *FinanceRepository) DeleteRecord(ctx context.Context, recordID string) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	tag, err := repository.DB(ctx, r.db).Exec(ctx, `DELETE FROM finance_records WHERE id = $1::uuid AND approval_status IN ('draft', 'rejected')`, recordID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrFinanceRecordNotFound
	}
	return nil
}

func (r *FinanceRepository) SubmitRecord(ctx context.Context, recordID string, actorID string) (model.FinanceRecord, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	query := `
		UPDATE finance_records
		SET approval_status = 'pending_review',
			submitted_by = $2::uuid,
			updated_at = NOW()
		WHERE id = $1::uuid AND approval_status = 'draft'
		RETURNING id::text, category_id::text, type, amount, description, record_date, record_month, record_year, approval_status, submitted_by::text, reviewed_by::text, reviewed_at, approved_by::text, approved_at, created_at, updated_at
	`
	record, err := r.scanRecordRow(ctx, repository.DB(ctx, r.db).QueryRow(ctx, query, recordID, actorID))
	if err != nil {
		return model.FinanceRecord{}, err
	}
	return r.hydrateCategoryName(ctx, record)
}

func (r *FinanceRepository) ReviewRecord(ctx context.Context, recordID string, decision string, actorID string) (model.FinanceRecord, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	query := `
		UPDATE finance_records
		SET approval_status = $2,
			reviewed_by = $3::uuid,
			reviewed_at = NOW(),
			approved_by = CASE WHEN $2 = 'approved' THEN $3::uuid ELSE NULL END,
			approved_at = CASE WHEN $2 = 'approved' THEN NOW() ELSE NULL END,
			updated_at = NOW()
		WHERE id = $1::uuid AND approval_status = 'pending_review'
		RETURNING id::text, category_id::text, type, amount, description, record_date, record_month, record_year, approval_status, submitted_by::text, reviewed_by::text, reviewed_at, approved_by::text, approved_at, created_at, updated_at
	`
	record, err := r.scanRecordRow(ctx, repository.DB(ctx, r.db).QueryRow(ctx, query, recordID, decision, actorID))
	if err != nil {
		return model.FinanceRecord{}, err
	}
	return r.hydrateCategoryName(ctx, record)
}

func (r *FinanceRepository) Summary(ctx context.Context, year int) (model.FinanceSummary, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT
			record_month,
			COALESCE(SUM(CASE WHEN type = 'income' AND approval_status = 'approved' THEN amount ELSE 0 END), 0) AS income,
			COALESCE(SUM(CASE WHEN type = 'outcome' AND approval_status = 'approved' THEN amount ELSE 0 END), 0) AS outcome
		FROM finance_records
		WHERE record_year = $1
		GROUP BY record_month
		ORDER BY record_month ASC
	`, year)
	if err != nil {
		return model.FinanceSummary{}, err
	}
	defer rows.Close()

	monthlyMap := map[int]model.FinanceSummaryMonth{}
	for rows.Next() {
		var item model.FinanceSummaryMonth
		if err := rows.Scan(&item.Month, &item.Income, &item.Outcome); err != nil {
			return model.FinanceSummary{}, err
		}
		monthlyMap[item.Month] = item
	}
	if err := rows.Err(); err != nil {
		return model.FinanceSummary{}, err
	}

	monthly := make([]model.FinanceSummaryMonth, 0, 12)
	var totalIncome int64
	var totalOutcome int64
	for month := 1; month <= 12; month++ {
		item, ok := monthlyMap[month]
		if !ok {
			item = model.FinanceSummaryMonth{Month: month}
		}
		totalIncome += item.Income
		totalOutcome += item.Outcome
		monthly = append(monthly, item)
	}

	byCategory := map[string]int64{}
	categoryRows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT finance_categories.name, COALESCE(SUM(finance_records.amount), 0)
		FROM finance_records
		INNER JOIN finance_categories ON finance_categories.id = finance_records.category_id
		WHERE finance_records.record_year = $1 AND finance_records.approval_status = 'approved'
		GROUP BY finance_categories.name
		ORDER BY finance_categories.name ASC
	`, year)
	if err != nil {
		return model.FinanceSummary{}, err
	}
	defer categoryRows.Close()

	for categoryRows.Next() {
		var name string
		var amount int64
		if err := categoryRows.Scan(&name, &amount); err != nil {
			return model.FinanceSummary{}, err
		}
		byCategory[name] = amount
	}
	if err := categoryRows.Err(); err != nil {
		return model.FinanceSummary{}, err
	}

	currentMonth := int(time.Now().Month())
	netThisMonth := int64(0)
	if time.Now().Year() == year {
		item := monthly[currentMonth-1]
		netThisMonth = item.Income - item.Outcome
	}

	return model.FinanceSummary{
		Year:               year,
		Monthly:            monthly,
		TotalIncome:        totalIncome,
		TotalOutcome:       totalOutcome,
		NetProfitThisMonth: netThisMonth,
		ByCategory:         byCategory,
	}, nil
}

func (r *FinanceRepository) ListForExport(ctx context.Context, params ListFinanceExportParams) ([]model.FinanceRecord, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	filters := []string{"finance_records.record_year = $1"}
	args := []interface{}{params.Year}
	index := 2
	if params.Month > 0 {
		filters = append(filters, fmt.Sprintf("finance_records.record_month = $%d", index))
		args = append(args, params.Month)
	}

	query := `
		SELECT
			finance_records.id::text,
			finance_records.category_id::text,
			finance_categories.name,
			finance_records.type,
			finance_records.amount,
			finance_records.description,
			finance_records.record_date,
			finance_records.record_month,
			finance_records.record_year,
			finance_records.approval_status,
			finance_records.submitted_by::text,
			finance_records.reviewed_by::text,
			finance_records.reviewed_at,
			finance_records.approved_by::text,
			finance_records.approved_at,
			finance_records.created_at,
			finance_records.updated_at
		FROM finance_records
		INNER JOIN finance_categories ON finance_categories.id = finance_records.category_id
		WHERE ` + strings.Join(filters, " AND ") + `
		ORDER BY finance_records.record_date DESC, finance_records.created_at DESC
	`
	rows, err := repository.DB(ctx, r.db).Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.FinanceRecord, 0)
	for rows.Next() {
		var item model.FinanceRecord
		if err := rows.Scan(
			&item.ID,
			&item.CategoryID,
			&item.CategoryName,
			&item.Type,
			&item.Amount,
			&item.Description,
			&item.RecordDate,
			&item.RecordMonth,
			&item.RecordYear,
			&item.ApprovalStatus,
			&item.SubmittedBy,
			&item.ReviewedBy,
			&item.ReviewedAt,
			&item.ApprovedBy,
			&item.ApprovedAt,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *FinanceRepository) scanRecordRow(ctx context.Context, row pgx.Row) (model.FinanceRecord, error) {
	var item model.FinanceRecord
	err := row.Scan(
		&item.ID,
		&item.CategoryID,
		&item.Type,
		&item.Amount,
		&item.Description,
		&item.RecordDate,
		&item.RecordMonth,
		&item.RecordYear,
		&item.ApprovalStatus,
		&item.SubmittedBy,
		&item.ReviewedBy,
		&item.ReviewedAt,
		&item.ApprovedBy,
		&item.ApprovedAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.FinanceRecord{}, ErrFinanceRecordNotFound
		}
		return model.FinanceRecord{}, err
	}
	return item, nil
}

func (r *FinanceRepository) hydrateCategoryName(ctx context.Context, record model.FinanceRecord) (model.FinanceRecord, error) {
	if err := repository.DB(ctx, r.db).QueryRow(ctx, `SELECT name FROM finance_categories WHERE id = $1::uuid`, record.CategoryID).Scan(&record.CategoryName); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.FinanceRecord{}, ErrFinanceCategoryNotFound
		}
		return model.FinanceRecord{}, err
	}
	return record, nil
}

func mapFinanceError(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	if pgErr.ConstraintName == "uq_finance_categories_name_type" {
		return ErrFinanceCategoryExists
	}
	return err
}
