package operational

import (
	"context"

	"github.com/kana-consultant/kantor/backend/internal/model"
	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

type DiscordReminderRepository struct {
	db repository.DBTX
}

func NewDiscordReminderRepository(db repository.DBTX) *DiscordReminderRepository {
	return &DiscordReminderRepository{db: db}
}

type UpdateDiscordReminderConfigParams struct {
	Enabled      bool
	WebhookURL   string
	SharedSecret string
	SendHour     int
	WeekdaysOnly bool
	Timezone     string
}

func (r *DiscordReminderRepository) EnsureRow(ctx context.Context) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	_, err := repository.DB(ctx, r.db).Exec(ctx, `
		INSERT INTO discord_reminder_configs (tenant_id)
		VALUES (current_setting('app.current_tenant')::uuid)
		ON CONFLICT (tenant_id) DO NOTHING
	`)
	return err
}

func (r *DiscordReminderRepository) Get(ctx context.Context) (model.DiscordReminderConfig, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var c model.DiscordReminderConfig
	err := repository.DB(ctx, r.db).QueryRow(ctx, `
		SELECT tenant_id::text, enabled, webhook_url, shared_secret, send_hour, weekdays_only, timezone,
		       created_at, updated_at
		FROM discord_reminder_configs
		LIMIT 1
	`).Scan(
		&c.TenantID,
		&c.Enabled,
		&c.WebhookURL,
		&c.SharedSecret,
		&c.SendHour,
		&c.WeekdaysOnly,
		&c.Timezone,
		&c.CreatedAt,
		&c.UpdatedAt,
	)
	return c, err
}

func (r *DiscordReminderRepository) Update(ctx context.Context, p UpdateDiscordReminderConfigParams) (model.DiscordReminderConfig, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var c model.DiscordReminderConfig
	err := repository.DB(ctx, r.db).QueryRow(ctx, `
		UPDATE discord_reminder_configs
		SET enabled = $1,
		    webhook_url = $2,
		    shared_secret = $3,
		    send_hour = $4,
		    weekdays_only = $5,
		    timezone = $6,
		    updated_at = NOW()
		RETURNING tenant_id::text, enabled, webhook_url, shared_secret, send_hour, weekdays_only, timezone,
		          created_at, updated_at
	`, p.Enabled, p.WebhookURL, p.SharedSecret, p.SendHour, p.WeekdaysOnly, p.Timezone).Scan(
		&c.TenantID,
		&c.Enabled,
		&c.WebhookURL,
		&c.SharedSecret,
		&c.SendHour,
		&c.WeekdaysOnly,
		&c.Timezone,
		&c.CreatedAt,
		&c.UpdatedAt,
	)
	return c, err
}

// ListOpenTasksByEmployee returns every open (non-done) kanban task assigned
// to an active user, ordered by employee so the caller can bucket rows into
// a per-employee digest. RLS scopes this to the current tenant automatically.
func (r *DiscordReminderRepository) ListOpenTasksByEmployee(ctx context.Context) ([]model.DiscordReminderTaskRow, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT u.id::text, u.full_name, kt.id::text, kt.title, p.name, kc.name,
		       COALESCE(to_char(kt.due_date, 'YYYY-MM-DD'), ''), kt.priority
		FROM kanban_tasks kt
		JOIN kanban_columns kc ON kc.id = kt.column_id
		JOIN projects p ON p.id = kt.project_id
		JOIN users u ON u.id = kt.assignee_id
		WHERE kc.column_type <> 'done' AND kt.assignee_id IS NOT NULL AND u.is_active = TRUE
		ORDER BY u.full_name, kt.due_date NULLS LAST
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]model.DiscordReminderTaskRow, 0)
	for rows.Next() {
		var row model.DiscordReminderTaskRow
		if err := rows.Scan(
			&row.UserID,
			&row.FullName,
			&row.TaskID,
			&row.Title,
			&row.ProjectName,
			&row.ColumnName,
			&row.DueDate,
			&row.Priority,
		); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
