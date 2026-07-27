package model

import "time"

// DiscordReminderConfig is the per-tenant configuration for pushing the daily
// task-reminder digest to an external Discord bot over HTTP. Stored in the DB
// (not env) because prod env is managed by an external nix-config.
type DiscordReminderConfig struct {
	TenantID     string    `json:"tenant_id"`
	Enabled      bool      `json:"enabled"`
	WebhookURL   string    `json:"webhook_url"`
	SharedSecret string    `json:"-"` // never serialize the plaintext secret
	SendHour     int       `json:"send_hour"`
	WeekdaysOnly bool      `json:"weekdays_only"`
	Timezone     string    `json:"timezone"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// DiscordReminderTaskRow is a single open kanban task assigned to an active
// employee, as returned by the digest source query.
type DiscordReminderTaskRow struct {
	UserID      string
	FullName    string
	TaskID      string
	Title       string
	ProjectName string
	ColumnName  string
	DueDate     string
	Priority    string
}
