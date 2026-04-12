package rbac

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

func SeedDefaults(ctx context.Context, db repository.DBTX) error {
	tx, err := repository.DB(ctx, db).Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin rbac seed transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	if err = seedModules(ctx, tx); err != nil {
		return err
	}

	if err = seedPermissions(ctx, tx); err != nil {
		return err
	}

	roleIDs, err := seedRoles(ctx, tx)
	if err != nil {
		return err
	}

	if err = seedRolePermissions(ctx, tx, roleIDs); err != nil {
		return err
	}

	if err = seedSettings(ctx, tx, roleIDs); err != nil {
		return err
	}

	if err = migrateDeprecatedAssignments(ctx, tx, roleIDs); err != nil {
		return err
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit rbac seed transaction: %w", err)
	}

	return nil
}

func seedModules(ctx context.Context, tx pgx.Tx) error {
	query := `
		INSERT INTO modules (id, name, description, display_order)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (id)
		DO UPDATE SET
			name = EXCLUDED.name,
			description = EXCLUDED.description,
			display_order = EXCLUDED.display_order
	`

	for _, module := range Modules() {
		if _, err := tx.Exec(ctx, query, module.ID, module.Name, module.Description, module.DisplayOrder); err != nil {
			return fmt.Errorf("upsert module %s: %w", module.ID, err)
		}
	}

	return nil
}

func seedPermissions(ctx context.Context, tx pgx.Tx) error {
	query := `
		INSERT INTO permissions (id, module_id, resource, action, description, is_sensitive)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (id)
		DO UPDATE SET
			module_id = EXCLUDED.module_id,
			resource = EXCLUDED.resource,
			action = EXCLUDED.action,
			description = EXCLUDED.description,
			is_sensitive = EXCLUDED.is_sensitive
	`

	for _, permission := range DefaultPermissions() {
		if _, err := tx.Exec(
			ctx,
			query,
			permission.ID,
			permission.ModuleID,
			permission.Resource,
			permission.Action,
			permission.Description,
			permission.IsSensitive,
		); err != nil {
			return fmt.Errorf("upsert permission %s: %w", permission.ID, err)
		}
	}

	return nil
}

func seedRoles(ctx context.Context, tx pgx.Tx) (map[string]string, error) {
	query := `
		INSERT INTO roles (name, slug, description, is_system, hierarchy_level)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (tenant_id, slug)
		DO UPDATE SET
			description = EXCLUDED.description,
			is_system = EXCLUDED.is_system,
			hierarchy_level = EXCLUDED.hierarchy_level,
			updated_at = NOW()
		RETURNING id::text
	`

	roleIDs := make(map[string]string, len(SystemRoles()))
	for _, role := range SystemRoles() {
		var roleID string
		if err := tx.QueryRow(ctx, query, role.Name, role.Slug, role.Description, role.IsSystem, role.HierarchyLevel).Scan(&roleID); err != nil {
			return nil, fmt.Errorf("upsert role %s: %w", role.Slug, err)
		}
		roleIDs[role.Slug] = roleID
	}

	return roleIDs, nil
}

func seedRolePermissions(ctx context.Context, tx pgx.Tx, roleIDs map[string]string) error {
	countQuery := `SELECT COUNT(*) FROM role_permissions WHERE role_id = $1::uuid`
	insertQuery := `
		INSERT INTO role_permissions (role_id, permission_id)
		VALUES ($1::uuid, $2)
		ON CONFLICT DO NOTHING
	`

	for _, role := range SystemRoles() {
		if role.Slug == RoleSuperAdmin {
			continue
		}

		roleID, ok := roleIDs[role.Slug]
		if !ok {
			return fmt.Errorf("system role %s missing from seed map", role.Slug)
		}

		var count int
		if err := tx.QueryRow(ctx, countQuery, roleID).Scan(&count); err != nil {
			return fmt.Errorf("count permissions for role %s: %w", role.Slug, err)
		}
		if count > 0 {
			continue
		}

		for _, permissionID := range SystemRolePermissionIDs(role.Slug) {
			if _, err := tx.Exec(ctx, insertQuery, roleID, permissionID); err != nil {
				return fmt.Errorf("assign permission %s to role %s: %w", permissionID, role.Slug, err)
			}
		}
	}

	return nil
}

