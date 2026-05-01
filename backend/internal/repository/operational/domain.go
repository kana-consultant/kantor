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

var (
	ErrDomainNotFound = errors.New("domain not found")
)

type DomainRepository struct {
	db repository.DBTX
}

func NewDomainRepository(db repository.DBTX) *DomainRepository {
	return &DomainRepository{db: db}
}

type CreateDomainParams struct {
	Name                    string
	Registrar               string
	Nameservers             []string
	ExpiryDate              *time.Time
	CostAmount              int64
	CostCurrency            string
	BillingCycle            string
	Status                  string
	Tags                    []string
	Notes                   string
	DNSCheckEnabled         bool
	DNSExpectedIP           string
	DNSCheckIntervalSeconds int
	WhoisSyncEnabled        bool
	CreatedBy               string
}

type UpdateDomainParams struct {
	Name                    string
	Registrar               string
	Nameservers             []string
	ExpiryDate              *time.Time
	CostAmount              int64
	CostCurrency            string
	BillingCycle            string
	Status                  string
	Tags                    []string
	Notes                   string
	DNSCheckEnabled         bool
	DNSExpectedIP           string
	DNSCheckIntervalSeconds int
	WhoisSyncEnabled        bool
}

type ListDomainParams struct {
	Status    string
	Registrar string
	Tag       string
	Search    string
}

const domainColumns = `
	id::text, tenant_id::text, name, registrar, nameservers, expiry_date,
	cost_amount, cost_currency, billing_cycle, status, tags, notes,
	dns_check_enabled, dns_expected_ip, dns_check_interval_seconds,
	dns_last_status, dns_last_resolved_ips, dns_last_error,
	dns_last_check_at, dns_last_status_changed_at,
	dns_consecutive_fails, dns_alert_active, dns_alert_last_sent_at,
	whois_sync_enabled, whois_last_sync_at, whois_last_error,
	created_by::text, created_at, updated_at
`

func scanDomain(row pgx.Row, d *model.Domain) error {
	var (
		expiryDate             *time.Time
		dnsLastCheckAt         *time.Time
		dnsLastStatusChangedAt *time.Time
		dnsAlertLastSentAt     *time.Time
		whoisLastSyncAt        *time.Time
		createdBy              *string
	)
	if err := row.Scan(
		&d.ID, &d.TenantID, &d.Name, &d.Registrar, &d.Nameservers, &expiryDate,
		&d.CostAmount, &d.CostCurrency, &d.BillingCycle, &d.Status, &d.Tags, &d.Notes,
		&d.DNSCheckEnabled, &d.DNSExpectedIP, &d.DNSCheckIntervalSeconds,
		&d.DNSLastStatus, &d.DNSLastResolvedIPs, &d.DNSLastError,
		&dnsLastCheckAt, &dnsLastStatusChangedAt,
		&d.DNSConsecutiveFails, &d.DNSAlertActive, &dnsAlertLastSentAt,
		&d.WhoisSyncEnabled, &whoisLastSyncAt, &d.WhoisLastError,
		&createdBy, &d.CreatedAt, &d.UpdatedAt,
	); err != nil {
		return err
	}
	d.ExpiryDate = expiryDate
	d.DNSLastCheckAt = dnsLastCheckAt
	d.DNSLastStatusChangedAt = dnsLastStatusChangedAt
	d.DNSAlertLastSentAt = dnsAlertLastSentAt
	d.WhoisLastSyncAt = whoisLastSyncAt
	d.CreatedBy = createdBy
	return nil
}

func (r *DomainRepository) CreateDomain(ctx context.Context, p CreateDomainParams) (model.Domain, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	tags := p.Tags
	if tags == nil {
		tags = []string{}
	}
	nameservers := p.Nameservers
	if nameservers == nil {
		nameservers = []string{}
	}

	intervalSec := p.DNSCheckIntervalSeconds
	if intervalSec <= 0 {
		intervalSec = 3600
	}

	query := fmt.Sprintf(`
		INSERT INTO domains (
			name, registrar, nameservers, expiry_date,
			cost_amount, cost_currency, billing_cycle, status, tags, notes,
			dns_check_enabled, dns_expected_ip, dns_check_interval_seconds,
			whois_sync_enabled, created_by
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, NULLIF($15, '')::uuid)
		RETURNING %s
	`, domainColumns)

	var d model.Domain
	row := repository.DB(ctx, r.db).QueryRow(ctx, query,
		strings.ToLower(strings.TrimSpace(p.Name)),
		p.Registrar, nameservers, p.ExpiryDate,
		p.CostAmount, p.CostCurrency, p.BillingCycle, p.Status, tags, p.Notes,
		p.DNSCheckEnabled, p.DNSExpectedIP, intervalSec,
		p.WhoisSyncEnabled, p.CreatedBy,
	)
	if err := scanDomain(row, &d); err != nil {
		return model.Domain{}, err
	}
	return d, nil
}

