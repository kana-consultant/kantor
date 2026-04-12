package model

import "time"

type ReminderChannels struct {
	InApp    bool `json:"in_app"`
	Email    bool `json:"email"`
	WhatsApp bool `json:"whatsapp"`
}

type ReimbursementReminderRule struct {
	Enabled  bool             `json:"enabled"`
	Cron     string           `json:"cron"`
	Channels ReminderChannels `json:"channels"`
}

type ReimbursementReminderSetting struct {
	Enabled bool                      `json:"enabled"`
	Review  ReimbursementReminderRule `json:"review"`
	Payment ReimbursementReminderRule `json:"payment"`
}

type ReimbursementReminderRecipient struct {
	UserID    string  `json:"user_id"`
	UserName  string  `json:"user_name"`
	UserEmail string  `json:"user_email"`
	Phone     *string `json:"phone,omitempty"`
}

type ReimbursementReminderItem struct {
	ReimbursementID string    `json:"reimbursement_id"`
	Title           string    `json:"title"`
	EmployeeName    string    `json:"employee_name"`
	Amount          int64     `json:"amount"`
	CreatedAt       time.Time `json:"created_at"`
}

type ReimbursementReminderDigest struct {
	Kind            string                      `json:"kind"`
	Status          string                      `json:"status"`
	Title           string                      `json:"title"`
	PendingCount    int                         `json:"pending_count"`
	TotalAmount     int64                       `json:"total_amount"`
	OldestCreatedAt *time.Time                  `json:"oldest_created_at,omitempty"`
	Items           []ReimbursementReminderItem `json:"items"`
}