func seedSettings(ctx context.Context, tx pgx.Tx, roleIDs map[string]string) error {
	defaultRoles := map[string]string{
		ModuleOperational: roleIDs[RoleViewer],
		ModuleHRIS:        roleIDs[RoleViewer],
		ModuleMarketing:   roleIDs[RoleViewer],
	}

	defaultRolesJSON, err := json.Marshal(defaultRoles)
	if err != nil {
		return fmt.Errorf("marshal default roles setting: %w", err)
	}

	autoCreateEmployeeJSON, err := json.Marshal(map[string]any{
		"enabled":               true,
		"default_department_id": nil,
	})
	if err != nil {
		return fmt.Errorf("marshal auto create employee setting: %w", err)
	}

	mailDeliveryJSON, err := json.Marshal(map[string]any{
		"enabled":                       false,
		"provider":                      "resend",
		"sender_name":                   "",
		"sender_email":                  "",
		"reply_to_email":                nil,
		"api_key_encrypted":             "",
		"password_reset_enabled":        false,
		"password_reset_expiry_minutes": 30,
		"notification_enabled":          false,
	})
	if err != nil {
		return fmt.Errorf("marshal mail delivery setting: %w", err)
	}

	reimbursementReminderJSON, err := json.Marshal(map[string]any{
		"enabled": false,
		"review": map[string]any{
			"enabled": true,
			"cron":    "0 9 * * 1-5",
			"channels": map[string]any{
				"in_app":   true,
				"email":    false,
				"whatsapp": false,
			},
		},
		"payment": map[string]any{
			"enabled": true,
			"cron":    "0 10 * * 1-5",
			"channels": map[string]any{
				"in_app":   true,
				"email":    false,
				"whatsapp": false,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("marshal reimbursement reminder setting: %w", err)
	}

	query := `
		INSERT INTO system_settings (key, value, description)
		VALUES ($1, $2::jsonb, $3)
		ON CONFLICT (tenant_id, key) DO NOTHING
	`

	if _, err := tx.Exec(
		ctx,
		query,
		"default_roles",
		string(defaultRolesJSON),
		"Default role per module untuk user baru saat register",
	); err != nil {
		return fmt.Errorf("seed default_roles setting: %w", err)
	}

	if _, err := tx.Exec(
		ctx,
		query,
		"auto_create_employee",
		string(autoCreateEmployeeJSON),
		"Otomatis buat record employee saat user baru register",
	); err != nil {
		return fmt.Errorf("seed auto_create_employee setting: %w", err)
	}

	if _, err := tx.Exec(
		ctx,
		query,
		"mail_delivery",
		string(mailDeliveryJSON),
		"Konfigurasi pengiriman email tenant",
	); err != nil {
		return fmt.Errorf("seed mail_delivery setting: %w", err)
	}

	if _, err := tx.Exec(
		ctx,
		query,
		"reimbursement_reminder",
		string(reimbursementReminderJSON),
		"Konfigurasi reminder reimbursement tenant",
	); err != nil {
		return fmt.Errorf("seed reimbursement_reminder setting: %w", err)
	}

	return nil
}

func migrateDeprecatedAssignments(ctx context.Context, tx pgx.Tx, roleIDs map[string]string) error {
	var deprecatedExists bool
	if err := tx.QueryRow(ctx, `SELECT to_regclass('public.roles_deprecated') IS NOT NULL`).Scan(&deprecatedExists); err != nil {
		return fmt.Errorf("check deprecated roles table: %w", err)
	}
	if !deprecatedExists {
		return nil
	}

	if _, err := tx.Exec(ctx, `
		UPDATE users
		SET is_super_admin = TRUE
		WHERE EXISTS (
			SELECT 1
			FROM user_roles_deprecated ur
			INNER JOIN roles_deprecated r ON r.id = ur.role_id
			WHERE ur.user_id = users.id
				AND r.name = 'super_admin'
		)
	`); err != nil {
		return fmt.Errorf("migrate super admin flags: %w", err)
	}

	supportedSlugs := ReservedRoleSlugs()
	insertQuery := `
		INSERT INTO user_module_roles (user_id, module_id, role_id, assigned_at)
		SELECT
			ur.user_id,
			r_old.module,
			r_new.id,
			COALESCE(ur.assigned_at, NOW())
		FROM user_roles_deprecated ur
		INNER JOIN roles_deprecated r_old ON r_old.id = ur.role_id
		INNER JOIN roles r_new ON r_new.slug = r_old.name
		WHERE COALESCE(r_old.module, '') <> ''
			AND r_old.name = ANY($1::text[])
		ON CONFLICT (tenant_id, user_id, module_id) DO NOTHING
	`

	if _, err := tx.Exec(ctx, insertQuery, supportedSlugs); err != nil {
		return fmt.Errorf("migrate module role assignments: %w", err)
	}

	return nil
}

func IsReservedRoleSlug(slug string) bool {
	return slices.Contains(ReservedRoleSlugs(), slug)
}
