package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v5"

	"github.com/kana-consultant/kantor/backend/internal/dto"
	"github.com/kana-consultant/kantor/backend/internal/model"
	"github.com/kana-consultant/kantor/backend/internal/rbac"
	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

var (
	ErrRoleNotFound        = errors.New("role not found")
	ErrRoleSlugExists      = errors.New("role slug already exists")
	ErrSystemRoleImmutable = errors.New("system role cannot be deleted")
	ErrRoleHasAssignments  = errors.New("role is still assigned to users")
	ErrReservedRoleSlug    = errors.New("role slug is reserved")
	ErrInvalidModuleRole   = errors.New("invalid module role assignment")
	ErrCannotToggleSelf    = errors.New("cannot toggle super admin status for current user")
)

type RoleListParams struct {
	Search   string
	IsSystem *bool
	IsActive *bool
}

type RoleListItem struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Slug            string `json:"slug"`
	Description     string `json:"description"`
	IsSystem        bool   `json:"is_system"`
	IsActive        bool   `json:"is_active"`
	HierarchyLevel  int    `json:"hierarchy_level"`
	PermissionCount int    `json:"permissions_count"`
	UserCount       int    `json:"users_count"`
}

type RoleDetail struct {
	RoleListItem
	PermissionIDs []string `json:"permission_ids"`
}

type UpsertRoleParams struct {
	Name           string
	Slug           string
	Description    string
	HierarchyLevel int
	PermissionIDs  []string
}

type PermissionItem struct {
	ID          string `json:"id"`
	Resource    string `json:"resource"`
	Action      string `json:"action"`
	Description string `json:"description"`
	IsSensitive bool   `json:"is_sensitive"`
}

type PermissionGroup struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Permissions []PermissionItem `json:"permissions"`
}

type ModuleItem struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	DisplayOrder int    `json:"display_order"`
}

type AdminUserSummary struct {
	User               model.User                 `json:"user"`
	ModuleRoles        map[string]rbac.ModuleRole `json:"module_roles"`
	IsSuperAdmin       bool                       `json:"is_super_admin"`
	HasEmployeeProfile bool                       `json:"has_employee_profile"`
	EmployeeID         *string                    `json:"employee_id,omitempty"`
}

type AdminUserDetail struct {
	User                 model.User                 `json:"user"`
	ModuleRoles          map[string]rbac.ModuleRole `json:"module_roles"`
	EffectivePermissions []string                   `json:"effective_permissions"`
	IsSuperAdmin         bool                       `json:"is_super_admin"`
	HasEmployeeProfile   bool                       `json:"has_employee_profile"`
	EmployeeID           *string                    `json:"employee_id,omitempty"`
}

type SettingsResponse struct {
	DefaultRoles       map[string]*RoleReference `json:"default_roles"`
	AutoCreateEmployee AutoCreateEmployeeSetting `json:"auto_create_employee"`
	MailDelivery       MailDeliverySetting       `json:"mail_delivery"`
}

type RoleReference struct {
	RoleID   *string `json:"role_id"`
	RoleName *string `json:"role_name"`
	RoleSlug *string `json:"role_slug"`
}

type AutoCreateEmployeeSetting struct {
	Enabled             bool    `json:"enabled"`
	DefaultDepartmentID *string `json:"default_department_id"`
}

type MailDeliverySetting struct {
	Enabled                    bool    `json:"enabled"`
	Provider                   string  `json:"provider"`
	SenderName                 string  `json:"sender_name"`
	SenderEmail                string  `json:"sender_email"`
	ReplyToEmail               *string `json:"reply_to_email,omitempty"`
	HasAPIKey                  bool    `json:"has_api_key"`
	PasswordResetEnabled       bool    `json:"password_reset_enabled"`
	PasswordResetExpiryMinutes int     `json:"password_reset_expiry_minutes"`
	NotificationEnabled        bool    `json:"notification_enabled"`
}

type MailDeliverySettingRecord struct {
	Enabled                    bool    `json:"enabled"`
	Provider                   string  `json:"provider"`
	SenderName                 string  `json:"sender_name"`
	SenderEmail                string  `json:"sender_email"`
	ReplyToEmail               *string `json:"reply_to_email,omitempty"`
	APIKeyEncrypted            string  `json:"api_key_encrypted,omitempty"`
	PasswordResetEnabled       bool    `json:"password_reset_enabled"`
	PasswordResetExpiryMinutes int     `json:"password_reset_expiry_minutes"`
	NotificationEnabled        bool    `json:"notification_enabled"`
}

