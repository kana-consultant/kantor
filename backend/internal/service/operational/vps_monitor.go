package operational

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/model"
	notificationsrepo "github.com/kana-consultant/kantor/backend/internal/repository/notifications"
	operationalrepo "github.com/kana-consultant/kantor/backend/internal/repository/operational"
)

// downBeforeAlert is how many consecutive failures must accumulate before
// the monitor fires its first alert. 3 keeps spurious one-off blips quiet.
const downBeforeAlert = 3

// alertCooldown is the minimum gap between repeat "still down" alerts on the
// same check while it remains down. Recovery resets this.
const alertCooldown = 30 * time.Minute

// vpsMonitorNotifications is the slice of notifications repository operations
// the monitor relies on. Kept narrow so tests can stub a fake.
type vpsMonitorNotifications interface {
	CreateMany(ctx context.Context, params []notificationsrepo.CreateParams) error
}

// vpsMonitorAuthLookup returns the user IDs the monitor should alert when a
// check transitions. Reuses the existing auth repo permission-based lookup.
type vpsMonitorAuthLookup interface {
	ListUserIDsByPermission(ctx context.Context, permissionID string) ([]string, error)
}

// VPSMonitorService runs the periodic uptime / SSL checks and dispatches
// alerts on transition. It is invoked from the per-tenant background
// scheduler in app.go.
type VPSMonitorService struct {
	repo     vpsRepository
	notifs   vpsMonitorNotifications
	authRepo vpsMonitorAuthLookup
	httpClient *http.Client
}

func NewVPSMonitorService(repo vpsRepository, notifs vpsMonitorNotifications, authRepo vpsMonitorAuthLookup) *VPSMonitorService {
	return &VPSMonitorService{
		repo:     repo,
		notifs:   notifs,
		authRepo: authRepo,
		// Single shared client; per-call timeout is set on the request itself.
		// DisableKeepAlives keeps probe latency honest by not measuring a
		// reused connection.
		httpClient: &http.Client{
			Transport: &http.Transport{
				DisableKeepAlives:   true,
				MaxIdleConnsPerHost: -1,
			},
		},
	}
}

// RunDueChecks executes every check whose interval has elapsed. Probes run
// in parallel up to a small worker pool so one slow VPS does not block the
// rest. Status snapshots and alerts are dispatched inside the worker.
//
// Idempotent: safe to call once per tenant per scheduler tick (10s in app.go).
func (s *VPSMonitorService) RunDueChecks(ctx context.Context, now time.Time) error {
	checks, err := s.repo.ListDueChecks(ctx, now)
	if err != nil {
		return fmt.Errorf("list due checks: %w", err)
	}
	if len(checks) == 0 {
		return nil
	}

	const workers = 8
	jobs := make(chan model.VPSHealthCheck, len(checks))
	for _, c := range checks {
		jobs <- c
	}
	close(jobs)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					slog.ErrorContext(ctx, "vps monitor worker panic", "panic", r)
				}
			}()
			for c := range jobs {
				s.processCheck(ctx, c, time.Now().UTC())
			}
		}()
	}
	wg.Wait()
	return nil
}

func (s *VPSMonitorService) processCheck(ctx context.Context, check model.VPSHealthCheck, now time.Time) {
	probeResult := s.probe(ctx, check)
	probeResult.CheckID = check.ID
	probeResult.Timestamp = now

	updated, statusChanged, err := s.repo.RecordCheckResult(ctx, check.VPSID, probeResult)
	if err != nil {
		slog.ErrorContext(ctx, "record vps check result failed", "check_id", check.ID, "error", err)
		return
	}

	// Refresh the rolled-up vps_servers.last_status from all enabled checks.
	if err := s.refreshVPSStatusSnapshot(ctx, check.VPSID, now); err != nil {
		slog.WarnContext(ctx, "vps status snapshot update failed", "vps_id", check.VPSID, "error", err)
	}

	// Alert decisions:
	//   - down + reached threshold + (no active alert OR cooldown elapsed) → fire
	//   - up after being alert_active=true → fire recovery + clear
	switch {
	case updated.LastStatus == "down" && updated.ConsecutiveFails >= downBeforeAlert:
		if !updated.AlertActive || cooldownExpired(updated.AlertLastSentAt, now) {
			s.dispatchDownAlert(ctx, check, updated, statusChanged)
			if err := s.repo.MarkCheckAlertActive(ctx, check.ID, now); err != nil {
				slog.WarnContext(ctx, "mark vps alert active failed", "check_id", check.ID, "error", err)
			}
		}
	case updated.LastStatus == "up" && updated.AlertActive:
		s.dispatchRecoveryAlert(ctx, check, updated)
		if err := s.repo.ClearCheckAlert(ctx, check.ID); err != nil {
			slog.WarnContext(ctx, "clear vps alert failed", "check_id", check.ID, "error", err)
		}
	}
}

