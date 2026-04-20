package model

import "time"

type TrackerReminderConfig struct {
	TenantID              string    `json:"tenant_id"`
	Enabled               bool      `json:"enabled"`
	StartHour             int       `json:"start_hour"`
	EndHour               int       `json:"end_hour"`
	WeekdaysOnly          bool      `json:"weekdays_only"`
	Timezone              string    `json:"timezone"`
	HeartbeatStaleMinutes int       `json:"heartbeat_stale_minutes"`
	NotifyInApp           bool      `json:"notify_in_app"`
	NotifyWhatsapp        bool      `json:"notify_whatsapp"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type TrackerReminderCandidate struct {
	UserID   string
	FullName string
	Phone    *string
}