type PublicAuthOptions struct {
	ForgotPasswordEnabled bool `json:"forgot_password_enabled"`
}

func (r *Repository) ListSettingsDepartments(ctx context.Context) ([]model.Department, error) {
	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT id::text, name, description, head_id::text, NULL::text, created_at
		FROM departments
		ORDER BY name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.Department, 0)
	for rows.Next() {
		var item model.Department
		if err := rows.Scan(
			&item.ID,
			&item.Name,
			&item.Description,
			&item.HeadID,
			&item.HeadName,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, rows.Err()
}

func (r *Repository) ListRoles(ctx context.Context, params RoleListParams) ([]RoleListItem, error) {
	filters := []string{"1=1"}
	args := make([]any, 0)
	index := 1

	if search := strings.TrimSpace(params.Search); search != "" {
		filters = append(filters, fmt.Sprintf("(roles.name ILIKE $%d OR roles.slug ILIKE $%d)", index, index))
		args = append(args, "%"+search+"%")
		index++
	}
	if params.IsSystem != nil {
		filters = append(filters, fmt.Sprintf("roles.is_system = $%d", index))
		args = append(args, *params.IsSystem)
		index++
	}
	if params.IsActive != nil {
		filters = append(filters, fmt.Sprintf("roles.is_active = $%d", index))
		args = append(args, *params.IsActive)
		index++
	}

	query := fmt.Sprintf(`
		SELECT
			roles.id::text,
			roles.name,
			roles.slug,
			COALESCE(roles.description, ''),
			roles.is_system,
			roles.is_active,
			roles.hierarchy_level,
			(SELECT COUNT(*) FROM role_permissions rp WHERE rp.role_id = roles.id)::int AS permissions_count,
			(SELECT COUNT(*) FROM user_module_roles umr WHERE umr.role_id = roles.id)::int AS users_count
		FROM roles
		WHERE %s
		ORDER BY roles.hierarchy_level DESC, roles.name ASC
	`, strings.Join(filters, " AND "))

	rows, err := repository.DB(ctx, r.db).Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]RoleListItem, 0)
	for rows.Next() {
		var item RoleListItem
		if err := rows.Scan(
			&item.ID,
			&item.Name,
			&item.Slug,
			&item.Description,
			&item.IsSystem,
			&item.IsActive,
			&item.HierarchyLevel,
			&item.PermissionCount,
			&item.UserCount,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, rows.Err()
}

func (r *Repository) GetRoleDetail(ctx context.Context, roleID string) (RoleDetail, error) {
	var detail RoleDetail
	err := repository.DB(ctx, r.db).QueryRow(ctx, `
		SELECT
			roles.id::text,
			roles.name,
			roles.slug,
			COALESCE(roles.description, ''),
			roles.is_system,
			roles.is_active,
			roles.hierarchy_level,
			(SELECT COUNT(*) FROM role_permissions rp WHERE rp.role_id = roles.id)::int AS permissions_count,
			(SELECT COUNT(*) FROM user_module_roles umr WHERE umr.role_id = roles.id)::int AS users_count
		FROM roles
		WHERE roles.id = $1::uuid
	`, roleID).Scan(
		&detail.ID,
		&detail.Name,
		&detail.Slug,
		&detail.Description,
		&detail.IsSystem,
		&detail.IsActive,
		&detail.HierarchyLevel,
		&detail.PermissionCount,
		&detail.UserCount,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RoleDetail{}, ErrRoleNotFound
		}
		return RoleDetail{}, err
	}

	rows, err := repository.DB(ctx, r.db).Query(ctx, `SELECT permission_id FROM role_permissions WHERE role_id = $1::uuid ORDER BY permission_id`, roleID)
	if err != nil {
		return RoleDetail{}, err
	}
	defer rows.Close()

	detail.PermissionIDs = make([]string, 0)
	for rows.Next() {
		var permissionID string
		if err := rows.Scan(&permissionID); err != nil {
			return RoleDetail{}, err
		}
		detail.PermissionIDs = append(detail.PermissionIDs, permissionID)
	}

	return detail, rows.Err()
}