// refreshVPSStatusSnapshot recomputes vps_servers.last_status from the live
// state of every enabled check on this VPS. Aggregation rules:
//   - any check still 'unknown' AND none others 'down'  → unknown
//   - all checks 'up'                                   → up
//   - some checks 'up', some 'down'                     → degraded
//   - all checks 'down'                                 → down
//   - no enabled checks                                 → unknown
func (s *VPSMonitorService) refreshVPSStatusSnapshot(ctx context.Context, vpsID string, now time.Time) error {
	checks, err := s.repo.ListEnabledChecksForVPS(ctx, vpsID)
	if err != nil {
		return err
	}
	if len(checks) == 0 {
		return s.repo.UpdateVPSStatusSnapshot(ctx, vpsID, "unknown", now)
	}

	var up, down, unknown int
	for _, c := range checks {
		switch c.LastStatus {
		case "up":
			up++
		case "down":
			down++
		default:
			unknown++
		}
	}

	rolled := "unknown"
	switch {
	case down == len(checks):
		rolled = "down"
	case up == len(checks):
		rolled = "up"
	case down > 0 && up > 0:
		rolled = "degraded"
	case down > 0 && unknown > 0:
		rolled = "down"
	}

	return s.repo.UpdateVPSStatusSnapshot(ctx, vpsID, rolled, now)
}

// probe performs the actual network probe for the check and returns a
// CheckResult ready to be persisted by the caller. Errors are converted to
// a "down" status with the error message captured.
func (s *VPSMonitorService) probe(ctx context.Context, c model.VPSHealthCheck) operationalrepo.CheckResult {
	timeout := time.Duration(c.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	switch c.Type {
	case "tcp":
		return probeTCP(ctx, c.Target, timeout)
	case "http":
		return probeHTTP(ctx, s.httpClient, c.Target, timeout, false)
	case "https":
		return probeHTTP(ctx, s.httpClient, c.Target, timeout, true)
	case "icmp":
		// ICMP raw sockets need root on Linux. Most managed boxes (Docker,
		// non-privileged containers) don't allow that. We approximate via
		// TCP connect to port 80 — enough to know the host is reachable
		// over the network. Switch to a real ping (e.g. go-ping) once the
		// runtime privileges allow it.
		target := c.Target
		if !strings.Contains(target, ":") {
			target = target + ":80"
		}
		res := probeTCP(ctx, target, timeout)
		// Make the user-facing error match the probe type even though we
		// fell back to TCP underneath.
		if res.Status == "down" && res.ErrorMessage != "" {
			res.ErrorMessage = "icmp fallback (tcp:80): " + res.ErrorMessage
		}
		return res
	default:
		return operationalrepo.CheckResult{
			Status:       "down",
			ErrorMessage: fmt.Sprintf("unsupported check type %q", c.Type),
		}
	}
}

func probeTCP(ctx context.Context, target string, timeout time.Duration) operationalrepo.CheckResult {
	dialer := &net.Dialer{Timeout: timeout}
	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	conn, err := dialer.DialContext(dialCtx, "tcp", target)
	latency := int(time.Since(start).Milliseconds())
	if err != nil {
		msg := err.Error()
		return operationalrepo.CheckResult{Status: "down", ErrorMessage: msg}
	}
	_ = conn.Close()
	return operationalrepo.CheckResult{Status: "up", LatencyMS: &latency}
}

func probeHTTP(ctx context.Context, client *http.Client, target string, timeout time.Duration, captureCert bool) operationalrepo.CheckResult {
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if _, err := url.Parse(target); err != nil {
		return operationalrepo.CheckResult{Status: "down", ErrorMessage: "invalid url: " + err.Error()}
	}
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, target, nil)
	if err != nil {
		return operationalrepo.CheckResult{Status: "down", ErrorMessage: "request build: " + err.Error()}
	}
	req.Header.Set("User-Agent", "kantor-vps-monitor/1.0")

	start := time.Now()
	resp, err := client.Do(req)
	latency := int(time.Since(start).Milliseconds())
	if err != nil {
		return operationalrepo.CheckResult{Status: "down", ErrorMessage: err.Error()}
	}
	defer resp.Body.Close()

	res := operationalrepo.CheckResult{LatencyMS: &latency}
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		res.Status = "up"
	} else {
		res.Status = "down"
		res.ErrorMessage = fmt.Sprintf("http status %d", resp.StatusCode)
	}

	if captureCert && resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		cert := resp.TLS.PeerCertificates[0]
		notAfter := cert.NotAfter
		res.SSLExpiresAt = &notAfter
		res.SSLIssuer = trimCertIssuer(cert.Issuer.String())
		// Surface near-expiry as a soft degradation: still "up" because the
		// site responded, but the message warns operators.
		if days := int(time.Until(notAfter).Hours() / 24); days <= 14 && days >= 0 {
			if res.ErrorMessage == "" {
				res.ErrorMessage = fmt.Sprintf("ssl expires in %d days", days)
			}
		}
	}
	return res
}

func trimCertIssuer(s string) string {
	// Issuer DNs can be huge. Truncate to keep storage sane.
	if len(s) > 200 {
		return s[:200]
	}
	return s
}

func cooldownExpired(lastSent *time.Time, now time.Time) bool {
	if lastSent == nil {
		return true
	}
	return now.Sub(*lastSent) >= alertCooldown
}

