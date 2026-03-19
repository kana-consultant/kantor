package operational

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kana-consultant/kantor/backend/internal/model"
	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

var (
	ErrProjectNotFound       = errors.New("project not found")
	ErrProjectMemberNotFound = errors.New("project member user not found")
)

type ProjectsRepository struct {
	db *pgxpool.Pool
}

type ListProjectsParams struct {
	Page     int
	PerPage  int
	Search   string
	Status   string
	Priority string
}

type CreateProjectParams struct {
	Name        string
	Description *string
	Deadline    *string
	Status      string
	Priority    string
	CreatedBy   string
}

type UpdateProjectParams struct {
	Name           string
	Description    *string
	Deadline       *string
	Status         string
	Priority       string
	AutoAssignMode *string
}

type ProjectMemberMutationParams struct {
	Operation     string
	UserID        string
	UserEmail     string
	RoleInProject string
}

func NewProjectsRepository(db *pgxpool.Pool) *ProjectsRepository {
	return &ProjectsRepository{db: db}
}

func (r *ProjectsRepository) CreateProject(ctx context.Context, params CreateProjectParams) (model.Project, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	query := `
		INSERT INTO projects (name, description, deadline, status, priority, created_by)
		VALUES ($1, NULLIF($2, ''), $3::timestamptz, $4, $5, $6::uuid)
		RETURNING id::text, name, description, deadline, status, priority, auto_assign_mode, created_by::text, created_at, updated_at
	`

	var project model.Project
	err := r.db.QueryRow(
		ctx,
		query,
		params.Name,
		nullableString(params.Description),
		nullableTimestampString(params.Deadline),
		params.Status,
		params.Priority,
		params.CreatedBy,
	).Scan(
		&project.ID,
		&project.Name,
		&project.Description,
		&project.Deadline,
		&project.Status,
		&project.Priority,
		&project.AutoAssignMode,
		&project.CreatedBy,
		&project.CreatedAt,
		&project.UpdatedAt,
	)
	if err != nil {
		return model.Project{}, err
	}

	return project, nil
}

func (r *ProjectsRepository) ListProjects(ctx context.Context, params ListProjectsParams) ([]model.Project, int64, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	filters := []string{"1=1"}
	args := make([]interface{}, 0)
	index := 1

	if search := strings.TrimSpace(params.Search); search != "" {
		filters = append(filters, fmt.Sprintf("projects.name ILIKE $%d", index))
		args = append(args, "%"+search+"%")
		index++
	}

	if params.Status != "" {
		filters = append(filters, fmt.Sprintf("projects.status = $%d", index))
		args = append(args, params.Status)
		index++
	}

	if params.Priority != "" {
		filters = append(filters, fmt.Sprintf("projects.priority = $%d", index))
		args = append(args, params.Priority)
		index++
	}

	whereClause := strings.Join(filters, " AND ")

	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM projects WHERE %s`, whereClause)
	var total int64
	if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (params.Page - 1) * params.PerPage
	listQuery := fmt.Sprintf(`
		SELECT
			projects.id::text,
			projects.name,
			projects.description,
			projects.deadline,
			projects.status,
			projects.priority,
			projects.auto_assign_mode,
			projects.created_by::text,
			projects.created_at,
			projects.updated_at,
			COUNT(project_members.user_id)::int AS member_count
		FROM projects
		LEFT JOIN project_members ON project_members.project_id = projects.id
		WHERE %s
		GROUP BY projects.id
		ORDER BY projects.updated_at DESC, projects.created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, index, index+1)
	args = append(args, params.PerPage, offset)

	rows, err := r.db.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	projects := make([]model.Project, 0)
	for rows.Next() {
		var project model.Project
		if err := rows.Scan(
			&project.ID,
			&project.Name,
			&project.Description,
			&project.Deadline,
			&project.Status,
			&project.Priority,
			&project.AutoAssignMode,
			&project.CreatedBy,
			&project.CreatedAt,
			&project.UpdatedAt,
			&project.MemberCount,
		); err != nil {
			return nil, 0, err
		}
		projects = append(projects, project)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return projects, total, nil
}

