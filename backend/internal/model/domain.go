package model

import "time"

// Domain is a registered domain tracked by the operational module.
// Inventory + renewal alert + DNS resolution check + WHOIS auto-sync.
// Mirror of VPSServer pattern but flatter — typically 1 DNS check per
// domain so all check fields live inline.
type Domain struct {
	ID           string     `json:"id"`
	TenantID     string     `json:"tenant_id"`
	Name         string     `json:"name"`
	Registrar    string     `json:"registrar"`
	Nameservers  []string   `json:"nameservers"`
	ExpiryDate   *time.Time `json:"expiry_date,omitempty"`
	CostAmount   int64      `json:"cost_amount"`
	CostCurrency string     `json:"cost_currency"`
	BillingCycle string     `json:"billing_cycle"`
	Status       string     `json:"status"`
	Tags         []string   `json:"tags"`
	Notes        string     `json:"notes"`

	// DNS resolution check
	DNSCheckEnabled         bool       `json:"dns_check_enabled"`
	DNSExpectedIP           string     `json:"dns_expected_ip"`
	DNSCheckIntervalSeconds int        `json:"dns_check_interval_seconds"`
	DNSLastStatus           string     `json:"dns_last_status"`
	DNSLastResolvedIPs      []string   `json:"dns_last_resolved_ips"`
	DNSLastError            string     `json:"dns_last_error"`
	DNSLastCheckAt          *time.Time `json:"dns_last_check_at,omitempty"`
	DNSLastStatusChangedAt  *time.Time `json:"dns_last_status_changed_at,omitempty"`
	DNSConsecutiveFails     int        `json:"dns_consecutive_fails"`
	DNSAlertActive          bool       `json:"dns_alert_active"`
	DNSAlertLastSentAt      *time.Time `json:"dns_alert_last_sent_at,omitempty"`

	// WHOIS auto-sync
	WhoisSyncEnabled bool       `json:"whois_sync_enabled"`
	WhoisLastSyncAt  *time.Time `json:"whois_last_sync_at,omitempty"`
	WhoisLastError   string     `json:"whois_last_error"`

	CreatedBy *string   `json:"created_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DomainHealthEvent is a single probe / sync result. Retention 7 days.
type DomainHealthEvent struct {
	ID        string    `json:"id"`
	DomainID  string    `json:"domain_id"`
	EventType string    `json:"event_type"` // 'dns' | 'whois'
	Status    string    `json:"status"`     // 'up' | 'down' | 'synced' | 'error'
	Detail    string    `json:"detail"`
	CreatedAt time.Time `json:"created_at"`
}
