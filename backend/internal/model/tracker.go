package model

import "time"

type ActivitySession struct {
	ID                    string     `json:"id"`
	UserID                string     `json:"user_id"`
	Date                  time.Time  `json:"date"`
	TimezoneOffsetMinutes int        `json:"timezone_offset_minutes"`
	StartTime             time.Time  `json:"start_time"`
	EndTime               *time.Time `json:"end_time,omitempty"`
	TotalActiveSeconds    int        `json:"total_active_seconds"`
	TotalIdleSeconds      int        `json:"total_idle_seconds"`
	IsActive              bool       `json:"is_active"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

type ActivityEntry struct {
	ID              string    `json:"id"`
	SessionID       string    `json:"session_id"`
	UserID          string    `json:"user_id"`
	URL             string    `json:"url"`
	Domain          string    `json:"domain"`
	PageTitle       *string   `json:"page_title,omitempty"`
	Category        string    `json:"category"`
	IsProductive    bool      `json:"is_productive"`
	DurationSeconds int       `json:"duration_seconds"`
	StartedAt       time.Time `json:"started_at"`
	EndedAt         time.Time `json:"ended_at"`
	CreatedAt       time.Time `json:"created_at"`
}

type DomainCategory struct {
	ID            string    `json:"id"`
	DomainPattern string    `json:"domain_pattern"`
	Category      string    `json:"category"`
	IsProductive  bool      `json:"is_productive"`
	CreatedAt     time.Time `json:"created_at"`
}

type ActivityConsent struct {
	ID          string     `json:"id"`
	UserID      string     `json:"user_id"`
	Consented   bool       `json:"consented"`
	ConsentedAt *time.Time `json:"consented_at,omitempty"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
	IPAddress   *string    `json:"ip_address,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

type TrackerConsentAudit struct {
	UserID                     string     `json:"user_id"`
	UserName                   string     `json:"user_name"`
	UserEmail                  string     `json:"user_email"`
	Consented                  bool       `json:"consented"`
	ConsentedAt                *time.Time `json:"consented_at,omitempty"`
	RevokedAt                  *time.Time `json:"revoked_at,omitempty"`
	IPAddress                  *string    `json:"ip_address,omitempty"`
	BrowserTimezone            *string    `json:"browser_timezone,omitempty"`
	TrackerExtensionVersion    *string    `json:"tracker_extension_version,omitempty"`
	TrackerExtensionReportedAt *time.Time `json:"tracker_extension_reported_at,omitempty"`
	LastSessionStartedAt       *time.Time `json:"last_session_started_at,omitempty"`
	LastActivityAt             *time.Time `json:"last_activity_at,omitempty"`
}

type TrackerCategoryBreakdown struct {
	Category        string `json:"category"`
	DurationSeconds int64  `json:"duration_seconds"`
	IsProductive    bool   `json:"is_productive"`
}

type TrackerHourlyBreakdown struct {
	Hour            int    `json:"hour"`
	Label           string `json:"label"`
	DurationSeconds int64  `json:"duration_seconds"`
}

type TrackerTopDomain struct {
	Domain          string  `json:"domain"`
	Category        string  `json:"category"`
	DurationSeconds int64   `json:"duration_seconds"`
	IsProductive    bool    `json:"is_productive"`
	Percentage      float64 `json:"percentage"`
}

type TrackerActivityOverview struct {
	UserID             string                     `json:"user_id"`
	UserName           string                     `json:"user_name"`
	TotalActiveSeconds int64                      `json:"total_active_seconds"`
	TotalIdleSeconds   int64                      `json:"total_idle_seconds"`
	ProductivityScore  float64                    `json:"productivity_score"`
	MostUsedDomain     *string                    `json:"most_used_domain,omitempty"`
	CategoryBreakdown  []TrackerCategoryBreakdown `json:"category_breakdown"`
	HourlyBreakdown    []TrackerHourlyBreakdown   `json:"hourly_breakdown"`
	TopDomains         []TrackerTopDomain         `json:"top_domains"`
}

type TrackerUserSummary struct {
	UserID            string           `json:"user_id"`
	UserName          string           `json:"user_name"`
	ActiveSeconds     int64            `json:"active_seconds"`
	IdleSeconds       int64            `json:"idle_seconds"`
	ProductivityScore float64          `json:"productivity_score"`
	TopDomain         *string          `json:"top_domain,omitempty"`
	CategoryBreakdown map[string]int64 `json:"category_breakdown"`
}

type TrackerTeamOverview struct {
	MembersTracked        int64                `json:"members_tracked"`
	AvgActiveSeconds      int64                `json:"avg_active_seconds"`
	TopProductiveMember   *string              `json:"top_productive_member,omitempty"`
	LeastProductiveMember *string              `json:"least_productive_member,omitempty"`
	Users                 []TrackerUserSummary `json:"users"`
}

type TrackerDailySummary struct {
	TotalUsers             int64              `json:"total_users"`
	AvgActiveSeconds       int64              `json:"avg_active_seconds"`
	TopProductiveDomains   []TrackerTopDomain `json:"top_productive_domains"`
	TopUnproductiveDomains []TrackerTopDomain `json:"top_unproductive_domains"`
}
