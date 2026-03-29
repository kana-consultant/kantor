package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/kana-consultant/kantor/backend/internal/model"
	"github.com/kana-consultant/kantor/backend/internal/rbac"
	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

var ErrNotFound = errors.New("resource not found")

type Repository struct {
	db repository.DBTX
}

type CreateUserParams struct {
	Email        string
	PasswordHash string
	FullName     string
	Department   *string
	Skills       []string
}

type CreateRefreshTokenParams struct {
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	UserAgent string
	IPAddress string
}

func New(db repository.DBTX) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreateUser(ctx context.Context, params CreateUserParams) (model.User, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	return r.CreateUserWithRoles(ctx, params, nil)
}

func (r *Repository) EnsureUserWithRoles(ctx context.Context, params CreateUserParams, roles []rbac.RoleKey) (model.User, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	tx, err := repository.DB(ctx, r.db).Begin(ctx)
	if err != nil {
		return model.User{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	query := `
		INSERT INTO users (email, password_hash, full_name, department, skills, is_active, is_super_admin)
		VALUES ($1, $2, $3, NULLIF($4, ''), COALESCE($5::text[], '{}'::text[]), TRUE, FALSE)
		ON CONFLICT (tenant_id, email)
		DO UPDATE SET
			password_hash = EXCLUDED.password_hash,
			full_name = EXCLUDED.full_name,
			department = EXCLUDED.department,
			skills = EXCLUDED.skills,
			is_active = TRUE,
			updated_at = NOW()
		RETURNING id::text, email, password_hash, full_name, avatar_url, department, skills, is_active, is_super_admin, failed_login_attempts, locked_until, created_at, updated_at
	`

	var user model.User
	err = tx.QueryRow(ctx, query, params.Email, params.PasswordHash, params.FullName, nullableText(params.Department), params.Skills).Scan(
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
	)
	if err != nil {
		return model.User{}, err
	}

	if err = r.assignRoleKeys(ctx, tx, user.ID, roles); err != nil {
		return model.User{}, err
	}

	if err = r.ensureEmployeeForUserForNewAccount(ctx, tx, user); err != nil {
		return model.User{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return model.User{}, err
	}

	return user, nil
}

func (r *Repository) CreateUserWithRoles(ctx context.Context, params CreateUserParams, roles []rbac.RoleKey) (model.User, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	tx, err := repository.DB(ctx, r.db).Begin(ctx)
	if err != nil {
		return model.User{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	query := `
		INSERT INTO users (email, password_hash, full_name, department, skills, is_super_admin)
		VALUES ($1, $2, $3, NULLIF($4, ''), COALESCE($5::text[], '{}'::text[]), FALSE)
		RETURNING id::text, email, password_hash, full_name, avatar_url, department, skills, is_active, is_super_admin, failed_login_attempts, locked_until, created_at, updated_at
	`

	var user model.User
	err = tx.QueryRow(ctx, query, params.Email, params.PasswordHash, params.FullName, nullableText(params.Department), params.Skills).Scan(
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
	)
	if err != nil {
		return model.User{}, err
	}

	if err = r.assignRoleKeys(ctx, tx, user.ID, roles); err != nil {
		return model.User{}, err
	}

	// Auto-create or link employee record
	if err = r.ensureEmployeeForUserForNewAccount(ctx, tx, user); err != nil {
		return model.User{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return model.User{}, err
	}

	return user, nil
}

func (r *Repository) GetUserByEmail(ctx context.Context, email string) (model.User, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	query := `
		SELECT id::text, email, password_hash, full_name, avatar_url, department, skills, is_active, is_super_admin, failed_login_attempts, locked_until, created_at, updated_at
		FROM users
		WHERE email = $1
	`

	var user model.User
	err := repository.DB(ctx, r.db).QueryRow(ctx, query, email).Scan(
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
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.User{}, ErrNotFound
		}

		return model.User{}, err
	}

	return user, nil
}

func (r *Repository) GetUserByID(ctx context.Context, userID string) (model.User, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	query := `
		SELECT id::text, email, password_hash, full_name, avatar_url, department, skills, is_active, is_super_admin, failed_login_attempts, locked_until, created_at, updated_at
		FROM users
		WHERE id = $1
	`

	var user model.User
	err := repository.DB(ctx, r.db).QueryRow(ctx, query, userID).Scan(
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
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.User{}, ErrNotFound
		}

		return model.User{}, err
	}

	return user, nil
}

func (r *Repository) GetUserRolesAndPermissions(ctx context.Context, userID string) ([]string, []string, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	var isSuperAdmin bool
	if err := repository.DB(ctx, r.db).QueryRow(ctx, `SELECT is_super_admin FROM users WHERE id = $1::uuid`, userID).Scan(&isSuperAdmin); err != nil {
		return nil, nil, err
	}

	if isSuperAdmin {
		permissionRows, err := repository.DB(ctx, r.db).Query(ctx, `SELECT id FROM permissions ORDER BY id`)
		if err != nil {
			return nil, nil, err
		}
		defer permissionRows.Close()

		permissions := make([]string, 0)
		for permissionRows.Next() {
			var permission string
			if err := permissionRows.Scan(&permission); err != nil {
				return nil, nil, err
			}
			permissions = append(permissions, permission)
		}
		if err := permissionRows.Err(); err != nil {
			return nil, nil, err
		}

		return []string{"super_admin"}, permissions, nil
	}

	rolesQuery := `
		SELECT DISTINCT
			roles.slug || ':' || user_module_roles.module_id AS role_key
		FROM user_module_roles
		INNER JOIN roles ON roles.id = user_module_roles.role_id
		WHERE user_module_roles.user_id = $1
		ORDER BY role_key
	`

	roleRows, err := repository.DB(ctx, r.db).Query(ctx, rolesQuery, userID)
	if err != nil {
		return nil, nil, err
	}
	defer roleRows.Close()

	roles := make([]string, 0)
	for roleRows.Next() {
		var role string
		if scanErr := roleRows.Scan(&role); scanErr != nil {
			return nil, nil, scanErr
		}
		roles = append(roles, role)
	}

	if err := roleRows.Err(); err != nil {
		return nil, nil, err
	}

	permissionsQuery := `
		SELECT DISTINCT permissions.id
		FROM user_module_roles
		INNER JOIN roles ON roles.id = user_module_roles.role_id
		INNER JOIN role_permissions ON role_permissions.role_id = roles.id
		INNER JOIN permissions ON permissions.id = role_permissions.permission_id
		WHERE user_module_roles.user_id = $1
			AND roles.is_active = TRUE
			AND permissions.module_id = user_module_roles.module_id
		ORDER BY permissions.id
	`

	permissionRows, err := repository.DB(ctx, r.db).Query(ctx, permissionsQuery, userID)
	if err != nil {
		return nil, nil, err
	}
	defer permissionRows.Close()

	permissions := make([]string, 0)
	for permissionRows.Next() {
		var permission string
		if scanErr := permissionRows.Scan(&permission); scanErr != nil {
			return nil, nil, scanErr
		}
		permissions = append(permissions, permission)
	}

	if err := permissionRows.Err(); err != nil {
		return nil, nil, err
	}

	return roles, permissions, nil
}

func (r *Repository) CreateRefreshToken(ctx context.Context, params CreateRefreshTokenParams) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	query := `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at, user_agent, ip_address)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, '')::inet)
	`

	_, err := repository.DB(ctx, r.db).Exec(ctx, query, params.UserID, params.TokenHash, params.ExpiresAt, params.UserAgent, params.IPAddress)
	return err
}

func (r *Repository) GetRefreshTokenByHash(ctx context.Context, tokenHash string) (model.RefreshToken, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	query := `
		SELECT id::text, user_id::text, token_hash, expires_at, revoked_at, created_at, last_used_at
		FROM refresh_tokens
		WHERE token_hash = $1
	`

	var refreshToken model.RefreshToken
	err := repository.DB(ctx, r.db).QueryRow(ctx, query, tokenHash).Scan(
		&refreshToken.ID,
		&refreshToken.UserID,
		&refreshToken.TokenHash,
		&refreshToken.ExpiresAt,
		&refreshToken.RevokedAt,
		&refreshToken.CreatedAt,
		&refreshToken.LastUsedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.RefreshToken{}, ErrNotFound
		}

		return model.RefreshToken{}, err
	}

	return refreshToken, nil
}

func (r *Repository) RotateRefreshToken(ctx context.Context, oldTokenHash string, params CreateRefreshTokenParams) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	tx, err := repository.DB(ctx, r.db).Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	updateQuery := `
		UPDATE refresh_tokens
		SET revoked_at = NOW(), last_used_at = NOW()
		WHERE token_hash = $1 AND revoked_at IS NULL
	`

	if _, err = tx.Exec(ctx, updateQuery, oldTokenHash); err != nil {
		return err
	}

	insertQuery := `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at, user_agent, ip_address)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, '')::inet)
	`

	if _, err = tx.Exec(ctx, insertQuery, params.UserID, params.TokenHash, params.ExpiresAt, params.UserAgent, params.IPAddress); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *Repository) RevokeAllUserTokens(ctx context.Context, userID string) error {
	_, err := repository.DB(ctx, r.db).Exec(
		ctx,
		`UPDATE refresh_tokens SET revoked_at = NOW() WHERE user_id = $1 AND revoked_at IS NULL`,
		userID,
	)
	return err
}

func (r *Repository) UpdatePasswordHash(ctx context.Context, userID string, passwordHash string) error {
	tag, err := repository.DB(ctx, r.db).Exec(
		ctx,
		`UPDATE users SET password_hash = $1, updated_at = NOW() WHERE id = $2`,
		passwordHash, userID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) ChangePasswordAndRevokeTokens(ctx context.Context, userID string, passwordHash string) error {
	tx, err := repository.DB(ctx, r.db).Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(
		ctx,
		`UPDATE users SET password_hash = $1, updated_at = NOW() WHERE id = $2`,
		passwordHash, userID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	if _, err := tx.Exec(
		ctx,
		`UPDATE refresh_tokens SET revoked_at = NOW() WHERE user_id = $1 AND revoked_at IS NULL`,
		userID,
	); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *Repository) RevokeRefreshToken(ctx context.Context, tokenHash string) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	tag, err := repository.DB(ctx, r.db).Exec(
		ctx,
		`
			UPDATE refresh_tokens
			SET revoked_at = NOW(), last_used_at = NOW()
			WHERE token_hash = $1 AND revoked_at IS NULL
		`,
		tokenHash,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) IncrementFailedLoginAttempts(ctx context.Context, userID string, maxAttempts int, lockDuration time.Duration) error {
	query := `
		UPDATE users
		SET failed_login_attempts = failed_login_attempts + 1,
			locked_until = CASE
				WHEN failed_login_attempts + 1 >= $2 THEN NOW() + ($3 || ' seconds')::interval
				ELSE locked_until
			END,
			updated_at = NOW()
		WHERE id = $1
	`
	_, err := repository.DB(ctx, r.db).Exec(ctx, query, userID, maxAttempts, int(lockDuration.Seconds()))
	return err
}

func (r *Repository) ResetFailedLoginAttempts(ctx context.Context, userID string) error {
	_, err := repository.DB(ctx, r.db).Exec(
		ctx,
		`UPDATE users SET failed_login_attempts = 0, locked_until = NULL, updated_at = NOW() WHERE id = $1`,
		userID,
	)
	return err
}

func (r *Repository) IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func (r *Repository) CountUsers(ctx context.Context) (int64, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	var count int64
	if err := repository.DB(ctx, r.db).QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}

	return count, nil
}

func (r *Repository) ListUserIDsByRole(ctx context.Context, roleName string, module string) ([]string, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	query := `
		SELECT DISTINCT users.id::text
		FROM users
		INNER JOIN user_module_roles ON user_module_roles.user_id = users.id
		INNER JOIN roles ON roles.id = user_module_roles.role_id
		WHERE roles.slug = $1 AND user_module_roles.module_id = $2
		ORDER BY users.id::text
	`

	if roleName == "super_admin" {
		query = `
			SELECT DISTINCT id::text
			FROM users
			WHERE is_super_admin = TRUE
			ORDER BY id::text
		`
		rows, err := repository.DB(ctx, r.db).Query(ctx, query)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		items := make([]string, 0)
		for rows.Next() {
			var userID string
			if err := rows.Scan(&userID); err != nil {
				return nil, err
			}
			items = append(items, userID)
		}

		return items, rows.Err()
	}

	rows, err := repository.DB(ctx, r.db).Query(ctx, query, roleName, module)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]string, 0)
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		items = append(items, userID)
	}

	return items, rows.Err()
}

