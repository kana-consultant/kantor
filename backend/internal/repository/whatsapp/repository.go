package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/kana-consultant/kantor/backend/internal/model"
	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

var (
	ErrTemplateNotFound = errors.New("wa template not found")
	ErrScheduleNotFound = errors.New("wa schedule not found")
	ErrSystemTemplate   = errors.New("cannot delete system template")
)

type Repository struct {
	db repository.DBTX
}

func New(db repository.DBTX) *Repository {
	return &Repository{db: db}
}

// --------------- WA Config ---------------

type WAConfig struct {
	APIURL           string
	APIKey           string
	SessionName      string
	Enabled          bool
	MaxDailyMessages int
	MinDelayMS       int
	MaxDelayMS       int
	ReminderCron     string
	WeeklyDigestCron string
}

func (r *Repository) GetWAConfig(ctx context.Context) (WAConfig, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var cfg WAConfig
	err := repository.DB(ctx, r.db).QueryRow(ctx, `
		SELECT api_url, api_key, session_name, enabled,
			   max_daily_messages, min_delay_ms, max_delay_ms,
			   reminder_cron, weekly_digest_cron
		FROM tenant_wa_configs
		LIMIT 1
	`).Scan(
		&cfg.APIURL, &cfg.APIKey, &cfg.SessionName, &cfg.Enabled,
		&cfg.MaxDailyMessages, &cfg.MinDelayMS, &cfg.MaxDelayMS,
		&cfg.ReminderCron, &cfg.WeeklyDigestCron,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return WAConfig{}, nil
		}
		return WAConfig{}, fmt.Errorf("load wa config: %w", err)
	}
	return cfg, nil
}

func (r *Repository) UpsertWAConfig(ctx context.Context, cfg WAConfig) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	_, err := repository.DB(ctx, r.db).Exec(ctx, `
		INSERT INTO tenant_wa_configs (api_url, api_key, session_name, enabled,
			max_daily_messages, min_delay_ms, max_delay_ms,
			reminder_cron, weekly_digest_cron, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
		ON CONFLICT (tenant_id) DO UPDATE SET
			api_url = EXCLUDED.api_url,
			api_key = EXCLUDED.api_key,
			session_name = EXCLUDED.session_name,
			enabled = EXCLUDED.enabled,
			max_daily_messages = EXCLUDED.max_daily_messages,
			min_delay_ms = EXCLUDED.min_delay_ms,
			max_delay_ms = EXCLUDED.max_delay_ms,
			reminder_cron = EXCLUDED.reminder_cron,
			weekly_digest_cron = EXCLUDED.weekly_digest_cron,
			updated_at = NOW()
	`, cfg.APIURL, cfg.APIKey, cfg.SessionName, cfg.Enabled,
		cfg.MaxDailyMessages, cfg.MinDelayMS, cfg.MaxDelayMS,
		cfg.ReminderCron, cfg.WeeklyDigestCron,
	)
	return err
}

func (r *Repository) GetTenantPrimaryDomain(ctx context.Context) (string, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var domain string
	err := repository.DB(ctx, r.db).QueryRow(ctx, `
		SELECT domain
		FROM tenant_domains
		WHERE tenant_id = current_setting('app.current_tenant', true)::uuid
		ORDER BY is_primary DESC, created_at ASC
		LIMIT 1
	`).Scan(&domain)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("load tenant primary domain: %w", err)
	}

	return strings.TrimSpace(domain), nil
}

// --------------- Templates ---------------

type CreateTemplateParams struct {
	Name               string
	Slug               string
	Category           string
	TriggerType        string
	BodyTemplate       string
	Description        *string
	AvailableVariables []string
	IsActive           bool
	CreatedBy          *string
}

type UpdateTemplateParams struct {
	Name               *string
	Category           *string
	TriggerType        *string
	BodyTemplate       string
	Description        *string
	AvailableVariables []string
	IsActive           bool
}

type defaultTemplateSeed struct {
	Name               string
	Slug               string
	Category           string
	TriggerType        string
	BodyTemplate       string
	Description        string
	AvailableVariables []string
}

