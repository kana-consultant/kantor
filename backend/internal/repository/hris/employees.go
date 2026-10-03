package hris

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v5"

	"github.com/kana-consultant/kantor/backend/internal/model"
	repository "github.com/kana-consultant/kantor/backend/internal/repository"
	"github.com/kana-consultant/kantor/backend/internal/security"
)

var (
	ErrEmployeeNotFound        = errors.New("employee not found")
	ErrEmployeeEmailExists     = errors.New("employee email already exists")
	ErrEmployeeUserAlreadyUsed = errors.New("user account is already linked to another employee")
	ErrEmployeeAvatarNotFound  = errors.New("employee avatar not found")
	// ErrEmployeeHasDocuments: a payslip or an employment contract (both ON
	// DELETE RESTRICT) still references the employee.
	ErrEmployeeHasDocuments = errors.New("employee has payslips or contracts")
)

type EmployeesRepository struct {
	db repository.DBTX
	// encrypter seals employees.bank_account_encrypted (see bank_account.go).
	encrypter *security.Encrypter
	// clearBankAccountPlaintext is the BANK_ACCOUNT_CLEAR_PLAINTEXT opt-in:
	// false (default) writes the plaintext column next to the ciphertext.
	clearBankAccountPlaintext bool
}

type ListEmployeesParams struct {
	Page             int
	PerPage          int
	Search           string
	Department       string
	EmploymentStatus string
}

type UpsertEmployeeParams struct {
	FullName          string
	Email             string
	Phone             *string
	Position          string
	Department        *string
	DateJoined        time.Time
	EmploymentStatus  string
	Address           *string
	EmergencyContact  *string
	AvatarURL         *string
	BankAccountNumber *string
	BankName          *string
	LinkedInProfile   *string
	SSHKeys           *string
	// KeepStoredBankAccount (updates only) leaves the stored bank account
	// columns as they are instead of writing BankAccountNumber: used when
	// the stored number cannot be decrypted and the caller did not submit a
	// new one, so a routine save never discards it.
	KeepStoredBankAccount bool
}

func NewEmployeesRepository(db repository.DBTX, encrypter *security.Encrypter) *EmployeesRepository {
	return &EmployeesRepository{db: db, encrypter: encrypter}
}

// employeeColumns is the column list scanEmployee reads. Both bank account
// columns are selected: the ciphertext and the plaintext (see
// openBankAccount for which one a read uses).
const employeeColumns = `id::text, user_id::text, full_name, email, phone, position, department, date_joined, employment_status, address, emergency_contact, avatar_url, bank_account_number, bank_account_encrypted, bank_name, linkedin_profile, ssh_keys, created_at, updated_at`

func (r *EmployeesRepository) scanEmployee(row pgx.Row) (model.Employee, error) {
	var employee model.Employee
	var bankAccountPlain, bankAccountSealed *string
	if err := row.Scan(
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
		&bankAccountPlain,
		&bankAccountSealed,
		&employee.BankName,
		&employee.LinkedInProfile,
		&employee.SSHKeys,
		&employee.CreatedAt,
		&employee.UpdatedAt,
	); err != nil {
		return model.Employee{}, err
	}
	employee.BankAccountNumber, employee.BankAccountUnreadable = readBankAccount(r.encrypter, employee.ID, bankAccountPlain, bankAccountSealed)
	return employee, nil
}

func (r *EmployeesRepository) CreateEmployee(ctx context.Context, params UpsertEmployeeParams) (model.Employee, error) {
	bankAccount, err := sealBankAccount(r.encrypter, params.BankAccountNumber)
	if err != nil {
		return model.Employee{}, err
	}

	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	query := `
		INSERT INTO employees (
			full_name,
			email,
			phone,
			position,
			department,
			date_joined,
			employment_status,
			address,
			emergency_contact,
			avatar_url,
			bank_account_encrypted,
			bank_name,
			linkedin_profile,
			ssh_keys,
			bank_account_number
		)
		VALUES (
			$1,
			$2,
			NULLIF($3, ''),
			$4,
			NULLIF($5, ''),
			$6::date,
			$7,
			NULLIF($8, ''),
			NULLIF($9, ''),
			NULLIF($10, ''),
			$11,
			NULLIF($12, ''),
			NULLIF($13, ''),
			NULLIF($14, ''),
			$15
		)
		RETURNING ` + employeeColumns

	employee, err := r.scanEmployee(repository.DB(ctx, r.db).QueryRow(
		ctx,
		query,
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
		bankAccount,
		nullableString(params.BankName),
		nullableString(params.LinkedInProfile),
		nullableString(params.SSHKeys),
		r.plaintextBankAccount(params.BankAccountNumber),
	))
	if err != nil {
		return model.Employee{}, mapEmployeeDBError(err)
	}

	return employee, nil
}

