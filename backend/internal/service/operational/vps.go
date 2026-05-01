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

// ErrVPS* sentinels surface clean 4xx codes from the handler layer.
var (
	ErrVPSNotFound      = errors.New("vps not found")
	ErrVPSCheckNotFound = errors.New("vps health check not found")
	ErrVPSAppNotFound   = errors.New("vps app not found")
	ErrVPSCheckOnOther  = errors.New("vps app cannot link to a check on a different vps")
	ErrInvalidVPSCheck  = errors.New("invalid vps health check")
)

type vpsRepository interface {
	CreateVPS(ctx context.Context, p operationalrepo.CreateVPSParams) (model.VPSServer, error)
	UpdateVPS(ctx context.Context, vpsID string, p operationalrepo.UpdateVPSParams) (model.VPSServer, error)
	DeleteVPS(ctx context.Context, vpsID string) error
	GetVPSByID(ctx context.Context, vpsID string) (model.VPSServer, error)
	ListVPS(ctx context.Context, p operationalrepo.ListVPSParams) ([]model.VPSServer, error)
	ListVPSSummary(ctx context.Context, p operationalrepo.ListVPSParams) ([]model.VPSServerSummary, error)
	UpdateVPSStatusSnapshot(ctx context.Context, vpsID string, status string, now time.Time) error

	CreateCheck(ctx context.Context, p operationalrepo.CreateVPSCheckParams) (model.VPSHealthCheck, error)
	UpdateCheck(ctx context.Context, checkID string, p operationalrepo.UpdateVPSCheckParams) (model.VPSHealthCheck, error)
	DeleteCheck(ctx context.Context, checkID string) error
	ListChecksForVPS(ctx context.Context, vpsID string) ([]model.VPSHealthCheck, error)
	ListEnabledChecksForVPS(ctx context.Context, vpsID string) ([]model.VPSHealthCheck, error)
	ListDueChecks(ctx context.Context, now time.Time) ([]model.VPSHealthCheck, error)
	RecordCheckResult(ctx context.Context, vpsID string, result operationalrepo.CheckResult) (model.VPSHealthCheck, bool, error)
	MarkCheckAlertActive(ctx context.Context, checkID string, now time.Time) error
	ClearCheckAlert(ctx context.Context, checkID string) error

	CreateApp(ctx context.Context, p operationalrepo.CreateVPSAppParams) (model.VPSApp, error)
	UpdateApp(ctx context.Context, appID string, p operationalrepo.UpdateVPSAppParams) (model.VPSApp, error)
	DeleteApp(ctx context.Context, appID string) error
	ListAppsForVPS(ctx context.Context, vpsID string) ([]model.VPSApp, error)

	ListEventsForVPS(ctx context.Context, vpsID string, limit int) ([]model.VPSHealthEvent, error)
	ListDailySummaryForVPS(ctx context.Context, vpsID string, since time.Time) ([]model.VPSHealthDailySummary, error)
	ListVPSWithRenewalBefore(ctx context.Context, cutoff time.Time) ([]model.VPSServer, error)
	PurgeOldEvents(ctx context.Context, cutoff time.Time) (int64, error)
	RollupDailySummary(ctx context.Context, day time.Time) error
}

type VPSService struct {
	repo vpsRepository
}

func NewVPSService(repo vpsRepository) *VPSService {
	return &VPSService{repo: repo}
}

// ---------------------------------------------------------------------------
// VPS server CRUD
// ---------------------------------------------------------------------------

func (s *VPSService) CreateVPS(ctx context.Context, req operationaldto.CreateVPSRequest, actorID string) (model.VPSServer, error) {
	renewal, err := parseRenewalDate(req.RenewalDate)
	if err != nil {
		return model.VPSServer{}, err
	}
	currency := strings.ToUpper(strings.TrimSpace(req.CostCurrency))
	if currency == "" {
		currency = "IDR"
	}
	tags := normaliseTags(req.Tags)

	return s.repo.CreateVPS(ctx, operationalrepo.CreateVPSParams{
		Label:        strings.TrimSpace(req.Label),
		Provider:     strings.TrimSpace(req.Provider),
		Hostname:     strings.TrimSpace(req.Hostname),
		IPAddress:    strings.TrimSpace(req.IPAddress),
		Region:       strings.TrimSpace(req.Region),
		CPUCores:     req.CPUCores,
		RAMMB:        req.RAMMB,
		DiskGB:       req.DiskGB,
		CostAmount:   req.CostAmount,
		CostCurrency: currency,
		BillingCycle: req.BillingCycle,
		RenewalDate:  renewal,
		Status:       req.Status,
		Tags:         tags,
		Notes:        strings.TrimSpace(req.Notes),
		CreatedBy:    actorID,
	})
}

