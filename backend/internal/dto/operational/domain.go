package operational

// CreateDomainRequest is the form payload for adding a new domain to the
// inventory. Validators check structural invariants only — registrar,
// nameservers, etc. stay free-form.
type CreateDomainRequest struct {
	Name                    string   `json:"name" validate:"required,min=3,max=253"`
	Registrar               string   `json:"registrar" validate:"max=80"`
	Nameservers             []string `json:"nameservers"`
	ExpiryDate              *string  `json:"expiry_date,omitempty"` // ISO date YYYY-MM-DD
	CostAmount              int64    `json:"cost_amount" validate:"gte=0"`
	CostCurrency            string   `json:"cost_currency" validate:"max=8"`
	BillingCycle            string   `json:"billing_cycle" validate:"required,oneof=monthly yearly"`
	Status                  string   `json:"status" validate:"required,oneof=active expired transferring parked"`
	Tags                    []string `json:"tags"`
	Notes                   string   `json:"notes"`
	DNSCheckEnabled         *bool    `json:"dns_check_enabled,omitempty"`
	DNSExpectedIP           string   `json:"dns_expected_ip" validate:"max=64"`
	DNSCheckIntervalSeconds int      `json:"dns_check_interval_seconds" validate:"omitempty,min=60,max=86400"`
	WhoisSyncEnabled        *bool    `json:"whois_sync_enabled,omitempty"`
}

type UpdateDomainRequest = CreateDomainRequest
