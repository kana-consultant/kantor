package operational

type UpdateTrackerReminderConfigRequest struct {
	Enabled               bool   `json:"enabled"`
	StartHour             int    `json:"start_hour" validate:"gte=0,lte=23"`
	EndHour               int    `json:"end_hour" validate:"gte=1,lte=24"`
	WeekdaysOnly          bool   `json:"weekdays_only"`
	Timezone              string `json:"timezone" validate:"required,max=64"`
	HeartbeatStaleMinutes int    `json:"heartbeat_stale_minutes" validate:"gte=1,lte=1440"`
	NotifyInApp           bool   `json:"notify_in_app"`
	NotifyWhatsapp        bool   `json:"notify_whatsapp"`
}

type TrackerReminderConfigResponse struct {
	TenantID              string  `json:"tenant_id"`
	Enabled               bool    `json:"enabled"`
	StartHour             int     `json:"start_hour"`
	EndHour               int     `json:"end_hour"`
	WeekdaysOnly          bool    `json:"weekdays_only"`
	Timezone              string  `json:"timezone"`
	HeartbeatStaleMinutes int     `json:"heartbeat_stale_minutes"`
	NotifyInApp           bool    `json:"notify_in_app"`
	NotifyWhatsapp        bool    `json:"notify_whatsapp"`
	NextReminderAt        *string `json:"next_reminder_at,omitempty"`
	UpdatedAt             string  `json:"updated_at"`
}

type TrackerReminderTestResponse struct {
	DeliveredInApp    bool    `json:"delivered_in_app"`
	DeliveredWhatsapp bool    `json:"delivered_whatsapp"`
	WhatsappError     *string `json:"whatsapp_error,omitempty"`
}
