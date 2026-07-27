package operational

type UpdateDiscordReminderConfigRequest struct {
	WebhookURL   string `json:"webhook_url" validate:"omitempty,url"`
	SharedSecret string `json:"shared_secret"`
	Enabled      bool   `json:"enabled"`
	SendHour     int    `json:"send_hour" validate:"min=0,max=23"`
	WeekdaysOnly bool   `json:"weekdays_only"`
	Timezone     string `json:"timezone" validate:"required,max=64"`
}

// DiscordReminderConfigResponse never includes the plaintext shared secret —
// this endpoint auto-becomes an MCP tool readable by module users, so
// HasSecret is the only signal exposed about whether one is configured.
type DiscordReminderConfigResponse struct {
	TenantID     string `json:"tenant_id"`
	Enabled      bool   `json:"enabled"`
	WebhookURL   string `json:"webhook_url"`
	HasSecret    bool   `json:"has_secret"`
	SendHour     int    `json:"send_hour"`
	WeekdaysOnly bool   `json:"weekdays_only"`
	Timezone     string `json:"timezone"`
	UpdatedAt    string `json:"updated_at"`
}

type DiscordReminderTestResponse struct {
	Sent int `json:"sent"`
}
