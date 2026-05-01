package model

import "time"

// VPSServerSummary is a VPSServer enriched with per-VPS counts (apps,
// checks, down checks) so the list view can render badges without an
// extra round-trip.
type VPSServerSummary struct {
	VPSServer
	AppsCount       int `json:"apps_count"`
	ChecksCount     int `json:"checks_count"`
	DownChecksCount int `json:"down_checks_count"`
}

// VPSServer is a single virtual server tracked by the operational module.
// Inventory only — no credentials are stored, only public-ish metadata that
// helps the owner understand what runs where and when each lease renews.
type VPSServer struct {
	ID                  string     `json:"id"`
	TenantID            string     `json:"tenant_id"`
	Label               string     `json:"label"`
	Provider            string     `json:"provider"`
	Hostname            string     `json:"hostname"`
	IPAddress           string     `json:"ip_address"`
	Region              string     `json:"region"`
	CPUCores            int        `json:"cpu_cores"`
	RAMMB               int        `json:"ram_mb"`
	DiskGB              int        `json:"disk_gb"`
	CostAmount          int64      `json:"cost_amount"`
	CostCurrency        string     `json:"cost_currency"`
	BillingCycle        string     `json:"billing_cycle"`
	RenewalDate         *time.Time `json:"renewal_date,omitempty"`
	Status              string     `json:"status"`
	Tags                []string   `json:"tags"`
	Notes               string     `json:"notes"`
	LastStatus          string     `json:"last_status"`
	LastStatusChangedAt *time.Time `json:"last_status_changed_at,omitempty"`
	LastCheckAt         *time.Time `json:"last_check_at,omitempty"`
	CreatedBy           *string    `json:"created_by,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// VPSApp is something running on a VPS. The check_id link is optional — apps
// without a check are pure documentation rows.
type VPSApp struct {
	ID        string    `json:"id"`
	VPSID     string    `json:"vps_id"`
	Name      string    `json:"name"`
	AppType   string    `json:"app_type"`
	Port      *int      `json:"port,omitempty"`
	URL       string    `json:"url"`
	Notes     string    `json:"notes"`
	CheckID   *string   `json:"check_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// VPSHealthCheck is one probe configured against a VPS. The runtime fields
// (last_*, consecutive_*, alert_*) are mutated by the monitor goroutine and
// surfaced to the UI so users see the latest state without scanning the
// event log.
type VPSHealthCheck struct {
	ID                   string     `json:"id"`
	VPSID                string     `json:"vps_id"`
	Label                string     `json:"label"`
	Type                 string     `json:"type"`
	Target               string     `json:"target"`
	IntervalSeconds      int        `json:"interval_seconds"`
	TimeoutSeconds       int        `json:"timeout_seconds"`
	Enabled              bool       `json:"enabled"`
	LastStatus           string     `json:"last_status"`
	LastLatencyMS        *int       `json:"last_latency_ms,omitempty"`
	LastError            string     `json:"last_error"`
	LastCheckAt          *time.Time `json:"last_check_at,omitempty"`
	LastStatusChangedAt  *time.Time `json:"last_status_changed_at,omitempty"`
	ConsecutiveFails     int        `json:"consecutive_fails"`
	ConsecutiveSuccesses int        `json:"consecutive_successes"`
	AlertActive          bool       `json:"alert_active"`
	AlertLastSentAt      *time.Time `json:"alert_last_sent_at,omitempty"`
	SSLExpiresAt         *time.Time `json:"ssl_expires_at,omitempty"`
	SSLIssuer            string     `json:"ssl_issuer"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

// VPSHealthEvent is a single probe result. Retention is handled by a
// background sweeper — see operational service for the policy.
type VPSHealthEvent struct {
	ID           string    `json:"id"`
	VPSID        string    `json:"vps_id"`
	CheckID      string    `json:"check_id"`
	Status       string    `json:"status"`
	LatencyMS    *int      `json:"latency_ms,omitempty"`
	ErrorMessage string    `json:"error_message"`
	CreatedAt    time.Time `json:"created_at"`
}

// VPSHealthDailySummary holds the per-check uptime aggregation. Kept
// indefinitely so the UI can render long-range charts without scanning the
// raw event log.
type VPSHealthDailySummary struct {
	VPSID        string    `json:"vps_id"`
	CheckID      string    `json:"check_id"`
	SummaryDate  time.Time `json:"summary_date"`
	TotalChecks  int       `json:"total_checks"`
	UpCount      int       `json:"up_count"`
	DownCount    int       `json:"down_count"`
	UptimePct    float64   `json:"uptime_pct"`
	AvgLatencyMS *int      `json:"avg_latency_ms,omitempty"`
	P95LatencyMS *int      `json:"p95_latency_ms,omitempty"`
}
