package operational

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	operationaldto "github.com/kana-consultant/kantor/backend/internal/dto/operational"
	"github.com/kana-consultant/kantor/backend/internal/model"
	operationalrepo "github.com/kana-consultant/kantor/backend/internal/repository/operational"
)

var (
	ErrDomainNotFound = errors.New("domain not found")
	ErrInvalidDomain  = errors.New("invalid domain")
)

type domainRepository interface {
	CreateDomain(ctx context.Context, p operationalrepo.CreateDomainParams) (model.Domain, error)
	UpdateDomain(ctx context.Context, domainID string, p operationalrepo.UpdateDomainParams) (model.Domain, error)
	DeleteDomain(ctx context.Context, domainID string) error
	GetDomainByID(ctx context.Context, domainID string) (model.Domain, error)
	ListDomains(ctx context.Context, p operationalrepo.ListDomainParams) ([]model.Domain, error)
	ListDomainsWithExpiryBefore(ctx context.Context, cutoff time.Time) ([]model.Domain, error)
	ListDueDNSChecks(ctx context.Context, now time.Time) ([]model.Domain, error)
	ListWhoisSyncDue(ctx context.Context, now time.Time) ([]model.Domain, error)
	RecordDNSCheckResult(ctx context.Context, domainID string, result operationalrepo.DNSCheckResult) (model.Domain, bool, error)
	MarkDNSAlertActive(ctx context.Context, domainID string, now time.Time) error
	ClearDNSAlert(ctx context.Context, domainID string) error
	SetWhoisSyncResult(ctx context.Context, domainID string, now time.Time, expiry *time.Time, errMsg string) error
	CreateEvent(ctx context.Context, domainID, eventType, status, detail string) error
	ListEventsForDomain(ctx context.Context, domainID string, limit int) ([]model.DomainHealthEvent, error)
	PurgeOldEvents(ctx context.Context, cutoff time.Time) (int64, error)
}

type DomainService struct {
	repo domainRepository
}

func NewDomainService(repo domainRepository) *DomainService {
	return &DomainService{repo: repo}
}

// DomainDetail bundles a domain with recent event log for the detail page.
type DomainDetail struct {
	Domain model.Domain               `json:"domain"`
	Events []model.DomainHealthEvent  `json:"events"`
}

func (s *DomainService) CreateDomain(ctx context.Context, req operationaldto.CreateDomainRequest, actorID string) (model.Domain, error) {
	name := strings.ToLower(strings.TrimSpace(req.Name))
	if !looksLikeDomain(name) {
		return model.Domain{}, fmt.Errorf("%w: name must be a valid domain like example.com", ErrInvalidDomain)
	}
	expiry, err := parseExpiryDate(req.ExpiryDate)
	if err != nil {
		return model.Domain{}, err
	}
	currency := strings.ToUpper(strings.TrimSpace(req.CostCurrency))
	if currency == "" {
		currency = "IDR"
	}
	dnsEnabled := true
	if req.DNSCheckEnabled != nil {
		dnsEnabled = *req.DNSCheckEnabled
	}
	whoisEnabled := true
	if req.WhoisSyncEnabled != nil {
		whoisEnabled = *req.WhoisSyncEnabled
	}
	interval := req.DNSCheckIntervalSeconds
	if interval <= 0 {
		interval = 3600
	}

	return s.repo.CreateDomain(ctx, operationalrepo.CreateDomainParams{
		Name:                    name,
		Registrar:               strings.TrimSpace(req.Registrar),
		Nameservers:             normaliseNameservers(req.Nameservers),
		ExpiryDate:              expiry,
		CostAmount:              req.CostAmount,
		CostCurrency:            currency,
		BillingCycle:            req.BillingCycle,
		Status:                  req.Status,
		Tags:                    normaliseTags(req.Tags),
		Notes:                   strings.TrimSpace(req.Notes),
		DNSCheckEnabled:         dnsEnabled,
		DNSExpectedIP:           strings.TrimSpace(req.DNSExpectedIP),
		DNSCheckIntervalSeconds: interval,
		WhoisSyncEnabled:        whoisEnabled,
		CreatedBy:               actorID,
	})
}