func (r *EmployeesRepository) ListEmployees(ctx context.Context, params ListEmployeesParams) ([]model.Employee, int64, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
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
	if err := repository.DB(ctx, r.db).QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (params.Page - 1) * params.PerPage
	listQuery := fmt.Sprintf(`
		SELECT `+employeeColumns+`
		FROM employees
		WHERE %s
		ORDER BY full_name ASC, created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, index, index+1)
	args = append(args, params.PerPage, offset)

	rows, err := repository.DB(ctx, r.db).Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	employees := make([]model.Employee, 0)
	for rows.Next() {
		employee, err := r.scanEmployee(rows)
		if err != nil {
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
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	query := `
		SELECT ` + employeeColumns + `
		FROM employees
		WHERE id = $1::uuid
	`

	employee, err := r.scanEmployee(repository.DB(ctx, r.db).QueryRow(ctx, query, employeeID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Employee{}, ErrEmployeeNotFound
		}

		return model.Employee{}, err
	}

	return employee, nil
}

func (r *EmployeesRepository) GetEmployeeByUserID(ctx context.Context, userID string) (model.Employee, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	query := `
		SELECT ` + employeeColumns + `
		FROM employees
		WHERE user_id = $1::uuid
	`

	employee, err := r.scanEmployee(repository.DB(ctx, r.db).QueryRow(ctx, query, userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Employee{}, ErrEmployeeNotFound
		}
		return model.Employee{}, err
	}

	return employee, nil
}

func (r *EmployeesRepository) UpdateEmployee(ctx context.Context, employeeID string, params UpsertEmployeeParams) (model.Employee, error) {
	bankAccount, err := sealBankAccount(r.encrypter, params.BankAccountNumber)
	if err != nil {
		return model.Employee{}, err
	}

	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	query := `
		UPDATE employees
		SET
			full_name = $2,
			email = $3,
			phone = NULLIF($4, ''),
			position = $5,
			department = NULLIF($6, ''),
			date_joined = $7::date,
			employment_status = $8,
			address = NULLIF($9, ''),
			emergency_contact = NULLIF($10, ''),
			avatar_url = NULLIF($11, ''),
			bank_account_encrypted = CASE WHEN $16::boolean THEN bank_account_encrypted ELSE $12 END,
			bank_account_number = CASE WHEN $16::boolean THEN bank_account_number ELSE $17 END,
			bank_name = NULLIF($13, ''),
			linkedin_profile = NULLIF($14, ''),
			ssh_keys = NULLIF($15, ''),
			updated_at = NOW()
		WHERE id = $1::uuid
		RETURNING ` + employeeColumns

	employee, err := r.scanEmployee(repository.DB(ctx, r.db).QueryRow(
		ctx,
		query,
		employeeID,
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
		bankAccount,
		nullableString(params.BankName),
		nullableString(params.LinkedInProfile),
		nullableString(params.SSHKeys),
		params.KeepStoredBankAccount,
		r.plaintextBankAccount(params.BankAccountNumber),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Employee{}, ErrEmployeeNotFound
		}

		return model.Employee{}, mapEmployeeDBError(err)
	}

	return employee, nil
}

func (r *EmployeesRepository) UpdateEmployeeAvatar(ctx context.Context, employeeID string, avatarURL *string) (model.Employee, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	query := `
		UPDATE employees
		SET
			avatar_url = NULLIF($2, ''),
			updated_at = NOW()
		WHERE id = $1::uuid
		RETURNING ` + employeeColumns

	employee, err := r.scanEmployee(repository.DB(ctx, r.db).QueryRow(ctx, query, employeeID, nullableString(avatarURL)))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Employee{}, ErrEmployeeNotFound
		}

		return model.Employee{}, err
	}

	return employee, nil
}

func (r *EmployeesRepository) DeleteEmployee(ctx context.Context, employeeID string) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	commandTag, err := repository.DB(ctx, r.db).Exec(ctx, `DELETE FROM employees WHERE id = $1::uuid`, employeeID)
	if err != nil {
		// payslips_employee_id_fkey / employment_contracts_employee_id_fkey
		// (and any other RESTRICT reference): legal records, 409.
		if isForeignKeyViolation(err) {
			return ErrEmployeeHasDocuments
		}
		return err
	}

	if commandTag.RowsAffected() == 0 {
		return ErrEmployeeNotFound
	}

	return nil
}

func (r *EmployeesRepository) RenameDepartmentReferences(ctx context.Context, oldName string, newName string) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	_, err := repository.DB(ctx, r.db).Exec(ctx, `UPDATE employees SET department = $2, updated_at = NOW() WHERE department = $1`, oldName, newName)
	return err
}

func (r *EmployeesRepository) ClearDepartmentReferences(ctx context.Context, departmentName string) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	_, err := repository.DB(ctx, r.db).Exec(ctx, `UPDATE employees SET department = NULL, updated_at = NOW() WHERE department = $1`, departmentName)
	return err
}

func (r *EmployeesRepository) UpdateEmployeeProfile(
	ctx context.Context,
	userID string,
	fullName string,
	phone *string,
	address *string,
	emergencyContact *string,
	avatarURL *string,
	bankAccountNumber *string,
	bankName *string,
	linkedInProfile *string,
	sshKeys *string,
	keepStoredBankAccount bool,
) (model.Employee, error) {
	sealedBankAccount, err := sealBankAccount(r.encrypter, bankAccountNumber)
	if err != nil {
		return model.Employee{}, err
	}

	query := `
		UPDATE employees
		SET
			full_name = $2,
			phone = NULLIF($3, ''),
			address = NULLIF($4, ''),
			emergency_contact = NULLIF($5, ''),
			avatar_url = NULLIF($6, ''),
			bank_account_encrypted = CASE WHEN $11::boolean THEN bank_account_encrypted ELSE $7 END,
			bank_account_number = CASE WHEN $11::boolean THEN bank_account_number ELSE $12 END,
			bank_name = NULLIF($8, ''),
			linkedin_profile = NULLIF($9, ''),
			ssh_keys = NULLIF($10, ''),
			updated_at = NOW()
		WHERE user_id = $1::uuid
		RETURNING ` + employeeColumns

	employee, err := r.scanEmployee(repository.DB(ctx, r.db).QueryRow(ctx, query,
		userID,
		fullName,
		normalizePhone(phone),
		nullableString(address),
		nullableString(emergencyContact),
		nullableString(avatarURL),
		sealedBankAccount,
		nullableString(bankName),
		nullableString(linkedInProfile),
		nullableString(sshKeys),
		keepStoredBankAccount,
		r.plaintextBankAccount(bankAccountNumber),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Employee{}, ErrEmployeeNotFound
		}
		return model.Employee{}, err
	}
	return employee, nil
}

func (r *EmployeesRepository) SyncUserFieldsToEmployee(ctx context.Context, userID string, fullName string, email string) error {
	_, err := repository.DB(ctx, r.db).Exec(ctx,
		`UPDATE employees SET full_name = $2, email = $3, updated_at = NOW() WHERE user_id = $1::uuid`,
		userID, fullName, strings.ToLower(strings.TrimSpace(email)))
	return err
}

func (r *EmployeesRepository) FindAvatarPath(ctx context.Context, employeeID string, filename string) (string, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var avatarPath *string
	err := repository.DB(ctx, r.db).QueryRow(ctx, `SELECT avatar_url FROM employees WHERE id = $1::uuid`, employeeID).Scan(&avatarPath)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrEmployeeNotFound
		}
		return "", err
	}

	if avatarPath == nil || strings.TrimSpace(*avatarPath) == "" {
		return "", ErrEmployeeAvatarNotFound
	}

	trimmed := filepath.ToSlash(strings.TrimSpace(*avatarPath))
	if pathBase(trimmed) != filename {
		return "", ErrEmployeeAvatarNotFound
	}

	return trimmed, nil
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

func normalizePhone(phone *string) string {
	if phone == nil {
		return ""
	}
	p := strings.TrimSpace(*phone)
	p = strings.ReplaceAll(p, " ", "")
	p = strings.ReplaceAll(p, "-", "")
	if strings.HasPrefix(p, "+") {
		p = p[1:]
	}
	if strings.HasPrefix(p, "08") {
		p = "62" + p[1:]
	}
	return p
}

func pathBase(value string) string {
	parts := strings.Split(value, "/")
	return parts[len(parts)-1]
}
