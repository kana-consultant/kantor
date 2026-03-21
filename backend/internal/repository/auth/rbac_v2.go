package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/kana-consultant/kantor/backend/internal/rbac"
)

func (r *Repository) GetDefaultRoleAssignments(ctx context.Context) ([]rbac.RoleKey, error) {
	var raw []byte
	if err := r.db.QueryRow(ctx, `SELECT value FROM system_settings WHERE key = 'default_roles'`).Scan(&raw); err != nil {
		return nil, err
	}

	var roleIDs map[string]*string
	if err := json.Unmarshal(raw, &roleIDs); err != nil {
		return nil, fmt.Errorf("parse default_roles setting: %w", err)
	}

	moduleIDs := make([]string, 0, len(roleIDs))
	for moduleID := range roleIDs {
		moduleIDs = append(moduleIDs, moduleID)
	}
	sort.Strings(moduleIDs)

	assignments := make([]rbac.RoleKey, 0, len(moduleIDs))
	for _, moduleID := range moduleIDs {
		roleID := roleIDs[moduleID]
		if roleID == nil || strings.TrimSpace(*roleID) == "" {
			continue
		}

		var roleSlug string
		if err := r.db.QueryRow(ctx, `SELECT slug FROM roles WHERE id = $1::uuid`, strings.TrimSpace(*roleID)).Scan(&roleSlug); err != nil {
			return nil, fmt.Errorf("resolve default role for module %s: %w", moduleID, err)
		}

		assignments = append(assignments, rbac.RoleKey{
			Name:   roleSlug,
			Module: moduleID,
		})
	}

	return assignments, nil
}

func (r *Repository) GetUserModuleRoles(ctx context.Context, userID string) (map[string]rbac.ModuleRole, error) {
	rows, err := r.db.Query(ctx, `
		SELECT umr.module_id, r.id::text, r.slug, r.name
		FROM user_module_roles umr
		INNER JOIN roles r ON r.id = umr.role_id
		WHERE umr.user_id = $1::uuid
			AND r.is_active = TRUE
		ORDER BY umr.module_id
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	moduleRoles := make(map[string]rbac.ModuleRole)
	for rows.Next() {
		var moduleID string
		var role rbac.ModuleRole
		if err := rows.Scan(&moduleID, &role.RoleID, &role.RoleSlug, &role.RoleName); err != nil {
			return nil, err
		}
		moduleRoles[moduleID] = role
	}

	return moduleRoles, rows.Err()
}

func (r *Repository) GetEffectivePermissions(ctx context.Context, userID string) ([]string, error) {
	rows, err := r.db.Query(ctx, `
		SELECT DISTINCT p.id
		FROM user_module_roles umr
		INNER JOIN roles r ON r.id = umr.role_id
		INNER JOIN role_permissions rp ON rp.role_id = r.id
		INNER JOIN permissions p ON p.id = rp.permission_id
		WHERE umr.user_id = $1::uuid
			AND r.is_active = TRUE
			AND p.module_id = umr.module_id
		ORDER BY p.id
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	permissions := make([]string, 0)
	for rows.Next() {
		var permission string
		if err := rows.Scan(&permission); err != nil {
			return nil, err
		}
		permissions = append(permissions, permission)
	}

	return permissions, rows.Err()
}
