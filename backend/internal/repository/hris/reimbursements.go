package hris

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/kana-consultant/kantor/backend/internal/model"
	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

var ErrReimbursementNotFound = errors.New("reimbursement not found")
var ErrReimbursementAttachmentNotFound = errors.New("reimbursement attachment not found")

type ReimbursementsRepository struct {
	db repository.DBTX
}

type CreateReimbursementParams struct {
	EmployeeID      string
	Title           string
	Category        string
	Amount          int64
	TransactionDate time.Time
	Description     string
	SubmittedBy     string
}

type ListReimbursementsParams struct {
	Page       int
	PerPage    int
	Status     string
	EmployeeID string
	Department string
	Month      int
	Year       int
}

type ReviewReimbursementParams struct {
	Decision string
	ActorID  string
	Notes    *string
}

func NewReimbursementsRepository(db repository.DBTX) *ReimbursementsRepository {
	return &ReimbursementsRepository{db: db}
}

func (r *ReimbursementsRepository) Create(ctx context.Context, params CreateReimbursementParams) (model.Reimbursement, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	query := `
		INSERT INTO reimbursements (employee_id, title, category, amount, transaction_date, description, submitted_by, status)
		VALUES ($1::uuid, $2, $3, $4, $5::date, $6, $7::uuid, 'submitted')
		RETURNING id::text, employee_id::text, title, category, amount, transaction_date, description, status, attachments, submitted_by::text, manager_id::text, manager_action_at, manager_notes, finance_id::text, finance_action_at, finance_notes, paid_at, created_at, updated_at
	`
	item, err := r.scanRow(ctx, repository.DB(ctx, r.db).QueryRow(
		ctx,
		query,
		params.EmployeeID,
		params.Title,
		params.Category,
		params.Amount,
		params.TransactionDate,
		params.Description,
		params.SubmittedBy,
	))
	if err != nil {
		return model.Reimbursement{}, err
	}
	return r.hydrateEmployeeName(ctx, item)
}