var defaultTemplateSeeds = []defaultTemplateSeed{
	{
		Name:        "Task Jatuh Tempo Hari Ini",
		Slug:        "task_due_today",
		Category:    "operational",
		TriggerType: "auto_scheduled",
		BodyTemplate: "Halo {{name}}, task *{{task_title}}* di project *{{project_name}}* jatuh tempo hari ini dan belum selesai. " +
			"Segera diselesaikan ya!\n\nAkses KANTOR: {{app_url}}",
		Description:        "Dikirim otomatis setiap pagi untuk task yang due date hari ini",
		AvailableVariables: []string{"name", "task_title", "project_name", "due_date", "app_url"},
	},
	{
		Name:        "Task Overdue",
		Slug:        "task_overdue",
		Category:    "operational",
		TriggerType: "auto_scheduled",
		BodyTemplate: "Halo {{name}}, task *{{task_title}}* di project *{{project_name}}* sudah melewati deadline ({{due_date}}). " +
			"Mohon segera update progress.\n\nAkses KANTOR: {{app_url}}",
		Description:        "Dikirim otomatis setiap pagi untuk task yang sudah melewati deadline",
		AvailableVariables: []string{"name", "task_title", "project_name", "due_date", "app_url"},
	},
	{
		Name:        "Task Baru Di-Assign",
		Slug:        "task_assigned",
		Category:    "operational",
		TriggerType: "event_triggered",
		BodyTemplate: "Halo {{name}}, kamu di-assign ke task baru:\n\n" +
			"- {{task_title}}\n" +
			"- Project: {{project_name}}\n" +
			"- Deadline: {{due_date}}\n" +
			"- Prioritas: {{priority}}\n\n" +
			"Cek detail di KANTOR: {{app_url}}",
		Description:        "Dikirim saat task di-assign ke seseorang (manual atau auto-assign)",
		AvailableVariables: []string{"name", "task_title", "project_name", "due_date", "priority", "app_url"},
	},
	{
		Name:        "Project Deadline H-3",
		Slug:        "project_deadline_h3",
		Category:    "operational",
		TriggerType: "auto_scheduled",
		BodyTemplate: "Reminder: Project *{{project_name}}* deadline dalam 3 hari ({{deadline}}).\n\n" +
			"Status saat ini: {{project_status}}\n" +
			"Task belum selesai: {{open_tasks_count}} dari {{total_tasks_count}}\n\n" +
			"Pastikan semua task terselesaikan tepat waktu. Akses: {{app_url}}",
		Description:        "Dikirim otomatis H-3 sebelum deadline project ke semua member project",
		AvailableVariables: []string{"name", "project_name", "deadline", "project_status", "open_tasks_count", "total_tasks_count", "app_url"},
	},
	{
		Name:        "Weekly Digest",
		Slug:        "weekly_digest",
		Category:    "operational",
		TriggerType: "auto_scheduled",
		BodyTemplate: "Weekly Digest KANTOR ({{week_start}} - {{week_end}})\n\n" +
			"Halo {{name}}, ini ringkasan minggu lalu:\n\n" +
			"- Task selesai: {{completed_count}}\n" +
			"- Task masih open: {{open_count}}\n" +
			"- Task overdue: {{overdue_count}}\n\n" +
			"Semangat untuk minggu ini!\nAkses: {{app_url}}",
		Description:        "Dikirim setiap Senin pagi, ringkasan task per user untuk minggu sebelumnya",
		AvailableVariables: []string{"name", "week_start", "week_end", "completed_count", "open_count", "overdue_count", "app_url"},
	},
	{
		Name:        "Reimbursement Status Update",
		Slug:        "reimbursement_status",
		Category:    "hris",
		TriggerType: "event_triggered",
		BodyTemplate: "Halo {{name}}, update status reimbursement kamu:\n\n" +
			"- {{reimbursement_title}}\n" +
			"- Nominal: {{amount}}\n" +
			"- Status: {{new_status}}\n" +
			"{{reviewer_notes_section}}\n\n" +
			"Cek detail di KANTOR: {{app_url}}",
		Description:        "Dikirim saat status reimbursement berubah (approved/rejected/paid)",
		AvailableVariables: []string{"name", "reimbursement_title", "amount", "new_status", "reviewer_notes_section", "app_url"},
	},
	{
		Name:        "Reminder Review Reimbursement",
		Slug:        "reimbursement_review_reminder",
		Category:    "hris",
		TriggerType: "auto_scheduled",
		BodyTemplate: "Halo {{name}}, ada {{pending_count}} reimbursement menunggu review.\n\n" +
			"Total nominal: {{total_amount}}\n" +
			"Item terlama: {{oldest_date}}\n" +
			"Contoh item:\n{{items_summary}}\n\n" +
			"Tindak lanjuti di KANTOR: {{app_url}}/hris/reimbursements?status=submitted",
		Description:        "Dikirim terjadwal ke approver reimbursement yang memiliki akses view_all",
		AvailableVariables: []string{"name", "pending_count", "total_amount", "oldest_date", "items_summary", "app_url"},
	},
	{
		Name:        "Reminder Pembayaran Reimbursement",
		Slug:        "reimbursement_payment_reminder",
		Category:    "hris",
		TriggerType: "auto_scheduled",
		BodyTemplate: "Halo {{name}}, ada {{pending_count}} reimbursement approved yang menunggu pembayaran.\n\n" +
			"Total nominal: {{total_amount}}\n" +
			"Item terlama: {{oldest_date}}\n" +
			"Contoh item:\n{{items_summary}}\n\n" +
			"Tindak lanjuti di KANTOR: {{app_url}}/hris/reimbursements?status=approved",
		Description:        "Dikirim terjadwal ke user yang memiliki akses mark_paid dan view_all",
		AvailableVariables: []string{"name", "pending_count", "total_amount", "oldest_date", "items_summary", "app_url"},
	},
}