func (s *VPSService) UpdateVPS(ctx context.Context, vpsID string, req operationaldto.UpdateVPSRequest) (model.VPSServer, error) {
	renewal, err := parseRenewalDate(req.RenewalDate)
	if err != nil {
		return model.VPSServer{}, err
	}
	currency := strings.ToUpper(strings.TrimSpace(req.CostCurrency))
	if currency == "" {
		currency = "IDR"
	}
	tags := normaliseTags(req.Tags)

	v, err := s.repo.UpdateVPS(ctx, vpsID, operationalrepo.UpdateVPSParams{
		Label:        strings.TrimSpace(req.Label),
		Provider:     strings.TrimSpace(req.Provider),
		Hostname:     strings.TrimSpace(req.Hostname),
		IPAddress:    strings.TrimSpace(req.IPAddress),
		Region:       strings.TrimSpace(req.Region),
		CPUCores:     req.CPUCores,
		RAMMB:        req.RAMMB,
		DiskGB:       req.DiskGB,
		CostAmount:   req.CostAmount,
		CostCurrency: currency,
		BillingCycle: req.BillingCycle,
		RenewalDate:  renewal,
		Status:       req.Status,
		Tags:         tags,
		Notes:        strings.TrimSpace(req.Notes),
	})
	return v, mapVPSError(err)
}

func (s *VPSService) DeleteVPS(ctx context.Context, vpsID string) error {
	return mapVPSError(s.repo.DeleteVPS(ctx, vpsID))
}

func (s *VPSService) GetVPSByID(ctx context.Context, vpsID string) (model.VPSServer, error) {
	v, err := s.repo.GetVPSByID(ctx, vpsID)
	return v, mapVPSError(err)
}

func (s *VPSService) ListVPS(ctx context.Context, p operationalrepo.ListVPSParams) ([]model.VPSServerSummary, error) {
	return s.repo.ListVPSSummary(ctx, p)
}

// VPSDetail bundles a server with its checks, apps, and recent events for the
// detail page. The handler renders this directly as JSON.
type VPSDetail struct {
	Server  model.VPSServer               `json:"server"`
	Checks  []model.VPSHealthCheck        `json:"checks"`
	Apps    []model.VPSApp                `json:"apps"`
	Events  []model.VPSHealthEvent        `json:"events"`
	Daily   []model.VPSHealthDailySummary `json:"daily"`
}

func (s *VPSService) GetVPSDetail(ctx context.Context, vpsID string) (VPSDetail, error) {
	server, err := s.repo.GetVPSByID(ctx, vpsID)
	if err != nil {
		return VPSDetail{}, mapVPSError(err)
	}
	checks, err := s.repo.ListChecksForVPS(ctx, vpsID)
	if err != nil {
		return VPSDetail{}, err
	}
	apps, err := s.repo.ListAppsForVPS(ctx, vpsID)
	if err != nil {
		return VPSDetail{}, err
	}
	events, err := s.repo.ListEventsForVPS(ctx, vpsID, 100)
	if err != nil {
		return VPSDetail{}, err
	}
	daily, err := s.repo.ListDailySummaryForVPS(ctx, vpsID, time.Now().AddDate(0, 0, -30))
	if err != nil {
		return VPSDetail{}, err
	}
	return VPSDetail{Server: server, Checks: checks, Apps: apps, Events: events, Daily: daily}, nil
}

// ---------------------------------------------------------------------------
// Checks
// ---------------------------------------------------------------------------