func (r *ReimbursementsRepository) List(ctx context.Context, params ListReimbursementsParams) ([]model.Reimbursement, int64, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	filters := []string{"1=1"}
	args := make([]interface{}, 0)
	index := 1

	if params.Status != "" {
		filters = append(filters, fmt.Sprintf("reimbursements.status = $%d", index))
		args = append(args, params.Status)
		index++
	}
	if params.EmployeeID != "" {
		filters = append(filters, fmt.Sprintf("reimbursements.employee_id = $%d::uuid", index))
		args = append(args, params.EmployeeID)
		index++
	}
	if params.Department != "" {
		filters = append(filters, fmt.Sprintf("employees.department = $%d", index))
		args = append(args, params.Department)
		index++
	}
	if params.Month > 0 {
		filters = append(filters, fmt.Sprintf("EXTRACT(MONTH FROM reimbursements.transaction_date) = $%d", index))
		args = append(args, params.Month)
		index++
	}
	if params.Year > 0 {
		filters = append(filters, fmt.Sprintf("EXTRACT(YEAR FROM reimbursements.transaction_date) = $%d", index))
		args = append(args, params.Year)
		index++
	}

	whereClause := strings.Join(filters, " AND ")
	countQuery := `
		SELECT COUNT(*)
		FROM reimbursements
		INNER JOIN employees ON employees.id = reimbursements.employee_id
		WHERE ` + whereClause
	var total int64
	if err := repository.DB(ctx, r.db).QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (params.Page - 1) * params.PerPage
	query := fmt.Sprintf(`
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
		WHERE %s
		ORDER BY reimbursements.created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, index, index+1)
	args = append(args, params.PerPage, offset)

	rows, err := repository.DB(ctx, r.db).Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := make([]model.Reimbursement, 0)
	for rows.Next() {
		item, err := scanReimbursement(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (r *ReimbursementsRepository) GetByID(ctx context.Context, reimbursementID string) (model.Reimbursement, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	query := `
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
		WHERE reimbursements.id = $1::uuid
	`
	item, err := scanReimbursementRow(repository.DB(ctx, r.db).QueryRow(ctx, query, reimbursementID))
	if err != nil {
		return model.Reimbursement{}, err
	}
	return item, nil
}

func (r *ReimbursementsRepository) FindAttachmentPath(ctx context.Context, reimbursementID string, filename string) (string, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	item, err := r.GetByID(ctx, reimbursementID)
	if err != nil {
		return "", err
	}

	target := filepath.Base(strings.TrimSpace(filename))
	for _, attachment := range item.Attachments {
		if filepath.Base(filepath.FromSlash(attachment)) == target {
			return attachment, nil
		}
	}

	return "", ErrReimbursementAttachmentNotFound
}

func (r *ReimbursementsRepository) AddAttachments(ctx context.Context, reimbursementID string, attachments []string) (model.Reimbursement, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	payload, err := json.Marshal(attachments)
	if err != nil {
		return model.Reimbursement{}, err
	}
	query := `
		UPDATE reimbursements
		SET attachments = COALESCE(attachments, '[]'::jsonb) || $2::jsonb,
			updated_at = NOW()
		WHERE id = $1::uuid
		RETURNING id::text, employee_id::text, title, category, amount, transaction_date, description, status, attachments, submitted_by::text, manager_id::text, manager_action_at, manager_notes, finance_id::text, finance_action_at, finance_notes, paid_at, created_at, updated_at
	`
	item, err := r.scanRow(ctx, repository.DB(ctx, r.db).QueryRow(ctx, query, reimbursementID, string(payload)))
	if err != nil {
		return model.Reimbursement{}, err
	}
	return r.hydrateEmployeeName(ctx, item)
}

func (r *ReimbursementsRepository) ApplyManagerReview(ctx context.Context, reimbursementID string, params ReviewReimbursementParams) (model.Reimbursement, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	nextStatus := "approved"
	if params.Decision == "rejected" {
		nextStatus = "rejected"
	}
	query := `
		UPDATE reimbursements
		SET status = $2,
			manager_id = $3::uuid,
			manager_action_at = NOW(),
			manager_notes = NULLIF($4, ''),
			updated_at = NOW()
		WHERE id = $1::uuid AND status = 'submitted'
		RETURNING id::text, employee_id::text, title, category, amount, transaction_date, description, status, attachments, submitted_by::text, manager_id::text, manager_action_at, manager_notes, finance_id::text, finance_action_at, finance_notes, paid_at, created_at, updated_at
	`
	item, err := r.scanRow(ctx, repository.DB(ctx, r.db).QueryRow(ctx, query, reimbursementID, nextStatus, params.ActorID, nullableText(params.Notes)))
	if err != nil {
		return model.Reimbursement{}, err
	}
	return r.hydrateEmployeeName(ctx, item)
}

func (r *ReimbursementsRepository) ApplyFinanceReview(ctx context.Context, reimbursementID string, params ReviewReimbursementParams) (model.Reimbursement, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	return r.ApplyManagerReview(ctx, reimbursementID, params)
}

func (r *ReimbursementsRepository) MarkPaid(ctx context.Context, reimbursementID string, actorID string, notes *string) (model.Reimbursement, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	query := `
		UPDATE reimbursements
		SET status = 'paid',
			finance_id = $2::uuid,
			finance_action_at = NOW(),
			finance_notes = COALESCE(NULLIF($3, ''), finance_notes),
			paid_at = NOW(),
			updated_at = NOW()
		WHERE id = $1::uuid AND status = 'approved'
		RETURNING id::text, employee_id::text, title, category, amount, transaction_date, description, status, attachments, submitted_by::text, manager_id::text, manager_action_at, manager_notes, finance_id::text, finance_action_at, finance_notes, paid_at, created_at, updated_at
	`
	item, err := r.scanRow(ctx, repository.DB(ctx, r.db).QueryRow(ctx, query, reimbursementID, actorID, nullableText(notes)))
	if err != nil {
		return model.Reimbursement{}, err
	}
	return r.hydrateEmployeeName(ctx, item)
}

func (r *ReimbursementsRepository) Summary(ctx context.Context, month int, year int, employeeID string, department string) (model.ReimbursementSummary, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	filters := []string{"EXTRACT(MONTH FROM reimbursements.transaction_date) = $1", "EXTRACT(YEAR FROM reimbursements.transaction_date) = $2"}
	args := []interface{}{month, year}
	index := 3

	if employeeID != "" {
		filters = append(filters, fmt.Sprintf("reimbursements.employee_id = $%d::uuid", index))
		args = append(args, employeeID)
		index++
	}
	if department != "" {
		filters = append(filters, fmt.Sprintf("employees.department = $%d", index))
		args = append(args, department)
	}

	whereClause := strings.Join(filters, " AND ")
	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT reimbursements.status, COUNT(*)
		FROM reimbursements
		INNER JOIN employees ON employees.id = reimbursements.employee_id
		WHERE `+whereClause+`
		GROUP BY reimbursements.status
	`, args...)
	if err != nil {
		return model.ReimbursementSummary{}, err
	}
	defer rows.Close()

	counts := map[string]int{
		"submitted": 0,
		"approved":  0,
		"rejected":  0,
		"paid":      0,
	}

	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return model.ReimbursementSummary{}, err
		}
		counts[status] = count
	}
	if err := rows.Err(); err != nil {
		return model.ReimbursementSummary{}, err
	}

	approvedQuery := `
		SELECT COALESCE(SUM(reimbursements.amount), 0)
		FROM reimbursements
		INNER JOIN employees ON employees.id = reimbursements.employee_id
		WHERE ` + whereClause + ` AND reimbursements.status IN ('approved', 'paid')
	`
	var approvedAmount int64
	if err := repository.DB(ctx, r.db).QueryRow(ctx, approvedQuery, args...).Scan(&approvedAmount); err != nil {
		return model.ReimbursementSummary{}, err
	}

	return model.ReimbursementSummary{
		Month:               month,
		Year:                year,
		CountsByStatus:      counts,
		ApprovedAmountMonth: approvedAmount,
	}, nil
}

