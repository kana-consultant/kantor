package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kana-consultant/kantor/backend/internal/model"
	"github.com/kana-consultant/kantor/backend/internal/rbac"
)

var ErrNotFound = errors.New("resource not found")

type Repository struct {
	db *pgxpool.Pool
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

func New(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreateUser(ctx context.Context, params CreateUserParams) (model.User, error) {
	return r.CreateUserWithRoles(ctx, params, nil)
}

func (r *Repository) EnsureUserWithRoles(ctx context.Context, params CreateUserParams, roles []rbac.RoleKey) (model.User, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return model.User{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	query := `
		INSERT INTO users (email, password_hash, full_name, department, skills, is_active)
		VALUES ($1, $2, $3, NULLIF($4, ''), COALESCE($5::text[], '{}'::text[]), TRUE)
		ON CONFLICT (email)
		DO UPDATE SET
			password_hash = EXCLUDED.password_hash,
			full_name = EXCLUDED.full_name,
			department = EXCLUDED.department,
			skills = EXCLUDED.skills,
			is_active = TRUE,
			updated_at = NOW()
		RETURNING id::text, email, password_hash, full_name, avatar_url, department, skills, is_active, created_at, updated_at
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
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		return model.User{}, err
	}

	if err = r.assignRoleKeys(ctx, tx, user.ID, roles); err != nil {
		return model.User{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return model.User{}, err
	}

	return user, nil
}

func (r *Repository) CreateUserWithRoles(ctx context.Context, params CreateUserParams, roles []rbac.RoleKey) (model.User, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return model.User{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	query := `
		INSERT INTO users (email, password_hash, full_name, department, skills)
		VALUES ($1, $2, $3, NULLIF($4, ''), COALESCE($5::text[], '{}'::text[]))
		RETURNING id::text, email, password_hash, full_name, avatar_url, department, skills, is_active, created_at, updated_at
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
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		return model.User{}, err
	}

	if err = r.assignRoleKeys(ctx, tx, user.ID, roles); err != nil {
		return model.User{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return model.User{}, err
	}

	return user, nil
}

func (r *Repository) GetUserByEmail(ctx context.Context, email string) (model.User, error) {
	query := `
		SELECT id::text, email, password_hash, full_name, avatar_url, department, skills, is_active, created_at, updated_at
		FROM users
		WHERE email = $1
	`

	var user model.User
	err := r.db.QueryRow(ctx, query, email).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.FullName,
		&user.AvatarURL,
		&user.Department,
		&user.Skills,
		&user.IsActive,
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
	query := `
		SELECT id::text, email, password_hash, full_name, avatar_url, department, skills, is_active, created_at, updated_at
		FROM users
		WHERE id = $1
	`

	var user model.User
	err := r.db.QueryRow(ctx, query, userID).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.FullName,
		&user.AvatarURL,
		&user.Department,
		&user.Skills,
		&user.IsActive,
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
	rolesQuery := `
		SELECT DISTINCT
			CASE
				WHEN roles.module IS NULL OR roles.module = '' THEN roles.name
				ELSE roles.name || ':' || roles.module
			END AS role_key
		FROM user_roles
		INNER JOIN roles ON roles.id = user_roles.role_id
		WHERE user_roles.user_id = $1
		ORDER BY role_key
	`

	roleRows, err := r.db.Query(ctx, rolesQuery, userID)
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
		SELECT DISTINCT permissions.name
		FROM user_roles
		INNER JOIN role_permissions ON role_permissions.role_id = user_roles.role_id
		INNER JOIN permissions ON permissions.id = role_permissions.permission_id
		WHERE user_roles.user_id = $1
		ORDER BY permissions.name
	`

	permissionRows, err := r.db.Query(ctx, permissionsQuery, userID)
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
	query := `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at, user_agent, ip_address)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, '')::inet)
	`

	_, err := r.db.Exec(ctx, query, params.UserID, params.TokenHash, params.ExpiresAt, params.UserAgent, params.IPAddress)
	return err
}

func (r *Repository) GetRefreshTokenByHash(ctx context.Context, tokenHash string) (model.RefreshToken, error) {
	query := `
		SELECT id::text, user_id::text, token_hash, expires_at, revoked_at, created_at, last_used_at
		FROM refresh_tokens
		WHERE token_hash = $1
	`

	var refreshToken model.RefreshToken
	err := r.db.QueryRow(ctx, query, tokenHash).Scan(
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
	tx, err := r.db.Begin(ctx)
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
	_, err := r.db.Exec(
		ctx,
		`UPDATE refresh_tokens SET revoked_at = NOW() WHERE user_id = $1 AND revoked_at IS NULL`,
		userID,
	)
	return err
}

func (r *Repository) UpdatePasswordHash(ctx context.Context, userID string, passwordHash string) error {
	tag, err := r.db.Exec(
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

func (r *Repository) RevokeRefreshToken(ctx context.Context, tokenHash string) error {
	tag, err := r.db.Exec(
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

func (r *Repository) IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func (r *Repository) CountUsers(ctx context.Context) (int64, error) {
	var count int64
	if err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}

	return count, nil
}

func (r *Repository) ListUserIDsByRole(ctx context.Context, roleName string, module string) ([]string, error) {
	query := `
		SELECT DISTINCT users.id::text
		FROM users
		INNER JOIN user_roles ON user_roles.user_id = users.id
		INNER JOIN roles ON roles.id = user_roles.role_id
		WHERE roles.name = $1 AND COALESCE(roles.module, '') = $2
		ORDER BY users.id::text
	`

	rows, err := r.db.Query(ctx, query, roleName, module)
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
	query := `
		SELECT DISTINCT users.id::text
		FROM users
		INNER JOIN user_roles ON user_roles.user_id = users.id
		INNER JOIN roles ON roles.id = user_roles.role_id
		WHERE roles.name = $1
			AND COALESCE(roles.module, '') = $2
			AND COALESCE(users.department, '') = $3
		ORDER BY users.id::text
	`

	rows, err := r.db.Query(ctx, query, roleName, module, department)
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
	if len(roles) == 0 {
		return nil
	}

	selectRoleQuery := `
		SELECT id::text
		FROM roles
		WHERE name = $1 AND COALESCE(module, '') = $2
	`
	insertAssignmentQuery := `
		INSERT INTO user_roles (user_id, role_id)
		VALUES ($1, $2::uuid)
		ON CONFLICT DO NOTHING
	`

	for _, role := range roles {
		var roleID string
		if err := tx.QueryRow(ctx, selectRoleQuery, role.Name, role.Module).Scan(&roleID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("role %s:%s is not seeded", role.Name, role.Module)
			}

			return err
		}

		if _, err := tx.Exec(ctx, insertAssignmentQuery, userID, roleID); err != nil {
			return err
		}
	}

	return nil
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