func (r *Repository) ListTemplates(ctx context.Context, category string, triggerType string) ([]model.WAMessageTemplate, error) {
	query := `SELECT id, name, slug, category, trigger_type, body_template, description,
		available_variables, is_active, is_system, created_by, created_at, updated_at
		FROM wa_message_templates WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if category != "" {
		query += fmt.Sprintf(" AND category = $%d", argIdx)
		args = append(args, category)
		argIdx++
	}
	if triggerType != "" {
		query += fmt.Sprintf(" AND trigger_type = $%d", argIdx)
		args = append(args, triggerType)
		argIdx++
	}

	query += " ORDER BY is_system DESC, name ASC"

	rows, err := repository.DB(ctx, r.db).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list templates: %w", err)
	}
	defer rows.Close()

	var templates []model.WAMessageTemplate
	for rows.Next() {
		var t model.WAMessageTemplate
		if err := rows.Scan(&t.ID, &t.Name, &t.Slug, &t.Category, &t.TriggerType,
			&t.BodyTemplate, &t.Description, &t.AvailableVariables, &t.IsActive,
			&t.IsSystem, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan template: %w", err)
		}
		templates = append(templates, t)
	}
	return templates, nil
}

func (r *Repository) GetTemplateByID(ctx context.Context, id string) (model.WAMessageTemplate, error) {
	var t model.WAMessageTemplate
	err := repository.DB(ctx, r.db).QueryRow(ctx, `SELECT id, name, slug, category, trigger_type, body_template,
		description, available_variables, is_active, is_system, created_by, created_at, updated_at
		FROM wa_message_templates WHERE id = $1`, id).Scan(
		&t.ID, &t.Name, &t.Slug, &t.Category, &t.TriggerType, &t.BodyTemplate,
		&t.Description, &t.AvailableVariables, &t.IsActive, &t.IsSystem,
		&t.CreatedBy, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, ErrTemplateNotFound
	}
	return t, err
}

func (r *Repository) GetTemplateBySlug(ctx context.Context, slug string) (model.WAMessageTemplate, error) {
	var t model.WAMessageTemplate
	candidates := templateSlugCandidates(slug)
	err := repository.DB(ctx, r.db).QueryRow(ctx, `SELECT id, name, slug, category, trigger_type, body_template,
		description, available_variables, is_active, is_system, created_by, created_at, updated_at
		FROM wa_message_templates
		WHERE slug = ANY($1::text[])
		ORDER BY CASE WHEN slug = $2 THEN 0 ELSE 1 END
		LIMIT 1`, candidates, slug).Scan(
		&t.ID, &t.Name, &t.Slug, &t.Category, &t.TriggerType, &t.BodyTemplate,
		&t.Description, &t.AvailableVariables, &t.IsActive, &t.IsSystem,
		&t.CreatedBy, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, ErrTemplateNotFound
	}
	return t, err
}

func templateSlugCandidates(slug string) []string {
	switch strings.TrimSpace(slug) {
	case "project_deadline_h3", "project_deadline_warning":
		return []string{"project_deadline_h3", "project_deadline_warning"}
	default:
		return []string{slug}
	}
}

func (r *Repository) CreateTemplate(ctx context.Context, params CreateTemplateParams) (model.WAMessageTemplate, error) {
	var t model.WAMessageTemplate
	err := repository.DB(ctx, r.db).QueryRow(ctx, `INSERT INTO wa_message_templates
		(name, slug, category, trigger_type, body_template, description, available_variables, is_active, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, name, slug, category, trigger_type, body_template, description,
		available_variables, is_active, is_system, created_by, created_at, updated_at`,
		params.Name, params.Slug, params.Category, params.TriggerType, params.BodyTemplate,
		params.Description, params.AvailableVariables, params.IsActive, params.CreatedBy).Scan(
		&t.ID, &t.Name, &t.Slug, &t.Category, &t.TriggerType, &t.BodyTemplate,
		&t.Description, &t.AvailableVariables, &t.IsActive, &t.IsSystem,
		&t.CreatedBy, &t.CreatedAt, &t.UpdatedAt)
	return t, err
}

func (r *Repository) UpdateTemplate(ctx context.Context, id string, params UpdateTemplateParams, isSystem bool) (model.WAMessageTemplate, error) {
	var query string
	var args []interface{}

	if isSystem {
		// System templates: only body_template and is_active can be changed
		query = `UPDATE wa_message_templates SET body_template = $1, is_active = $2, updated_at = NOW()
			WHERE id = $3
			RETURNING id, name, slug, category, trigger_type, body_template, description,
			available_variables, is_active, is_system, created_by, created_at, updated_at`
		args = []interface{}{params.BodyTemplate, params.IsActive, id}
	} else {
		query = `UPDATE wa_message_templates SET
			name = COALESCE($1, name), category = COALESCE($2, category),
			trigger_type = COALESCE($3, trigger_type), body_template = $4,
			description = $5, available_variables = $6, is_active = $7, updated_at = NOW()
			WHERE id = $8
			RETURNING id, name, slug, category, trigger_type, body_template, description,
			available_variables, is_active, is_system, created_by, created_at, updated_at`
		args = []interface{}{params.Name, params.Category, params.TriggerType, params.BodyTemplate,
			params.Description, params.AvailableVariables, params.IsActive, id}
	}

	var t model.WAMessageTemplate
	err := repository.DB(ctx, r.db).QueryRow(ctx, query, args...).Scan(
		&t.ID, &t.Name, &t.Slug, &t.Category, &t.TriggerType, &t.BodyTemplate,
		&t.Description, &t.AvailableVariables, &t.IsActive, &t.IsSystem,
		&t.CreatedBy, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, ErrTemplateNotFound
	}
	return t, err
}

func (r *Repository) EnsureDefaultTemplates(ctx context.Context) (model.WADefaultTemplatesSeedResult, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	result := model.WADefaultTemplatesSeedResult{
		TotalCount: len(defaultTemplateSeeds),
	}

	for _, template := range defaultTemplateSeeds {
		var insertedSlug string
		err := repository.DB(ctx, r.db).QueryRow(ctx, `
			WITH inserted AS (
				INSERT INTO wa_message_templates (
					name, slug, category, trigger_type, body_template, description,
					available_variables, is_active, is_system, created_by
				)
				SELECT $1, $2, $3, $4, $5, $6, $7, true, true, NULL
				WHERE NOT EXISTS (
					SELECT 1
					FROM wa_message_templates
					WHERE slug = ANY($8::text[])
				)
				ON CONFLICT (tenant_id, slug) DO NOTHING
				RETURNING slug
			)
			SELECT COALESCE((SELECT slug FROM inserted), '')
		`,
			template.Name,
			template.Slug,
			template.Category,
			template.TriggerType,
			template.BodyTemplate,
			template.Description,
			template.AvailableVariables,
			templateSlugCandidates(template.Slug),
		).Scan(&insertedSlug)
		if err != nil {
			return model.WADefaultTemplatesSeedResult{}, fmt.Errorf("ensure default template %q: %w", template.Slug, err)
		}

		if insertedSlug != "" {
			result.InsertedCount++
			result.InsertedSlugs = append(result.InsertedSlugs, insertedSlug)
		}
	}

	result.ExistingCount = result.TotalCount - result.InsertedCount
	return result, nil
}

func (r *Repository) DeleteTemplate(ctx context.Context, id string) error {
	var isSystem bool
	err := repository.DB(ctx, r.db).QueryRow(ctx, "SELECT is_system FROM wa_message_templates WHERE id = $1", id).Scan(&isSystem)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrTemplateNotFound
	}
	if err != nil {
		return err
	}
	if isSystem {
		return ErrSystemTemplate
	}

	_, err = repository.DB(ctx, r.db).Exec(ctx, "DELETE FROM wa_message_templates WHERE id = $1", id)
	return err
}

// --------------- Schedules ---------------

type CreateScheduleParams struct {
	Name           string
	TemplateID     string
	ScheduleType   string
	CronExpression *string
	TargetType     string
	TargetConfig   *string
	IsActive       bool
	CreatedBy      *string
}

type UpdateScheduleParams struct {
	Name           string
	TemplateID     string
	ScheduleType   string
	CronExpression *string
	TargetType     string
	TargetConfig   *string
	IsActive       bool
}

func (r *Repository) ListSchedules(ctx context.Context) ([]model.WABroadcastSchedule, error) {
	rows, err := repository.DB(ctx, r.db).Query(ctx, `SELECT s.id, s.name, s.template_id, t.name, s.schedule_type,
		s.cron_expression, s.target_type, s.target_config::text, s.is_active,
		s.last_run_at, s.next_run_at, s.created_by, s.created_at, s.updated_at
		FROM wa_broadcast_schedules s
		JOIN wa_message_templates t ON t.id = s.template_id
		ORDER BY s.created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list schedules: %w", err)
	}
	defer rows.Close()

	var schedules []model.WABroadcastSchedule
	for rows.Next() {
		var s model.WABroadcastSchedule
		if err := rows.Scan(&s.ID, &s.Name, &s.TemplateID, &s.TemplateName, &s.ScheduleType,
			&s.CronExpression, &s.TargetType, &s.TargetConfig, &s.IsActive,
			&s.LastRunAt, &s.NextRunAt, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan schedule: %w", err)
		}
		schedules = append(schedules, s)
	}
	return schedules, nil
}

