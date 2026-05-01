package operational

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/kana-consultant/kantor/backend/internal/model"
	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

// ErrVPSNotFound is returned when a row lookup fails for vps_servers,
// vps_health_checks, vps_apps, or vps_health_events.
var (
	ErrVPSNotFound      = errors.New("vps not found")
	ErrVPSCheckNotFound = errors.New("vps health check not found")
	ErrVPSAppNotFound   = errors.New("vps app not found")
)

type VPSRepository struct {
	db repository.DBTX
}

func NewVPSRepository(db repository.DBTX) *VPSRepository {
	return &VPSRepository{db: db}
}

// ---------------------------------------------------------------------------
// Servers
// ---------------------------------------------------------------------------

type CreateVPSParams struct {
	Label        string
	Provider     string
	Hostname     string
	IPAddress    string
	Region       string
	CPUCores     int
	RAMMB        int
	DiskGB       int
	CostAmount   int64
	CostCurrency string
	BillingCycle string
	RenewalDate  *time.Time
	Status       string
	Tags         []string
	Notes        string
	CreatedBy    string
}

type UpdateVPSParams struct {
	Label        string
	Provider     string
	Hostname     string
	IPAddress    string
	Region       string
	CPUCores     int
	RAMMB        int
	DiskGB       int
	CostAmount   int64
	CostCurrency string
	BillingCycle string
	RenewalDate  *time.Time
	Status       string
	Tags         []string
	Notes        string
}

type ListVPSParams struct {
	Status   string
	Provider string
	Tag      string
	Search   string
}

const vpsServerColumns = `
	id::text, tenant_id::text, label, provider, hostname, ip_address, region,
	cpu_cores, ram_mb, disk_gb,
	cost_amount, cost_currency, billing_cycle, renewal_date,
	status, tags, notes,
	last_status, last_status_changed_at, last_check_at,
	created_by::text, created_at, updated_at
`

func scanVPSServer(row pgx.Row, v *model.VPSServer) error {
	var renewalDate *time.Time
	var lastStatusChangedAt *time.Time
	var lastCheckAt *time.Time
	var createdBy *string
	if err := row.Scan(
		&v.ID, &v.TenantID, &v.Label, &v.Provider, &v.Hostname, &v.IPAddress, &v.Region,
		&v.CPUCores, &v.RAMMB, &v.DiskGB,
		&v.CostAmount, &v.CostCurrency, &v.BillingCycle, &renewalDate,
		&v.Status, &v.Tags, &v.Notes,
		&v.LastStatus, &lastStatusChangedAt, &lastCheckAt,
		&createdBy, &v.CreatedAt, &v.UpdatedAt,
	); err != nil {
		return err
	}
	v.RenewalDate = renewalDate
	v.LastStatusChangedAt = lastStatusChangedAt
	v.LastCheckAt = lastCheckAt
	v.CreatedBy = createdBy
	return nil
}

func (r *VPSRepository) CreateVPS(ctx context.Context, p CreateVPSParams) (model.VPSServer, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	tags := p.Tags
	if tags == nil {
		tags = []string{}
	}

	query := fmt.Sprintf(`
		INSERT INTO vps_servers (
			label, provider, hostname, ip_address, region,
			cpu_cores, ram_mb, disk_gb,
			cost_amount, cost_currency, billing_cycle, renewal_date,
			status, tags, notes, created_by
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15, NULLIF($16, '')::uuid)
		RETURNING %s
	`, vpsServerColumns)

	var v model.VPSServer
	if err := scanVPSServer(repository.DB(ctx, r.db).QueryRow(ctx, query,
		p.Label, p.Provider, p.Hostname, p.IPAddress, p.Region,
		p.CPUCores, p.RAMMB, p.DiskGB,
		p.CostAmount, p.CostCurrency, p.BillingCycle, p.RenewalDate,
		p.Status, tags, p.Notes, p.CreatedBy,
	), &v); err != nil {
		return model.VPSServer{}, err
	}
	return v, nil
}

