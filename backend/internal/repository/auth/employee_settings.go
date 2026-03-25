package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/kana-consultant/kantor/backend/internal/model"
	"github.com/kana-consultant/kantor/backend/internal/repository"
)

func (r *Repository) EnsureEmployeeProfileForUser(ctx context.Context, userID string) (model.Employee, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return model.Employee{}, ErrNotFound
	}

	tx, err := repository.DB(ctx, r.db).Begin(ctx)
	if err != nil {
		return model.Employee{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var user model.User
	if err := tx.QueryRow(ctx, `
		SELECT id::text, email, password_hash, full_name, avatar_url, department, skills, is_active, is_super_admin, failed_login_attempts, locked_until, created_at, updated_at
		FROM users
		WHERE id = $1::uuid
	`, userID).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.FullName,
		&user.AvatarURL,
		&user.Department,
		&user.Skills,
		&user.IsActive,
		&user.IsSuperAdmin,
		&user.FailedLoginAttempts,
		&user.LockedUntil,
		&user.CreatedAt,
		&user.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Employee{}, ErrNotFound
		}
		return model.Employee{}, err
	}

	var current model.Employee
	if err := tx.QueryRow(ctx, `
		SELECT id::text, user_id::text, full_name, email, phone, position, department, date_joined, employment_status, address, emergency_contact, avatar_url, bank_account_number, bank_name, linkedin_profile, ssh_keys, created_at, updated_at
		FROM employees
		WHERE user_id = $1::uuid
	`, userID).Scan(
		&current.ID,
		&current.UserID,
		&current.FullName,
		&current.Email,
		&current.Phone,
		&current.Position,
		&current.Department,
		&current.DateJoined,
		&current.EmploymentStatus,
		&current.Address,
		&current.EmergencyContact,
		&current.AvatarURL,
		&current.BankAccountNumber,
		&current.BankName,
		&current.LinkedInProfile,
		&current.SSHKeys,
		&current.CreatedAt,
		&current.UpdatedAt,
	); err == nil {
		if err := tx.Commit(ctx); err != nil {
			return model.Employee{}, err
		}
		return current, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return model.Employee{}, err
	}

	defaultDepartmentName, err := r.resolveAutoCreateEmployeeDefaultDepartmentName(ctx, tx)
	if err != nil {
		return model.Employee{}, err
	}

	if err := r.ensureEmployeeForUserWithDefaultDepartment(ctx, tx, user, defaultDepartmentName); err != nil {
		return model.Employee{}, err
	}

	var employee model.Employee
	if err := tx.QueryRow(ctx, `
		SELECT id::text, user_id::text, full_name, email, phone, position, department, date_joined, employment_status, address, emergency_contact, avatar_url, bank_account_number, bank_name, linkedin_profile, ssh_keys, created_at, updated_at
		FROM employees
		WHERE user_id = $1::uuid
	`, userID).Scan(
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
		&employee.BankAccountNumber,
		&employee.BankName,
		&employee.LinkedInProfile,
		&employee.SSHKeys,
		&employee.CreatedAt,
		&employee.UpdatedAt,
	); err != nil {
		return model.Employee{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return model.Employee{}, err
	}
	return employee, nil
}

// ensureEmployeeForUserForNewAccount links an existing unlinked employee (by email)
// or creates a new employee record when the system setting enables it.
func (r *Repository) ensureEmployeeForUserForNewAccount(ctx context.Context, tx pgx.Tx, user model.User) error {
	var settingsRaw []byte
	err := tx.QueryRow(ctx, `SELECT value FROM system_settings WHERE key = 'auto_create_employee'`).Scan(&settingsRaw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("load auto_create_employee setting: %w", err)
	}

	var settings struct {
		Enabled             bool    `json:"enabled"`
		DefaultDepartmentID *string `json:"default_department_id"`
	}
	if err := json.Unmarshal(settingsRaw, &settings); err != nil {
		return fmt.Errorf("parse auto_create_employee setting: %w", err)
	}
	if !settings.Enabled {
		return nil
	}

	defaultDepartmentName, err := r.resolveAutoCreateEmployeeDefaultDepartmentName(ctx, tx)
	if err != nil {
		return err
	}

	return r.ensureEmployeeForUserWithDefaultDepartment(ctx, tx, user, defaultDepartmentName)
}

func (r *Repository) resolveAutoCreateEmployeeDefaultDepartmentName(ctx context.Context, tx pgx.Tx) (*string, error) {
	var settingsRaw []byte
	err := tx.QueryRow(ctx, `SELECT value FROM system_settings WHERE key = 'auto_create_employee'`).Scan(&settingsRaw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("load auto_create_employee setting: %w", err)
	}

	var settings struct {
		DefaultDepartmentID *string `json:"default_department_id"`
	}
	if err := json.Unmarshal(settingsRaw, &settings); err != nil {
		return nil, fmt.Errorf("parse auto_create_employee setting: %w", err)
	}

	if settings.DefaultDepartmentID == nil || strings.TrimSpace(*settings.DefaultDepartmentID) == "" {
		return nil, nil
	}

	var name string
	if err := tx.QueryRow(ctx, `SELECT name FROM departments WHERE id = $1::uuid`, strings.TrimSpace(*settings.DefaultDepartmentID)).Scan(&name); err == nil {
		return &name, nil
	}

	return nil, nil
}

func (r *Repository) ensureEmployeeForUserWithDefaultDepartment(ctx context.Context, tx pgx.Tx, user model.User, defaultDepartmentName *string) error {
	linkQuery := `UPDATE employees SET user_id = $1::uuid, updated_at = NOW() WHERE email = $2 AND user_id IS NULL`
	tag, err := tx.Exec(ctx, linkQuery, user.ID, user.Email)
	if err != nil {
		return fmt.Errorf("link employee: %w", err)
	}
	if tag.RowsAffected() > 0 {
		_, _ = tx.Exec(ctx,
			`UPDATE users SET phone = e.phone FROM employees e WHERE e.user_id = $1::uuid AND users.id = $1::uuid AND e.phone IS NOT NULL AND e.phone != ''`,
			user.ID)
		return nil
	}

	createQuery := `
		INSERT INTO employees (user_id, full_name, email, position, department, date_joined, employment_status)
		VALUES ($1::uuid, $2, $3, 'Belum Ditentukan', NULLIF($4, ''), NOW()::date, 'active')
		ON CONFLICT (tenant_id, user_id) DO NOTHING
	`
	if _, err = tx.Exec(ctx, createQuery, user.ID, user.FullName, user.Email, nullableText(defaultDepartmentName)); err != nil {
		return fmt.Errorf("create employee for user: %w", err)
	}
	return nil
}
