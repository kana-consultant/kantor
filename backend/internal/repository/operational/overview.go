package operational

import (
	"context"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/model"
	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

type OverviewRepository struct {
	db repository.DBTX
}

func NewOverviewRepository(db repository.DBTX) *OverviewRepository {
	return &OverviewRepository{db: db}
}

func (r *OverviewRepository) GetOverview(ctx context.Context, now time.Time) (model.OperationalOverview, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	overview := model.OperationalOverview{}

	if err := repository.DB(ctx, r.db).QueryRow(ctx, `SELECT COUNT(*) FROM projects`).Scan(&overview.TotalProjects); err != nil {
		return model.OperationalOverview{}, err
	}

	if err := repository.DB(ctx, r.db).QueryRow(ctx, `
		SELECT COUNT(*)
		FROM kanban_tasks
		INNER JOIN kanban_columns ON kanban_columns.id = kanban_tasks.column_id
		WHERE kanban_columns.column_type = 'in_progress'
	`).Scan(&overview.ActiveTasks); err != nil {
		return model.OperationalOverview{}, err
	}

	if err := repository.DB(ctx, r.db).QueryRow(ctx, `
		SELECT COUNT(*)
		FROM kanban_tasks
		INNER JOIN kanban_columns ON kanban_columns.id = kanban_tasks.column_id
		WHERE kanban_tasks.due_date IS NOT NULL
		  AND kanban_tasks.due_date < $1
		  AND kanban_columns.column_type <> 'done'
	`, now).Scan(&overview.OverdueTasks); err != nil {
		return model.OperationalOverview{}, err
	}

	if err := repository.DB(ctx, r.db).QueryRow(ctx, `SELECT COUNT(DISTINCT user_id) FROM project_members`).Scan(&overview.TeamMembers); err != nil {
		return model.OperationalOverview{}, err
	}

	completedByWeek, err := r.listCompletedByWeek(ctx, now)
	if err != nil {
		return model.OperationalOverview{}, err
	}
	overview.CompletedByWeek = completedByWeek

	recentTasks, err := r.listRecentTasks(ctx)
	if err != nil {
		return model.OperationalOverview{}, err
	}
	overview.RecentTasks = recentTasks

	return overview, nil
}

func (r *OverviewRepository) listCompletedByWeek(ctx context.Context, now time.Time) ([]model.OverviewSeriesPoint, error) {
	startOfCurrentWeek := startOfWeek(now)
	startWindow := startOfCurrentWeek.AddDate(0, 0, -21)

	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT DATE_TRUNC('week', kanban_tasks.updated_at)::date AS week_start, COUNT(*)::bigint
		FROM kanban_tasks
		INNER JOIN kanban_columns ON kanban_columns.id = kanban_tasks.column_id
		WHERE kanban_columns.column_type = 'done'
		  AND kanban_tasks.updated_at >= $1
		GROUP BY week_start
		ORDER BY week_start ASC
	`, startWindow)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int64, 4)
	for rows.Next() {
		var weekStart time.Time
		var count int64
		if err := rows.Scan(&weekStart, &count); err != nil {
			return nil, err
		}
		counts[weekStart.Format("2006-01-02")] = count
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	points := make([]model.OverviewSeriesPoint, 0, 4)
	for week := 0; week < 4; week++ {
		current := startWindow.AddDate(0, 0, week*7)
		key := current.Format("2006-01-02")
		points = append(points, model.OverviewSeriesPoint{
			Key:   key,
			Label: current.Format("02 Jan"),
			Value: counts[key],
		})
	}

	return points, nil
}

func (r *OverviewRepository) listRecentTasks(ctx context.Context) ([]model.OperationalRecentTask, error) {
	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT
			kanban_tasks.id::text,
			projects.id::text,
			projects.name,
			kanban_tasks.title,
			kanban_columns.name,
			kanban_tasks.priority,
			kanban_tasks.assignee_id::text,
			users.full_name,
			users.avatar_url,
			kanban_tasks.due_date,
			kanban_tasks.updated_at
		FROM kanban_tasks
		INNER JOIN projects ON projects.id = kanban_tasks.project_id
		INNER JOIN kanban_columns ON kanban_columns.id = kanban_tasks.column_id
		LEFT JOIN users ON users.id = kanban_tasks.assignee_id
		ORDER BY kanban_tasks.updated_at DESC
		LIMIT 5
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.OperationalRecentTask, 0, 5)
	for rows.Next() {
		var item model.OperationalRecentTask
		if err := rows.Scan(
			&item.ID,
			&item.ProjectID,
			&item.ProjectName,
			&item.Title,
			&item.Status,
			&item.Priority,
			&item.AssigneeID,
			&item.AssigneeName,
			&item.AssigneeAvatar,
			&item.DueDate,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, rows.Err()
}

func startOfWeek(value time.Time) time.Time {
	normalized := time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, value.Location())
	offset := (int(normalized.Weekday()) + 6) % 7
	return normalized.AddDate(0, 0, -offset)
}