func (r *Repository) GetScheduleByID(ctx context.Context, id string) (model.WABroadcastSchedule, error) {
	var s model.WABroadcastSchedule
	err := repository.DB(ctx, r.db).QueryRow(ctx, `SELECT s.id, s.name, s.template_id, t.name, s.schedule_type,
		s.cron_expression, s.target_type, s.target_config::text, s.is_active,
		s.last_run_at, s.next_run_at, s.created_by, s.created_at, s.updated_at
		FROM wa_broadcast_schedules s
		JOIN wa_message_templates t ON t.id = s.template_id
		WHERE s.id = $1`, id).Scan(
		&s.ID, &s.Name, &s.TemplateID, &s.TemplateName, &s.ScheduleType,
		&s.CronExpression, &s.TargetType, &s.TargetConfig, &s.IsActive,
		&s.LastRunAt, &s.NextRunAt, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return s, ErrScheduleNotFound
	}
	return s, err
}

func (r *Repository) CreateSchedule(ctx context.Context, params CreateScheduleParams) (model.WABroadcastSchedule, error) {
	var s model.WABroadcastSchedule
	err := repository.DB(ctx, r.db).QueryRow(ctx, `INSERT INTO wa_broadcast_schedules
		(name, template_id, schedule_type, cron_expression, target_type, target_config, is_active, created_by)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8)
		RETURNING id, name, template_id, schedule_type, cron_expression, target_type,
		target_config::text, is_active, last_run_at, next_run_at, created_by, created_at, updated_at`,
		params.Name, params.TemplateID, params.ScheduleType, params.CronExpression,
		params.TargetType, params.TargetConfig, params.IsActive, params.CreatedBy).Scan(
		&s.ID, &s.Name, &s.TemplateID, &s.ScheduleType, &s.CronExpression,
		&s.TargetType, &s.TargetConfig, &s.IsActive, &s.LastRunAt, &s.NextRunAt,
		&s.CreatedBy, &s.CreatedAt, &s.UpdatedAt)
	return s, err
}

func (r *Repository) UpdateSchedule(ctx context.Context, id string, params UpdateScheduleParams) (model.WABroadcastSchedule, error) {
	var s model.WABroadcastSchedule
	err := repository.DB(ctx, r.db).QueryRow(ctx, `UPDATE wa_broadcast_schedules SET
		name = $1, template_id = $2, schedule_type = $3, cron_expression = $4,
		target_type = $5, target_config = $6::jsonb, is_active = $7, updated_at = NOW()
		WHERE id = $8
		RETURNING id, name, template_id, schedule_type, cron_expression, target_type,
		target_config::text, is_active, last_run_at, next_run_at, created_by, created_at, updated_at`,
		params.Name, params.TemplateID, params.ScheduleType, params.CronExpression,
		params.TargetType, params.TargetConfig, params.IsActive, id).Scan(
		&s.ID, &s.Name, &s.TemplateID, &s.ScheduleType, &s.CronExpression,
		&s.TargetType, &s.TargetConfig, &s.IsActive, &s.LastRunAt, &s.NextRunAt,
		&s.CreatedBy, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return s, ErrScheduleNotFound
	}
	return s, err
}

