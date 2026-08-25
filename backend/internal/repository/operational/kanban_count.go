package operational

import (
	"context"
	"strings"

	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

func (r *KanbanRepository) CountTasks(ctx context.Context, projectID string, filter ListKanbanTasksFilter) (int, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var total int
	err := repository.DB(ctx, r.db).QueryRow(ctx, `
		SELECT COUNT(*)
		FROM kanban_tasks
		WHERE kanban_tasks.project_id = $1::uuid
		  AND ($2 = '' OR kanban_tasks.column_id = $2::uuid)
	`, projectID, strings.TrimSpace(filter.ColumnID)).Scan(&total)

	return total, err
}
