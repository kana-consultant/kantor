package model

import "time"

type WAMessageTemplate struct {
	ID                 string    `json:"id"`
	Name               string    `json:"name"`
	Slug               string    `json:"slug"`
	Category           string    `json:"category"`
	TriggerType        string    `json:"trigger_type"`
	BodyTemplate       string    `json:"body_template"`
	Description        *string   `json:"description,omitempty"`
	AvailableVariables []string  `json:"available_variables"`
	IsActive           bool      `json:"is_active"`
	IsSystem           bool      `json:"is_system"`
	CreatedBy          *string   `json:"created_by,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type WABroadcastSchedule struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	TemplateID     string     `json:"template_id"`
	TemplateName   string     `json:"template_name,omitempty"`
	ScheduleType   string     `json:"schedule_type"`
	CronExpression *string    `json:"cron_expression,omitempty"`
	TargetType     string     `json:"target_type"`
	TargetConfig   *string    `json:"target_config,omitempty"`
	IsActive       bool       `json:"is_active"`
	LastRunAt      *time.Time `json:"last_run_at,omitempty"`
	NextRunAt      *time.Time `json:"next_run_at,omitempty"`
	CreatedBy      *string    `json:"created_by,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type WABroadcastLog struct {
	ID              string     `json:"id"`
	ScheduleID      *string    `json:"schedule_id,omitempty"`
	TemplateID      *string    `json:"template_id,omitempty"`
	TemplateSlug    *string    `json:"template_slug,omitempty"`
	TriggerType     string     `json:"trigger_type"`
	RecipientUserID *string    `json:"recipient_user_id,omitempty"`
	RecipientPhone  string     `json:"recipient_phone"`
	RecipientName   *string    `json:"recipient_name,omitempty"`
	MessageBody     string     `json:"message_body"`
	Status          string     `json:"status"`
	ErrorMessage    *string    `json:"error_message,omitempty"`
	ReferenceType   *string    `json:"reference_type,omitempty"`
	ReferenceID     *string    `json:"reference_id,omitempty"`
	SentAt          *time.Time `json:"sent_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

type WALogSummary struct {
	TotalSent    int `json:"total_sent"`
	TotalFailed  int `json:"total_failed"`
	TotalSkipped int `json:"total_skipped"`
	DailyLimit   int `json:"daily_limit"`
	SentToday    int `json:"sent_today"`
}