func (r *Repository) ToggleSchedule(ctx context.Context, id string, active bool) (model.WABroadcastSchedule, error) {
	var s model.WABroadcastSchedule
	err := repository.DB(ctx, r.db).QueryRow(ctx, `UPDATE wa_broadcast_schedules SET is_active = $1, updated_at = NOW()
		WHERE id = $2
		RETURNING id, name, template_id, schedule_type, cron_expression, target_type,
		target_config::text, is_active, last_run_at, next_run_at, created_by, created_at, updated_at`,
		active, id).Scan(
		&s.ID, &s.Name, &s.TemplateID, &s.ScheduleType, &s.CronExpression,
		&s.TargetType, &s.TargetConfig, &s.IsActive, &s.LastRunAt, &s.NextRunAt,
		&s.CreatedBy, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return s, ErrScheduleNotFound
	}
	return s, err
}

func (r *Repository) DeleteSchedule(ctx context.Context, id string) error {
	tag, err := repository.DB(ctx, r.db).Exec(ctx, "DELETE FROM wa_broadcast_schedules WHERE id = $1", id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrScheduleNotFound
	}
	return nil
}

func (r *Repository) UpdateScheduleLastRun(ctx context.Context, id string) error {
	_, err := repository.DB(ctx, r.db).Exec(ctx, "UPDATE wa_broadcast_schedules SET last_run_at = NOW() WHERE id = $1", id)
	return err
}

func (r *Repository) UpdateScheduleRunMetadata(ctx context.Context, id string, lastRunAt time.Time, nextRunAt *time.Time) error {
	_, err := repository.DB(ctx, r.db).Exec(ctx, `
		UPDATE wa_broadcast_schedules
		SET last_run_at = $2, next_run_at = $3, updated_at = NOW()
		WHERE id = $1
	`, id, lastRunAt, nextRunAt)
	return err
}

// --------------- Broadcast Logs ---------------

type CreateLogParams struct {
	ScheduleID      *string
	TemplateID      *string
	TemplateSlug    *string
	TriggerType     string
	RecipientUserID *string
	RecipientPhone  string
	MessageBody     string
	Status          string
	ErrorMessage    *string
	ReferenceType   *string
	ReferenceID     *string
}

type ListLogsParams struct {
	Page         int
	PerPage      int
	ScheduleID   string
	TriggerType  string
	TemplateSlug string
	Status       string
	DateFrom     string
	DateTo       string
	Search       string
}

func (r *Repository) CreateLog(ctx context.Context, params CreateLogParams) (model.WABroadcastLog, error) {
	sentAt := (*time.Time)(nil)
	if params.Status == "sent" {
		now := time.Now().UTC()
		sentAt = &now
	}

	var log model.WABroadcastLog
	err := repository.DB(ctx, r.db).QueryRow(ctx, `INSERT INTO wa_broadcast_logs
		(schedule_id, template_id, template_slug, trigger_type, recipient_user_id, recipient_phone,
		 message_body, status, error_message, reference_type, reference_id, sent_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, schedule_id, template_id, template_slug, trigger_type, recipient_user_id,
		recipient_phone, message_body, status, error_message, reference_type, reference_id,
		sent_at, created_at`,
		params.ScheduleID, params.TemplateID, params.TemplateSlug, params.TriggerType,
		params.RecipientUserID, params.RecipientPhone, params.MessageBody, params.Status,
		params.ErrorMessage, params.ReferenceType, params.ReferenceID, sentAt).Scan(
		&log.ID, &log.ScheduleID, &log.TemplateID, &log.TemplateSlug, &log.TriggerType,
		&log.RecipientUserID, &log.RecipientPhone, &log.MessageBody, &log.Status,
		&log.ErrorMessage, &log.ReferenceType, &log.ReferenceID, &log.SentAt, &log.CreatedAt)
	return log, err
}