func (r *VPSRepository) UpdateVPS(ctx context.Context, vpsID string, p UpdateVPSParams) (model.VPSServer, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	tags := p.Tags
	if tags == nil {
		tags = []string{}
	}

	query := fmt.Sprintf(`
		UPDATE vps_servers SET
			label = $1, provider = $2, hostname = $3, ip_address = $4, region = $5,
			cpu_cores = $6, ram_mb = $7, disk_gb = $8,
			cost_amount = $9, cost_currency = $10, billing_cycle = $11, renewal_date = $12,
			status = $13, tags = $14, notes = $15,
			updated_at = NOW()
		WHERE id = $16::uuid
		RETURNING %s
	`, vpsServerColumns)

	var v model.VPSServer
	err := scanVPSServer(repository.DB(ctx, r.db).QueryRow(ctx, query,
		p.Label, p.Provider, p.Hostname, p.IPAddress, p.Region,
		p.CPUCores, p.RAMMB, p.DiskGB,
		p.CostAmount, p.CostCurrency, p.BillingCycle, p.RenewalDate,
		p.Status, tags, p.Notes, vpsID,
	), &v)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.VPSServer{}, ErrVPSNotFound
		}
		return model.VPSServer{}, err
	}
	return v, nil
}

func (r *VPSRepository) DeleteVPS(ctx context.Context, vpsID string) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	tag, err := repository.DB(ctx, r.db).Exec(ctx, `DELETE FROM vps_servers WHERE id = $1::uuid`, vpsID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrVPSNotFound
	}
	return nil
}

func (r *VPSRepository) GetVPSByID(ctx context.Context, vpsID string) (model.VPSServer, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	query := fmt.Sprintf(`SELECT %s FROM vps_servers WHERE id = $1::uuid`, vpsServerColumns)
	var v model.VPSServer
	err := scanVPSServer(repository.DB(ctx, r.db).QueryRow(ctx, query, vpsID), &v)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.VPSServer{}, ErrVPSNotFound
		}
		return model.VPSServer{}, err
	}
	return v, nil
}