func (r *ProjectsRepository) GetProjectByID(ctx context.Context, projectID string) (model.Project, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	query := `
		SELECT
			projects.id::text,
			projects.name,
			projects.description,
			projects.deadline,
			projects.status,
			projects.priority,
			projects.auto_assign_mode,
			projects.auto_assign_cursor,
			projects.created_by::text,
			projects.created_at,
			projects.updated_at,
			COUNT(project_members.user_id)::int AS member_count
		FROM projects
		LEFT JOIN project_members ON project_members.project_id = projects.id
		WHERE projects.id = $1::uuid
		GROUP BY projects.id
	`

	var project model.Project
	err := r.db.QueryRow(ctx, query, projectID).Scan(
		&project.ID,
		&project.Name,
		&project.Description,
		&project.Deadline,
		&project.Status,
		&project.Priority,
		&project.AutoAssignMode,
		&project.AutoAssignCursor,
		&project.CreatedBy,
		&project.CreatedAt,
		&project.UpdatedAt,
		&project.MemberCount,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Project{}, ErrProjectNotFound
		}

		return model.Project{}, err
	}

	return project, nil
}

func (r *ProjectsRepository) UpdateProject(ctx context.Context, projectID string, params UpdateProjectParams) (model.Project, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	autoAssignMode := ""
	if params.AutoAssignMode != nil && *params.AutoAssignMode != "" {
		autoAssignMode = *params.AutoAssignMode
	}
	query := `
		UPDATE projects
		SET
			name = $2,
			description = NULLIF($3, ''),
			deadline = $4::timestamptz,
			status = $5,
			priority = $6,
			auto_assign_mode = COALESCE(NULLIF($7, ''), auto_assign_mode),
			updated_at = NOW()
		WHERE id = $1::uuid
		RETURNING id::text, name, description, deadline, status, priority, auto_assign_mode, created_by::text, created_at, updated_at
	`

	var project model.Project
	err := r.db.QueryRow(
		ctx,
		query,
		projectID,
		params.Name,
		nullableString(params.Description),
		nullableTimestampString(params.Deadline),
		params.Status,
		params.Priority,
		autoAssignMode,
	).Scan(
		&project.ID,
		&project.Name,
		&project.Description,
		&project.Deadline,
		&project.Status,
		&project.Priority,
		&project.AutoAssignMode,
		&project.CreatedBy,
		&project.CreatedAt,
		&project.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Project{}, ErrProjectNotFound
		}

		return model.Project{}, err
	}

	project.MemberCount = r.countProjectMembers(ctx, projectID)
	return project, nil
}

func (r *ProjectsRepository) DeleteProject(ctx context.Context, projectID string) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	commandTag, err := r.db.Exec(ctx, `DELETE FROM projects WHERE id = $1::uuid`, projectID)
	if err != nil {
		return err
	}

	if commandTag.RowsAffected() == 0 {
		return ErrProjectNotFound
	}

	return nil
}

