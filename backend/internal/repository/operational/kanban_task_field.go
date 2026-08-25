package operational

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/kana-consultant/kantor/backend/internal/model"
	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

type KanbanTaskFieldParams struct {
	Name  string
	Value string
}

type KanbanTaskFieldRepository struct {
	db repository.DBTX
}

func NewKanbanTaskFieldRepository(db repository.DBTX) *KanbanTaskFieldRepository {
	return &KanbanTaskFieldRepository{db: db}
}

func (r *KanbanTaskFieldRepository) ListByTask(ctx context.Context, taskID string) ([]model.KanbanTaskField, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	return scanTaskFields(ctx, repository.DB(ctx, r.db), `
		SELECT id::text, task_id::text, name, value, position, created_at, updated_at
		FROM kanban_task_fields
		WHERE task_id = $1::uuid
		ORDER BY position ASC, created_at ASC
	`, taskID)
}

func (r *KanbanTaskFieldRepository) Replace(ctx context.Context, taskID string, params []KanbanTaskFieldParams) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	db := repository.DB(ctx, r.db)
	if tx, ok := db.(pgx.Tx); ok {
		return replaceTaskFields(ctx, tx, taskID, params)
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	if err = replaceTaskFields(ctx, tx, taskID, params); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func replaceTaskFields(ctx context.Context, tx pgx.Tx, taskID string, params []KanbanTaskFieldParams) error {
	sanitized := sanitizeTaskFieldParams(params)

	if _, err := tx.Exec(
		ctx,
		`DELETE FROM kanban_task_fields WHERE task_id = $1::uuid AND NOT (name = ANY($2::text[]))`,
		taskID,
		taskFieldNames(sanitized),
	); err != nil {
		return err
	}

	for index, field := range sanitized {
		if _, err := tx.Exec(
			ctx,
			`
				INSERT INTO kanban_task_fields (task_id, name, value, position)
				VALUES ($1::uuid, $2, $3, $4)
				ON CONFLICT (task_id, name) DO UPDATE
				SET value = EXCLUDED.value, position = EXCLUDED.position, updated_at = NOW()
			`,
			taskID,
			field.Name,
			field.Value,
			index+1,
		); err != nil {
			return err
		}
	}

	return nil
}

func sanitizeTaskFieldParams(params []KanbanTaskFieldParams) []KanbanTaskFieldParams {
	sanitized := make([]KanbanTaskFieldParams, 0, len(params))
	seen := make(map[string]struct{}, len(params))

	for _, field := range params {
		name := strings.TrimSpace(field.Name)
		value := strings.TrimSpace(field.Value)
		if name == "" || value == "" {
			continue
		}

		key := strings.ToLower(name)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}

		sanitized = append(sanitized, KanbanTaskFieldParams{Name: name, Value: value})
	}

	return sanitized
}

func taskFieldNames(params []KanbanTaskFieldParams) []string {
	names := make([]string, 0, len(params))
	for _, field := range params {
		names = append(names, field.Name)
	}

	return names
}

func scanTaskFields(ctx context.Context, db repository.DBTX, query string, args ...interface{}) ([]model.KanbanTaskField, error) {
	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	fields := make([]model.KanbanTaskField, 0)
	for rows.Next() {
		var field model.KanbanTaskField
		if err := rows.Scan(
			&field.ID,
			&field.TaskID,
			&field.Name,
			&field.Value,
			&field.Position,
			&field.CreatedAt,
			&field.UpdatedAt,
		); err != nil {
			return nil, err
		}
		fields = append(fields, field)
	}

	return fields, rows.Err()
}