func (r *VPSRepository) ListVPS(ctx context.Context, p ListVPSParams) ([]model.VPSServer, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	filters := []string{"1=1"}
	args := make([]any, 0, 4)
	idx := 1

	if status := strings.TrimSpace(p.Status); status != "" {
		filters = append(filters, fmt.Sprintf("status = $%d", idx))
		args = append(args, status)
		idx++
	}
	if provider := strings.TrimSpace(p.Provider); provider != "" {
		filters = append(filters, fmt.Sprintf("provider = $%d", idx))
		args = append(args, provider)
		idx++
	}
	if tag := strings.TrimSpace(p.Tag); tag != "" {
		filters = append(filters, fmt.Sprintf("$%d = ANY(tags)", idx))
		args = append(args, tag)
		idx++
	}
	if search := strings.TrimSpace(p.Search); search != "" {
		filters = append(filters, fmt.Sprintf("(label ILIKE $%d OR ip_address ILIKE $%d OR hostname ILIKE $%d)", idx, idx, idx))
		args = append(args, "%"+search+"%")
		idx++
	}

	query := fmt.Sprintf(`
		SELECT %s FROM vps_servers
		WHERE %s
		ORDER BY label ASC
	`, vpsServerColumns, strings.Join(filters, " AND "))

	rows, err := repository.DB(ctx, r.db).Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]model.VPSServer, 0)
	for rows.Next() {
		var v model.VPSServer
		if err := scanVPSServer(rows, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ListVPSSummary returns servers enriched with per-VPS counts (apps,
// total checks, down checks). Subqueries instead of joins to keep the
// query plain when the counts tables are small.
func (r *VPSRepository) ListVPSSummary(ctx context.Context, p ListVPSParams) ([]model.VPSServerSummary, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	filters := []string{"1=1"}
	args := make([]any, 0, 4)
	idx := 1

	if status := strings.TrimSpace(p.Status); status != "" {
		filters = append(filters, fmt.Sprintf("v.status = $%d", idx))
		args = append(args, status)
		idx++
	}
	if provider := strings.TrimSpace(p.Provider); provider != "" {
		filters = append(filters, fmt.Sprintf("v.provider = $%d", idx))
		args = append(args, provider)
		idx++
	}
	if tag := strings.TrimSpace(p.Tag); tag != "" {
		filters = append(filters, fmt.Sprintf("$%d = ANY(v.tags)", idx))
		args = append(args, tag)
		idx++
	}
	if search := strings.TrimSpace(p.Search); search != "" {
		filters = append(filters, fmt.Sprintf("(v.label ILIKE $%d OR v.ip_address ILIKE $%d OR v.hostname ILIKE $%d)", idx, idx, idx))
		args = append(args, "%"+search+"%")
		idx++
	}

	query := fmt.Sprintf(`
		SELECT
			v.id::text, v.tenant_id::text, v.label, v.provider, v.hostname, v.ip_address, v.region,
			v.cpu_cores, v.ram_mb, v.disk_gb,
			v.cost_amount, v.cost_currency, v.billing_cycle, v.renewal_date,
			v.status, v.tags, v.notes,
			v.last_status, v.last_status_changed_at, v.last_check_at,
			v.created_by::text, v.created_at, v.updated_at,
			COALESCE((SELECT COUNT(*) FROM vps_apps a WHERE a.vps_id = v.id), 0) AS apps_count,
			COALESCE((SELECT COUNT(*) FROM vps_health_checks c WHERE c.vps_id = v.id), 0) AS checks_count,
			COALESCE((SELECT COUNT(*) FROM vps_health_checks c WHERE c.vps_id = v.id AND c.enabled AND c.last_status = 'down'), 0) AS down_checks_count
		FROM vps_servers v
		WHERE %s
		ORDER BY v.label ASC
	`, strings.Join(filters, " AND "))

	rows, err := repository.DB(ctx, r.db).Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]model.VPSServerSummary, 0)
	for rows.Next() {
		var s model.VPSServerSummary
		var renewalDate, lastStatusChangedAt, lastCheckAt *time.Time
		var createdBy *string
		if err := rows.Scan(
			&s.ID, &s.TenantID, &s.Label, &s.Provider, &s.Hostname, &s.IPAddress, &s.Region,
			&s.CPUCores, &s.RAMMB, &s.DiskGB,
			&s.CostAmount, &s.CostCurrency, &s.BillingCycle, &renewalDate,
			&s.Status, &s.Tags, &s.Notes,
			&s.LastStatus, &lastStatusChangedAt, &lastCheckAt,
			&createdBy, &s.CreatedAt, &s.UpdatedAt,
			&s.AppsCount, &s.ChecksCount, &s.DownChecksCount,
		); err != nil {
			return nil, err
		}
		s.RenewalDate = renewalDate
		s.LastStatusChangedAt = lastStatusChangedAt
		s.LastCheckAt = lastCheckAt
		s.CreatedBy = createdBy
		out = append(out, s)
	}
	return out, rows.Err()
}

// UpdateVPSStatusSnapshot rolls up the per-check statuses into the
// last_status field on vps_servers so the list view can render quickly.
// Called by the monitor after each check completes.
func (r *VPSRepository) UpdateVPSStatusSnapshot(ctx context.Context, vpsID string, status string, now time.Time) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	_, err := repository.DB(ctx, r.db).Exec(ctx, `
		UPDATE vps_servers
		SET
			last_status = $2,
			last_check_at = $3,
			last_status_changed_at = CASE WHEN last_status IS DISTINCT FROM $2 THEN $3 ELSE last_status_changed_at END,
			updated_at = NOW()
		WHERE id = $1::uuid
	`, vpsID, status, now)
	return err
}

