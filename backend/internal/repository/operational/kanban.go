package operational

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kana-consultant/kantor/backend/internal/model"
)

var (
	ErrKanbanColumnNotFound = errors.New("kanban column not found")
	ErrKanbanTaskNotFound   = errors.New("kanban task not found")
)

type KanbanRepository struct {
	db *pgxpool.Pool
}

type CreateKanbanColumnParams struct {
	Name     string
	Color    *string
	Position *int
}

type UpdateKanbanColumnParams struct {
	Name  string
	Color *string
}

type CreateKanbanTaskParams struct {
	ColumnID    string
	Title       string
	Description *string
	AssigneeID  *string
	DueDate     *string
	Priority    string
	Label       *string
	CreatedBy   string
}

type UpdateKanbanTaskParams struct {
	Title       string
	Description *string
	AssigneeID  *string
	DueDate     *string
	Priority    string
	Label       *string
}

type queryRowExecutor interface {
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
}

type KanbanSnapshot struct {
	Columns []model.KanbanColumn `json:"columns"`
	Tasks   []model.KanbanTask   `json:"tasks"`
}

func NewKanbanRepository(db *pgxpool.Pool) *KanbanRepository {
	return &KanbanRepository{db: db}
}

func (r *KanbanRepository) CreateDefaultColumns(ctx context.Context, projectID string) error {
	defaults := []struct {
		Name  string
		Color string
	}{
		{Name: "Backlog", Color: "#94A3B8"},
		{Name: "To Do", Color: "#38BDF8"},
		{Name: "In Progress", Color: "#F59E0B"},
		{Name: "Review", Color: "#8B5CF6"},
		{Name: "Done", Color: "#22C55E"},
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	for index, column := range defaults {
		_, err = tx.Exec(
			ctx,
			`INSERT INTO kanban_columns (project_id, name, position, color) VALUES ($1::uuid, $2, $3, $4)`,
			projectID,
			column.Name,
			index+1,
			column.Color,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (r *KanbanRepository) ListColumns(ctx context.Context, projectID string) ([]model.KanbanColumn, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, project_id::text, name, position, color, created_at
		FROM kanban_columns
		WHERE project_id = $1::uuid
		ORDER BY position ASC, created_at ASC
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns := make([]model.KanbanColumn, 0)
	for rows.Next() {
		var column model.KanbanColumn
		if err := rows.Scan(
			&column.ID,
			&column.ProjectID,
			&column.Name,
			&column.Position,
			&column.Color,
			&column.CreatedAt,
		); err != nil {
			return nil, err
		}
		columns = append(columns, column)
	}

	return columns, rows.Err()
}

func (r *KanbanRepository) CreateColumn(ctx context.Context, projectID string, params CreateKanbanColumnParams) (model.KanbanColumn, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return model.KanbanColumn{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	position, err := r.resolveColumnInsertPosition(ctx, tx, projectID, params.Position)
	if err != nil {
		return model.KanbanColumn{}, err
	}

	if _, err = tx.Exec(
		ctx,
		`UPDATE kanban_columns SET position = position + 1 WHERE project_id = $1::uuid AND position >= $2`,
		projectID,
		position,
	); err != nil {
		return model.KanbanColumn{}, err
	}

	var column model.KanbanColumn
	err = tx.QueryRow(
		ctx,
		`
			INSERT INTO kanban_columns (project_id, name, position, color)
			VALUES ($1::uuid, $2, $3, NULLIF($4, ''))
			RETURNING id::text, project_id::text, name, position, color, created_at
		`,
		projectID,
		params.Name,
		position,
		nullableText(params.Color),
	).Scan(
		&column.ID,
		&column.ProjectID,
		&column.Name,
		&column.Position,
		&column.Color,
		&column.CreatedAt,
	)
	if err != nil {
		return model.KanbanColumn{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return model.KanbanColumn{}, err
	}

	return column, nil
}

func (r *KanbanRepository) UpdateColumn(ctx context.Context, projectID string, columnID string, params UpdateKanbanColumnParams) (model.KanbanColumn, error) {
	var column model.KanbanColumn
	err := r.db.QueryRow(
		ctx,
		`
			UPDATE kanban_columns
			SET name = $3, color = NULLIF($4, '')
			WHERE project_id = $1::uuid AND id = $2::uuid
			RETURNING id::text, project_id::text, name, position, color, created_at
		`,
		projectID,
		columnID,
		params.Name,
		nullableText(params.Color),
	).Scan(
		&column.ID,
		&column.ProjectID,
		&column.Name,
		&column.Position,
		&column.Color,
		&column.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.KanbanColumn{}, ErrKanbanColumnNotFound
		}

		return model.KanbanColumn{}, err
	}

	return column, nil
}

func (r *KanbanRepository) DeleteColumn(ctx context.Context, projectID string, columnID string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	var position int
	err = tx.QueryRow(
		ctx,
		`DELETE FROM kanban_columns WHERE project_id = $1::uuid AND id = $2::uuid RETURNING position`,
		projectID,
		columnID,
	).Scan(&position)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrKanbanColumnNotFound
		}

		return err
	}

	if _, err = tx.Exec(
		ctx,
		`UPDATE kanban_columns SET position = position - 1 WHERE project_id = $1::uuid AND position > $2`,
		projectID,
		position,
	); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *KanbanRepository) ReorderColumns(ctx context.Context, projectID string, columnIDs []string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	var count int
	if err = tx.QueryRow(ctx, `SELECT COUNT(*) FROM kanban_columns WHERE project_id = $1::uuid`, projectID).Scan(&count); err != nil {
		return err
	}

	if count != len(columnIDs) {
		return fmt.Errorf("column reorder payload must contain every project column")
	}

	for index, columnID := range columnIDs {
		commandTag, execErr := tx.Exec(
			ctx,
			`UPDATE kanban_columns SET position = $3 WHERE project_id = $1::uuid AND id = $2::uuid`,
			projectID,
			columnID,
			index+1,
		)
		if execErr != nil {
			return execErr
		}

		if commandTag.RowsAffected() == 0 {
			return ErrKanbanColumnNotFound
		}
	}

	return tx.Commit(ctx)
}

func (r *KanbanRepository) ListTasks(ctx context.Context, projectID string) ([]model.KanbanTask, error) {
	rows, err := r.db.Query(ctx, `
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
		WHERE kanban_tasks.project_id = $1::uuid
		ORDER BY kanban_tasks.column_id, kanban_tasks.position ASC, kanban_tasks.created_at ASC
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := make([]model.KanbanTask, 0)
	for rows.Next() {
		var task model.KanbanTask
		if err := rows.Scan(
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
		); err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}

	return tasks, rows.Err()
}

func (r *KanbanRepository) CreateTask(ctx context.Context, projectID string, params CreateKanbanTaskParams) (model.KanbanTask, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return model.KanbanTask{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	if err = r.ensureColumnBelongsToProject(ctx, tx, projectID, params.ColumnID); err != nil {
		return model.KanbanTask{}, err
	}

	var position int
	if err = tx.QueryRow(
		ctx,
		`SELECT COALESCE(MAX(position), 0) + 1 FROM kanban_tasks WHERE project_id = $1::uuid AND column_id = $2::uuid`,
		projectID,
		params.ColumnID,
	).Scan(&position); err != nil {
		return model.KanbanTask{}, err
	}

	var task model.KanbanTask
	err = tx.QueryRow(
		ctx,
		`
			INSERT INTO kanban_tasks (
				column_id, project_id, title, description, assignee_id, due_date, priority, label, assigned_via, position, created_by
			)
			VALUES (
				$1::uuid, $2::uuid, $3, NULLIF($4, ''), NULLIF($5, '')::uuid, $6::timestamptz, $7, NULLIF($8, ''), 'manual', $9, $10::uuid
			)
			RETURNING id::text, column_id::text, project_id::text, title, description, assignee_id::text, due_date, priority, label, assigned_via, position, created_by::text, created_at, updated_at
		`,
		params.ColumnID,
		projectID,
		params.Title,
		nullableText(params.Description),
		nullableUUID(params.AssigneeID),
		nullableTimestampString(params.DueDate),
		params.Priority,
		nullableText(params.Label),
		position,
		params.CreatedBy,
	).Scan(
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
		return model.KanbanTask{}, err
	}

	if task.AssigneeID != nil {
		assignName, avatarURL, loadErr := r.lookupAssignee(ctx, tx, *task.AssigneeID)
		if loadErr != nil {
			return model.KanbanTask{}, loadErr
		}
		task.AssigneeName = assignName
		task.AvatarURL = avatarURL
	}

	if err = tx.Commit(ctx); err != nil {
		return model.KanbanTask{}, err
	}

	return task, nil
}

func (r *KanbanRepository) UpdateTask(ctx context.Context, projectID string, taskID string, params UpdateKanbanTaskParams) (model.KanbanTask, error) {
	var task model.KanbanTask
	err := r.db.QueryRow(
		ctx,
		`
			UPDATE kanban_tasks
			SET
				title = $3,
				description = NULLIF($4, ''),
				assignee_id = NULLIF($5, '')::uuid,
				due_date = $6::timestamptz,
				priority = $7,
				label = NULLIF($8, ''),
				assigned_via = CASE
					WHEN NULLIF($5, '')::uuid IS DISTINCT FROM assignee_id THEN 'manual'
					ELSE assigned_via
				END,
				updated_at = NOW()
			WHERE project_id = $1::uuid AND id = $2::uuid
			RETURNING id::text, column_id::text, project_id::text, title, description, assignee_id::text, due_date, priority, label, assigned_via, position, created_by::text, created_at, updated_at
		`,
		projectID,
		taskID,
		params.Title,
		nullableText(params.Description),
		nullableUUID(params.AssigneeID),
		nullableTimestampString(params.DueDate),
		params.Priority,
		nullableText(params.Label),
	).Scan(
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
		assignName, avatarURL, loadErr := r.lookupAssignee(ctx, r.db, *task.AssigneeID)
		if loadErr != nil {
			return model.KanbanTask{}, loadErr
		}
		task.AssigneeName = assignName
		task.AvatarURL = avatarURL
	}

	return task, nil
}

func (r *KanbanRepository) DeleteTask(ctx context.Context, projectID string, taskID string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	var columnID string
	var position int
	err = tx.QueryRow(
		ctx,
		`DELETE FROM kanban_tasks WHERE project_id = $1::uuid AND id = $2::uuid RETURNING column_id::text, position`,
		projectID,
		taskID,
	).Scan(&columnID, &position)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrKanbanTaskNotFound
		}

		return err
	}

	if _, err = tx.Exec(
		ctx,
		`UPDATE kanban_tasks SET position = position - 1 WHERE project_id = $1::uuid AND column_id = $2::uuid AND position > $3`,
		projectID,
		columnID,
		position,
	); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *KanbanRepository) MoveTask(ctx context.Context, projectID string, taskID string, destinationColumnID string, destinationPosition int) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	var currentColumnID string
	var currentPosition int
	err = tx.QueryRow(
		ctx,
		`SELECT column_id::text, position FROM kanban_tasks WHERE project_id = $1::uuid AND id = $2::uuid`,
		projectID,
		taskID,
	).Scan(&currentColumnID, &currentPosition)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrKanbanTaskNotFound
		}

		return err
	}

	if err = r.ensureColumnBelongsToProject(ctx, tx, projectID, destinationColumnID); err != nil {
		return err
	}

	maxPosition, err := r.maxTaskPosition(ctx, tx, projectID, destinationColumnID)
	if err != nil {
		return err
	}

	if currentColumnID == destinationColumnID {
		if maxPosition == 0 {
			destinationPosition = 1
		} else if destinationPosition > maxPosition {
			destinationPosition = maxPosition
		}

		if destinationPosition == currentPosition {
			return tx.Commit(ctx)
		}

		if destinationPosition < currentPosition {
			_, err = tx.Exec(
				ctx,
				`
					UPDATE kanban_tasks
					SET position = position + 1
					WHERE project_id = $1::uuid AND column_id = $2::uuid AND position >= $3 AND position < $4
				`,
				projectID,
				currentColumnID,
				destinationPosition,
				currentPosition,
			)
		} else {
			_, err = tx.Exec(
				ctx,
				`
					UPDATE kanban_tasks
					SET position = position - 1
					WHERE project_id = $1::uuid AND column_id = $2::uuid AND position > $3 AND position <= $4
				`,
				projectID,
				currentColumnID,
				currentPosition,
				destinationPosition,
			)
		}
		if err != nil {
			return err
		}
	} else {
		if destinationPosition > maxPosition+1 {
			destinationPosition = maxPosition + 1
		}

		if _, err = tx.Exec(
			ctx,
			`UPDATE kanban_tasks SET position = position - 1 WHERE project_id = $1::uuid AND column_id = $2::uuid AND position > $3`,
			projectID,
			currentColumnID,
			currentPosition,
		); err != nil {
			return err
		}

		if _, err = tx.Exec(
			ctx,
			`UPDATE kanban_tasks SET position = position + 1 WHERE project_id = $1::uuid AND column_id = $2::uuid AND position >= $3`,
			projectID,
			destinationColumnID,
			destinationPosition,
		); err != nil {
			return err
		}
	}

	if _, err = tx.Exec(
		ctx,
		`
			UPDATE kanban_tasks
			SET column_id = $3::uuid, position = $4, updated_at = NOW()
			WHERE project_id = $1::uuid AND id = $2::uuid
		`,
		projectID,
		taskID,
		destinationColumnID,
		destinationPosition,
	); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *KanbanRepository) Snapshot(ctx context.Context, projectID string) (KanbanSnapshot, error) {
	columns, err := r.ListColumns(ctx, projectID)
	if err != nil {
		return KanbanSnapshot{}, err
	}

	tasks, err := r.ListTasks(ctx, projectID)
	if err != nil {
		return KanbanSnapshot{}, err
	}

	sort.SliceStable(tasks, func(i int, j int) bool {
		if tasks[i].ColumnID == tasks[j].ColumnID {
			return tasks[i].Position < tasks[j].Position
		}

		return tasks[i].ColumnID < tasks[j].ColumnID
	})

	return KanbanSnapshot{
		Columns: columns,
		Tasks:   tasks,
	}, nil
}

func (r *KanbanRepository) resolveColumnInsertPosition(ctx context.Context, tx pgx.Tx, projectID string, requested *int) (int, error) {
	var maxPosition int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(position), 0) FROM kanban_columns WHERE project_id = $1::uuid`, projectID).Scan(&maxPosition); err != nil {
		return 0, err
	}

	if requested == nil || *requested > maxPosition+1 {
		return maxPosition + 1, nil
	}

	if *requested < 1 {
		return 1, nil
	}

	return *requested, nil
}

func (r *KanbanRepository) ensureColumnBelongsToProject(ctx context.Context, tx queryRowExecutor, projectID string, columnID string) error {
	var found bool
	err := tx.QueryRow(ctx, `SELECT TRUE FROM kanban_columns WHERE project_id = $1::uuid AND id = $2::uuid`, projectID, columnID).Scan(&found)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrKanbanColumnNotFound
		}

		return err
	}

	return nil
}

func (r *KanbanRepository) maxTaskPosition(ctx context.Context, tx queryRowExecutor, projectID string, columnID string) (int, error) {
	var maxPosition int
	err := tx.QueryRow(
		ctx,
		`SELECT COALESCE(MAX(position), 0) FROM kanban_tasks WHERE project_id = $1::uuid AND column_id = $2::uuid`,
		projectID,
		columnID,
	).Scan(&maxPosition)
	return maxPosition, err
}

func (r *KanbanRepository) lookupAssignee(ctx context.Context, tx queryRowExecutor, assigneeID string) (*string, *string, error) {
	var fullName *string
	var avatarURL *string
	err := tx.QueryRow(ctx, `SELECT full_name, avatar_url FROM users WHERE id = $1::uuid`, assigneeID).Scan(&fullName, &avatarURL)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, nil
		}

		return nil, nil, err
	}

	return fullName, avatarURL, nil
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

func nullableUUID(value *string) interface{} {
	if value == nil || strings.TrimSpace(*value) == "" {
		return ""
	}

	return strings.TrimSpace(*value)
}
