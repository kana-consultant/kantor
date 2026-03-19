package hris

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kana-consultant/kantor/backend/internal/model"
	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

var (
	ErrDepartmentNotFound    = errors.New("department not found")
	ErrDepartmentNameExists  = errors.New("department name already exists")
	ErrDepartmentHeadMissing = errors.New("department head employee not found")
)

type DepartmentsRepository struct {
	db *pgxpool.Pool
}

type UpsertDepartmentParams struct {
	Name        string
	Description *string
	HeadID      *string
}

func NewDepartmentsRepository(db *pgxpool.Pool) *DepartmentsRepository {
	return &DepartmentsRepository{db: db}
}

func (r *DepartmentsRepository) CreateDepartment(ctx context.Context, params UpsertDepartmentParams) (model.Department, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	if err := r.ensureHeadEmployeeExists(ctx, params.HeadID); err != nil {
		return model.Department{}, err
	}

	query := `
		INSERT INTO departments (name, description, head_id)
		VALUES ($1, NULLIF($2, ''), $3::uuid)
		RETURNING id::text, name, description, head_id::text, created_at
	`

	var department model.Department
	err := r.db.QueryRow(ctx, query, strings.TrimSpace(params.Name), nullableString(params.Description), nullableUUID(params.HeadID)).Scan(
		&department.ID,
		&department.Name,
		&department.Description,
		&department.HeadID,
		&department.CreatedAt,
	)
	if err != nil {
		return model.Department{}, mapDepartmentDBError(err)
	}

	return r.GetDepartmentByID(ctx, department.ID)
}

func (r *DepartmentsRepository) ListDepartments(ctx context.Context) ([]model.Department, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	query := `
		SELECT
			departments.id::text,
			departments.name,
			departments.description,
			departments.head_id::text,
			employees.full_name,
			departments.created_at
		FROM departments
		LEFT JOIN employees ON employees.id = departments.head_id
		ORDER BY departments.name ASC
	`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	departments := make([]model.Department, 0)
	for rows.Next() {
		var department model.Department
		if err := rows.Scan(
			&department.ID,
			&department.Name,
			&department.Description,
			&department.HeadID,
			&department.HeadName,
			&department.CreatedAt,
		); err != nil {
			return nil, err
		}
		departments = append(departments, department)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return departments, nil
}

func (r *DepartmentsRepository) GetDepartmentByID(ctx context.Context, departmentID string) (model.Department, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	query := `
		SELECT
			departments.id::text,
			departments.name,
			departments.description,
			departments.head_id::text,
			employees.full_name,
			departments.created_at
		FROM departments
		LEFT JOIN employees ON employees.id = departments.head_id
		WHERE departments.id = $1::uuid
	`

	var department model.Department
	err := r.db.QueryRow(ctx, query, departmentID).Scan(
		&department.ID,
		&department.Name,
		&department.Description,
		&department.HeadID,
		&department.HeadName,
		&department.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Department{}, ErrDepartmentNotFound
		}

		return model.Department{}, err
	}

	return department, nil
}

func (r *DepartmentsRepository) UpdateDepartment(ctx context.Context, departmentID string, params UpsertDepartmentParams) (model.Department, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	if err := r.ensureHeadEmployeeExists(ctx, params.HeadID); err != nil {
		return model.Department{}, err
	}

	query := `
		UPDATE departments
		SET
			name = $2,
			description = NULLIF($3, ''),
			head_id = $4::uuid
		WHERE id = $1::uuid
		RETURNING id::text, name, description, head_id::text, created_at
	`

	var department model.Department
	err := r.db.QueryRow(ctx, query, departmentID, strings.TrimSpace(params.Name), nullableString(params.Description), nullableUUID(params.HeadID)).Scan(
		&department.ID,
		&department.Name,
		&department.Description,
		&department.HeadID,
		&department.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Department{}, ErrDepartmentNotFound
		}

		return model.Department{}, mapDepartmentDBError(err)
	}

	return r.GetDepartmentByID(ctx, department.ID)
}

func (r *DepartmentsRepository) DeleteDepartment(ctx context.Context, departmentID string) (string, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	var deletedName string
	err := r.db.QueryRow(ctx, `DELETE FROM departments WHERE id = $1::uuid RETURNING name`, departmentID).Scan(&deletedName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrDepartmentNotFound
		}

		return "", err
	}

	return deletedName, nil
}

func (r *DepartmentsRepository) ensureHeadEmployeeExists(ctx context.Context, headID *string) error {
	if headID == nil || strings.TrimSpace(*headID) == "" {
		return nil
	}

	var exists bool
	if err := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM employees WHERE id = $1::uuid)`, strings.TrimSpace(*headID)).Scan(&exists); err != nil {
		return err
	}

	if !exists {
		return ErrDepartmentHeadMissing
	}

	return nil
}

func mapDepartmentDBError(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}

	switch pgErr.ConstraintName {
	case "uq_departments_name":
		return ErrDepartmentNameExists
	}

	return err
}