func (r *ReimbursementsRepository) scanRow(ctx context.Context, row pgx.Row) (model.Reimbursement, error) {
	var item model.Reimbursement
	var attachmentsJSON []byte
	err := row.Scan(
		&item.ID,
		&item.EmployeeID,
		&item.Title,
		&item.Category,
		&item.Amount,
		&item.TransactionDate,
		&item.Description,
		&item.Status,
		&attachmentsJSON,
		&item.SubmittedBy,
		&item.ManagerID,
		&item.ManagerActionAt,
		&item.ManagerNotes,
		&item.FinanceID,
		&item.FinanceActionAt,
		&item.FinanceNotes,
		&item.PaidAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Reimbursement{}, ErrReimbursementNotFound
		}
		return model.Reimbursement{}, err
	}
	if len(attachmentsJSON) > 0 {
		if err := json.Unmarshal(attachmentsJSON, &item.Attachments); err != nil {
			return model.Reimbursement{}, err
		}
	}
	return item, nil
}

func scanReimbursement(rows pgx.Rows) (model.Reimbursement, error) {
	var item model.Reimbursement
	var attachmentsJSON []byte
	err := rows.Scan(
		&item.ID,
		&item.EmployeeID,
		&item.EmployeeName,
		&item.Title,
		&item.Category,
		&item.Amount,
		&item.TransactionDate,
		&item.Description,
		&item.Status,
		&attachmentsJSON,
		&item.SubmittedBy,
		&item.ManagerID,
		&item.ManagerActionAt,
		&item.ManagerNotes,
		&item.FinanceID,
		&item.FinanceActionAt,
		&item.FinanceNotes,
		&item.PaidAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return model.Reimbursement{}, err
	}
	if len(attachmentsJSON) > 0 {
		if err := json.Unmarshal(attachmentsJSON, &item.Attachments); err != nil {
			return model.Reimbursement{}, err
		}
	}
	return item, nil
}

func scanReimbursementRow(row pgx.Row) (model.Reimbursement, error) {
	var item model.Reimbursement
	var attachmentsJSON []byte
	err := row.Scan(
		&item.ID,
		&item.EmployeeID,
		&item.EmployeeName,
		&item.Title,
		&item.Category,
		&item.Amount,
		&item.TransactionDate,
		&item.Description,
		&item.Status,
		&attachmentsJSON,
		&item.SubmittedBy,
		&item.ManagerID,
		&item.ManagerActionAt,
		&item.ManagerNotes,
		&item.FinanceID,
		&item.FinanceActionAt,
		&item.FinanceNotes,
		&item.PaidAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Reimbursement{}, ErrReimbursementNotFound
		}
		return model.Reimbursement{}, err
	}
	if len(attachmentsJSON) > 0 {
		if err := json.Unmarshal(attachmentsJSON, &item.Attachments); err != nil {
			return model.Reimbursement{}, err
		}
	}
	return item, nil
}

func (r *ReimbursementsRepository) hydrateEmployeeName(ctx context.Context, item model.Reimbursement) (model.Reimbursement, error) {
	if err := repository.DB(ctx, r.db).QueryRow(ctx, `SELECT full_name FROM employees WHERE id = $1::uuid`, item.EmployeeID).Scan(&item.EmployeeName); err != nil {
		return model.Reimbursement{}, err
	}
	return item, nil
}

func nullableText(value *string) interface{} {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil
	}
	return strings.TrimSpace(*value)
}