func (r *Repository) ListLogs(ctx context.Context, params ListLogsParams) ([]model.WABroadcastLog, int64, error) {
	page := params.Page
	if page < 1 {
		page = 1
	}
	perPage := params.PerPage
	if perPage < 1 {
		perPage = 20
	}

	where := "WHERE 1=1"
	args := []interface{}{}
	argIdx := 1

	if params.ScheduleID != "" {
		where += fmt.Sprintf(" AND l.schedule_id = $%d", argIdx)
		args = append(args, params.ScheduleID)
		argIdx++
	}
	if params.TriggerType != "" {
		where += fmt.Sprintf(" AND l.trigger_type = $%d", argIdx)
		args = append(args, params.TriggerType)
		argIdx++
	}
	if params.TemplateSlug != "" {
		where += fmt.Sprintf(" AND l.template_slug = $%d", argIdx)
		args = append(args, params.TemplateSlug)
		argIdx++
	}
	if params.Status != "" {
		where += fmt.Sprintf(" AND l.status = $%d", argIdx)
		args = append(args, params.Status)
		argIdx++
	}
	if params.DateFrom != "" {
		where += fmt.Sprintf(" AND l.created_at >= $%d::timestamptz", argIdx)
		args = append(args, params.DateFrom)
		argIdx++
	}
	if params.DateTo != "" {
		where += fmt.Sprintf(" AND l.created_at <= $%d::timestamptz", argIdx)
		args = append(args, params.DateTo+"T23:59:59Z")
		argIdx++
	}
	if params.Search != "" {
		where += fmt.Sprintf(" AND (l.recipient_phone ILIKE $%d OR u.full_name ILIKE $%d)", argIdx, argIdx)
		args = append(args, "%"+params.Search+"%")
		argIdx++
	}

	var total int64
	countArgs := make([]interface{}, len(args))
	copy(countArgs, args)
	err := repository.DB(ctx, r.db).QueryRow(ctx,
		"SELECT COUNT(*) FROM wa_broadcast_logs l LEFT JOIN users u ON u.id = l.recipient_user_id "+where,
		countArgs...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count logs: %w", err)
	}

	query := fmt.Sprintf(`SELECT l.id, l.schedule_id, l.template_id, l.template_slug, l.trigger_type,
		l.recipient_user_id, l.recipient_phone, u.full_name, l.message_body, l.status,
		l.error_message, l.reference_type, l.reference_id, l.sent_at, l.created_at
		FROM wa_broadcast_logs l
		LEFT JOIN users u ON u.id = l.recipient_user_id
		%s ORDER BY l.created_at DESC
		LIMIT $%d OFFSET $%d`, where, argIdx, argIdx+1)
	args = append(args, perPage, (page-1)*perPage)

	rows, err := repository.DB(ctx, r.db).Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list logs: %w", err)
	}
	defer rows.Close()

	var logs []model.WABroadcastLog
	for rows.Next() {
		var log model.WABroadcastLog
		if err := rows.Scan(&log.ID, &log.ScheduleID, &log.TemplateID, &log.TemplateSlug,
			&log.TriggerType, &log.RecipientUserID, &log.RecipientPhone, &log.RecipientName,
			&log.MessageBody, &log.Status, &log.ErrorMessage, &log.ReferenceType,
			&log.ReferenceID, &log.SentAt, &log.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan log: %w", err)
		}
		logs = append(logs, log)
	}
	return logs, total, nil
}

func (r *Repository) GetLogSummary(ctx context.Context, date string) (model.WALogSummary, error) {
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	var summary model.WALogSummary
	err := repository.DB(ctx, r.db).QueryRow(ctx, `SELECT
		COALESCE(SUM(CASE WHEN status = 'sent' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN status LIKE 'skipped%' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN status = 'sent' AND created_at::date = $1::date THEN 1 ELSE 0 END), 0)
		FROM wa_broadcast_logs WHERE created_at::date = $1::date`, date).Scan(
		&summary.TotalSent, &summary.TotalFailed, &summary.TotalSkipped, &summary.SentToday)
	return summary, err
}

func (r *Repository) CountSentLogsToday(ctx context.Context) (int, error) {
	var count int
	err := repository.DB(ctx, r.db).QueryRow(ctx, `
		SELECT COUNT(*)
		FROM wa_broadcast_logs
		WHERE status = 'sent' AND created_at::date = CURRENT_DATE
	`).Scan(&count)
	return count, err
}

// CheckDuplicateToday checks if a message was already sent today for a given user+template+reference combo.
func (r *Repository) CheckDuplicateToday(ctx context.Context, userID string, templateSlug string, referenceID string) (bool, error) {
	var count int
	err := repository.DB(ctx, r.db).QueryRow(ctx, `SELECT COUNT(*) FROM wa_broadcast_logs
		WHERE recipient_user_id = $1 AND template_slug = $2 AND reference_id = $3
		AND created_at::date = CURRENT_DATE AND status = 'sent'`,
		userID, templateSlug, referenceID).Scan(&count)
	return count > 0, err
}

// --------------- Query helpers for scheduled jobs ---------------

type TaskDueInfo struct {
	TaskID      string
	TaskTitle   string
	ProjectID   string
	ProjectName string
	AssigneeID  string
	UserName    string
	UserEmail   string
	UserPhone   *string
	DueDate     string
	Priority    string
}

type BroadcastRecipient struct {
	UserID   string
	UserName string
	Phone    *string
}

func (r *Repository) ListActiveRecipients(ctx context.Context) ([]BroadcastRecipient, error) {
	return r.queryRecipients(ctx, `
		SELECT id::text, full_name, phone
		FROM users
		WHERE is_active = TRUE
		ORDER BY full_name ASC, id ASC
	`)
}

func (r *Repository) ListDepartmentRecipients(ctx context.Context, department string) ([]BroadcastRecipient, error) {
	return r.queryRecipients(ctx, `
		SELECT id::text, full_name, phone
		FROM users
		WHERE is_active = TRUE AND COALESCE(department, '') = $1
		ORDER BY full_name ASC, id ASC
	`, strings.TrimSpace(department))
}

func (r *Repository) ListSpecificRecipients(ctx context.Context, userIDs []string) ([]BroadcastRecipient, error) {
	trimmedIDs := make([]string, 0, len(userIDs))
	for _, userID := range userIDs {
		trimmedUserID := strings.TrimSpace(userID)
		if trimmedUserID == "" {
			continue
		}
		trimmedIDs = append(trimmedIDs, trimmedUserID)
	}
	if len(trimmedIDs) == 0 {
		return []BroadcastRecipient{}, nil
	}

	args := make([]interface{}, 0, len(trimmedIDs))
	placeholders := make([]string, 0, len(trimmedIDs))
	for index, userID := range trimmedIDs {
		args = append(args, userID)
		placeholders = append(placeholders, fmt.Sprintf("$%d::uuid", index+1))
	}

	query := fmt.Sprintf(`
		SELECT id::text, full_name, phone
		FROM users
		WHERE is_active = TRUE AND id IN (%s)
		ORDER BY full_name ASC, id ASC
	`, strings.Join(placeholders, ", "))

	return r.queryRecipients(ctx, query, args...)
}