func (r *Repository) ListUserIDsByPermission(ctx context.Context, permissionID string) ([]string, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT DISTINCT users.id::text
		FROM users
		LEFT JOIN user_module_roles ON user_module_roles.user_id = users.id
		LEFT JOIN roles ON roles.id = user_module_roles.role_id
		LEFT JOIN role_permissions ON role_permissions.role_id = roles.id
		LEFT JOIN permissions ON permissions.id = role_permissions.permission_id
		WHERE users.is_active = TRUE
			AND (
				users.is_super_admin = TRUE
				OR (
					roles.is_active = TRUE
					AND role_permissions.permission_id = $1
					AND permissions.module_id = user_module_roles.module_id
				)
			)
		ORDER BY users.id::text
	`, permissionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]string, 0)
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		items = append(items, userID)
	}

	return items, rows.Err()
}

func (r *Repository) ListUserIDsByRoleAndDepartment(ctx context.Context, roleName string, module string, department string) ([]string, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	query := `
		SELECT DISTINCT users.id::text
		FROM users
		INNER JOIN user_module_roles ON user_module_roles.user_id = users.id
		INNER JOIN roles ON roles.id = user_module_roles.role_id
		WHERE roles.slug = $1
			AND user_module_roles.module_id = $2
			AND COALESCE(users.department, '') = $3
		ORDER BY users.id::text
	`

	if roleName == "super_admin" {
		query = `
			SELECT DISTINCT id::text
			FROM users
			WHERE is_super_admin = TRUE
				AND COALESCE(department, '') = $1
			ORDER BY id::text
		`
		rows, err := repository.DB(ctx, r.db).Query(ctx, query, department)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		items := make([]string, 0)
		for rows.Next() {
			var userID string
			if err := rows.Scan(&userID); err != nil {
				return nil, err
			}
			items = append(items, userID)
		}

		return items, rows.Err()
	}

	rows, err := repository.DB(ctx, r.db).Query(ctx, query, roleName, module, department)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]string, 0)
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		items = append(items, userID)
	}

	return items, rows.Err()
}

func (r *Repository) assignRoleKeys(ctx context.Context, tx pgx.Tx, userID string, roles []rbac.RoleKey) error {
	if _, err := tx.Exec(ctx, `UPDATE users SET is_super_admin = FALSE, updated_at = NOW() WHERE id = $1::uuid`, userID); err != nil {
		return err
	}

	if len(roles) == 0 {
		return nil
	}

	selectRoleQuery := `
		SELECT id::text
		FROM roles
		WHERE slug = $1
	`
	insertAssignmentQuery := `
		INSERT INTO user_module_roles (user_id, module_id, role_id)
		VALUES ($1::uuid, $2, $3::uuid)
		ON CONFLICT (tenant_id, user_id, module_id)
		DO UPDATE SET role_id = EXCLUDED.role_id, assigned_at = NOW()
	`

	for _, role := range roles {
		if role.Name == rbac.RoleSuperAdmin {
			if _, err := tx.Exec(ctx, `UPDATE users SET is_super_admin = TRUE, updated_at = NOW() WHERE id = $1::uuid`, userID); err != nil {
				return err
			}
			continue
		}

		if strings.TrimSpace(role.Module) == "" {
			return fmt.Errorf("module assignment is required for role %s", role.Name)
		}

		var roleID string
		if err := tx.QueryRow(ctx, selectRoleQuery, role.Name).Scan(&roleID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("role slug %s is not seeded", role.Name)
			}

			return err
		}

		if _, err := tx.Exec(ctx, insertAssignmentQuery, userID, role.Module, roleID); err != nil {
			return err
		}
	}

	return nil
}

// ensureEmployeeForUser links an existing unlinked employee (by email) or creates
// a new employee record within the given transaction.
func (r *Repository) ensureEmployeeForUser(ctx context.Context, tx pgx.Tx, user model.User) error {
	// Try to link an existing employee that has the same email but no user_id
	linkQuery := `UPDATE employees SET user_id = $1::uuid, updated_at = NOW() WHERE email = $2 AND user_id IS NULL`
	tag, err := tx.Exec(ctx, linkQuery, user.ID, user.Email)
	if err != nil {
		return fmt.Errorf("link employee: %w", err)
	}
	if tag.RowsAffected() > 0 {
		// Sync employee phone to users table so WA broadcast can find it
		_, _ = tx.Exec(ctx,
			`UPDATE users SET phone = e.phone FROM employees e WHERE e.user_id = $1::uuid AND users.id = $1::uuid AND e.phone IS NOT NULL AND e.phone != ''`,
			user.ID)
		return nil
	}

	// No existing employee found — create one
	createQuery := `
		INSERT INTO employees (user_id, full_name, email, position, date_joined, employment_status)
		VALUES ($1::uuid, $2, $3, 'Belum Ditentukan', NOW()::date, 'active')
		ON CONFLICT (tenant_id, user_id) DO NOTHING
	`
	if _, err = tx.Exec(ctx, createQuery, user.ID, user.FullName, user.Email); err != nil {
		return fmt.Errorf("create employee for user: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// User management (admin)
// ---------------------------------------------------------------------------

type ListUsersParams struct {
	Page    int
	PerPage int
	Search  string
}

type UserWithRoles struct {
	User        model.User `json:"user"`
	Roles       []string   `json:"roles"`
	Permissions []string   `json:"permissions,omitempty"`
}

func (r *Repository) ListUsers(ctx context.Context, params ListUsersParams) ([]UserWithRoles, int64, error) {
	filters := []string{"1=1"}
	args := make([]interface{}, 0)
	idx := 1

	if search := strings.TrimSpace(params.Search); search != "" {
		filters = append(filters, fmt.Sprintf("(u.full_name ILIKE $%d OR u.email ILIKE $%d)", idx, idx))
		args = append(args, "%"+search+"%")
		idx++
	}

	where := strings.Join(filters, " AND ")

	var total int64
	if err := repository.DB(ctx, r.db).QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM users u WHERE %s", where), args...).Scan(&total); err != nil {
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

	offset := (page - 1) * perPage
	listQuery := fmt.Sprintf(`
		SELECT u.id::text, u.email, u.password_hash, u.full_name, u.avatar_url, u.department, u.skills, u.is_active, u.is_super_admin, u.created_at, u.updated_at
		FROM users u
		WHERE %s
		ORDER BY u.created_at DESC
		LIMIT $%d OFFSET $%d
	`, where, idx, idx+1)
	args = append(args, perPage, offset)

	rows, err := repository.DB(ctx, r.db).Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	result := make([]UserWithRoles, 0)
	for rows.Next() {
		var u model.User
		if err := rows.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.FullName, &u.AvatarURL, &u.Department, &u.Skills, &u.IsActive, &u.IsSuperAdmin, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, 0, err
		}
		result = append(result, UserWithRoles{User: u})
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	userIDs := make([]string, 0, len(result))
	for _, item := range result {
		userIDs = append(userIDs, item.User.ID)
	}

	roleMap, err := r.listUserRoleKeys(ctx, userIDs)
	if err != nil {
		return nil, 0, err
	}

	for i := range result {
		result[i].Roles = roleMap[result[i].User.ID]
	}

	return result, total, nil
}

func (r *Repository) listUserRoleKeys(ctx context.Context, userIDs []string) (map[string][]string, error) {
	if len(userIDs) == 0 {
		return map[string][]string{}, nil
	}

	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT user_id, role_key
		FROM (
			SELECT u.id::text AS user_id, 'super_admin' AS role_key
			FROM users u
			WHERE u.is_super_admin = TRUE AND u.id = ANY($1::uuid[])

			UNION

			SELECT umr.user_id::text AS user_id, roles.slug || ':' || umr.module_id AS role_key
			FROM user_module_roles umr
			INNER JOIN roles ON roles.id = umr.role_id
			WHERE umr.user_id = ANY($1::uuid[])
		) role_map
		ORDER BY user_id ASC, role_key ASC
	`, userIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	roleMap := make(map[string][]string, len(userIDs))
	for rows.Next() {
		var userID string
		var roleKey string
		if err := rows.Scan(&userID, &roleKey); err != nil {
			return nil, err
		}
		roleMap[userID] = append(roleMap[userID], roleKey)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return roleMap, nil
}

func (r *Repository) SetUserActive(ctx context.Context, userID string, active bool) error {
	tag, err := repository.DB(ctx, r.db).Exec(ctx, `UPDATE users SET is_active = $2, updated_at = NOW() WHERE id = $1::uuid`, userID, active)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) ReplaceUserRoles(ctx context.Context, userID string, roles []rbac.RoleKey) error {
	tx, err := repository.DB(ctx, r.db).Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	if _, err = tx.Exec(ctx, `DELETE FROM user_module_roles WHERE user_id = $1::uuid`, userID); err != nil {
		return err
	}

	if err = r.assignRoleKeys(ctx, tx, userID, roles); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *Repository) UpdateUserFullName(ctx context.Context, userID string, fullName string) error {
	_, err := repository.DB(ctx, r.db).Exec(ctx, `UPDATE users SET full_name = $2, updated_at = NOW() WHERE id = $1::uuid`, userID, fullName)
	return err
}

func (r *Repository) UpdateUserFullNameAndPhone(ctx context.Context, userID string, fullName string, phone *string) error {
	normalized := normalizePhone(phone)
	_, err := repository.DB(ctx, r.db).Exec(ctx,
		`UPDATE users SET full_name = $2, phone = $3, updated_at = NOW() WHERE id = $1::uuid`,
		userID, fullName, normalized)
	return err
}

func (r *Repository) UpdateUserFields(ctx context.Context, userID string, fullName string, email string) error {
	_, err := repository.DB(ctx, r.db).Exec(ctx,
		`UPDATE users SET full_name = $2, email = $3, updated_at = NOW() WHERE id = $1::uuid`,
		userID, fullName, strings.ToLower(strings.TrimSpace(email)))
	return err
}

func (r *Repository) UpdateUserAvatar(ctx context.Context, userID string, avatarURL *string) error {
	_, err := repository.DB(ctx, r.db).Exec(
		ctx,
		`UPDATE users SET avatar_url = NULLIF($2, ''), updated_at = NOW() WHERE id = $1::uuid`,
		userID,
		nullableText(avatarURL),
	)
	return err
}

func (r *Repository) UpdateEmployeeEmailByUserID(ctx context.Context, userID string, email string) error {
	_, err := repository.DB(ctx, r.db).Exec(ctx,
		`UPDATE employees SET email = $2, updated_at = NOW() WHERE user_id = $1::uuid`,
		userID, strings.ToLower(strings.TrimSpace(email)))
	return err
}

func (r *Repository) UpdateEmployeeAvatarByUserID(ctx context.Context, userID string, avatarURL string) error {
	_, err := repository.DB(ctx, r.db).Exec(ctx,
		`UPDATE employees SET avatar_url = NULLIF($2, ''), updated_at = NOW() WHERE user_id = $1::uuid`,
		userID, avatarURL)
	return err
}

func normalizePhone(phone *string) interface{} {
	if phone == nil {
		return nil
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
	if p == "" {
		return nil
	}
	return p
}

func nullableText(value *string) interface{} {
	if value == nil {
		return nil
	}

	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return ""
	}

	return trimmed
}
