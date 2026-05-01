package operational

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/likexian/whois"
	whoisparser "github.com/likexian/whois-parser"

	"github.com/kana-consultant/kantor/backend/internal/model"
	notificationsrepo "github.com/kana-consultant/kantor/backend/internal/repository/notifications"
	operationalrepo "github.com/kana-consultant/kantor/backend/internal/repository/operational"
)

// downBeforeDomainAlert is how many consecutive DNS failures must accumulate
// before the monitor fires its first alert. 2 is enough — DNS doesn't flap
// like uptime checks.
const downBeforeDomainAlert = 2

// domainAlertCooldown is the minimum gap between repeat "still down" alerts.
const domainAlertCooldown = 6 * time.Hour

// DomainMonitorService runs the periodic DNS resolution + WHOIS sync and
// dispatches renewal alerts for domains nearing expiry.
type DomainMonitorService struct {
	repo     domainRepository
	notifs   vpsMonitorNotifications
	authRepo vpsMonitorAuthLookup
}

func NewDomainMonitorService(repo domainRepository, notifs vpsMonitorNotifications, authRepo vpsMonitorAuthLookup) *DomainMonitorService {
	return &DomainMonitorService{
		repo:     repo,
		notifs:   notifs,
		authRepo: authRepo,
	}
}

// RunDueDNSChecks resolves every domain whose DNS check is due. Worker
// pool of 8 to keep slow resolvers from blocking the rest.
func (s *DomainMonitorService) RunDueDNSChecks(ctx context.Context, now time.Time) error {
	domains, err := s.repo.ListDueDNSChecks(ctx, now)
	if err != nil {
		return fmt.Errorf("list due dns checks: %w", err)
	}
	if len(domains) == 0 {
		return nil
	}

	const workers = 8
	jobs := make(chan model.Domain, len(domains))
	for _, d := range domains {
		jobs <- d
	}
	close(jobs)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					slog.ErrorContext(ctx, "domain dns worker panic", "panic", r)
				}
			}()
			for d := range jobs {
				s.processDNSCheck(ctx, d, time.Now().UTC())
			}
		}()
	}
	wg.Wait()
	return nil
}

func (s *DomainMonitorService) processDNSCheck(ctx context.Context, d model.Domain, now time.Time) {
	result := probeDNS(ctx, d.Name, d.DNSExpectedIP)
	result.Timestamp = now

	updated, statusChanged, err := s.repo.RecordDNSCheckResult(ctx, d.ID, result)
	if err != nil {
		slog.ErrorContext(ctx, "record domain dns result failed", "domain_id", d.ID, "error", err)
		return
	}

	// Append event log
	detail := strings.Join(result.ResolvedIPs, ",")
	if result.Status == "down" {
		detail = result.ErrorMessage
	}
	if err := s.repo.CreateEvent(ctx, d.ID, "dns", result.Status, detail); err != nil {
		slog.WarnContext(ctx, "domain event log failed", "domain_id", d.ID, "error", err)
	}

	switch {
	case updated.DNSLastStatus == "down" && updated.DNSConsecutiveFails >= downBeforeDomainAlert:
		if !updated.DNSAlertActive || domainCooldownExpired(updated.DNSAlertLastSentAt, now) {
			s.dispatchDNSDownAlert(ctx, updated, statusChanged)
			if err := s.repo.MarkDNSAlertActive(ctx, d.ID, now); err != nil {
				slog.WarnContext(ctx, "mark domain alert active failed", "domain_id", d.ID, "error", err)
			}
		}
	case updated.DNSLastStatus == "up" && updated.DNSAlertActive:
		s.dispatchDNSRecoveryAlert(ctx, updated)
		if err := s.repo.ClearDNSAlert(ctx, d.ID); err != nil {
			slog.WarnContext(ctx, "clear domain alert failed", "domain_id", d.ID, "error", err)
		}
	}
}

