package operational

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/kana-consultant/kantor/backend/internal/model"
	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

var (
	ErrTrackerConsentNotFound = errors.New("activity consent not found")
	ErrTrackerSessionNotFound = errors.New("activity session not found")
	ErrDomainCategoryNotFound = errors.New("domain category not found")
)

type TrackerHeartbeatParams struct {
	SessionID string
	UserID    string
	URL       string
	Domain    string
	PageTitle *string
	IsIdle    bool
	Timestamp time.Time
}

type TrackerActivityRange struct {
	DateFrom time.Time
	DateTo   time.Time
}

type TrackerBatchResult struct {
	Processed int `json:"processed"`
	Skipped   int `json:"skipped"`
}

type UpsertDomainCategoryParams struct {
	DomainPattern string
	Category      string
	IsProductive  bool
}

type TrackerRepository struct {
	db repository.DBTX
}

func NewTrackerRepository(db repository.DBTX) *TrackerRepository {
	return &TrackerRepository{db: db}
}

func (r *TrackerRepository) GetConsent(ctx context.Context, userID string) (model.ActivityConsent, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var consent model.ActivityConsent
	var consentedAt sql.NullTime
	var revokedAt sql.NullTime
	var ipAddress sql.NullString
	err := repository.DB(ctx, r.db).QueryRow(ctx, `
		SELECT id::text, user_id::text, consented, consented_at, revoked_at, ip_address, created_at
		FROM activity_consents
		WHERE user_id = $1::uuid
	`, userID).Scan(
		&consent.ID,
		&consent.UserID,
		&consent.Consented,
		&consentedAt,
		&revokedAt,
		&ipAddress,
		&consent.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.ActivityConsent{}, ErrTrackerConsentNotFound
		}
		return model.ActivityConsent{}, err
	}

	consent.ConsentedAt = nullTimePointer(consentedAt)
	consent.RevokedAt = nullTimePointer(revokedAt)
	consent.IPAddress = nullStringPointer(ipAddress)

	return consent, nil
}

func (r *TrackerRepository) UpsertConsent(ctx context.Context, userID string, consented bool, ipAddress string, now time.Time) (model.ActivityConsent, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var consent model.ActivityConsent
	var consentedAt sql.NullTime
	var revokedAt sql.NullTime
	var ipAddressValue sql.NullString
	err := repository.DB(ctx, r.db).QueryRow(ctx, `
		INSERT INTO activity_consents (user_id, consented, consented_at, revoked_at, ip_address)
		VALUES (
			$1::uuid,
			$2,
			CASE WHEN $2 THEN $3::timestamptz ELSE NULL::timestamptz END,
			CASE WHEN $2 THEN NULL::timestamptz ELSE $3::timestamptz END,
			NULLIF($4, '')
		)
		ON CONFLICT (tenant_id, user_id) DO UPDATE
		SET
			consented = EXCLUDED.consented,
			consented_at = CASE WHEN EXCLUDED.consented THEN EXCLUDED.consented_at ELSE activity_consents.consented_at END,
			revoked_at = CASE WHEN EXCLUDED.consented THEN NULL ELSE EXCLUDED.revoked_at END,
			ip_address = EXCLUDED.ip_address
		RETURNING id::text, user_id::text, consented, consented_at, revoked_at, ip_address, created_at
	`, userID, consented, now.UTC(), strings.TrimSpace(ipAddress)).Scan(
		&consent.ID,
		&consent.UserID,
		&consent.Consented,
		&consentedAt,
		&revokedAt,
		&ipAddressValue,
		&consent.CreatedAt,
	)
	if err != nil {
		slog.Error("tracker consent upsert query failed", "error", err, "user_id", userID, "consented", consented)
		return model.ActivityConsent{}, err
	}

	consent.ConsentedAt = nullTimePointer(consentedAt)
	consent.RevokedAt = nullTimePointer(revokedAt)
	consent.IPAddress = nullStringPointer(ipAddressValue)

	return consent, nil
}