// ---------------------------------------------------------------------------
// Alerts
// ---------------------------------------------------------------------------

func (s *VPSMonitorService) dispatchDownAlert(ctx context.Context, check model.VPSHealthCheck, updated model.VPSHealthCheck, firstAlert bool) {
	if s.notifs == nil || s.authRepo == nil {
		return
	}
	recipients, err := s.recipientsForVPSAlert(ctx)
	if err != nil {
		slog.WarnContext(ctx, "vps alert recipient lookup failed", "error", err)
		return
	}
	if len(recipients) == 0 {
		return
	}

	title := fmt.Sprintf("VPS check DOWN: %s", check.Label)
	message := fmt.Sprintf(
		"%s (%s) gagal %d kali berturut. Last error: %s",
		check.Label,
		check.Target,
		updated.ConsecutiveFails,
		fallbackError(updated.LastError),
	)
	if !firstAlert {
		title = "VPS still DOWN: " + check.Label
	}

	s.fireNotifications(ctx, recipients, "operational.vps.down", title, message, "vps_check", check.ID)
}

func (s *VPSMonitorService) dispatchRecoveryAlert(ctx context.Context, check model.VPSHealthCheck, updated model.VPSHealthCheck) {
	if s.notifs == nil || s.authRepo == nil {
		return
	}
	recipients, err := s.recipientsForVPSAlert(ctx)
	if err != nil {
		slog.WarnContext(ctx, "vps recovery recipient lookup failed", "error", err)
		return
	}
	if len(recipients) == 0 {
		return
	}
	title := "VPS check RECOVERED: " + check.Label
	latency := "n/a"
	if updated.LastLatencyMS != nil {
		latency = fmt.Sprintf("%dms", *updated.LastLatencyMS)
	}
	message := fmt.Sprintf("%s (%s) kembali UP. Latency %s.", check.Label, check.Target, latency)
	s.fireNotifications(ctx, recipients, "operational.vps.up", title, message, "vps_check", check.ID)
}

func (s *VPSMonitorService) fireNotifications(ctx context.Context, userIDs []string, kind, title, message, refType, refID string) {
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
		slog.WarnContext(ctx, "vps notification dispatch failed", "kind", kind, "error", err)
	}
}

// recipientsForVPSAlert returns user IDs that should be notified when a VPS
// check changes state. Anyone with operational:vps:edit (admin / super_admin
// in the default seed) gets the alert.
func (s *VPSMonitorService) recipientsForVPSAlert(ctx context.Context) ([]string, error) {
	return s.authRepo.ListUserIDsByPermission(ctx, "operational:vps:edit")
}

func fallbackError(s string) string {
	if strings.TrimSpace(s) == "" {
		return "(unknown)"
	}
	return s
}

// ---------------------------------------------------------------------------
// Renewal alerts + maintenance jobs
// ---------------------------------------------------------------------------

// SendRenewalAlerts dispatches in-app reminders for VPS whose lease renews
// within `cutoff` days. Daily idempotent — duplicates are deduped at the
// notification layer because we use a stable refType+refID.
func (s *VPSMonitorService) SendRenewalAlerts(ctx context.Context, now time.Time, withinDays int) error {
	if s.notifs == nil || s.authRepo == nil {
		return nil
	}
	cutoff := now.AddDate(0, 0, withinDays)
	servers, err := s.repo.ListVPSWithRenewalBefore(ctx, cutoff)
	if err != nil {
		return fmt.Errorf("list renewal-due vps: %w", err)
	}
	if len(servers) == 0 {
		return nil
	}
	recipients, err := s.recipientsForVPSAlert(ctx)
	if err != nil || len(recipients) == 0 {
		return err
	}

	for _, v := range servers {
		days := -1
		if v.RenewalDate != nil {
			days = int(time.Until(*v.RenewalDate).Hours() / 24)
		}
		title := fmt.Sprintf("VPS renewal H-%d: %s", max0(days), v.Label)
		message := fmt.Sprintf("%s (%s) jatuh tempo %s. Pastikan auto-renew aktif atau perpanjang manual.",
			v.Label,
			fallbackError(v.Provider),
			v.RenewalDate.Format("2006-01-02"),
		)
		s.fireNotifications(ctx, recipients, "operational.vps.renewal", title, message, "vps_server", v.ID)
	}
	return nil
}

func max0(v int) int {
	if v < 0 {
		return 0
	}
	return v
}

// PurgeOldEvents removes raw events older than retainDays. Returns deleted
// row count for logging.
func (s *VPSMonitorService) PurgeOldEvents(ctx context.Context, now time.Time, retainDays int) (int64, error) {
	cutoff := now.AddDate(0, 0, -retainDays)
	return s.repo.PurgeOldEvents(ctx, cutoff)
}

// RollupYesterday aggregates the previous calendar day into the daily summary
// table. Run once per day after midnight UTC.
func (s *VPSMonitorService) RollupYesterday(ctx context.Context, now time.Time) error {
	yesterday := now.AddDate(0, 0, -1)
	return s.repo.RollupDailySummary(ctx, yesterday)
}

