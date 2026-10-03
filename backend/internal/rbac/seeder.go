package rbac

import (
	"context"
	"encoding/json"
	"errors"
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

	roleIDs, insertedRoles, err := seedRoles(ctx, tx)
	if err != nil {
		return err
	}

	if err = seedRolePermissions(ctx, tx, roleIDs, insertedRoles); err != nil {
		return err
	}

	if err = ensureBaselinePermissions(ctx, tx, roleIDs); err != nil {
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
		WHERE (modules.name, modules.description, modules.display_order)
			IS DISTINCT FROM (EXCLUDED.name, EXCLUDED.description, EXCLUDED.display_order)
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
		WHERE (permissions.module_id, permissions.resource, permissions.action, permissions.description, permissions.is_sensitive)
			IS DISTINCT FROM (EXCLUDED.module_id, EXCLUDED.resource, EXCLUDED.action, EXCLUDED.description, EXCLUDED.is_sensitive)
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

// seedRoles inserts the system roles a tenant does not have yet and returns
// the id of every system role, plus the slugs inserted by this call. An
// existing role row is never written: admins may change the description and
// hierarchy level of a system role (PUT /admin/roles/{id} locks only name
// and slug), and a start must not reset them.
func seedRoles(ctx context.Context, tx pgx.Tx) (map[string]string, map[string]bool, error) {
	insertQuery := `
		INSERT INTO roles (name, slug, description, is_system, hierarchy_level)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (tenant_id, slug) DO NOTHING
		RETURNING id::text
	`
	selectQuery := `
		SELECT id::text FROM roles
		WHERE tenant_id = current_setting('app.current_tenant')::uuid AND slug = $1
	`

	roleIDs := make(map[string]string, len(SystemRoles()))
	inserted := make(map[string]bool, len(SystemRoles()))
	for _, role := range SystemRoles() {
		var roleID string
		err := tx.QueryRow(ctx, insertQuery, role.Name, role.Slug, role.Description, role.IsSystem, role.HierarchyLevel).Scan(&roleID)
		switch {
		case err == nil:
			inserted[role.Slug] = true
		case errors.Is(err, pgx.ErrNoRows):
			if err := tx.QueryRow(ctx, selectQuery, role.Slug).Scan(&roleID); err != nil {
				return nil, nil, fmt.Errorf("find role %s: %w", role.Slug, err)
			}
		default:
			return nil, nil, fmt.Errorf("insert role %s: %w", role.Slug, err)
		}
		roleIDs[role.Slug] = roleID
	}

	return roleIDs, inserted, nil
}

// seedRolePermissions grants the default permissions to the system roles
// inserted by this start (see seedRoles). Existing roles keep the grants
// their admins chose, an emptied one included; permissions added in a later
// release reach them through ensureBaselinePermissions.
func seedRolePermissions(ctx context.Context, tx pgx.Tx, roleIDs map[string]string, insertedRoles map[string]bool) error {
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
		if !insertedRoles[role.Slug] {
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

var baselinePermissionVersions = map[int][]string{
	1: {
		"hris:compensation_policy:view",
		"hris:compensation_policy:manage",
		"hris:salary_safety:view",
	},
	// v2: HR documents. SystemRolePermissionIDs keeps these for Admin only
	// (all are IsSensitive and in managerExcludedPermissions).
	2: {
		"hris:employee_identity:view",
		"hris:employee_identity:edit",
		"hris:contract:view",
		"hris:contract:manage",
		"hris:contract:send",
		"hris:payslip:view",
		"hris:payslip:manage",
		"hris:payslip:send",
	},
}

const currentBaselineVersion = 2

func ensureBaselinePermissions(ctx context.Context, tx pgx.Tx, roleIDs map[string]string) error {
	// The applied version is the highest of the legacy marker
	// 'rbac_baseline_version' (written up to v1, and on fresh databases) and
	// the per-version markers 'rbac_baseline_v<N>'. Markers are insert-only:
	// an upgrade adds a new row instead of rewriting an existing one, and the
	// previous release, which reads only the legacy key, still sees its own
	// baseline as applied.
	var stored int
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX((value->>'version')::int), 0)
		FROM system_settings
		WHERE key = 'rbac_baseline_version'
			OR key ~ '^rbac_baseline_v[0-9]+$'
	`).Scan(&stored); err != nil {
		return fmt.Errorf("read rbac baseline version: %w", err)
	}
	if stored >= currentBaselineVersion {
		return nil
	}

	pending := make(map[string]struct{})
	for version := stored + 1; version <= currentBaselineVersion; version++ {
		for _, permissionID := range baselinePermissionVersions[version] {
			pending[permissionID] = struct{}{}
		}
	}

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

		for _, permissionID := range SystemRolePermissionIDs(role.Slug) {
			if _, isPending := pending[permissionID]; !isPending {
				continue
			}
			if _, err := tx.Exec(ctx, insertQuery, roleID, permissionID); err != nil {
				return fmt.Errorf("grant baseline permission %s to role %s: %w", permissionID, role.Slug, err)
			}
		}
	}

	// Insert-only: an existing legacy marker is left as it is (fresh
	// databases get one), and the new version is recorded as its own row.
	value := fmt.Sprintf(`{"version": %d}`, currentBaselineVersion)
	for _, key := range []string{"rbac_baseline_version", fmt.Sprintf("rbac_baseline_v%d", currentBaselineVersion)} {
		if _, err := tx.Exec(ctx, `
			INSERT INTO system_settings (key, value, description)
			VALUES ($1, $2::jsonb, 'Versi baseline permission system role yang sudah di-grant')
			ON CONFLICT (tenant_id, key) DO NOTHING
		`, key, value); err != nil {
			return fmt.Errorf("persist rbac baseline version: %w", err)
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

	registrationJSON, err := json.Marshal(map[string]any{
		"enabled":                false,
		"code_hash":              "",
		"code_expires_at":        nil,
		"last_rolled_by":         nil,
		"last_rolled_at":         nil,
		"rotation_interval_days": 7,
		"allowed_email_domains":  []string{},
	})
	if err != nil {
		return fmt.Errorf("marshal registration setting: %w", err)
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

	// Company profile used by generated documents (payslips, contracts).
	// Mirrors authrepo.DefaultCompanyProfileRecord; the tenant fills in the
	// legal details in Admin > Settings > Profil Perusahaan.
	companyProfileJSON, err := json.Marshal(map[string]any{
		"legal_name":        "",
		"address":           "",
		"business_type":     "",
		"city":              "",
		"signer_name":       "",
		"signer_title":      "",
		"hr_contact_email":  "",
		"doc_code":          "",
		"payday_day":        25,
		"annual_leave_days": 12,
		"logo_updated_at":   nil,
	})
	if err != nil {
		return fmt.Errorf("marshal company profile setting: %w", err)
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

	if _, err := tx.Exec(
		ctx,
		query,
		"registration",
		string(registrationJSON),
		"Konfigurasi self-registration: kode, domain allowlist, rotasi",
	); err != nil {
		return fmt.Errorf("seed registration setting: %w", err)
	}

	if _, err := tx.Exec(
		ctx,
		query,
		"company_profile",
		string(companyProfileJSON),
		"Profil perusahaan untuk dokumen (slip gaji, kontrak kerja)",
	); err != nil {
		return fmt.Errorf("seed company_profile setting: %w", err)
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