func (r *TrackerRepository) StartSession(ctx context.Context, userID string, startedAt time.Time) (model.ActivitySession, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	tx, err := repository.DB(ctx, r.db).Begin(ctx)
	if err != nil {
		return model.ActivitySession{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	var existing model.ActivitySession
	err = tx.QueryRow(ctx, `
		SELECT id::text, user_id::text, date, start_time, end_time, total_active_seconds, total_idle_seconds, is_active, created_at, updated_at
		FROM activity_sessions
		WHERE user_id = $1::uuid AND is_active = TRUE
		ORDER BY start_time DESC
		LIMIT 1
		FOR UPDATE
	`, userID).Scan(
		&existing.ID,
		&existing.UserID,
		&existing.Date,
		&existing.StartTime,
		&existing.EndTime,
		&existing.TotalActiveSeconds,
		&existing.TotalIdleSeconds,
		&existing.IsActive,
		&existing.CreatedAt,
		&existing.UpdatedAt,
	)
	if err == nil {
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return model.ActivitySession{}, commitErr
		}
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return model.ActivitySession{}, err
	}

	var session model.ActivitySession
	err = tx.QueryRow(ctx, `
		INSERT INTO activity_sessions (user_id, date, start_time, end_time, total_active_seconds, total_idle_seconds, is_active)
		VALUES ($1::uuid, $2::date, $3, $3, 0, 0, TRUE)
		RETURNING id::text, user_id::text, date, start_time, end_time, total_active_seconds, total_idle_seconds, is_active, created_at, updated_at
	`, userID, startedAt.UTC().Format("2006-01-02"), startedAt.UTC()).Scan(
		&session.ID,
		&session.UserID,
		&session.Date,
		&session.StartTime,
		&session.EndTime,
		&session.TotalActiveSeconds,
		&session.TotalIdleSeconds,
		&session.IsActive,
		&session.CreatedAt,
		&session.UpdatedAt,
	)
	if err != nil {
		return model.ActivitySession{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return model.ActivitySession{}, err
	}

	return session, nil
}

func (r *TrackerRepository) EndSession(ctx context.Context, userID string, sessionID string, endedAt time.Time) (model.ActivitySession, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var session model.ActivitySession
	err := repository.DB(ctx, r.db).QueryRow(ctx, `
		UPDATE activity_sessions
		SET end_time = $3, is_active = FALSE, updated_at = $3
		WHERE id = $1::uuid AND user_id = $2::uuid
		RETURNING id::text, user_id::text, date, start_time, end_time, total_active_seconds, total_idle_seconds, is_active, created_at, updated_at
	`, sessionID, userID, endedAt.UTC()).Scan(
		&session.ID,
		&session.UserID,
		&session.Date,
		&session.StartTime,
		&session.EndTime,
		&session.TotalActiveSeconds,
		&session.TotalIdleSeconds,
		&session.IsActive,
		&session.CreatedAt,
		&session.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.ActivitySession{}, ErrTrackerSessionNotFound
		}
		return model.ActivitySession{}, err
	}

	return session, nil
}

func (r *TrackerRepository) RecordHeartbeat(ctx context.Context, params TrackerHeartbeatParams) (model.ActivityEntry, model.ActivitySession, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	tx, err := repository.DB(ctx, r.db).Begin(ctx)
	if err != nil {
		return model.ActivityEntry{}, model.ActivitySession{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	session, err := r.getSessionForUpdate(ctx, tx, params.UserID, params.SessionID)
	if err != nil {
		return model.ActivityEntry{}, model.ActivitySession{}, err
	}

	if !session.IsActive {
		return model.ActivityEntry{}, model.ActivitySession{}, ErrTrackerSessionNotFound
	}

	timestamp := params.Timestamp.UTC()
	if timestamp.Before(session.StartTime) {
		timestamp = session.StartTime
	}

	deltaSeconds := int(timestamp.Sub(session.UpdatedAt).Seconds())
	if deltaSeconds < 0 {
		deltaSeconds = 0
	}

	var entry model.ActivityEntry
	if !params.IsIdle && deltaSeconds > 0 {
		category, productive, resolveErr := r.resolveDomainCategory(ctx, tx, params.Domain)
		if resolveErr != nil {
			return model.ActivityEntry{}, model.ActivitySession{}, resolveErr
		}

		entry, err = r.upsertActivityEntry(ctx, tx, params, category, productive, deltaSeconds, timestamp)
		if err != nil {
			return model.ActivityEntry{}, model.ActivitySession{}, err
		}
	}

	session, err = r.updateSessionAfterHeartbeat(ctx, tx, session, deltaSeconds, params.IsIdle, timestamp)
	if err != nil {
		return model.ActivityEntry{}, model.ActivitySession{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return model.ActivityEntry{}, model.ActivitySession{}, err
	}

	return entry, session, nil
}

func (r *TrackerRepository) GetActivityOverview(ctx context.Context, userID string, activityRange TrackerActivityRange) (model.TrackerActivityOverview, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	overview := model.TrackerActivityOverview{
		UserID:            userID,
		CategoryBreakdown: make([]model.TrackerCategoryBreakdown, 0),
		HourlyBreakdown:   make([]model.TrackerHourlyBreakdown, 0, 24),
		TopDomains:        make([]model.TrackerTopDomain, 0),
	}

	if err := repository.DB(ctx, r.db).QueryRow(ctx, `
		SELECT COALESCE(full_name, email) FROM users WHERE id = $1::uuid
	`, userID).Scan(&overview.UserName); err != nil {
		return model.TrackerActivityOverview{}, fmt.Errorf("load user name: %w", err)
	}

	if err := repository.DB(ctx, r.db).QueryRow(ctx, `
		SELECT COALESCE(SUM(total_active_seconds), 0), COALESCE(SUM(total_idle_seconds), 0)
		FROM activity_sessions
		WHERE user_id = $1::uuid
		  AND date BETWEEN $2::date AND $3::date
	`, userID, activityRange.DateFrom, activityRange.DateTo).Scan(&overview.TotalActiveSeconds, &overview.TotalIdleSeconds); err != nil {
		return model.TrackerActivityOverview{}, err
	}

	productiveSeconds, err := r.getProductiveSeconds(ctx, userID, activityRange)
	if err != nil {
		return model.TrackerActivityOverview{}, err
	}
	if overview.TotalActiveSeconds > 0 {
		overview.ProductivityScore = (float64(productiveSeconds) / float64(overview.TotalActiveSeconds)) * 100
	}

	if overview.CategoryBreakdown, err = r.listCategoryBreakdown(ctx, userID, activityRange); err != nil {
		return model.TrackerActivityOverview{}, err
	}
	if overview.HourlyBreakdown, err = r.listHourlyBreakdown(ctx, userID, activityRange); err != nil {
		return model.TrackerActivityOverview{}, err
	}
	if overview.TopDomains, err = r.listTopDomains(ctx, userID, activityRange, 10); err != nil {
		return model.TrackerActivityOverview{}, err
	}
	if len(overview.TopDomains) > 0 {
		overview.MostUsedDomain = &overview.TopDomains[0].Domain
	}

	return overview, nil
}

func (r *TrackerRepository) GetTeamActivity(ctx context.Context, activityRange TrackerActivityRange, userID *string) (model.TrackerTeamOverview, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	filters := []string{"s.date BETWEEN $1::date AND $2::date"}
	args := []interface{}{activityRange.DateFrom, activityRange.DateTo}
	if userID != nil && strings.TrimSpace(*userID) != "" {
		filters = append(filters, "s.user_id = $3::uuid")
		args = append(args, strings.TrimSpace(*userID))
	}
	whereClause := strings.Join(filters, " AND ")

	rows, err := repository.DB(ctx, r.db).Query(ctx, fmt.Sprintf(`
		WITH session_totals AS (
			SELECT s.user_id::text, COALESCE(SUM(s.total_active_seconds), 0)::bigint AS active_seconds, COALESCE(SUM(s.total_idle_seconds), 0)::bigint AS idle_seconds
			FROM activity_sessions s
			WHERE %s
			GROUP BY s.user_id
		),
		productive AS (
			SELECT
				e.user_id::text,
				COALESCE(SUM(e.duration_seconds) FILTER (WHERE COALESCE(dc.is_productive, FALSE)), 0)::bigint AS productive_seconds
			FROM activity_entries e
			LEFT JOIN LATERAL (
				SELECT is_productive
				FROM domain_categories
				WHERE e.domain = domain_pattern OR e.domain LIKE ('%%.' || domain_pattern)
				ORDER BY LENGTH(domain_pattern) DESC
				LIMIT 1
			) dc ON TRUE
			WHERE e.started_at::date BETWEEN $1::date AND $2::date
			%s
			GROUP BY e.user_id
		),
		top_domains AS (
			SELECT DISTINCT ON (user_id)
				user_id,
				domain
			FROM (
				SELECT e.user_id::text AS user_id, e.domain, SUM(e.duration_seconds)::bigint AS duration_seconds
				FROM activity_entries e
				WHERE e.started_at::date BETWEEN $1::date AND $2::date
				%s
				GROUP BY e.user_id, e.domain
			) ranked
			ORDER BY user_id, duration_seconds DESC, domain ASC
		)
		SELECT
			u.id::text,
			u.full_name,
			st.active_seconds,
			st.idle_seconds,
			COALESCE(p.productive_seconds, 0)::bigint,
			td.domain
		FROM session_totals st
		INNER JOIN users u ON u.id::text = st.user_id
		LEFT JOIN productive p ON p.user_id = st.user_id
		LEFT JOIN top_domains td ON td.user_id = st.user_id
		ORDER BY u.full_name ASC
	`, whereClause, optionalEntryUserFilter(userID, "e.user_id"), optionalEntryUserFilter(userID, "e.user_id")), args...)
	if err != nil {
		return model.TrackerTeamOverview{}, err
	}
	defer rows.Close()

	items := make([]model.TrackerUserSummary, 0)
	for rows.Next() {
		var item model.TrackerUserSummary
		var productiveSeconds int64
		if err := rows.Scan(&item.UserID, &item.UserName, &item.ActiveSeconds, &item.IdleSeconds, &productiveSeconds, &item.TopDomain); err != nil {
			return model.TrackerTeamOverview{}, err
		}
		if item.ActiveSeconds > 0 {
			item.ProductivityScore = (float64(productiveSeconds) / float64(item.ActiveSeconds)) * 100
		}
		item.CategoryBreakdown = map[string]int64{}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return model.TrackerTeamOverview{}, err
	}

	breakdownRows, err := repository.DB(ctx, r.db).Query(ctx, fmt.Sprintf(`
		SELECT e.user_id::text, e.category, COALESCE(SUM(e.duration_seconds), 0)::bigint
		FROM activity_entries e
		WHERE e.started_at::date BETWEEN $1::date AND $2::date
		%s
		GROUP BY e.user_id, e.category
	`, optionalEntryUserFilter(userID, "e.user_id")), args...)
	if err != nil {
		return model.TrackerTeamOverview{}, err
	}
	defer breakdownRows.Close()

	breakdowns := make(map[string]map[string]int64, len(items))
	for breakdownRows.Next() {
		var userID string
		var category string
		var duration int64
		if err := breakdownRows.Scan(&userID, &category, &duration); err != nil {
			return model.TrackerTeamOverview{}, err
		}
		if _, ok := breakdowns[userID]; !ok {
			breakdowns[userID] = map[string]int64{}
		}
		breakdowns[userID][category] = duration
	}
	if err := breakdownRows.Err(); err != nil {
		return model.TrackerTeamOverview{}, err
	}

	overview := model.TrackerTeamOverview{Users: items}
	if len(items) == 0 {
		return overview, nil
	}

	var totalActive int64
	for index := range overview.Users {
		totalActive += overview.Users[index].ActiveSeconds
		if mapped, ok := breakdowns[overview.Users[index].UserID]; ok {
			overview.Users[index].CategoryBreakdown = mapped
		}
	}

	overview.MembersTracked = int64(len(overview.Users))
	overview.AvgActiveSeconds = totalActive / int64(len(overview.Users))
	overview.TopProductiveMember = topProductiveMember(overview.Users)
	overview.LeastProductiveMember = leastProductiveMember(overview.Users)

	return overview, nil
}

func (r *TrackerRepository) GetDailySummary(ctx context.Context, date time.Time) (model.TrackerDailySummary, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	summary := model.TrackerDailySummary{
		TopProductiveDomains:   make([]model.TrackerTopDomain, 0),
		TopUnproductiveDomains: make([]model.TrackerTopDomain, 0),
	}

	if err := repository.DB(ctx, r.db).QueryRow(ctx, `
		SELECT COUNT(DISTINCT user_id)::bigint, COALESCE(AVG(total_active_seconds), 0)::bigint
		FROM activity_sessions
		WHERE date = $1::date
	`, date).Scan(&summary.TotalUsers, &summary.AvgActiveSeconds); err != nil {
		return model.TrackerDailySummary{}, err
	}

	productiveDomains, err := r.listDomainsByProductivity(ctx, date, true)
	if err != nil {
		return model.TrackerDailySummary{}, err
	}
	unproductiveDomains, err := r.listDomainsByProductivity(ctx, date, false)
	if err != nil {
		return model.TrackerDailySummary{}, err
	}
	summary.TopProductiveDomains = productiveDomains
	summary.TopUnproductiveDomains = unproductiveDomains

	return summary, nil
}

func (r *TrackerRepository) ListDomainCategories(ctx context.Context) ([]model.DomainCategory, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT id::text, domain_pattern, category, is_productive, created_at
		FROM domain_categories
		ORDER BY domain_pattern ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.DomainCategory, 0)
	for rows.Next() {
		var item model.DomainCategory
		if err := rows.Scan(&item.ID, &item.DomainPattern, &item.Category, &item.IsProductive, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, rows.Err()
}

func (r *TrackerRepository) ListConsentAudit(ctx context.Context) ([]model.TrackerConsentAudit, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT
			u.id::text,
			u.full_name,
			u.email,
			c.consented,
			c.consented_at,
			c.revoked_at,
			c.ip_address,
			MAX(s.start_time) AS last_session_started_at,
			MAX(s.updated_at) AS last_activity_at
		FROM activity_consents c
		INNER JOIN users u ON u.id = c.user_id
		LEFT JOIN activity_sessions s ON s.user_id = c.user_id
		GROUP BY u.id, u.full_name, u.email, c.consented, c.consented_at, c.revoked_at, c.ip_address
		ORDER BY u.full_name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.TrackerConsentAudit, 0)
	for rows.Next() {
		var item model.TrackerConsentAudit
		var consentedAt sql.NullTime
		var revokedAt sql.NullTime
		var ipAddress sql.NullString
		var lastSessionStartedAt sql.NullTime
		var lastActivityAt sql.NullTime
		if err := rows.Scan(
			&item.UserID,
			&item.UserName,
			&item.UserEmail,
			&item.Consented,
			&consentedAt,
			&revokedAt,
			&ipAddress,
			&lastSessionStartedAt,
			&lastActivityAt,
		); err != nil {
			return nil, err
		}
		item.ConsentedAt = nullTimePointer(consentedAt)
		item.RevokedAt = nullTimePointer(revokedAt)
		item.IPAddress = nullStringPointer(ipAddress)
		item.LastSessionStartedAt = nullTimePointer(lastSessionStartedAt)
		item.LastActivityAt = nullTimePointer(lastActivityAt)
		items = append(items, item)
	}

	return items, rows.Err()
}

func (r *TrackerRepository) CreateDomainCategory(ctx context.Context, params UpsertDomainCategoryParams) (model.DomainCategory, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var item model.DomainCategory
	err := repository.DB(ctx, r.db).QueryRow(ctx, `
		INSERT INTO domain_categories (domain_pattern, category, is_productive)
		VALUES ($1, $2, $3)
		RETURNING id::text, domain_pattern, category, is_productive, created_at
	`, strings.ToLower(strings.TrimSpace(params.DomainPattern)), strings.TrimSpace(params.Category), params.IsProductive).Scan(
		&item.ID,
		&item.DomainPattern,
		&item.Category,
		&item.IsProductive,
		&item.CreatedAt,
	)
	return item, err
}

func (r *TrackerRepository) UpdateDomainCategory(ctx context.Context, domainID string, params UpsertDomainCategoryParams) (model.DomainCategory, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var item model.DomainCategory
	err := repository.DB(ctx, r.db).QueryRow(ctx, `
		UPDATE domain_categories
		SET domain_pattern = $2, category = $3, is_productive = $4
		WHERE id = $1::uuid
		RETURNING id::text, domain_pattern, category, is_productive, created_at
	`, domainID, strings.ToLower(strings.TrimSpace(params.DomainPattern)), strings.TrimSpace(params.Category), params.IsProductive).Scan(
		&item.ID,
		&item.DomainPattern,
		&item.Category,
		&item.IsProductive,
		&item.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.DomainCategory{}, ErrDomainCategoryNotFound
		}
		return model.DomainCategory{}, err
	}
	return item, nil
}

func (r *TrackerRepository) DeleteDomainCategory(ctx context.Context, domainID string) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	tag, err := repository.DB(ctx, r.db).Exec(ctx, `DELETE FROM domain_categories WHERE id = $1::uuid`, domainID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrDomainCategoryNotFound
	}
	return nil
}

func (r *TrackerRepository) PurgeOldSessions(ctx context.Context, cutoff time.Time) (int64, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	tag, err := repository.DB(ctx, r.db).Exec(ctx, `
		DELETE FROM activity_sessions
		WHERE date < $1::date
	`, cutoff.UTC().Format("2006-01-02"))
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (r *TrackerRepository) getSessionForUpdate(ctx context.Context, tx pgx.Tx, userID string, sessionID string) (model.ActivitySession, error) {
	var session model.ActivitySession
	err := tx.QueryRow(ctx, `
		SELECT id::text, user_id::text, date, start_time, end_time, total_active_seconds, total_idle_seconds, is_active, created_at, updated_at
		FROM activity_sessions
		WHERE id = $1::uuid AND user_id = $2::uuid
		FOR UPDATE
	`, sessionID, userID).Scan(
		&session.ID,
		&session.UserID,
		&session.Date,
		&session.StartTime,
		&session.EndTime,
		&session.TotalActiveSeconds,
		&session.TotalIdleSeconds,
		&session.IsActive,
		&session.CreatedAt,
		&session.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.ActivitySession{}, ErrTrackerSessionNotFound
		}
		return model.ActivitySession{}, err
	}
	return session, nil
}

func (r *TrackerRepository) resolveDomainCategory(ctx context.Context, tx pgx.Tx, domain string) (string, bool, error) {
	var (
		category     string
		isProductive bool
	)
	err := tx.QueryRow(ctx, `
		SELECT category, is_productive
		FROM domain_categories
		WHERE $1 = domain_pattern OR $1 LIKE ('%.' || domain_pattern)
		ORDER BY LENGTH(domain_pattern) DESC
		LIMIT 1
	`, strings.ToLower(strings.TrimSpace(domain))).Scan(&category, &isProductive)
	if errors.Is(err, pgx.ErrNoRows) {
		return "uncategorized", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return category, isProductive, nil
}

func (r *TrackerRepository) upsertActivityEntry(ctx context.Context, tx pgx.Tx, params TrackerHeartbeatParams, category string, isProductive bool, deltaSeconds int, timestamp time.Time) (model.ActivityEntry, error) {
	var latest model.ActivityEntry
	err := tx.QueryRow(ctx, `
		SELECT id::text, session_id::text, user_id::text, url, domain, page_title, category, duration_seconds, started_at, ended_at, created_at
		FROM activity_entries
		WHERE session_id = $1::uuid
		ORDER BY ended_at DESC
		LIMIT 1
		FOR UPDATE
	`, params.SessionID).Scan(
		&latest.ID,
		&latest.SessionID,
		&latest.UserID,
		&latest.URL,
		&latest.Domain,
		&latest.PageTitle,
		&latest.Category,
		&latest.DurationSeconds,
		&latest.StartedAt,
		&latest.EndedAt,
		&latest.CreatedAt,
	)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return model.ActivityEntry{}, err
	}

	pageTitle := trimmedPointer(params.PageTitle)
	if err == nil && latest.URL == params.URL && latest.Domain == params.Domain && latest.Category == category {
		var updated model.ActivityEntry
		err = tx.QueryRow(ctx, `
			UPDATE activity_entries
			SET page_title = NULLIF($2, ''), duration_seconds = duration_seconds + $3, ended_at = $4
			WHERE id = $1::uuid
			RETURNING id::text, session_id::text, user_id::text, url, domain, page_title, category, duration_seconds, started_at, ended_at, created_at
		`, latest.ID, nullableTrackerText(pageTitle), deltaSeconds, timestamp).Scan(
			&updated.ID,
			&updated.SessionID,
			&updated.UserID,
			&updated.URL,
			&updated.Domain,
			&updated.PageTitle,
			&updated.Category,
			&updated.DurationSeconds,
			&updated.StartedAt,
			&updated.EndedAt,
			&updated.CreatedAt,
		)
		updated.IsProductive = isProductive
		return updated, err
	}

	var created model.ActivityEntry
	startedAt := timestamp.Add(-time.Duration(deltaSeconds) * time.Second)
	err = tx.QueryRow(ctx, `
		INSERT INTO activity_entries (session_id, user_id, url, domain, page_title, category, duration_seconds, started_at, ended_at)
		VALUES ($1::uuid, $2::uuid, $3, $4, NULLIF($5, ''), $6, $7, $8, $9)
		RETURNING id::text, session_id::text, user_id::text, url, domain, page_title, category, duration_seconds, started_at, ended_at, created_at
	`, params.SessionID, params.UserID, params.URL, strings.ToLower(strings.TrimSpace(params.Domain)), nullableTrackerText(pageTitle), category, deltaSeconds, startedAt, timestamp).Scan(
		&created.ID,
		&created.SessionID,
		&created.UserID,
		&created.URL,
		&created.Domain,
		&created.PageTitle,
		&created.Category,
		&created.DurationSeconds,
		&created.StartedAt,
		&created.EndedAt,
		&created.CreatedAt,
	)
	created.IsProductive = isProductive
	return created, err
}

func (r *TrackerRepository) updateSessionAfterHeartbeat(ctx context.Context, tx pgx.Tx, session model.ActivitySession, deltaSeconds int, isIdle bool, timestamp time.Time) (model.ActivitySession, error) {
	query := `
		UPDATE activity_sessions
		SET
			total_active_seconds = total_active_seconds + $3,
			total_idle_seconds = total_idle_seconds + $4,
			end_time = $5,
			updated_at = $5
		WHERE id = $1::uuid AND user_id = $2::uuid
		RETURNING id::text, user_id::text, date, start_time, end_time, total_active_seconds, total_idle_seconds, is_active, created_at, updated_at
	`
	activeDelta := 0
	idleDelta := 0
	if isIdle {
		idleDelta = deltaSeconds
	} else {
		activeDelta = deltaSeconds
	}

	var updated model.ActivitySession
	err := tx.QueryRow(ctx, query, session.ID, session.UserID, activeDelta, idleDelta, timestamp).Scan(
		&updated.ID,
		&updated.UserID,
		&updated.Date,
		&updated.StartTime,
		&updated.EndTime,
		&updated.TotalActiveSeconds,
		&updated.TotalIdleSeconds,
		&updated.IsActive,
		&updated.CreatedAt,
		&updated.UpdatedAt,
	)
	return updated, err
}

func (r *TrackerRepository) getProductiveSeconds(ctx context.Context, userID string, activityRange TrackerActivityRange) (int64, error) {
	var productiveSeconds int64
	err := repository.DB(ctx, r.db).QueryRow(ctx, `
		SELECT COALESCE(SUM(e.duration_seconds) FILTER (WHERE COALESCE(dc.is_productive, FALSE)), 0)::bigint
		FROM activity_entries e
		LEFT JOIN LATERAL (
			SELECT is_productive
			FROM domain_categories
			WHERE e.domain = domain_pattern OR e.domain LIKE ('%.' || domain_pattern)
			ORDER BY LENGTH(domain_pattern) DESC
			LIMIT 1
		) dc ON TRUE
		WHERE e.user_id = $1::uuid
		  AND e.started_at::date BETWEEN $2::date AND $3::date
	`, userID, activityRange.DateFrom, activityRange.DateTo).Scan(&productiveSeconds)
	return productiveSeconds, err
}

func (r *TrackerRepository) listCategoryBreakdown(ctx context.Context, userID string, activityRange TrackerActivityRange) ([]model.TrackerCategoryBreakdown, error) {
	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT
			e.category,
			COALESCE(SUM(e.duration_seconds), 0)::bigint AS duration_seconds,
			COALESCE(BOOL_OR(dc.is_productive), FALSE) AS is_productive
		FROM activity_entries e
		LEFT JOIN LATERAL (
			SELECT is_productive
			FROM domain_categories
			WHERE e.domain = domain_pattern OR e.domain LIKE ('%.' || domain_pattern)
			ORDER BY LENGTH(domain_pattern) DESC
			LIMIT 1
		) dc ON TRUE
		WHERE e.user_id = $1::uuid
		  AND e.started_at::date BETWEEN $2::date AND $3::date
		GROUP BY e.category
		ORDER BY duration_seconds DESC, e.category ASC
	`, userID, activityRange.DateFrom, activityRange.DateTo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.TrackerCategoryBreakdown, 0)
	for rows.Next() {
		var item model.TrackerCategoryBreakdown
		if err := rows.Scan(&item.Category, &item.DurationSeconds, &item.IsProductive); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *TrackerRepository) listHourlyBreakdown(ctx context.Context, userID string, activityRange TrackerActivityRange) ([]model.TrackerHourlyBreakdown, error) {
	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT EXTRACT(HOUR FROM started_at)::int AS hour_value, COALESCE(SUM(duration_seconds), 0)::bigint
		FROM activity_entries
		WHERE user_id = $1::uuid
		  AND started_at::date BETWEEN $2::date AND $3::date
		GROUP BY hour_value
		ORDER BY hour_value ASC
	`, userID, activityRange.DateFrom, activityRange.DateTo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	values := make(map[int]int64, 24)
	for rows.Next() {
		var hour int
		var duration int64
		if err := rows.Scan(&hour, &duration); err != nil {
			return nil, err
		}
		values[hour] = duration
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	items := make([]model.TrackerHourlyBreakdown, 0, 24)
	for hour := 0; hour < 24; hour++ {
		items = append(items, model.TrackerHourlyBreakdown{
			Hour:            hour,
			Label:           fmt.Sprintf("%02d:00", hour),
			DurationSeconds: values[hour],
		})
	}
	return items, nil
}

func (r *TrackerRepository) listTopDomains(ctx context.Context, userID string, activityRange TrackerActivityRange, limit int) ([]model.TrackerTopDomain, error) {
	// Query totalActive BEFORE opening the rows cursor — using a single
	// *pgxpool.Conn (from tenant middleware context), a second query while
	// rows are open causes "conn busy".
	var totalActive int64
	if err := repository.DB(ctx, r.db).QueryRow(ctx, `
		SELECT COALESCE(SUM(total_active_seconds), 0)::bigint
		FROM activity_sessions
		WHERE user_id = $1::uuid AND date BETWEEN $2::date AND $3::date
	`, userID, activityRange.DateFrom, activityRange.DateTo).Scan(&totalActive); err != nil {
		return nil, err
	}

	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT
			e.domain,
			COALESCE(MAX(dc.category), 'uncategorized') AS category,
			COALESCE(SUM(e.duration_seconds), 0)::bigint AS duration_seconds,
			COALESCE(BOOL_OR(dc.is_productive), FALSE) AS is_productive
		FROM activity_entries e
		LEFT JOIN LATERAL (
			SELECT category, is_productive
			FROM domain_categories
			WHERE e.domain = domain_pattern OR e.domain LIKE ('%.' || domain_pattern)
			ORDER BY LENGTH(domain_pattern) DESC
			LIMIT 1
		) dc ON TRUE
		WHERE e.user_id = $1::uuid
		  AND e.started_at::date BETWEEN $2::date AND $3::date
		GROUP BY e.domain
		ORDER BY duration_seconds DESC, e.domain ASC
		LIMIT $4
	`, userID, activityRange.DateFrom, activityRange.DateTo, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.TrackerTopDomain, 0)
	for rows.Next() {
		var item model.TrackerTopDomain
		if err := rows.Scan(&item.Domain, &item.Category, &item.DurationSeconds, &item.IsProductive); err != nil {
			return nil, err
		}
		if totalActive > 0 {
			item.Percentage = (float64(item.DurationSeconds) / float64(totalActive)) * 100
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *TrackerRepository) listDomainsByProductivity(ctx context.Context, date time.Time, productive bool) ([]model.TrackerTopDomain, error) {
	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT
			e.domain,
			COALESCE(MAX(dc.category), 'uncategorized') AS category,
			COALESCE(SUM(e.duration_seconds), 0)::bigint AS duration_seconds,
			COALESCE(BOOL_OR(dc.is_productive), FALSE) AS is_productive
		FROM activity_entries e
		LEFT JOIN LATERAL (
			SELECT category, is_productive
			FROM domain_categories
			WHERE e.domain = domain_pattern OR e.domain LIKE ('%.' || domain_pattern)
			ORDER BY LENGTH(domain_pattern) DESC
			LIMIT 1
		) dc ON TRUE
		WHERE e.started_at::date = $1::date
		  AND COALESCE(dc.is_productive, FALSE) = $2
		GROUP BY e.domain
		ORDER BY duration_seconds DESC, e.domain ASC
		LIMIT 5
	`, date, productive)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.TrackerTopDomain, 0)
	var total int64
	for rows.Next() {
		var item model.TrackerTopDomain
		if err := rows.Scan(&item.Domain, &item.Category, &item.DurationSeconds, &item.IsProductive); err != nil {
			return nil, err
		}
		total += item.DurationSeconds
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if total > 0 {
		for index := range items {
			items[index].Percentage = (float64(items[index].DurationSeconds) / float64(total)) * 100
		}
	}

	return items, nil
}

func optionalEntryUserFilter(userID *string, column string) string {
	if userID == nil || strings.TrimSpace(*userID) == "" {
		return ""
	}
	return fmt.Sprintf(" AND %s = $3::uuid", column)
}

func topProductiveMember(items []model.TrackerUserSummary) *string {
	var best *model.TrackerUserSummary
	for index := range items {
		if best == nil || items[index].ProductivityScore > best.ProductivityScore {
			best = &items[index]
		}
	}
	if best == nil {
		return nil
	}
	return &best.UserName
}

func leastProductiveMember(items []model.TrackerUserSummary) *string {
	var worst *model.TrackerUserSummary
	for index := range items {
		if worst == nil || items[index].ProductivityScore < worst.ProductivityScore {
			worst = &items[index]
		}
	}
	if worst == nil {
		return nil
	}
	return &worst.UserName
}

func trimmedPointer(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	return &trimmed
}

func nullableTrackerText(value *string) interface{} {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return ""
	}
	return trimmed
}

func nullTimePointer(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}

func nullStringPointer(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	result := strings.TrimSpace(value.String)
	return &result
}