// probeDNS resolves the domain. If expectedIP is non-empty the result is
// only "up" when the resolved set contains that IP.
func probeDNS(ctx context.Context, name string, expectedIP string) operationalrepo.DNSCheckResult {
	res := operationalrepo.DNSCheckResult{}
	resolveCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resolver := &net.Resolver{}
	ips, err := resolver.LookupHost(resolveCtx, name)
	if err != nil {
		res.Status = "down"
		res.ErrorMessage = err.Error()
		return res
	}
	res.ResolvedIPs = ips

	if expectedIP != "" {
		expected := strings.TrimSpace(expectedIP)
		for _, ip := range ips {
			if ip == expected {
				res.Status = "up"
				return res
			}
		}
		res.Status = "down"
		res.ErrorMessage = fmt.Sprintf("expected %s not found in resolved IPs", expected)
		return res
	}

	res.Status = "up"
	return res
}

func domainCooldownExpired(lastSent *time.Time, now time.Time) bool {
	if lastSent == nil {
		return true
	}
	return now.Sub(*lastSent) >= domainAlertCooldown
}

// SyncWhoisAll runs WHOIS lookups for domains due for sync (24h cadence).
// Updates expiry_date when the registry returns one. Errors are recorded
// per-domain but do not abort the loop.
func (s *DomainMonitorService) SyncWhoisAll(ctx context.Context, now time.Time) error {
	domains, err := s.repo.ListWhoisSyncDue(ctx, now)
	if err != nil {
		return fmt.Errorf("list whois sync due: %w", err)
	}
	for _, d := range domains {
		s.syncWhois(ctx, d)
	}
	return nil
}

func (s *DomainMonitorService) syncWhois(ctx context.Context, d model.Domain) {
	now := time.Now().UTC()
	raw, err := whois.Whois(d.Name)
	if err != nil {
		_ = s.repo.SetWhoisSyncResult(ctx, d.ID, now, nil, err.Error())
		_ = s.repo.CreateEvent(ctx, d.ID, "whois", "error", "whois lookup failed: "+err.Error())
		return
	}

	parsed, err := whoisparser.Parse(raw)
	if err != nil {
		_ = s.repo.SetWhoisSyncResult(ctx, d.ID, now, nil, err.Error())
		_ = s.repo.CreateEvent(ctx, d.ID, "whois", "error", "whois parse failed: "+err.Error())
		return
	}

	expiryStr := strings.TrimSpace(parsed.Domain.ExpirationDate)
	if expiryStr == "" {
		_ = s.repo.SetWhoisSyncResult(ctx, d.ID, now, nil, "registry did not return expiry date")
		_ = s.repo.CreateEvent(ctx, d.ID, "whois", "error", "no expiry in whois response")
		return
	}

	expiry := parseWhoisDate(expiryStr)
	if expiry == nil {
		_ = s.repo.SetWhoisSyncResult(ctx, d.ID, now, nil, "could not parse expiry date: "+expiryStr)
		_ = s.repo.CreateEvent(ctx, d.ID, "whois", "error", "unparseable expiry: "+expiryStr)
		return
	}

	_ = s.repo.SetWhoisSyncResult(ctx, d.ID, now, expiry, "")
	_ = s.repo.CreateEvent(ctx, d.ID, "whois", "synced", fmt.Sprintf("expiry=%s", expiry.Format("2006-01-02")))
}

// parseWhoisDate is a best-effort WHOIS expiry parser. The whois-parser lib
// already normalises a lot, but registries return wildly different formats
// so we try the common ones in order.
func parseWhoisDate(s string) *time.Time {
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
		"2006-01-02",
		"02-Jan-2006",
		"2006/01/02",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return &t
		}
	}
	return nil
}

