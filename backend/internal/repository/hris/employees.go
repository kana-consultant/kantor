package hris

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kana-consultant/kantor/backend/internal/model"
)

var (
	ErrEmployeeNotFound         = errors.New("employee not found")
	ErrEmployeeLinkedUserAbsent = errors.New("linked user account not found")
	ErrEmployeeEmailExists      = errors.New("employee email already exists")
	ErrEmployeeUserAlreadyUsed  = errors.New("user account is already linked to another employee")
)

type EmployeesRepository struct {
	db *pgxpool.Pool
}

type ListEmployeesParams struct {
	Page             int
	PerPage          int
	Search           string
	Department       string
	EmploymentStatus string
}

type UpsertEmployeeParams struct {
	UserID           *string
	FullName         string
	Email            string
	Phone            *string
	Position         string
	Department       *string
	DateJoined       time.Time
	EmploymentStatus string
	Address          *string
	EmergencyContact *string
	AvatarURL        *string
}

func NewEmployeesRepository(db *pgxpool.Pool) *EmployeesRepository {
	return &EmployeesRepository{db: db}
}

func (r *EmployeesRepository) CreateEmployee(ctx context.Context, params UpsertEmployeeParams) (model.Employee, error) {
	if err := r.ensureLinkedUserExists(ctx, params.UserID); err != nil {
		return model.Employee{}, err
	}

	query := `
		INSERT INTO employees (
			user_id,
			full_name,
			email,
			phone,
			position,
			department,
			date_joined,
			employment_status,
			address,
			emergency_contact,
			avatar_url
		)
		VALUES (
			$1::uuid,
			$2,
			$3,
			NULLIF($4, ''),
			$5,
			NULLIF($6, ''),
			$7::date,
			$8,
			NULLIF($9, ''),
			NULLIF($10, ''),
			NULLIF($11, '')
		)
		RETURNING id::text, user_id::text, full_name, email, phone, position, department, date_joined, employment_status, address, emergency_contact, avatar_url, created_at, updated_at
	`

	var employee model.Employee
	err := r.db.QueryRow(
		ctx,
		query,
		nullableUUID(params.UserID),
		params.FullName,
		strings.ToLower(strings.TrimSpace(params.Email)),
		nullableString(params.Phone),
		params.Position,
		nullableString(params.Department),
		params.DateJoined,
		params.EmploymentStatus,
		nullableString(params.Address),
		nullableString(params.EmergencyContact),
		nullableString(params.AvatarURL),
	).Scan(
		&employee.ID,
		&employee.UserID,
		&employee.FullName,
		&employee.Email,
		&employee.Phone,
		&employee.Position,
		&employee.Department,
		&employee.DateJoined,
		&employee.EmploymentStatus,
		&employee.Address,
		&employee.EmergencyContact,
		&employee.AvatarURL,
		&employee.CreatedAt,
		&employee.UpdatedAt,
	)
	if err != nil {
		return model.Employee{}, mapEmployeeDBError(err)
	}

	return employee, nil
}

func (r *EmployeesRepository) ListEmployees(ctx context.Context, params ListEmployeesParams) ([]model.Employee, int64, error) {
	filters := []string{"1=1"}
	args := make([]interface{}, 0)
	index := 1

	if search := strings.TrimSpace(params.Search); search != "" {
		filters = append(filters, fmt.Sprintf("employees.full_name ILIKE $%d", index))
		args = append(args, "%"+search+"%")
		index++
	}

	if department := strings.TrimSpace(params.Department); department != "" {
		filters = append(filters, fmt.Sprintf("employees.department = $%d", index))
		args = append(args, department)
		index++
	}

	if status := strings.TrimSpace(params.EmploymentStatus); status != "" {
		filters = append(filters, fmt.Sprintf("employees.employment_status = $%d", index))
		args = append(args, status)
		index++
	}

	whereClause := strings.Join(filters, " AND ")

	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM employees WHERE %s`, whereClause)
	var total int64
	if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (params.Page - 1) * params.PerPage
	listQuery := fmt.Sprintf(`
		SELECT id::text, user_id::text, full_name, email, phone, position, department, date_joined, employment_status, address, emergency_contact, avatar_url, created_at, updated_at
		FROM employees
		WHERE %s
		ORDER BY full_name ASC, created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, index, index+1)
	args = append(args, params.PerPage, offset)

	rows, err := r.db.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	employees := make([]model.Employee, 0)
	for rows.Next() {
		var employee model.Employee
		if err := rows.Scan(
			&employee.ID,
			&employee.UserID,
			&employee.FullName,
			&employee.Email,
			&employee.Phone,
			&employee.Position,
			&employee.Department,
			&employee.DateJoined,
			&employee.EmploymentStatus,
			&employee.Address,
			&employee.EmergencyContact,
			&employee.AvatarURL,
			&employee.CreatedAt,
			&employee.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		employees = append(employees, employee)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return employees, total, nil
}

func (r *EmployeesRepository) GetEmployeeByID(ctx context.Context, employeeID string) (model.Employee, error) {
	query := `
		SELECT id::text, user_id::text, full_name, email, phone, position, department, date_joined, employment_status, address, emergency_contact, avatar_url, created_at, updated_at
		FROM employees
		WHERE id = $1::uuid
	`

	var employee model.Employee
	err := r.db.QueryRow(ctx, query, employeeID).Scan(
		&employee.ID,
		&employee.UserID,
		&employee.FullName,
		&employee.Email,
		&employee.Phone,
		&employee.Position,
		&employee.Department,
		&employee.DateJoined,
		&employee.EmploymentStatus,
		&employee.Address,
		&employee.EmergencyContact,
		&employee.AvatarURL,
		&employee.CreatedAt,
		&employee.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Employee{}, ErrEmployeeNotFound
		}

		return model.Employee{}, err
	}

	return employee, nil
}

