package model

import "time"

type User struct {
	ID                           string     `json:"id"`
	Email                        string     `json:"email"`
	PasswordHash                 string     `json:"-"`
	FullName                     string     `json:"full_name"`
	AvatarURL                    *string    `json:"avatar_url,omitempty"`
	Department                   *string    `json:"department,omitempty"`
	Skills                       []string   `json:"skills,omitempty"`
	BrowserTimezone              *string    `json:"browser_timezone,omitempty"`
	BrowserTimezoneOffsetMinutes *int       `json:"browser_timezone_offset_minutes,omitempty"`
	BrowserLocale                *string    `json:"browser_locale,omitempty"`
	TrackerExtensionVersion      *string    `json:"tracker_extension_version,omitempty"`
	TrackerExtensionReportedAt   *time.Time `json:"tracker_extension_reported_at,omitempty"`
	IsActive                     bool       `json:"is_active"`
	IsSuperAdmin                 bool       `json:"is_super_admin"`
	FailedLoginAttempts          int        `json:"-"`
	LockedUntil                  *time.Time `json:"-"`
	CreatedAt                    time.Time  `json:"created_at"`
	UpdatedAt                    time.Time  `json:"updated_at"`
}

type RefreshToken struct {
	ID         string
	UserID     string
	TokenHash  string
	ExpiresAt  time.Time
	RevokedAt  *time.Time
	CreatedAt  time.Time
	LastUsedAt *time.Time
	UserAgent  *string
}