func (r *Repository) CreateRole(ctx context.Context, params UpsertRoleParams, createdBy string) (RoleDetail, error) {
	if rbac.IsReservedRoleSlug(params.Slug) {
		return RoleDetail{}, ErrReservedRoleSlug
	}

	tx, err := repository.DB(ctx, r.db).Begin(ctx)
	if err != nil {
		return RoleDetail{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var roleID string
	err = tx.QueryRow(ctx, `
		INSERT INTO roles (name, slug, description, is_system, hierarchy_level, created_by)
		VALUES ($1, $2, NULLIF($3, ''), FALSE, $4, NULLIF($5, '')::uuid)
		RETURNING id::text
	`, params.Name, params.Slug, params.Description, params.HierarchyLevel, createdBy).Scan(&roleID)
	if err != nil {
		if isRoleSlugUniqueViolation(err) {
			return RoleDetail{}, ErrRoleSlugExists
		}
		return RoleDetail{}, err
	}

	if err := r.replaceRolePermissions(ctx, tx, roleID, params.PermissionIDs); err != nil {
		return RoleDetail{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return RoleDetail{}, err
	}

	return r.GetRoleDetail(ctx, roleID)
}

func (r *Repository) UpdateRole(ctx context.Context, roleID string, params UpsertRoleParams) (RoleDetail, error) {
	tx, err := repository.DB(ctx, r.db).Begin(ctx)
	if err != nil {
		return RoleDetail{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var existing struct {
		Name     string
		Slug     string
		IsSystem bool
	}
	if err := tx.QueryRow(ctx, `SELECT name, slug, is_system FROM roles WHERE id = $1::uuid`, roleID).Scan(&existing.Name, &existing.Slug, &existing.IsSystem); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RoleDetail{}, ErrRoleNotFound
		}
		return RoleDetail{}, err
	}

	name := params.Name
	slug := params.Slug
	if existing.IsSystem {
		name = existing.Name
		slug = existing.Slug
	} else if rbac.IsReservedRoleSlug(params.Slug) {
		return RoleDetail{}, ErrReservedRoleSlug
	}

	_, err = tx.Exec(ctx, `
		UPDATE roles
		SET name = $2,
			slug = $3,
			description = NULLIF($4, ''),
			hierarchy_level = $5,
			updated_at = NOW()
		WHERE id = $1::uuid
	`, roleID, name, slug, params.Description, params.HierarchyLevel)
	if err != nil {
		if isRoleSlugUniqueViolation(err) {
			return RoleDetail{}, ErrRoleSlugExists
		}
		return RoleDetail{}, err
	}

	if err := r.replaceRolePermissions(ctx, tx, roleID, params.PermissionIDs); err != nil {
		return RoleDetail{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return RoleDetail{}, err
	}

	return r.GetRoleDetail(ctx, roleID)
}

func (r *Repository) DeleteRole(ctx context.Context, roleID string) error {
	tx, err := repository.DB(ctx, r.db).Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var isSystem bool
	if err := tx.QueryRow(ctx, `SELECT is_system FROM roles WHERE id = $1::uuid`, roleID).Scan(&isSystem); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrRoleNotFound
		}
		return err
	}
	if isSystem {
		return ErrSystemRoleImmutable
	}

	var usageCount int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*)::int FROM user_module_roles WHERE role_id = $1::uuid`, roleID).Scan(&usageCount); err != nil {
		return err
	}
	if usageCount > 0 {
		return ErrRoleHasAssignments
	}

	if _, err := tx.Exec(ctx, `DELETE FROM roles WHERE id = $1::uuid`, roleID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *Repository) ToggleRole(ctx context.Context, roleID string) (RoleDetail, error) {
	tag, err := repository.DB(ctx, r.db).Exec(ctx, `UPDATE roles SET is_active = NOT is_active, updated_at = NOW() WHERE id = $1::uuid`, roleID)
	if err != nil {
		return RoleDetail{}, err
	}
	if tag.RowsAffected() == 0 {
		return RoleDetail{}, ErrRoleNotFound
	}
	return r.GetRoleDetail(ctx, roleID)
}

func (r *Repository) DuplicateRole(ctx context.Context, roleID string, createdBy string) (RoleDetail, error) {
	existing, err := r.GetRoleDetail(ctx, roleID)
	if err != nil {
		return RoleDetail{}, err
	}

	newSlug, err := r.nextRoleCopySlug(ctx, existing.Slug)
	if err != nil {
		return RoleDetail{}, err
	}

	return r.CreateRole(ctx, UpsertRoleParams{
		Name:           existing.Name + " (Copy)",
		Slug:           newSlug,
		Description:    existing.Description,
		HierarchyLevel: existing.HierarchyLevel,
		PermissionIDs:  existing.PermissionIDs,
	}, createdBy)
}

func (r *Repository) ListPermissionGroups(ctx context.Context) ([]PermissionGroup, error) {
	query := `
		SELECT modules.id, modules.name, modules.description, permissions.id, permissions.resource, permissions.action, COALESCE(permissions.description, ''), permissions.is_sensitive
		FROM modules
		LEFT JOIN permissions ON permissions.module_id = modules.id
		ORDER BY modules.display_order ASC, permissions.resource ASC, permissions.action ASC
	`
	rows, err := repository.DB(ctx, r.db).Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	groupMap := make(map[string]*PermissionGroup)
	order := make([]string, 0)
	for rows.Next() {
		var moduleID string
		var moduleName string
		var moduleDescription *string
		var permissionID *string
		var resource *string
		var action *string
		var description *string
		var isSensitive *bool
		if err := rows.Scan(&moduleID, &moduleName, &moduleDescription, &permissionID, &resource, &action, &description, &isSensitive); err != nil {
			return nil, err
		}

		group, exists := groupMap[moduleID]
		if !exists {
			group = &PermissionGroup{
				ID:          moduleID,
				Name:        moduleName,
				Description: derefString(moduleDescription),
				Permissions: make([]PermissionItem, 0),
			}
			groupMap[moduleID] = group
			order = append(order, moduleID)
		}

		if permissionID != nil {
			group.Permissions = append(group.Permissions, PermissionItem{
				ID:          derefString(permissionID),
				Resource:    derefString(resource),
				Action:      derefString(action),
				Description: derefString(description),
				IsSensitive: derefBool(isSensitive),
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := make([]PermissionGroup, 0, len(order))
	for _, moduleID := range order {
		result = append(result, *groupMap[moduleID])
	}
	return result, nil
}

func (r *Repository) ListModules(ctx context.Context) ([]ModuleItem, error) {
	rows, err := repository.DB(ctx, r.db).Query(ctx, `SELECT id, name, COALESCE(description, ''), display_order FROM modules ORDER BY display_order ASC, name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]ModuleItem, 0)
	for rows.Next() {
		var item ModuleItem
		if err := rows.Scan(&item.ID, &item.Name, &item.Description, &item.DisplayOrder); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) ListAdminUsers(ctx context.Context, params dto.ListUsersQuery) ([]AdminUserSummary, int64, error) {
	filters := []string{"1=1"}
	args := make([]any, 0)
	index := 1

	if search := strings.TrimSpace(params.Search); search != "" {
		filters = append(filters, fmt.Sprintf("(users.full_name ILIKE $%d OR users.email ILIKE $%d)", index, index))
		args = append(args, "%"+search+"%")
		index++
	}
	if params.SuperAdmin != nil {
		filters = append(filters, fmt.Sprintf("users.is_super_admin = $%d", index))
		args = append(args, *params.SuperAdmin)
		index++
	}
	if moduleID := strings.TrimSpace(params.ModuleID); moduleID != "" {
		filters = append(filters, fmt.Sprintf("EXISTS (SELECT 1 FROM user_module_roles umr WHERE umr.user_id = users.id AND umr.module_id = $%d)", index))
		args = append(args, moduleID)
		index++
	}
	if roleID := strings.TrimSpace(params.RoleID); roleID != "" {
		filters = append(filters, fmt.Sprintf("EXISTS (SELECT 1 FROM user_module_roles umr WHERE umr.user_id = users.id AND umr.role_id = $%d::uuid)", index))
		args = append(args, roleID)
		index++
	}

	whereClause := strings.Join(filters, " AND ")
	var total int64
	if err := repository.DB(ctx, r.db).QueryRow(ctx, "SELECT COUNT(*) FROM users WHERE "+whereClause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	page := params.Page
	if page <= 0 {
		page = 1
	}
	perPage := params.PerPage
	if perPage <= 0 {
		perPage = 20
	}

	query := fmt.Sprintf(`
		SELECT %s
		FROM users
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, userSelectColumns, whereClause, index, index+1)
	args = append(args, perPage, (page-1)*perPage)

	rows, err := repository.DB(ctx, r.db).Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	// Collect all users first, then close rows before issuing sub-queries.
	// pgx does not allow concurrent queries on the same connection.
	var users []model.User
	for rows.Next() {
		var user model.User
		if err := scanUser(rows, &user); err != nil {
			return nil, 0, err
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	rows.Close()

	items := make([]AdminUserSummary, 0, len(users))
	for _, user := range users {
		moduleRoles, err := r.GetUserModuleRoles(ctx, user.ID)
		if err != nil {
			return nil, 0, err
		}
		employeeID, err := r.getEmployeeIDByUserID(ctx, user.ID)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, AdminUserSummary{
			User:               user,
			ModuleRoles:        moduleRoles,
			IsSuperAdmin:       user.IsSuperAdmin,
			HasEmployeeProfile: employeeID != nil,
			EmployeeID:         employeeID,
		})
	}

	return items, total, nil
}

func (r *Repository) GetAdminUserDetail(ctx context.Context, userID string) (AdminUserDetail, error) {
	user, err := r.GetUserByID(ctx, userID)
	if err != nil {
		return AdminUserDetail{}, err
	}

	moduleRoles, err := r.GetUserModuleRoles(ctx, userID)
	if err != nil {
		return AdminUserDetail{}, err
	}

	permissions, err := r.GetEffectivePermissions(ctx, userID)
	if err != nil {
		return AdminUserDetail{}, err
	}
	employeeID, err := r.getEmployeeIDByUserID(ctx, userID)
	if err != nil {
		return AdminUserDetail{}, err
	}

	return AdminUserDetail{
		User:                 user,
		ModuleRoles:          moduleRoles,
		EffectivePermissions: permissions,
		IsSuperAdmin:         user.IsSuperAdmin,
		HasEmployeeProfile:   employeeID != nil,
		EmployeeID:           employeeID,
	}, nil
}

func (r *Repository) getEmployeeIDByUserID(ctx context.Context, userID string) (*string, error) {
	var employeeID string
	err := repository.DB(ctx, r.db).QueryRow(ctx,
		`SELECT id::text FROM employees WHERE user_id = $1::uuid LIMIT 1`,
		userID,
	).Scan(&employeeID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &employeeID, nil
}

func (r *Repository) ReplaceUserModuleRoles(ctx context.Context, userID string, moduleRoles []dto.SetUserModuleRoleRequest) error {
	tx, err := repository.DB(ctx, r.db).Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM user_module_roles WHERE user_id = $1::uuid`, userID); err != nil {
		return err
	}

	for _, assignment := range moduleRoles {
		if assignment.RoleID == nil || strings.TrimSpace(*assignment.RoleID) == "" {
			continue
		}

		var roleExists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM roles WHERE id = $1::uuid AND is_active = TRUE)`, strings.TrimSpace(*assignment.RoleID)).Scan(&roleExists); err != nil {
			return err
		}
		if !roleExists {
			return ErrInvalidModuleRole
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO user_module_roles (user_id, module_id, role_id)
			VALUES ($1::uuid, $2, $3::uuid)
			ON CONFLICT (tenant_id, user_id, module_id)
			DO UPDATE SET role_id = EXCLUDED.role_id, assigned_at = NOW()
		`, userID, assignment.ModuleID, strings.TrimSpace(*assignment.RoleID)); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (r *Repository) SetUserSuperAdmin(ctx context.Context, userID string, enabled bool) error {
	tag, err := repository.DB(ctx, r.db).Exec(ctx, `UPDATE users SET is_super_admin = $2, updated_at = NOW() WHERE id = $1::uuid`, userID, enabled)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) GetSettings(ctx context.Context) (SettingsResponse, error) {
	settings := SettingsResponse{
		DefaultRoles: make(map[string]*RoleReference),
	}

	var defaultRolesRaw []byte
	if err := repository.DB(ctx, r.db).QueryRow(ctx, `SELECT value FROM system_settings WHERE key = 'default_roles'`).Scan(&defaultRolesRaw); err != nil {
		return SettingsResponse{}, err
	}

	var defaultRoles map[string]*string
	if err := json.Unmarshal(defaultRolesRaw, &defaultRoles); err != nil {
		return SettingsResponse{}, err
	}

	for moduleID, roleID := range defaultRoles {
		if roleID == nil || strings.TrimSpace(*roleID) == "" {
			settings.DefaultRoles[moduleID] = &RoleReference{}
			continue
		}

		ref := &RoleReference{RoleID: roleID}
		var roleName string
		var roleSlug string
		if err := repository.DB(ctx, r.db).QueryRow(ctx, `SELECT name, slug FROM roles WHERE id = $1::uuid`, strings.TrimSpace(*roleID)).Scan(&roleName, &roleSlug); err == nil {
			ref.RoleName = &roleName
			ref.RoleSlug = &roleSlug
		}
		settings.DefaultRoles[moduleID] = ref
	}

	var autoCreateRaw []byte
	if err := repository.DB(ctx, r.db).QueryRow(ctx, `SELECT value FROM system_settings WHERE key = 'auto_create_employee'`).Scan(&autoCreateRaw); err != nil {
		return SettingsResponse{}, err
	}
	if err := json.Unmarshal(autoCreateRaw, &settings.AutoCreateEmployee); err != nil {
		return SettingsResponse{}, err
	}

	mailDelivery, err := r.GetMailDeliveryRecord(ctx)
	if err != nil {
		return SettingsResponse{}, err
	}
	settings.MailDelivery = mailDelivery.publicView()

	return settings, nil
}

func (r *Repository) UpdateDefaultRoles(ctx context.Context, updatedBy string, mapping map[string]*string) error {
	normalized := make(map[string]*string, len(mapping))
	for moduleID, roleID := range mapping {
		if roleID == nil || strings.TrimSpace(*roleID) == "" {
			normalized[moduleID] = nil
			continue
		}

		trimmedRoleID := strings.TrimSpace(*roleID)
		var exists bool
		if err := repository.DB(ctx, r.db).QueryRow(
			ctx,
			`SELECT EXISTS(SELECT 1 FROM roles WHERE id = $1::uuid AND is_active = TRUE)`,
			trimmedRoleID,
		).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrInvalidModuleRole
		}

		copyRoleID := trimmedRoleID
		normalized[moduleID] = &copyRoleID
	}

	raw, err := json.Marshal(normalized)
	if err != nil {
		return err
	}
	_, err = repository.DB(ctx, r.db).Exec(ctx, `
		UPDATE system_settings
		SET value = $1::jsonb, updated_by = NULLIF($2, '')::uuid, updated_at = NOW()
		WHERE key = 'default_roles'
	`, string(raw), updatedBy)
	return err
}

