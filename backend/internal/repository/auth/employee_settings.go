package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/kana-consultant/kantor/backend/internal/model"
)

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

	var defaultDepartmentName *string
	if settings.DefaultDepartmentID != nil && strings.TrimSpace(*settings.DefaultDepartmentID) != "" {
		var name string
		if err := tx.QueryRow(ctx, `SELECT name FROM departments WHERE id = $1::uuid`, strings.TrimSpace(*settings.DefaultDepartmentID)).Scan(&name); err == nil {
			defaultDepartmentName = &name
		}
	}

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
		ON CONFLICT (user_id) DO NOTHING
	`
	if _, err = tx.Exec(ctx, createQuery, user.ID, user.FullName, user.Email, nullableText(defaultDepartmentName)); err != nil {
		return fmt.Errorf("create employee for user: %w", err)
	}
	return nil
}