// ListVPSWithRenewalBefore returns active VPS rows whose renewal_date falls
// in [today, cutoff]. Used by the renewal alert job.
func (r *VPSRepository) ListVPSWithRenewalBefore(ctx context.Context, cutoff time.Time) ([]model.VPSServer, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	query := fmt.Sprintf(`
		SELECT %s FROM vps_servers
		WHERE status = 'active'
		  AND renewal_date IS NOT NULL
		  AND renewal_date <= $1::date
		ORDER BY renewal_date ASC
	`, vpsServerColumns)

	rows, err := repository.DB(ctx, r.db).Query(ctx, query, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]model.VPSServer, 0)
	for rows.Next() {
		var v model.VPSServer
		if err := scanVPSServer(rows, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Health checks
// ---------------------------------------------------------------------------

type CreateVPSCheckParams struct {
	VPSID           string
	Label           string
	Type            string
	Target          string
	IntervalSeconds int
	TimeoutSeconds  int
	Enabled         bool
}

type UpdateVPSCheckParams struct {
	Label           string
	Type            string
	Target          string
	IntervalSeconds int
	TimeoutSeconds  int
	Enabled         bool
}

const vpsCheckColumns = `
	id::text, vps_id::text, label, type, target,
	interval_seconds, timeout_seconds, enabled,
	last_status, last_latency_ms, last_error, last_check_at, last_status_changed_at,
	consecutive_fails, consecutive_successes,
	alert_active, alert_last_sent_at,
	ssl_expires_at, ssl_issuer,
	created_at, updated_at
`

func scanVPSCheck(row pgx.Row, c *model.VPSHealthCheck) error {
	var lastLatencyMS *int
	var lastCheckAt *time.Time
	var lastStatusChangedAt *time.Time
	var alertLastSentAt *time.Time
	var sslExpiresAt *time.Time
	if err := row.Scan(
		&c.ID, &c.VPSID, &c.Label, &c.Type, &c.Target,
		&c.IntervalSeconds, &c.TimeoutSeconds, &c.Enabled,
		&c.LastStatus, &lastLatencyMS, &c.LastError, &lastCheckAt, &lastStatusChangedAt,
		&c.ConsecutiveFails, &c.ConsecutiveSuccesses,
		&c.AlertActive, &alertLastSentAt,
		&sslExpiresAt, &c.SSLIssuer,
		&c.CreatedAt, &c.UpdatedAt,
	); err != nil {
		return err
	}
	c.LastLatencyMS = lastLatencyMS
	c.LastCheckAt = lastCheckAt
	c.LastStatusChangedAt = lastStatusChangedAt
	c.AlertLastSentAt = alertLastSentAt
	c.SSLExpiresAt = sslExpiresAt
	return nil
}

func (r *VPSRepository) CreateCheck(ctx context.Context, p CreateVPSCheckParams) (model.VPSHealthCheck, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	query := fmt.Sprintf(`
		INSERT INTO vps_health_checks (
			vps_id, label, type, target, interval_seconds, timeout_seconds, enabled
		)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7)
		RETURNING %s
	`, vpsCheckColumns)

	var c model.VPSHealthCheck
	if err := scanVPSCheck(repository.DB(ctx, r.db).QueryRow(ctx, query,
		p.VPSID, p.Label, p.Type, p.Target, p.IntervalSeconds, p.TimeoutSeconds, p.Enabled,
	), &c); err != nil {
		return model.VPSHealthCheck{}, err
	}
	return c, nil
}

func (r *VPSRepository) UpdateCheck(ctx context.Context, checkID string, p UpdateVPSCheckParams) (model.VPSHealthCheck, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	query := fmt.Sprintf(`
		UPDATE vps_health_checks SET
			label = $1, type = $2, target = $3,
			interval_seconds = $4, timeout_seconds = $5, enabled = $6,
			updated_at = NOW()
		WHERE id = $7::uuid
		RETURNING %s
	`, vpsCheckColumns)

	var c model.VPSHealthCheck
	err := scanVPSCheck(repository.DB(ctx, r.db).QueryRow(ctx, query,
		p.Label, p.Type, p.Target, p.IntervalSeconds, p.TimeoutSeconds, p.Enabled, checkID,
	), &c)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.VPSHealthCheck{}, ErrVPSCheckNotFound
		}
		return model.VPSHealthCheck{}, err
	}
	return c, nil
}

func (r *VPSRepository) DeleteCheck(ctx context.Context, checkID string) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	tag, err := repository.DB(ctx, r.db).Exec(ctx, `DELETE FROM vps_health_checks WHERE id = $1::uuid`, checkID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrVPSCheckNotFound
	}
	return nil
}

