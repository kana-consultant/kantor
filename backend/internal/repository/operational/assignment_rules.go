package operational

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kana-consultant/kantor/backend/internal/model"
	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

var ErrAssignmentRuleNotFound = errors.New("assignment rule not found")

type AssignmentRulesRepository struct {
	db *pgxpool.Pool
}

type CreateAssignmentRuleParams struct {
	RuleType   string
	RuleConfig map[string]any
	Priority   int
	IsActive   bool
	CreatedBy  string
}

type UpdateAssignmentRuleParams struct {
	RuleType   string
	RuleConfig map[string]any
	Priority   int
	IsActive   bool
}

type AutoAssignTaskParams struct {
	AssigneeID  string
	ActorUserID string
	IPAddress   string
	Rule        model.AssignmentRule
}

func NewAssignmentRulesRepository(db *pgxpool.Pool) *AssignmentRulesRepository {
	return &AssignmentRulesRepository{db: db}
}

func (r *AssignmentRulesRepository) CreateRule(ctx context.Context, projectID string, params CreateAssignmentRuleParams) (model.AssignmentRule, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	configBytes, err := json.Marshal(params.RuleConfig)
	if err != nil {
		return model.AssignmentRule{}, err
	}

	query := `
		INSERT INTO assignment_rules (project_id, rule_type, rule_config, priority, is_active, created_by)
		VALUES ($1::uuid, $2, $3::jsonb, $4, $5, $6::uuid)
		RETURNING id::text, project_id::text, rule_type, rule_config, priority, is_active, created_by::text, created_at
	`

	var rule model.AssignmentRule
	var rawConfig []byte
	err = r.db.QueryRow(ctx, query, projectID, params.RuleType, configBytes, params.Priority, params.IsActive, params.CreatedBy).Scan(
		&rule.ID,
		&rule.ProjectID,
		&rule.RuleType,
		&rawConfig,
		&rule.Priority,
		&rule.IsActive,
		&rule.CreatedBy,
		&rule.CreatedAt,
	)
	if err != nil {
		return model.AssignmentRule{}, err
	}

	if err := json.Unmarshal(rawConfig, &rule.RuleConfig); err != nil {
		return model.AssignmentRule{}, err
	}

	return rule, nil
}

func (r *AssignmentRulesRepository) ListRules(ctx context.Context, projectID string) ([]model.AssignmentRule, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	rows, err := r.db.Query(ctx, `
		SELECT id::text, project_id::text, rule_type, rule_config, priority, is_active, created_by::text, created_at
		FROM assignment_rules
		WHERE project_id = $1::uuid
		ORDER BY priority ASC, created_at ASC
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rules := make([]model.AssignmentRule, 0)
	for rows.Next() {
		var rule model.AssignmentRule
		var rawConfig []byte
		if err := rows.Scan(
			&rule.ID,
			&rule.ProjectID,
			&rule.RuleType,
			&rawConfig,
			&rule.Priority,
			&rule.IsActive,
			&rule.CreatedBy,
			&rule.CreatedAt,
		); err != nil {
			return nil, err
		}

		if err := json.Unmarshal(rawConfig, &rule.RuleConfig); err != nil {
			return nil, err
		}

		rules = append(rules, rule)
	}

	return rules, rows.Err()
}

func (r *AssignmentRulesRepository) UpdateRule(ctx context.Context, projectID string, ruleID string, params UpdateAssignmentRuleParams) (model.AssignmentRule, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	configBytes, err := json.Marshal(params.RuleConfig)
	if err != nil {
		return model.AssignmentRule{}, err
	}

	var rule model.AssignmentRule
	var rawConfig []byte
	err = r.db.QueryRow(ctx, `
		UPDATE assignment_rules
		SET rule_type = $3, rule_config = $4::jsonb, priority = $5, is_active = $6
		WHERE project_id = $1::uuid AND id = $2::uuid
		RETURNING id::text, project_id::text, rule_type, rule_config, priority, is_active, created_by::text, created_at
	`, projectID, ruleID, params.RuleType, configBytes, params.Priority, params.IsActive).Scan(
		&rule.ID,
		&rule.ProjectID,
		&rule.RuleType,
		&rawConfig,
		&rule.Priority,
		&rule.IsActive,
		&rule.CreatedBy,
		&rule.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.AssignmentRule{}, ErrAssignmentRuleNotFound
		}
		return model.AssignmentRule{}, err
	}

	if err := json.Unmarshal(rawConfig, &rule.RuleConfig); err != nil {
		return model.AssignmentRule{}, err
	}

	return rule, nil
}

func (r *AssignmentRulesRepository) DeleteRule(ctx context.Context, projectID string, ruleID string) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	commandTag, err := r.db.Exec(ctx, `DELETE FROM assignment_rules WHERE project_id = $1::uuid AND id = $2::uuid`, projectID, ruleID)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return ErrAssignmentRuleNotFound
	}
	return nil
}

func (r *AssignmentRulesRepository) ListCandidates(ctx context.Context, projectID string) ([]model.AssignmentCandidate, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	rows, err := r.db.Query(ctx, `
		SELECT
			project_members.user_id::text,
			users.full_name,
			users.email,
			users.avatar_url,
			users.department,
			users.skills,
			project_members.role_in_project,
			project_members.assigned_at,
			COALESCE((
				SELECT COUNT(*)
				FROM kanban_tasks
				WHERE kanban_tasks.assignee_id = project_members.user_id
			), 0)::int AS workload
		FROM project_members
		INNER JOIN users ON users.id = project_members.user_id
		WHERE project_members.project_id = $1::uuid
		ORDER BY project_members.assigned_at ASC, users.full_name ASC
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	candidates := make([]model.AssignmentCandidate, 0)
	for rows.Next() {
		var candidate model.AssignmentCandidate
		if err := rows.Scan(
			&candidate.UserID,
			&candidate.FullName,
			&candidate.Email,
			&candidate.AvatarURL,
			&candidate.Department,
			&candidate.Skills,
			&candidate.RoleInProject,
			&candidate.AssignedAt,
			&candidate.Workload,
		); err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate)
	}

	return candidates, rows.Err()
}

