package rbac

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func SeedDefaults(ctx context.Context, db *pgxpool.Pool) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin rbac seed transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	roleIDs := map[string]string{}
	for _, role := range DefaultRoles() {
		var roleID string
		roleKey := role.Name
		if role.Module != "" {
			roleKey = role.Name + ":" + role.Module
		}

		query := `
			INSERT INTO roles (name, description, module)
			VALUES ($1, $2, NULLIF($3, ''))
			ON CONFLICT ON CONSTRAINT uq_roles_name_module
			DO UPDATE SET description = EXCLUDED.description
			RETURNING id::text
		`

		if err = tx.QueryRow(ctx, query, role.Name, role.Description, role.Module).Scan(&roleID); err != nil {
			return fmt.Errorf("upsert role %s: %w", roleKey, err)
		}

		roleIDs[roleKey] = roleID
	}

	permissionIDs := map[string]string{}
	for _, permission := range DefaultPermissions() {
		var permissionID string
		query := `
			INSERT INTO permissions (name, description, module, resource, action)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT ON CONSTRAINT uq_permissions_module_resource_action
			DO UPDATE SET name = EXCLUDED.name, description = EXCLUDED.description
			RETURNING id::text
		`

		if err = tx.QueryRow(
			ctx,
			query,
			permission.Name,
			permission.Description,
			permission.Module,
			permission.Resource,
			permission.Action,
		).Scan(&permissionID); err != nil {
			return fmt.Errorf("upsert permission %s: %w", permission.Name, err)
		}

		permissionIDs[permission.Name] = permissionID
	}

	rolePermissionQuery := `
		INSERT INTO role_permissions (role_id, permission_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`

	for _, role := range DefaultRoles() {
		roleKey := role.Name
		if role.Module != "" {
			roleKey = role.Name + ":" + role.Module
		}

		roleID, ok := roleIDs[roleKey]
		if !ok {
			return fmt.Errorf("role %s missing from seed map", roleKey)
		}

		// Clear existing role_permissions so revoked access is actually removed
		if _, err = tx.Exec(ctx, `DELETE FROM role_permissions WHERE role_id = $1::uuid`, roleID); err != nil {
			return fmt.Errorf("clear permissions for role %s: %w", roleKey, err)
		}

		for _, permissionName := range PermissionNamesForRole(role) {
			permissionID, permissionFound := permissionIDs[permissionName]
			if !permissionFound {
				return fmt.Errorf("permission %s missing from seed map", permissionName)
			}

			if _, err = tx.Exec(ctx, rolePermissionQuery, roleID, permissionID); err != nil {
				return fmt.Errorf("assign permission %s to role %s: %w", permissionName, roleKey, err)
			}
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit rbac seed transaction: %w", err)
	}

	return nil
}