func (r *Repository) ListProjectMemberRecipients(ctx context.Context, projectID string) ([]BroadcastRecipient, error) {
	return r.queryRecipients(ctx, `
		SELECT u.id::text, u.full_name, u.phone
		FROM project_members pm
		JOIN users u ON u.id = pm.user_id
		WHERE pm.project_id = $1::uuid AND u.is_active = TRUE
		ORDER BY pm.assigned_at ASC, u.id ASC
	`, strings.TrimSpace(projectID))
}

func (r *Repository) GetTasksDueToday(ctx context.Context) ([]TaskDueInfo, error) {
	return r.queryTasksByDue(ctx, `kt.due_date::date = CURRENT_DATE`)
}

func (r *Repository) GetTasksOverdue(ctx context.Context) ([]TaskDueInfo, error) {
	return r.queryTasksByDue(ctx, `kt.due_date::date < CURRENT_DATE`)
}

func (r *Repository) queryTasksByDue(ctx context.Context, dateCondition string) ([]TaskDueInfo, error) {
	query := fmt.Sprintf(`SELECT kt.id, kt.title, p.id, p.name, u.id, u.full_name, u.email, u.phone,
		COALESCE(to_char(kt.due_date, 'YYYY-MM-DD'), ''), kt.priority
		FROM kanban_tasks kt
		JOIN kanban_columns kc ON kc.id = kt.column_id
		JOIN projects p ON p.id = kt.project_id
		JOIN users u ON u.id = kt.assignee_id
		WHERE %s AND LOWER(kc.name) NOT IN ('done', 'archived')
		AND kt.assignee_id IS NOT NULL`, dateCondition)

	rows, err := repository.DB(ctx, r.db).Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query tasks: %w", err)
	}
	defer rows.Close()

	var tasks []TaskDueInfo
	for rows.Next() {
		var t TaskDueInfo
		if err := rows.Scan(&t.TaskID, &t.TaskTitle, &t.ProjectID, &t.ProjectName,
			&t.AssigneeID, &t.UserName, &t.UserEmail, &t.UserPhone, &t.DueDate, &t.Priority); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

func (r *Repository) queryRecipients(ctx context.Context, query string, args ...interface{}) ([]BroadcastRecipient, error) {
	rows, err := repository.DB(ctx, r.db).Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	recipients := make([]BroadcastRecipient, 0)
	for rows.Next() {
		var recipient BroadcastRecipient
		if err := rows.Scan(&recipient.UserID, &recipient.UserName, &recipient.Phone); err != nil {
			return nil, err
		}
		recipients = append(recipients, recipient)
	}

	return recipients, rows.Err()
}

type ProjectDeadlineInfo struct {
	ProjectID      string
	ProjectName    string
	Deadline       string
	Status         string
	OpenTaskCount  int
	TotalTaskCount int
	Members        []ProjectMemberInfo
}

type ProjectMemberInfo struct {
	UserID   string
	UserName string
	Phone    *string
}

func (r *Repository) GetProjectsDeadlineIn3Days(ctx context.Context) ([]ProjectDeadlineInfo, error) {
	rows, err := repository.DB(ctx, r.db).Query(ctx, `SELECT p.id, p.name,
		COALESCE(to_char(p.deadline, 'YYYY-MM-DD'), ''), p.status,
		COALESCE((
			SELECT COUNT(*)
			FROM kanban_tasks kt
			JOIN kanban_columns kc ON kc.id = kt.column_id
			WHERE kt.project_id = p.id AND LOWER(kc.name) NOT IN ('done', 'archived')
		), 0),
		COALESCE((SELECT COUNT(*) FROM kanban_tasks kt WHERE kt.project_id = p.id), 0)
		FROM projects p
		WHERE p.deadline::date = (CURRENT_DATE + INTERVAL '3 days')::date
		AND p.status IN ('active', 'in_progress')`)
	if err != nil {
		return nil, fmt.Errorf("query projects: %w", err)
	}
	defer rows.Close()

	var projects []ProjectDeadlineInfo
	for rows.Next() {
		var p ProjectDeadlineInfo
		if err := rows.Scan(&p.ProjectID, &p.ProjectName, &p.Deadline, &p.Status,
			&p.OpenTaskCount, &p.TotalTaskCount); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		projects = append(projects, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate projects: %w", err)
	}

	if len(projects) == 0 {
		return projects, nil
	}

	projectIDs := make([]string, 0, len(projects))
	for _, project := range projects {
		projectIDs = append(projectIDs, project.ProjectID)
	}

	memberMap, err := r.getProjectMembersByProjectIDs(ctx, projectIDs)
	if err != nil {
		return nil, err
	}

	for i := range projects {
		projects[i].Members = memberMap[projects[i].ProjectID]
	}

	return projects, nil
}

func (r *Repository) getProjectMembersByProjectIDs(ctx context.Context, projectIDs []string) (map[string][]ProjectMemberInfo, error) {
	args := make([]interface{}, 0, len(projectIDs))
	placeholders := make([]string, 0, len(projectIDs))
	for index, projectID := range projectIDs {
		args = append(args, projectID)
		placeholders = append(placeholders, fmt.Sprintf("$%d::uuid", index+1))
	}

	rows, err := repository.DB(ctx, r.db).Query(ctx, fmt.Sprintf(`SELECT pm.project_id::text, u.id, u.full_name, u.phone
		FROM project_members pm
		JOIN users u ON u.id = pm.user_id
		WHERE pm.project_id IN (%s)
		ORDER BY pm.project_id ASC, pm.assigned_at ASC, u.id ASC`, strings.Join(placeholders, ", ")), args...)
	if err != nil {
		return nil, fmt.Errorf("query members: %w", err)
	}
	defer rows.Close()

	memberMap := make(map[string][]ProjectMemberInfo, len(projectIDs))
	for rows.Next() {
		var projectID string
		var m ProjectMemberInfo
		if err := rows.Scan(&projectID, &m.UserID, &m.UserName, &m.Phone); err != nil {
			return nil, fmt.Errorf("scan member: %w", err)
		}
		memberMap[projectID] = append(memberMap[projectID], m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate members: %w", err)
	}
	return memberMap, nil
}

type WeeklyDigestInfo struct {
	UserID         string
	UserName       string
	UserEmail      string
	Phone          *string
	CompletedCount int
	OpenCount      int
	OverdueCount   int
}

func (r *Repository) GetWeeklyDigestData(ctx context.Context) ([]WeeklyDigestInfo, error) {
	rows, err := repository.DB(ctx, r.db).Query(ctx, `SELECT u.id, u.full_name, u.email, u.phone,
		COALESCE(SUM(CASE WHEN LOWER(kc.name) = 'done' AND kt.updated_at >= (CURRENT_DATE - INTERVAL '7 days') THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN LOWER(kc.name) NOT IN ('done', 'archived') THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN LOWER(kc.name) NOT IN ('done', 'archived') AND kt.due_date < CURRENT_DATE THEN 1 ELSE 0 END), 0)
		FROM users u
		JOIN kanban_tasks kt ON kt.assignee_id = u.id
		JOIN kanban_columns kc ON kc.id = kt.column_id
		WHERE u.is_active = true
		GROUP BY u.id, u.full_name, u.email, u.phone
		HAVING SUM(1) > 0`)
	if err != nil {
		return nil, fmt.Errorf("query weekly digest: %w", err)
	}
	defer rows.Close()

	var items []WeeklyDigestInfo
	for rows.Next() {
		var d WeeklyDigestInfo
		if err := rows.Scan(&d.UserID, &d.UserName, &d.UserEmail, &d.Phone,
			&d.CompletedCount, &d.OpenCount, &d.OverdueCount); err != nil {
			return nil, fmt.Errorf("scan digest: %w", err)
		}
		items = append(items, d)
	}
	return items, nil
}

// GetTaskWithProject returns task and project info for a single task.
func (r *Repository) GetTaskWithProject(ctx context.Context, taskID string) (*TaskDueInfo, error) {
	var t TaskDueInfo
	err := repository.DB(ctx, r.db).QueryRow(ctx, `SELECT kt.id, kt.title, p.id, p.name, u.id, u.full_name, u.email, u.phone,
		COALESCE(to_char(kt.due_date, 'YYYY-MM-DD'), ''), kt.priority
		FROM kanban_tasks kt
		JOIN projects p ON p.id = kt.project_id
		JOIN users u ON u.id = kt.assignee_id
		WHERE kt.id = $1`, taskID).Scan(
		&t.TaskID, &t.TaskTitle, &t.ProjectID, &t.ProjectName,
		&t.AssigneeID, &t.UserName, &t.UserEmail, &t.UserPhone, &t.DueDate, &t.Priority)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// GetReimbursementWithSubmitter returns reimbursement and submitter user info.
type ReimbursementNotifyInfo struct {
	ReimbursementID string
	Title           string
	Amount          int64
	Status          string
	SubmitterID     string
	SubmitterName   string
	SubmitterEmail  string
	SubmitterPhone  *string
}

func (r *Repository) GetReimbursementWithSubmitter(ctx context.Context, reimbursementID string) (*ReimbursementNotifyInfo, error) {
	var info ReimbursementNotifyInfo
	err := repository.DB(ctx, r.db).QueryRow(ctx, `SELECT r.id, r.title, r.amount, r.status,
		u.id, u.full_name, u.email, u.phone
		FROM reimbursements r
		JOIN users u ON u.id = r.submitted_by
		WHERE r.id = $1`, reimbursementID).Scan(
		&info.ReimbursementID, &info.Title, &info.Amount, &info.Status,
		&info.SubmitterID, &info.SubmitterName, &info.SubmitterEmail, &info.SubmitterPhone)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &info, nil
}

// GetUserPhone returns the phone number of a user.
func (r *Repository) GetUserPhone(ctx context.Context, userID string) (*string, error) {
	var phone *string
	err := repository.DB(ctx, r.db).QueryRow(ctx, "SELECT phone FROM users WHERE id = $1", userID).Scan(&phone)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return phone, err
}

// UpdateUserPhone updates the phone number of a user.
func (r *Repository) UpdateUserPhone(ctx context.Context, userID string, phone *string) error {
	normalized := (*string)(nil)
	if phone != nil {
		p := normalizePhone(strings.TrimSpace(*phone))
		if p != "" {
			normalized = &p
		}
	}
	_, err := repository.DB(ctx, r.db).Exec(ctx, "UPDATE users SET phone = $1, updated_at = NOW() WHERE id = $2", normalized, userID)
	if err != nil {
		return err
	}
	// Sync phone to employees table so both stay in sync.
	_, _ = repository.DB(ctx, r.db).Exec(ctx, "UPDATE employees SET phone = $1, updated_at = NOW() WHERE user_id = $2::uuid", normalized, userID)
	return nil
}

func normalizePhone(phone string) string {
	phone = strings.TrimSpace(phone)
	phone = strings.ReplaceAll(phone, " ", "")
	phone = strings.ReplaceAll(phone, "-", "")
	if strings.HasPrefix(phone, "+") {
		phone = phone[1:]
	}
	if strings.HasPrefix(phone, "08") {
		phone = "62" + phone[1:]
	}
	return phone
}
