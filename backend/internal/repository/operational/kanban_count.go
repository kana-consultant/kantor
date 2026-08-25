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
		  AND ($3 = '' OR kanban_tasks.assignee_id = $3::uuid)
		  AND ($4 = '' OR kanban_tasks.priority = $4)
		  AND ($5 = '' OR kanban_tasks.label ILIKE '%' || $5 || '%' ESCAPE '\')
		  AND ($6 = '' OR kanban_tasks.due_date::date = $6::date)
	`, projectID,
		strings.TrimSpace(filter.ColumnID),
		strings.TrimSpace(filter.AssigneeID),
		strings.TrimSpace(filter.Priority),
		escapeLikePattern(strings.TrimSpace(filter.Label)),
		strings.TrimSpace(filter.DueDate),
	).Scan(&total)

	return total, err
}

// escapeLikePattern escapes LIKE/ILIKE wildcards so a user-supplied filter term
// matches those characters literally. Pair it with `ESCAPE '\'` in the query.
func escapeLikePattern(value string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value)
}