func (r *VPSRepository) ListChecksForVPS(ctx context.Context, vpsID string) ([]model.VPSHealthCheck, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	query := fmt.Sprintf(`
		SELECT %s FROM vps_health_checks
		WHERE vps_id = $1::uuid
		ORDER BY created_at ASC
	`, vpsCheckColumns)

	rows, err := repository.DB(ctx, r.db).Query(ctx, query, vpsID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]model.VPSHealthCheck, 0)
	for rows.Next() {
		var c model.VPSHealthCheck
		if err := scanVPSCheck(rows, &c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ListDueChecks returns enabled checks whose last_check_at is older than
// (now - interval_seconds). NULL last_check_at counts as due so newly
// inserted checks are picked up immediately.
func (r *VPSRepository) ListDueChecks(ctx context.Context, now time.Time) ([]model.VPSHealthCheck, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	query := fmt.Sprintf(`
		SELECT %s FROM vps_health_checks
		WHERE enabled = TRUE
		  AND (last_check_at IS NULL
		       OR last_check_at + (interval_seconds || ' seconds')::interval <= $1)
		ORDER BY last_check_at NULLS FIRST
		LIMIT 500
	`, vpsCheckColumns)

	rows, err := repository.DB(ctx, r.db).Query(ctx, query, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]model.VPSHealthCheck, 0)
	for rows.Next() {
		var c model.VPSHealthCheck
		if err := scanVPSCheck(rows, &c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CheckResult is the outcome the monitor records back into vps_health_checks
// + vps_health_events in a single round-trip.
type CheckResult struct {
	CheckID      string
	Status       string // "up" or "down"
	LatencyMS    *int
	ErrorMessage string
	SSLExpiresAt *time.Time
	SSLIssuer    string
	Timestamp    time.Time
}

// RecordCheckResult updates the check's runtime fields and inserts a
// vps_health_events row. Returns the post-update check row so the monitor
// can decide whether to fire / clear an alert.
func (r *VPSRepository) RecordCheckResult(ctx context.Context, vpsID string, result CheckResult) (model.VPSHealthCheck, bool, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	tx, err := repository.DB(ctx, r.db).Begin(ctx)
	if err != nil {
		return model.VPSHealthCheck{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Pull existing state to compute consecutive counters.
	var existing model.VPSHealthCheck
	row := tx.QueryRow(ctx, fmt.Sprintf(`SELECT %s FROM vps_health_checks WHERE id = $1::uuid FOR UPDATE`, vpsCheckColumns), result.CheckID)
	if err := scanVPSCheck(row, &existing); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.VPSHealthCheck{}, false, ErrVPSCheckNotFound
		}
		return model.VPSHealthCheck{}, false, err
	}

	consecutiveFails := existing.ConsecutiveFails
	consecutiveSucc := existing.ConsecutiveSuccesses
	if result.Status == "up" {
		consecutiveSucc++
		consecutiveFails = 0
	} else {
		consecutiveFails++
		consecutiveSucc = 0
	}

	statusChanged := existing.LastStatus != result.Status
	lastStatusChangedAt := existing.LastStatusChangedAt
	if statusChanged {
		t := result.Timestamp
		lastStatusChangedAt = &t
	}

	updated := existing
	updated.LastStatus = result.Status
	updated.LastLatencyMS = result.LatencyMS
	updated.LastError = result.ErrorMessage
	updated.LastCheckAt = &result.Timestamp
	updated.LastStatusChangedAt = lastStatusChangedAt
	updated.ConsecutiveFails = consecutiveFails
	updated.ConsecutiveSuccesses = consecutiveSucc
	if result.SSLExpiresAt != nil {
		updated.SSLExpiresAt = result.SSLExpiresAt
	}
	if strings.TrimSpace(result.SSLIssuer) != "" {
		updated.SSLIssuer = result.SSLIssuer
	}

	if _, err := tx.Exec(ctx, `
		UPDATE vps_health_checks SET
			last_status = $1,
			last_latency_ms = $2,
			last_error = $3,
			last_check_at = $4,
			last_status_changed_at = $5,
			consecutive_fails = $6,
			consecutive_successes = $7,
			ssl_expires_at = COALESCE($8, ssl_expires_at),
			ssl_issuer = CASE WHEN $9 = '' THEN ssl_issuer ELSE $9 END,
			updated_at = NOW()
		WHERE id = $10::uuid
	`,
		updated.LastStatus, updated.LastLatencyMS, updated.LastError, updated.LastCheckAt, updated.LastStatusChangedAt,
		updated.ConsecutiveFails, updated.ConsecutiveSuccesses,
		result.SSLExpiresAt, result.SSLIssuer, result.CheckID,
	); err != nil {
		return model.VPSHealthCheck{}, false, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO vps_health_events (vps_id, check_id, status, latency_ms, error_message, created_at)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6)
	`, vpsID, result.CheckID, result.Status, result.LatencyMS, result.ErrorMessage, result.Timestamp); err != nil {
		return model.VPSHealthCheck{}, false, err
	}

	if err := tx.Commit(ctx); err != nil {
		return model.VPSHealthCheck{}, false, err
	}
	return updated, statusChanged, nil
}

// MarkCheckAlertActive sets alert_active and alert_last_sent_at when a fresh
// alert is dispatched.
func (r *VPSRepository) MarkCheckAlertActive(ctx context.Context, checkID string, now time.Time) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	_, err := repository.DB(ctx, r.db).Exec(ctx, `
		UPDATE vps_health_checks
		SET alert_active = TRUE, alert_last_sent_at = $2, updated_at = NOW()
		WHERE id = $1::uuid
	`, checkID, now)
	return err
}

// ClearCheckAlert clears alert_active when the check recovers.
func (r *VPSRepository) ClearCheckAlert(ctx context.Context, checkID string) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	_, err := repository.DB(ctx, r.db).Exec(ctx, `
		UPDATE vps_health_checks
		SET alert_active = FALSE, updated_at = NOW()
		WHERE id = $1::uuid
	`, checkID)
	return err
}

// ListEnabledChecksForVPS returns the live status of every enabled check on
// a VPS so the snapshot rollup can compute the summary status.
func (r *VPSRepository) ListEnabledChecksForVPS(ctx context.Context, vpsID string) ([]model.VPSHealthCheck, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	query := fmt.Sprintf(`SELECT %s FROM vps_health_checks WHERE vps_id = $1::uuid AND enabled = TRUE`, vpsCheckColumns)
	rows, err := repository.DB(ctx, r.db).Query(ctx, query, vpsID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]model.VPSHealthCheck, 0)
	for rows.Next() {
		var c model.VPSHealthCheck
		if err := scanVPSCheck(rows, &c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Apps
// ---------------------------------------------------------------------------

type CreateVPSAppParams struct {
	VPSID   string
	Name    string
	AppType string
	Port    *int
	URL     string
	Notes   string
	CheckID *string
}

type UpdateVPSAppParams struct {
	Name    string
	AppType string
	Port    *int
	URL     string
	Notes   string
	CheckID *string
}

const vpsAppColumns = `
	id::text, vps_id::text, name, app_type, port, url, notes,
	check_id::text, created_at, updated_at
`

func scanVPSApp(row pgx.Row, a *model.VPSApp) error {
	var port *int
	var checkID *string
	if err := row.Scan(
		&a.ID, &a.VPSID, &a.Name, &a.AppType, &port, &a.URL, &a.Notes,
		&checkID, &a.CreatedAt, &a.UpdatedAt,
	); err != nil {
		return err
	}
	a.Port = port
	a.CheckID = checkID
	return nil
}

func (r *VPSRepository) CreateApp(ctx context.Context, p CreateVPSAppParams) (model.VPSApp, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	query := fmt.Sprintf(`
		INSERT INTO vps_apps (vps_id, name, app_type, port, url, notes, check_id)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, NULLIF($7, '')::uuid)
		RETURNING %s
	`, vpsAppColumns)

	checkIDArg := ""
	if p.CheckID != nil {
		checkIDArg = *p.CheckID
	}

	var a model.VPSApp
	if err := scanVPSApp(repository.DB(ctx, r.db).QueryRow(ctx, query,
		p.VPSID, p.Name, p.AppType, p.Port, p.URL, p.Notes, checkIDArg,
	), &a); err != nil {
		return model.VPSApp{}, err
	}
	return a, nil
}

func (r *VPSRepository) UpdateApp(ctx context.Context, appID string, p UpdateVPSAppParams) (model.VPSApp, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	checkIDArg := ""
	if p.CheckID != nil {
		checkIDArg = *p.CheckID
	}

	query := fmt.Sprintf(`
		UPDATE vps_apps SET
			name = $1, app_type = $2, port = $3, url = $4, notes = $5,
			check_id = NULLIF($6, '')::uuid,
			updated_at = NOW()
		WHERE id = $7::uuid
		RETURNING %s
	`, vpsAppColumns)

	var a model.VPSApp
	err := scanVPSApp(repository.DB(ctx, r.db).QueryRow(ctx, query,
		p.Name, p.AppType, p.Port, p.URL, p.Notes, checkIDArg, appID,
	), &a)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.VPSApp{}, ErrVPSAppNotFound
		}
		return model.VPSApp{}, err
	}
	return a, nil
}

func (r *VPSRepository) DeleteApp(ctx context.Context, appID string) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	tag, err := repository.DB(ctx, r.db).Exec(ctx, `DELETE FROM vps_apps WHERE id = $1::uuid`, appID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrVPSAppNotFound
	}
	return nil
}

func (r *VPSRepository) ListAppsForVPS(ctx context.Context, vpsID string) ([]model.VPSApp, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	query := fmt.Sprintf(`
		SELECT %s FROM vps_apps WHERE vps_id = $1::uuid ORDER BY name ASC
	`, vpsAppColumns)

	rows, err := repository.DB(ctx, r.db).Query(ctx, query, vpsID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]model.VPSApp, 0)
	for rows.Next() {
		var a model.VPSApp
		if err := scanVPSApp(rows, &a); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Events + Daily summary
// ---------------------------------------------------------------------------

func (r *VPSRepository) ListEventsForVPS(ctx context.Context, vpsID string, limit int) ([]model.VPSHealthEvent, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT id::text, vps_id::text, check_id::text, status, latency_ms, error_message, created_at
		FROM vps_health_events
		WHERE vps_id = $1::uuid
		ORDER BY created_at DESC
		LIMIT $2
	`, vpsID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]model.VPSHealthEvent, 0)
	for rows.Next() {
		var e model.VPSHealthEvent
		if err := rows.Scan(&e.ID, &e.VPSID, &e.CheckID, &e.Status, &e.LatencyMS, &e.ErrorMessage, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// PurgeOldEvents deletes vps_health_events older than the cutoff.
// Returns rows affected so the caller can log a meaningful summary.
func (r *VPSRepository) PurgeOldEvents(ctx context.Context, cutoff time.Time) (int64, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()
	tag, err := repository.DB(ctx, r.db).Exec(ctx, `DELETE FROM vps_health_events WHERE created_at < $1`, cutoff)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// RollupDailySummary upserts (vps_id, check_id, summary_date) aggregations
// for the given date based on raw events. Idempotent.
func (r *VPSRepository) RollupDailySummary(ctx context.Context, day time.Time) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	dayStr := day.Format("2006-01-02")

	_, err := repository.DB(ctx, r.db).Exec(ctx, `
		INSERT INTO vps_health_daily_summary (
			vps_id, check_id, summary_date,
			total_checks, up_count, down_count, uptime_pct, avg_latency_ms
		)
		SELECT
			vps_id,
			check_id,
			$1::date,
			COUNT(*)::int                                     AS total_checks,
			COUNT(*) FILTER (WHERE status = 'up')::int        AS up_count,
			COUNT(*) FILTER (WHERE status = 'down')::int      AS down_count,
			ROUND(
				100.0 * COUNT(*) FILTER (WHERE status = 'up') / NULLIF(COUNT(*), 0),
				2
			)                                                 AS uptime_pct,
			NULLIF(AVG(latency_ms) FILTER (WHERE status = 'up'), NULL)::int AS avg_latency_ms
		FROM vps_health_events
		WHERE created_at >= $1::date
		  AND created_at <  ($1::date + INTERVAL '1 day')
		GROUP BY vps_id, check_id
		ON CONFLICT (tenant_id, check_id, summary_date) DO UPDATE
		SET total_checks   = EXCLUDED.total_checks,
		    up_count       = EXCLUDED.up_count,
		    down_count     = EXCLUDED.down_count,
		    uptime_pct     = EXCLUDED.uptime_pct,
		    avg_latency_ms = EXCLUDED.avg_latency_ms,
		    updated_at     = NOW()
	`, dayStr)
	return err
}

func (r *VPSRepository) ListDailySummaryForVPS(ctx context.Context, vpsID string, since time.Time) ([]model.VPSHealthDailySummary, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT vps_id::text, check_id::text, summary_date,
		       total_checks, up_count, down_count, uptime_pct, avg_latency_ms, p95_latency_ms
		FROM vps_health_daily_summary
		WHERE vps_id = $1::uuid AND summary_date >= $2::date
		ORDER BY summary_date ASC
	`, vpsID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]model.VPSHealthDailySummary, 0)
	for rows.Next() {
		var s model.VPSHealthDailySummary
		var avg, p95 *int
		if err := rows.Scan(&s.VPSID, &s.CheckID, &s.SummaryDate, &s.TotalChecks, &s.UpCount, &s.DownCount, &s.UptimePct, &avg, &p95); err != nil {
			return nil, err
		}
		s.AvgLatencyMS = avg
		s.P95LatencyMS = p95
		out = append(out, s)
	}
	return out, rows.Err()
}