func (r *EmployeesRepository) GetEmployeeByUserID(ctx context.Context, userID string) (model.Employee, error) {
	query := `
		SELECT id::text, user_id::text, full_name, email, phone, position, department, date_joined, employment_status, address, emergency_contact, avatar_url, created_at, updated_at
		FROM employees
		WHERE user_id = $1::uuid
	`

	var employee model.Employee
	err := r.db.QueryRow(ctx, query, userID).Scan(
		&employee.ID,
		&employee.UserID,
		&employee.FullName,
		&employee.Email,
		&employee.Phone,
		&employee.Position,
		&employee.Department,
		&employee.DateJoined,
		&employee.EmploymentStatus,
		&employee.Address,
		&employee.EmergencyContact,
		&employee.AvatarURL,
		&employee.CreatedAt,
		&employee.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Employee{}, ErrEmployeeNotFound
		}
		return model.Employee{}, err
	}

	return employee, nil
}

func (r *EmployeesRepository) UpdateEmployee(ctx context.Context, employeeID string, params UpsertEmployeeParams) (model.Employee, error) {
	if err := r.ensureLinkedUserExists(ctx, params.UserID); err != nil {
		return model.Employee{}, err
	}

	query := `
		UPDATE employees
		SET
			user_id = $2::uuid,
			full_name = $3,
			email = $4,
			phone = NULLIF($5, ''),
			position = $6,
			department = NULLIF($7, ''),
			date_joined = $8::date,
			employment_status = $9,
			address = NULLIF($10, ''),
			emergency_contact = NULLIF($11, ''),
			avatar_url = NULLIF($12, ''),
			updated_at = NOW()
		WHERE id = $1::uuid
		RETURNING id::text, user_id::text, full_name, email, phone, position, department, date_joined, employment_status, address, emergency_contact, avatar_url, created_at, updated_at
	`

	var employee model.Employee
	err := r.db.QueryRow(
		ctx,
		query,
		employeeID,
		nullableUUID(params.UserID),
		params.FullName,
		strings.ToLower(strings.TrimSpace(params.Email)),
		nullableString(params.Phone),
		params.Position,
		nullableString(params.Department),
		params.DateJoined,
		params.EmploymentStatus,
		nullableString(params.Address),
		nullableString(params.EmergencyContact),
		nullableString(params.AvatarURL),
	).Scan(
		&employee.ID,
		&employee.UserID,
		&employee.FullName,
		&employee.Email,
		&employee.Phone,
		&employee.Position,
		&employee.Department,
		&employee.DateJoined,
		&employee.EmploymentStatus,
		&employee.Address,
		&employee.EmergencyContact,
		&employee.AvatarURL,
		&employee.CreatedAt,
		&employee.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Employee{}, ErrEmployeeNotFound
		}

		return model.Employee{}, mapEmployeeDBError(err)
	}

	return employee, nil
}

func (r *EmployeesRepository) DeleteEmployee(ctx context.Context, employeeID string) error {
	commandTag, err := r.db.Exec(ctx, `DELETE FROM employees WHERE id = $1::uuid`, employeeID)
	if err != nil {
		return err
	}

	if commandTag.RowsAffected() == 0 {
		return ErrEmployeeNotFound
	}

	return nil
}

func (r *EmployeesRepository) RenameDepartmentReferences(ctx context.Context, oldName string, newName string) error {
	_, err := r.db.Exec(ctx, `UPDATE employees SET department = $2, updated_at = NOW() WHERE department = $1`, oldName, newName)
	return err
}

func (r *EmployeesRepository) ClearDepartmentReferences(ctx context.Context, departmentName string) error {
	_, err := r.db.Exec(ctx, `UPDATE employees SET department = NULL, updated_at = NOW() WHERE department = $1`, departmentName)
	return err
}

func (r *EmployeesRepository) ensureLinkedUserExists(ctx context.Context, userID *string) error {
	if userID == nil || strings.TrimSpace(*userID) == "" {
		return nil
	}

	var exists bool
	if err := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id = $1::uuid)`, strings.TrimSpace(*userID)).Scan(&exists); err != nil {
		return err
	}

	if !exists {
		return ErrEmployeeLinkedUserAbsent
	}

	return nil
}

func mapEmployeeDBError(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}

	switch pgErr.ConstraintName {
	case "uq_employees_email":
		return ErrEmployeeEmailExists
	case "employees_user_id_key":
		return ErrEmployeeUserAlreadyUsed
	}

	return err
}

func nullableUUID(value *string) interface{} {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil
	}

	return strings.TrimSpace(*value)
}

func nullableString(value *string) string {
	if value == nil {
		return ""
	}

	return strings.TrimSpace(*value)
}