func (s *DomainService) UpdateDomain(ctx context.Context, domainID string, req operationaldto.UpdateDomainRequest) (model.Domain, error) {
	name := strings.ToLower(strings.TrimSpace(req.Name))
	if !looksLikeDomain(name) {
		return model.Domain{}, fmt.Errorf("%w: name must be a valid domain like example.com", ErrInvalidDomain)
	}
	expiry, err := parseExpiryDate(req.ExpiryDate)
	if err != nil {
		return model.Domain{}, err
	}
	currency := strings.ToUpper(strings.TrimSpace(req.CostCurrency))
	if currency == "" {
		currency = "IDR"
	}
	dnsEnabled := true
	if req.DNSCheckEnabled != nil {
		dnsEnabled = *req.DNSCheckEnabled
	}
	whoisEnabled := true
	if req.WhoisSyncEnabled != nil {
		whoisEnabled = *req.WhoisSyncEnabled
	}
	interval := req.DNSCheckIntervalSeconds
	if interval <= 0 {
		interval = 3600
	}

	d, err := s.repo.UpdateDomain(ctx, domainID, operationalrepo.UpdateDomainParams{
		Name:                    name,
		Registrar:               strings.TrimSpace(req.Registrar),
		Nameservers:             normaliseNameservers(req.Nameservers),
		ExpiryDate:              expiry,
		CostAmount:              req.CostAmount,
		CostCurrency:            currency,
		BillingCycle:            req.BillingCycle,
		Status:                  req.Status,
		Tags:                    normaliseTags(req.Tags),
		Notes:                   strings.TrimSpace(req.Notes),
		DNSCheckEnabled:         dnsEnabled,
		DNSExpectedIP:           strings.TrimSpace(req.DNSExpectedIP),
		DNSCheckIntervalSeconds: interval,
		WhoisSyncEnabled:        whoisEnabled,
	})
	return d, mapDomainError(err)
}

func (s *DomainService) DeleteDomain(ctx context.Context, domainID string) error {
	return mapDomainError(s.repo.DeleteDomain(ctx, domainID))
}

func (s *DomainService) GetDomainByID(ctx context.Context, domainID string) (model.Domain, error) {
	d, err := s.repo.GetDomainByID(ctx, domainID)
	return d, mapDomainError(err)
}

func (s *DomainService) ListDomains(ctx context.Context, p operationalrepo.ListDomainParams) ([]model.Domain, error) {
	return s.repo.ListDomains(ctx, p)
}

func (s *DomainService) GetDomainDetail(ctx context.Context, domainID string) (DomainDetail, error) {
	d, err := s.repo.GetDomainByID(ctx, domainID)
	if err != nil {
		return DomainDetail{}, mapDomainError(err)
	}
	events, err := s.repo.ListEventsForDomain(ctx, domainID, 100)
	if err != nil {
		return DomainDetail{}, err
	}
	return DomainDetail{Domain: d, Events: events}, nil
}

// helpers ---------------------------------------------------------------------

func parseExpiryDate(raw *string) (*time.Time, error) {
	if raw == nil {
		return nil, nil
	}
	v := strings.TrimSpace(*raw)
	if v == "" {
		return nil, nil
	}
	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		return nil, fmt.Errorf("expiry_date must be YYYY-MM-DD: %w", err)
	}
	return &t, nil
}

func normaliseNameservers(items []string) []string {
	out := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, s := range items {
		v := strings.ToLower(strings.TrimSpace(s))
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// looksLikeDomain is a permissive sanity check — full FQDN validation is
// expensive and registrars accept oddities (punycode, etc).
func looksLikeDomain(name string) bool {
	if name == "" || strings.ContainsAny(name, " /\\?#@") {
		return false
	}
	if !strings.Contains(name, ".") {
		return false
	}
	return true
}

func mapDomainError(err error) error {
	if errors.Is(err, operationalrepo.ErrDomainNotFound) {
		return ErrDomainNotFound
	}
	return err
}
