package operational

// CreateVPSRequest is the form payload for adding a new VPS row to the
// inventory. Most fields are free-form metadata; the validators only check
// the structural invariants (status enum, billing cycle enum) so users can
// keep adding new providers / regions without touching the backend.
type CreateVPSRequest struct {
	Label        string   `json:"label" validate:"required,min=1,max=100"`
	Provider     string   `json:"provider" validate:"max=80"`
	Hostname     string   `json:"hostname" validate:"max=255"`
	IPAddress    string   `json:"ip_address" validate:"max=64"`
	Region       string   `json:"region" validate:"max=80"`
	CPUCores     int      `json:"cpu_cores" validate:"gte=0,lte=2048"`
	RAMMB        int      `json:"ram_mb" validate:"gte=0"`
	DiskGB       int      `json:"disk_gb" validate:"gte=0"`
	CostAmount   int64    `json:"cost_amount" validate:"gte=0"`
	CostCurrency string   `json:"cost_currency" validate:"max=8"`
	BillingCycle string   `json:"billing_cycle" validate:"required,oneof=monthly quarterly yearly"`
	RenewalDate  *string  `json:"renewal_date,omitempty"` // ISO date YYYY-MM-DD
	Status       string   `json:"status" validate:"required,oneof=active suspended decommissioned"`
	Tags         []string `json:"tags"`
	Notes        string   `json:"notes"`
}

type UpdateVPSRequest = CreateVPSRequest

// CreateVPSCheckRequest creates a single probe (icmp/tcp/http/https) for a
// VPS. Target validation is intentionally permissive — the backend will
// fail-fast at probe time if the value is malformed.
type CreateVPSCheckRequest struct {
	Label           string `json:"label" validate:"required,min=1,max=80"`
	Type            string `json:"type" validate:"required,oneof=icmp tcp http https"`
	Target          string `json:"target" validate:"required,min=1,max=255"`
	IntervalSeconds int    `json:"interval_seconds" validate:"required,min=30,max=86400"`
	TimeoutSeconds  int    `json:"timeout_seconds" validate:"required,min=1,max=60"`
	Enabled         *bool  `json:"enabled,omitempty"`
}

type UpdateVPSCheckRequest = CreateVPSCheckRequest

// CreateVPSAppRequest registers an app on a VPS. CheckID is optional — apps
// without a check are pure documentation rows.
type CreateVPSAppRequest struct {
	Name    string  `json:"name" validate:"required,min=1,max=80"`
	AppType string  `json:"app_type" validate:"max=40"`
	Port    *int    `json:"port,omitempty" validate:"omitempty,min=1,max=65535"`
	URL     string  `json:"url" validate:"max=512"`
	Notes   string  `json:"notes"`
	CheckID *string `json:"check_id,omitempty"`
}

type UpdateVPSAppRequest = CreateVPSAppRequest

// VPSDetailResponse is the payload returned by GET /vps/{id} — server with
// its associated checks and apps. Frontend renders three tabs from this.
type VPSDetailResponse struct {
	Server any   `json:"server"`
	Checks []any `json:"checks"`
	Apps   []any `json:"apps"`
}