// SendRenewalAlerts notifies recipients about domains due to expire within
// `withinDays`. Idempotent at the notification layer via stable refType.
func (s *DomainMonitorService) SendRenewalAlerts(ctx context.Context, now time.Time, withinDays int) error {
	if s.notifs == nil || s.authRepo == nil {
		return nil
	}
	cutoff := now.AddDate(0, 0, withinDays)
	domains, err := s.repo.ListDomainsWithExpiryBefore(ctx, cutoff)
	if err != nil {
		return fmt.Errorf("list expiring domains: %w", err)
	}
	if len(domains) == 0 {
		return nil
	}
	recipients, err := s.authRepo.ListUserIDsByPermission(ctx, "operational:domain:edit")
	if err != nil || len(recipients) == 0 {
		return err
	}

	for _, d := range domains {
		days := -1
		if d.ExpiryDate != nil {
			days = int(time.Until(*d.ExpiryDate).Hours() / 24)
		}
		title := fmt.Sprintf("Domain renewal H-%d: %s", max0(days), d.Name)
		message := fmt.Sprintf("%s (registrar: %s) jatuh tempo %s. Pastikan auto-renew aktif atau perpanjang manual.",
			d.Name, fallbackError(d.Registrar), d.ExpiryDate.Format("2006-01-02"))
		s.dispatchToRecipients(ctx, recipients, "operational.domain.renewal", title, message, "domain", d.ID)
	}
	return nil
}

// PurgeOldEvents removes domain events older than retainDays.
func (s *DomainMonitorService) PurgeOldEvents(ctx context.Context, now time.Time, retainDays int) (int64, error) {
	cutoff := now.AddDate(0, 0, -retainDays)
	return s.repo.PurgeOldEvents(ctx, cutoff)
}

// alerts ----------------------------------------------------------------------

func (s *DomainMonitorService) dispatchDNSDownAlert(ctx context.Context, d model.Domain, firstAlert bool) {
	if s.notifs == nil || s.authRepo == nil {
		return
	}
	recipients, err := s.authRepo.ListUserIDsByPermission(ctx, "operational:domain:edit")
	if err != nil {
		slog.WarnContext(ctx, "domain alert recipient lookup failed", "error", err)
		return
	}
	if len(recipients) == 0 {
		return
	}
	title := fmt.Sprintf("Domain DNS DOWN: %s", d.Name)
	if !firstAlert {
		title = "Domain still DOWN: " + d.Name
	}
	message := fmt.Sprintf("DNS resolution untuk %s gagal %d kali berturut. Last error: %s",
		d.Name, d.DNSConsecutiveFails, fallbackError(d.DNSLastError))
	s.dispatchToRecipients(ctx, recipients, "operational.domain.down", title, message, "domain", d.ID)
}

func (s *DomainMonitorService) dispatchDNSRecoveryAlert(ctx context.Context, d model.Domain) {
	if s.notifs == nil || s.authRepo == nil {
		return
	}
	recipients, err := s.authRepo.ListUserIDsByPermission(ctx, "operational:domain:edit")
	if err != nil {
		slog.WarnContext(ctx, "domain recovery recipient lookup failed", "error", err)
		return
	}
	if len(recipients) == 0 {
		return
	}
	title := "Domain DNS RECOVERED: " + d.Name
	resolved := strings.Join(d.DNSLastResolvedIPs, ", ")
	if resolved == "" {
		resolved = "(none)"
	}
	message := fmt.Sprintf("%s kembali resolve. IPs: %s", d.Name, resolved)
	s.dispatchToRecipients(ctx, recipients, "operational.domain.up", title, message, "domain", d.ID)
}

func (s *DomainMonitorService) dispatchToRecipients(ctx context.Context, userIDs []string, kind, title, message, refType, refID string) {
	params := make([]notificationsrepo.CreateParams, 0, len(userIDs))
	rt := refType
	rid := refID
	for _, uid := range userIDs {
		params = append(params, notificationsrepo.CreateParams{
			UserID:        uid,
			Type:          kind,
			Title:         title,
			Message:       message,
			ReferenceType: &rt,
			ReferenceID:   &rid,
		})
	}
	if err := s.notifs.CreateMany(ctx, params); err != nil {
		slog.WarnContext(ctx, "domain notification dispatch failed", "kind", kind, "error", err)
	}
}