func (r *Repository) UpdateAutoCreateEmployee(ctx context.Context, updatedBy string, setting AutoCreateEmployeeSetting) error {
	raw, err := json.Marshal(setting)
	if err != nil {
		return err
	}
	_, err = repository.DB(ctx, r.db).Exec(ctx, `
		UPDATE system_settings
		SET value = $1::jsonb, updated_by = NULLIF($2, '')::uuid, updated_at = NOW()
		WHERE key = 'auto_create_employee'
	`, string(raw), updatedBy)
	return err
}

func (r *Repository) replaceRolePermissions(ctx context.Context, tx pgx.Tx, roleID string, permissionIDs []string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM role_permissions WHERE role_id = $1::uuid`, roleID); err != nil {
		return err
	}

	for _, permissionID := range permissionIDs {
		if strings.TrimSpace(permissionID) == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO role_permissions (role_id, permission_id)
			VALUES ($1::uuid, $2)
			ON CONFLICT DO NOTHING
		`, roleID, permissionID); err != nil {
			return err
		}
	}

	return nil
}

func (r *Repository) nextRoleCopySlug(ctx context.Context, baseSlug string) (string, error) {
	base := baseSlug + "-copy"
	candidate := base
	index := 2

	for {
		var exists bool
		if err := repository.DB(ctx, r.db).QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM roles WHERE slug = $1)`, candidate).Scan(&exists); err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
		candidate = base + "-" + strconv.Itoa(index)
		index++
	}
}

func isRoleSlugUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func derefBool(value *bool) bool {
	return value != nil && *value
}