func (r *DomainRepository) UpdateDomain(ctx context.Context, domainID string, p UpdateDomainParams) (model.Domain, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	tags := p.Tags
	if tags == nil {
		tags = []string{}
	}
	nameservers := p.Nameservers
	if nameservers == nil {
		nameservers = []string{}
	}
	intervalSec := p.DNSCheckIntervalSeconds
	if intervalSec <= 0 {
		intervalSec = 3600
	}

	query := fmt.Sprintf(`
		UPDATE domains SET
			name = $2, registrar = $3, nameservers = $4, expiry_date = $5,
			cost_amount = $6, cost_currency = $7, billing_cycle = $8, status = $9,
			tags = $10, notes = $11,
			dns_check_enabled = $12, dns_expected_ip = $13, dns_check_interval_seconds = $14,
			whois_sync_enabled = $15,
			updated_at = NOW()
		WHERE id = $1::uuid
		RETURNING %s
	`, domainColumns)

	var d model.Domain
	row := repository.DB(ctx, r.db).QueryRow(ctx, query,
		domainID,
		strings.ToLower(strings.TrimSpace(p.Name)),
		p.Registrar, nameservers, p.ExpiryDate,
		p.CostAmount, p.CostCurrency, p.BillingCycle, p.Status, tags, p.Notes,
		p.DNSCheckEnabled, p.DNSExpectedIP, intervalSec,
		p.WhoisSyncEnabled,
	)
	if err := scanDomain(row, &d); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Domain{}, ErrDomainNotFound
		}
		return model.Domain{}, err
	}
	return d, nil
}

func (r *DomainRepository) DeleteDomain(ctx context.Context, domainID string) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	tag, err := repository.DB(ctx, r.db).Exec(ctx, `DELETE FROM domains WHERE id = $1::uuid`, domainID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrDomainNotFound
	}
	return nil
}

func (r *DomainRepository) GetDomainByID(ctx context.Context, domainID string) (model.Domain, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	query := fmt.Sprintf(`SELECT %s FROM domains WHERE id = $1::uuid`, domainColumns)
	row := repository.DB(ctx, r.db).QueryRow(ctx, query, domainID)
	var d model.Domain
	if err := scanDomain(row, &d); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Domain{}, ErrDomainNotFound
		}
		return model.Domain{}, err
	}
	return d, nil
}