func (s *VPSService) CreateCheck(ctx context.Context, vpsID string, req operationaldto.CreateVPSCheckRequest) (model.VPSHealthCheck, error) {
	if _, err := s.repo.GetVPSByID(ctx, vpsID); err != nil {
		return model.VPSHealthCheck{}, mapVPSError(err)
	}
	if err := validateCheckTarget(req.Type, req.Target); err != nil {
		return model.VPSHealthCheck{}, err
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	c, err := s.repo.CreateCheck(ctx, operationalrepo.CreateVPSCheckParams{
		VPSID:           vpsID,
		Label:           strings.TrimSpace(req.Label),
		Type:            req.Type,
		Target:          strings.TrimSpace(req.Target),
		IntervalSeconds: req.IntervalSeconds,
		TimeoutSeconds:  req.TimeoutSeconds,
		Enabled:         enabled,
	})
	return c, mapVPSError(err)
}

func (s *VPSService) UpdateCheck(ctx context.Context, checkID string, req operationaldto.UpdateVPSCheckRequest) (model.VPSHealthCheck, error) {
	if err := validateCheckTarget(req.Type, req.Target); err != nil {
		return model.VPSHealthCheck{}, err
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	c, err := s.repo.UpdateCheck(ctx, checkID, operationalrepo.UpdateVPSCheckParams{
		Label:           strings.TrimSpace(req.Label),
		Type:            req.Type,
		Target:          strings.TrimSpace(req.Target),
		IntervalSeconds: req.IntervalSeconds,
		TimeoutSeconds:  req.TimeoutSeconds,
		Enabled:         enabled,
	})
	return c, mapVPSError(err)
}

func (s *VPSService) DeleteCheck(ctx context.Context, checkID string) error {
	return mapVPSError(s.repo.DeleteCheck(ctx, checkID))
}

// ---------------------------------------------------------------------------
// Apps
// ---------------------------------------------------------------------------

func (s *VPSService) CreateApp(ctx context.Context, vpsID string, req operationaldto.CreateVPSAppRequest) (model.VPSApp, error) {
	if _, err := s.repo.GetVPSByID(ctx, vpsID); err != nil {
		return model.VPSApp{}, mapVPSError(err)
	}
	if err := s.ensureCheckBelongsToVPS(ctx, vpsID, req.CheckID); err != nil {
		return model.VPSApp{}, err
	}
	a, err := s.repo.CreateApp(ctx, operationalrepo.CreateVPSAppParams{
		VPSID:   vpsID,
		Name:    strings.TrimSpace(req.Name),
		AppType: strings.TrimSpace(req.AppType),
		Port:    req.Port,
		URL:     strings.TrimSpace(req.URL),
		Notes:   strings.TrimSpace(req.Notes),
		CheckID: req.CheckID,
	})
	return a, mapVPSError(err)
}

func (s *VPSService) UpdateApp(ctx context.Context, vpsID, appID string, req operationaldto.UpdateVPSAppRequest) (model.VPSApp, error) {
	if err := s.ensureCheckBelongsToVPS(ctx, vpsID, req.CheckID); err != nil {
		return model.VPSApp{}, err
	}
	a, err := s.repo.UpdateApp(ctx, appID, operationalrepo.UpdateVPSAppParams{
		Name:    strings.TrimSpace(req.Name),
		AppType: strings.TrimSpace(req.AppType),
		Port:    req.Port,
		URL:     strings.TrimSpace(req.URL),
		Notes:   strings.TrimSpace(req.Notes),
		CheckID: req.CheckID,
	})
	return a, mapVPSError(err)
}

func (s *VPSService) DeleteApp(ctx context.Context, appID string) error {
	return mapVPSError(s.repo.DeleteApp(ctx, appID))
}

func (s *VPSService) ensureCheckBelongsToVPS(ctx context.Context, vpsID string, checkID *string) error {
	if checkID == nil || strings.TrimSpace(*checkID) == "" {
		return nil
	}
	checks, err := s.repo.ListChecksForVPS(ctx, vpsID)
	if err != nil {
		return err
	}
	want := strings.TrimSpace(*checkID)
	for _, c := range checks {
		if c.ID == want {
			return nil
		}
	}
	return ErrVPSCheckOnOther
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func parseRenewalDate(raw *string) (*time.Time, error) {
	if raw == nil {
		return nil, nil
	}
	v := strings.TrimSpace(*raw)
	if v == "" {
		return nil, nil
	}
	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		return nil, fmt.Errorf("renewal_date must be YYYY-MM-DD: %w", err)
	}
	return &t, nil
}

func normaliseTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, t := range tags {
		v := strings.ToLower(strings.TrimSpace(t))
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

func validateCheckTarget(checkType string, target string) error {
	target = strings.TrimSpace(target)
	switch checkType {
	case "icmp":
		if target == "" {
			return fmt.Errorf("%w: icmp target (host or IP) is required", ErrInvalidVPSCheck)
		}
	case "tcp":
		// expect host:port
		idx := strings.LastIndex(target, ":")
		if idx <= 0 || idx == len(target)-1 {
			return fmt.Errorf("%w: tcp target must be host:port", ErrInvalidVPSCheck)
		}
	case "http", "https":
		if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
			return fmt.Errorf("%w: http/https target must be a full URL", ErrInvalidVPSCheck)
		}
	default:
		return fmt.Errorf("%w: unknown check type %q", ErrInvalidVPSCheck, checkType)
	}
	return nil
}

func mapVPSError(err error) error {
	switch {
	case errors.Is(err, operationalrepo.ErrVPSNotFound):
		return ErrVPSNotFound
	case errors.Is(err, operationalrepo.ErrVPSCheckNotFound):
		return ErrVPSCheckNotFound
	case errors.Is(err, operationalrepo.ErrVPSAppNotFound):
		return ErrVPSAppNotFound
	default:
		return err
	}
}