func (r *AssignmentRulesRepository) AutoAssignTask(ctx context.Context, projectID string, taskID string, params AutoAssignTaskParams) (model.KanbanTask, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return model.KanbanTask{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	previousTask, err := r.getTaskByID(ctx, tx, projectID, taskID)
	if err != nil {
		return model.KanbanTask{}, err
	}

	task, err := r.updateTaskAssignment(ctx, tx, projectID, taskID, params.AssigneeID)
	if err != nil {
		return model.KanbanTask{}, err
	}

	oldValue, err := json.Marshal(map[string]any{
		"assignee_id":  previousTask.AssigneeID,
		"assigned_via": previousTask.AssignedVia,
	})
	if err != nil {
		return model.KanbanTask{}, err
	}

	newValue, err := json.Marshal(map[string]any{
		"assignee_id":  task.AssigneeID,
		"assigned_via": task.AssignedVia,
		"rule_id":      params.Rule.ID,
		"rule_type":    params.Rule.RuleType,
	})
	if err != nil {
		return model.KanbanTask{}, err
	}

	if _, err = tx.Exec(ctx, `
		INSERT INTO audit_logs (user_id, action, module, resource, resource_id, old_value, new_value, ip_address)
		VALUES ($1::uuid, $2, $3, $4, $5, $6::jsonb, $7::jsonb, NULLIF($8, '')::inet)
	`, params.ActorUserID, "auto_assign", "operational", "kanban_task", taskID, oldValue, newValue, strings.TrimSpace(params.IPAddress)); err != nil {
		return model.KanbanTask{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return model.KanbanTask{}, err
	}

	return task, nil
}

func (r *AssignmentRulesRepository) getTaskByID(ctx context.Context, tx pgx.Tx, projectID string, taskID string) (model.KanbanTask, error) {
	var task model.KanbanTask
	err := tx.QueryRow(ctx, `
		SELECT
			kanban_tasks.id::text,
			kanban_tasks.column_id::text,
			kanban_tasks.project_id::text,
			kanban_tasks.title,
			kanban_tasks.description,
			kanban_tasks.assignee_id::text,
			users.full_name,
			users.avatar_url,
			kanban_tasks.due_date,
			kanban_tasks.priority,
			kanban_tasks.label,
			kanban_tasks.assigned_via,
			kanban_tasks.position,
			kanban_tasks.created_by::text,
			kanban_tasks.created_at,
			kanban_tasks.updated_at
		FROM kanban_tasks
		LEFT JOIN users ON users.id = kanban_tasks.assignee_id
		WHERE kanban_tasks.project_id = $1::uuid AND kanban_tasks.id = $2::uuid
	`, projectID, taskID).Scan(
		&task.ID,
		&task.ColumnID,
		&task.ProjectID,
		&task.Title,
		&task.Description,
		&task.AssigneeID,
		&task.AssigneeName,
		&task.AvatarURL,
		&task.DueDate,
		&task.Priority,
		&task.Label,
		&task.AssignedVia,
		&task.Position,
		&task.CreatedBy,
		&task.CreatedAt,
		&task.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.KanbanTask{}, ErrKanbanTaskNotFound
		}
		return model.KanbanTask{}, err
	}

	return task, nil
}

func (r *AssignmentRulesRepository) updateTaskAssignment(ctx context.Context, tx pgx.Tx, projectID string, taskID string, assigneeID string) (model.KanbanTask, error) {
	var task model.KanbanTask
	err := tx.QueryRow(ctx, `
		UPDATE kanban_tasks
		SET assignee_id = $3::uuid, assigned_via = 'auto', updated_at = NOW()
		WHERE project_id = $1::uuid AND id = $2::uuid
		RETURNING id::text, column_id::text, project_id::text, title, description, assignee_id::text, due_date, priority, label, assigned_via, position, created_by::text, created_at, updated_at
	`, projectID, taskID, assigneeID).Scan(
		&task.ID,
		&task.ColumnID,
		&task.ProjectID,
		&task.Title,
		&task.Description,
		&task.AssigneeID,
		&task.DueDate,
		&task.Priority,
		&task.Label,
		&task.AssignedVia,
		&task.Position,
		&task.CreatedBy,
		&task.CreatedAt,
		&task.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.KanbanTask{}, ErrKanbanTaskNotFound
		}
		return model.KanbanTask{}, err
	}

	if task.AssigneeID != nil {
		err = tx.QueryRow(ctx, `SELECT full_name, avatar_url FROM users WHERE id = $1::uuid`, *task.AssigneeID).Scan(&task.AssigneeName, &task.AvatarURL)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return model.KanbanTask{}, err
		}
	}

	return task, nil
}