func (r *DomainRepository) ListDomains(ctx context.Context, p ListDomainParams) ([]model.Domain, error) {
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
	if registrar := strings.TrimSpace(p.Registrar); registrar != "" {
		filters = append(filters, fmt.Sprintf("registrar = $%d", idx))
		args = append(args, registrar)
		idx++
	}
	if tag := strings.TrimSpace(p.Tag); tag != "" {
		filters = append(filters, fmt.Sprintf("$%d = ANY(tags)", idx))
		args = append(args, tag)
		idx++
	}
	if search := strings.TrimSpace(p.Search); search != "" {
		filters = append(filters, fmt.Sprintf("(name ILIKE $%d OR registrar ILIKE $%d)", idx, idx))
		args = append(args, "%"+search+"%")
		idx++
	}

	query := fmt.Sprintf(`
		SELECT %s FROM domains
		WHERE %s
		ORDER BY name ASC
	`, domainColumns, strings.Join(filters, " AND "))

	rows, err := repository.DB(ctx, r.db).Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]model.Domain, 0)
	for rows.Next() {
		var d model.Domain
		if err := scanDomain(rows, &d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ListDomainsWithExpiryBefore returns active domains with expiry_date <= cutoff.
// Used by the renewal alert scheduler.
func (r *DomainRepository) ListDomainsWithExpiryBefore(ctx context.Context, cutoff time.Time) ([]model.Domain, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	query := fmt.Sprintf(`
		SELECT %s FROM domains
		WHERE status = 'active' AND expiry_date IS NOT NULL AND expiry_date <= $1::date
		ORDER BY expiry_date ASC
	`, domainColumns)

	rows, err := repository.DB(ctx, r.db).Query(ctx, query, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]model.Domain, 0)
	for rows.Next() {
		var d model.Domain
		if err := scanDomain(rows, &d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ListDueDNSChecks returns enabled domains whose DNS check is due.
func (r *DomainRepository) ListDueDNSChecks(ctx context.Context, now time.Time) ([]model.Domain, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	query := fmt.Sprintf(`
		SELECT %s FROM domains
		WHERE dns_check_enabled = TRUE
		  AND (
		    dns_last_check_at IS NULL
		    OR dns_last_check_at + (dns_check_interval_seconds * INTERVAL '1 second') <= $1
		  )
	`, domainColumns)

	rows, err := repository.DB(ctx, r.db).Query(ctx, query, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]model.Domain, 0)
	for rows.Next() {
		var d model.Domain
		if err := scanDomain(rows, &d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ListWhoisSyncDue returns domains with whois sync enabled that haven't been
// synced in the past 24 hours (or never).
func (r *DomainRepository) ListWhoisSyncDue(ctx context.Context, now time.Time) ([]model.Domain, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	query := fmt.Sprintf(`
		SELECT %s FROM domains
		WHERE whois_sync_enabled = TRUE
		  AND (whois_last_sync_at IS NULL OR whois_last_sync_at < $1 - INTERVAL '24 hours')
	`, domainColumns)

	rows, err := repository.DB(ctx, r.db).Query(ctx, query, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]model.Domain, 0)
	for rows.Next() {
		var d model.Domain
		if err := scanDomain(rows, &d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

type DNSCheckResult struct {
	Status       string // 'up' | 'down'
	ResolvedIPs  []string
	ErrorMessage string
	Timestamp    time.Time
}

// RecordDNSCheckResult updates the per-domain DNS check state and returns
// the updated row + a flag indicating whether the status transitioned.
func (r *DomainRepository) RecordDNSCheckResult(ctx context.Context, domainID string, result DNSCheckResult) (model.Domain, bool, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	tx, err := repository.DB(ctx, r.db).Begin(ctx)
	if err != nil {
		return model.Domain{}, false, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	// Lock + read current state.
	var prevStatus string
	if err = tx.QueryRow(ctx, `SELECT dns_last_status FROM domains WHERE id = $1::uuid FOR UPDATE`, domainID).Scan(&prevStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			err = ErrDomainNotFound
		}
		return model.Domain{}, false, err
	}

	statusChanged := prevStatus != result.Status

	resolved := result.ResolvedIPs
	if resolved == nil {
		resolved = []string{}
	}

	updateQuery := fmt.Sprintf(`
		UPDATE domains SET
			dns_last_status = $2,
			dns_last_resolved_ips = $3,
			dns_last_error = $4,
			dns_last_check_at = $5,
			dns_last_status_changed_at = CASE WHEN dns_last_status <> $2 THEN $5 ELSE dns_last_status_changed_at END,
			dns_consecutive_fails = CASE WHEN $2 = 'down' THEN dns_consecutive_fails + 1 ELSE 0 END,
			updated_at = NOW()
		WHERE id = $1::uuid
		RETURNING %s
	`, domainColumns)

	var d model.Domain
	row := tx.QueryRow(ctx, updateQuery,
		domainID, result.Status, resolved, result.ErrorMessage, result.Timestamp,
	)
	if err = scanDomain(row, &d); err != nil {
		return model.Domain{}, false, err
	}

	if err = tx.Commit(ctx); err != nil {
		return model.Domain{}, false, err
	}
	return d, statusChanged, nil
}

func (r *DomainRepository) MarkDNSAlertActive(ctx context.Context, domainID string, now time.Time) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	_, err := repository.DB(ctx, r.db).Exec(ctx,
		`UPDATE domains SET dns_alert_active = TRUE, dns_alert_last_sent_at = $2, updated_at = NOW() WHERE id = $1::uuid`,
		domainID, now)
	return err
}

func (r *DomainRepository) ClearDNSAlert(ctx context.Context, domainID string) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	_, err := repository.DB(ctx, r.db).Exec(ctx,
		`UPDATE domains SET dns_alert_active = FALSE, dns_alert_last_sent_at = NULL, updated_at = NOW() WHERE id = $1::uuid`,
		domainID)
	return err
}

// SetWhoisSyncResult records the outcome of a WHOIS lookup. If expiry is
// non-nil the domain's expiry_date is updated.
func (r *DomainRepository) SetWhoisSyncResult(ctx context.Context, domainID string, now time.Time, expiry *time.Time, errMsg string) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	if expiry != nil {
		_, err := repository.DB(ctx, r.db).Exec(ctx,
			`UPDATE domains SET expiry_date = $2::date, whois_last_sync_at = $3, whois_last_error = $4, updated_at = NOW() WHERE id = $1::uuid`,
			domainID, expiry, now, errMsg)
		return err
	}
	_, err := repository.DB(ctx, r.db).Exec(ctx,
		`UPDATE domains SET whois_last_sync_at = $2, whois_last_error = $3, updated_at = NOW() WHERE id = $1::uuid`,
		domainID, now, errMsg)
	return err
}

// CreateEvent appends one event to the log.
func (r *DomainRepository) CreateEvent(ctx context.Context, domainID, eventType, status, detail string) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	_, err := repository.DB(ctx, r.db).Exec(ctx,
		`INSERT INTO domain_health_events (domain_id, event_type, status, detail) VALUES ($1::uuid, $2, $3, $4)`,
		domainID, eventType, status, detail)
	return err
}

func (r *DomainRepository) ListEventsForDomain(ctx context.Context, domainID string, limit int) ([]model.DomainHealthEvent, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT id::text, domain_id::text, event_type, status, detail, created_at
		FROM domain_health_events
		WHERE domain_id = $1::uuid
		ORDER BY created_at DESC
		LIMIT $2
	`, domainID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]model.DomainHealthEvent, 0, limit)
	for rows.Next() {
		var e model.DomainHealthEvent
		if err := rows.Scan(&e.ID, &e.DomainID, &e.EventType, &e.Status, &e.Detail, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *DomainRepository) PurgeOldEvents(ctx context.Context, cutoff time.Time) (int64, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	tag, err := repository.DB(ctx, r.db).Exec(ctx, `DELETE FROM domain_health_events WHERE created_at < $1`, cutoff)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