func (r *ProjectsRepository) ListProjectMembers(ctx context.Context, projectID string) ([]model.ProjectMember, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	query := `
		SELECT
			project_members.project_id::text,
			project_members.user_id::text,
			project_members.role_in_project,
			project_members.assigned_at,
			users.email,
			users.full_name,
			users.avatar_url
		FROM project_members
		INNER JOIN users ON users.id = project_members.user_id
		WHERE project_members.project_id = $1::uuid
		ORDER BY project_members.assigned_at ASC
	`

	rows, err := r.db.Query(ctx, query, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	members := make([]model.ProjectMember, 0)
	for rows.Next() {
		var member model.ProjectMember
		if err := rows.Scan(
			&member.ProjectID,
			&member.UserID,
			&member.RoleInProject,
			&member.AssignedAt,
			&member.UserEmail,
			&member.FullName,
			&member.AvatarURL,
		); err != nil {
			return nil, err
		}
		members = append(members, member)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return members, nil
}

func (r *ProjectsRepository) MutateProjectMember(ctx context.Context, projectID string, params ProjectMemberMutationParams) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	if _, err := r.GetProjectByID(ctx, projectID); err != nil {
		return err
	}

	userID, err := r.resolveProjectMemberUserID(ctx, params.UserID, params.UserEmail)
	if err != nil {
		return err
	}

	switch params.Operation {
	case "assign":
		query := `
			INSERT INTO project_members (project_id, user_id, role_in_project)
			VALUES ($1::uuid, $2::uuid, $3)
			ON CONFLICT (project_id, user_id)
			DO UPDATE SET role_in_project = EXCLUDED.role_in_project, assigned_at = NOW()
		`
		_, err := r.db.Exec(ctx, query, projectID, userID, params.RoleInProject)
		return err
	case "remove":
		_, err := r.db.Exec(ctx, `DELETE FROM project_members WHERE project_id = $1::uuid AND user_id = $2::uuid`, projectID, userID)
		return err
	default:
		return fmt.Errorf("unsupported project member operation")
	}
}

func (r *ProjectsRepository) AdvanceCursor(ctx context.Context, projectID string, newCursor int) error {
	_, err := r.db.Exec(ctx,
		`UPDATE projects SET auto_assign_cursor = $2 WHERE id = $1::uuid`,
		projectID, newCursor)
	return err
}

func (r *ProjectsRepository) ListMemberIDsOrdered(ctx context.Context, projectID string) ([]string, error) {
	rows, err := r.db.Query(ctx,
		`SELECT user_id::text FROM project_members WHERE project_id = $1::uuid ORDER BY assigned_at ASC, user_id ASC`,
		projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *ProjectsRepository) GetMemberWorkloads(ctx context.Context, projectID string) ([]MemberWorkload, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			pm.user_id::text,
			COALESCE((SELECT COUNT(*) FROM kanban_tasks WHERE assignee_id = pm.user_id AND project_id = pm.project_id), 0)::int AS workload
		FROM project_members pm
		WHERE pm.project_id = $1::uuid
		ORDER BY workload ASC, pm.assigned_at ASC, pm.user_id ASC
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []MemberWorkload
	for rows.Next() {
		var mw MemberWorkload
		if err := rows.Scan(&mw.UserID, &mw.Workload); err != nil {
			return nil, err
		}
		result = append(result, mw)
	}
	return result, rows.Err()
}

type MemberWorkload struct {
	UserID   string
	Workload int
}

type AvailableUser struct {
	ID        string  `json:"id"`
	Email     string  `json:"email"`
	FullName  string  `json:"full_name"`
	AvatarURL *string `json:"avatar_url"`
}

func (r *ProjectsRepository) ListActiveUsers(ctx context.Context) ([]AvailableUser, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, email, full_name, avatar_url
		FROM users
		WHERE is_active = TRUE
		ORDER BY full_name ASC, email ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []AvailableUser
	for rows.Next() {
		var u AvailableUser
		if err := rows.Scan(&u.ID, &u.Email, &u.FullName, &u.AvatarURL); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (r *ProjectsRepository) BulkAssignMembers(ctx context.Context, projectID string, userEmails []string, roleInProject string) error {
	for _, email := range userEmails {
		userID, err := r.resolveProjectMemberUserID(ctx, "", email)
		if err != nil {
			continue // skip users that don't exist
		}
		_, err = r.db.Exec(ctx, `
			INSERT INTO project_members (project_id, user_id, role_in_project)
			VALUES ($1::uuid, $2::uuid, $3)
			ON CONFLICT (project_id, user_id) DO NOTHING
		`, projectID, userID, roleInProject)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *ProjectsRepository) countProjectMembers(ctx context.Context, projectID string) int {
	var count int
	_ = r.db.QueryRow(ctx, `SELECT COUNT(*) FROM project_members WHERE project_id = $1::uuid`, projectID).Scan(&count)
	return count
}

func nullableString(value *string) interface{} {
	if value == nil {
		return nil
	}

	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return ""
	}

	return trimmed
}

func nullableTimestampString(value *string) interface{} {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil
	}

	return *value
}

func (r *ProjectsRepository) resolveProjectMemberUserID(ctx context.Context, userID string, userEmail string) (string, error) {
	trimmedUserID := strings.TrimSpace(userID)
	if trimmedUserID != "" {
		return trimmedUserID, nil
	}

	trimmedEmail := strings.ToLower(strings.TrimSpace(userEmail))
	if trimmedEmail == "" {
		return "", fmt.Errorf("user identifier is required")
	}

	var resolvedUserID string
	if err := r.db.QueryRow(ctx, `SELECT id::text FROM users WHERE email = $1`, trimmedEmail).Scan(&resolvedUserID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrProjectMemberNotFound
		}

		return "", err
	}

	return resolvedUserID, nil
}
