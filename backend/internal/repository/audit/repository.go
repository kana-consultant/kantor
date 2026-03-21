package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

type Entry struct {
	UserID     string
	Action     string
	Module     string
	Resource   string
	ResourceID string
	OldValue   interface{}
	NewValue   interface{}
	IPAddress  string
}

type ListParams struct {
	Module     string
	Action     string
	UserID     string
	Resource   string
	ResourceID string
	DateFrom   *time.Time
	DateTo     *time.Time
	Search     string
	Page       int
	PerPage    int
}

type LogRecord struct {
	ID            string          `json:"id"`
	UserID        *string         `json:"user_id,omitempty"`
	UserName      string          `json:"user_name"`
	UserEmail     string          `json:"user_email"`
	UserAvatarURL *string         `json:"user_avatar_url,omitempty"`
	Action        string          `json:"action"`
	Module        string          `json:"module"`
	Resource      string          `json:"resource"`
	ResourceID    string          `json:"resource_id"`
	OldValue      json.RawMessage `json:"old_value,omitempty"`
	NewValue      json.RawMessage `json:"new_value,omitempty"`
	IPAddress     string          `json:"ip_address,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}

type SummaryBucket struct {
	Key   string `json:"key"`
	Count int64  `json:"count"`
}

type UserActivityCount struct {
	UserID    *string `json:"user_id,omitempty"`
	UserName  string  `json:"user_name"`
	UserEmail string  `json:"user_email"`
	Count     int64   `json:"count"`
}

type Summary struct {
	TotalToday int64               `json:"total_today"`
	TotalWeek  int64               `json:"total_week"`
	ByModule   []SummaryBucket     `json:"by_module"`
	ByAction   []SummaryBucket     `json:"by_action"`
	TopUsers   []UserActivityCount `json:"top_users"`
}

type ActorOption struct {
	UserID        string  `json:"user_id"`
	UserName      string  `json:"user_name"`
	UserEmail     string  `json:"user_email"`
	UserAvatarURL *string `json:"user_avatar_url,omitempty"`
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Insert(ctx context.Context, entry Entry) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	oldJSON, err := marshalNullableJSON(entry.OldValue)
	if err != nil {
		return fmt.Errorf("marshal old_value: %w", err)
	}

	newJSON, err := marshalNullableJSON(entry.NewValue)
	if err != nil {
		return fmt.Errorf("marshal new_value: %w", err)
	}

	var ipAddr *net.IP
	if entry.IPAddress != "" {
		parsed := net.ParseIP(entry.IPAddress)
		if parsed != nil {
			ipAddr = &parsed
		}
	}

	_, err = r.db.Exec(ctx, `
		INSERT INTO audit_logs (user_id, action, module, resource, resource_id, old_value, new_value, ip_address)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, entry.UserID, entry.Action, entry.Module, entry.Resource, entry.ResourceID, oldJSON, newJSON, ipAddr)
	if err != nil {
		return fmt.Errorf("insert audit log: %w", err)
	}

	return nil
}

func (r *Repository) List(ctx context.Context, params ListParams) ([]LogRecord, int64, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	whereClause, args, nextArgIndex := buildWhereClause(params)

	var total int64
	if err := r.db.QueryRow(ctx, fmt.Sprintf(`
		SELECT COUNT(*)
		FROM audit_logs
		LEFT JOIN users ON users.id = audit_logs.user_id
		WHERE %s
	`, whereClause), args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count audit logs: %w", err)
	}

	page := params.Page
	if page <= 0 {
		page = 1
	}
	perPage := params.PerPage
	if perPage <= 0 {
		perPage = 20
	}

	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, perPage, (page-1)*perPage)

	rows, err := r.db.Query(ctx, fmt.Sprintf(`
		SELECT
			audit_logs.id::text,
			audit_logs.user_id::text,
			COALESCE(users.full_name, 'System'),
			COALESCE(users.email, ''),
			users.avatar_url,
			audit_logs.action,
			audit_logs.module,
			audit_logs.resource,
			audit_logs.resource_id,
			audit_logs.old_value,
			audit_logs.new_value,
			COALESCE(audit_logs.ip_address::text, ''),
			audit_logs.created_at
		FROM audit_logs
		LEFT JOIN users ON users.id = audit_logs.user_id
		WHERE %s
		ORDER BY audit_logs.created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, nextArgIndex, nextArgIndex+1), listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list audit logs: %w", err)
	}
	defer rows.Close()

	items := make([]LogRecord, 0)
	for rows.Next() {
		item, err := scanLogRecord(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}

	return items, total, rows.Err()
}

func (r *Repository) ListForExport(ctx context.Context, params ListParams) ([]LogRecord, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	whereClause, args, _ := buildWhereClause(params)
	rows, err := r.db.Query(ctx, fmt.Sprintf(`
		SELECT
			audit_logs.id::text,
			audit_logs.user_id::text,
			COALESCE(users.full_name, 'System'),
			COALESCE(users.email, ''),
			users.avatar_url,
			audit_logs.action,
			audit_logs.module,
			audit_logs.resource,
			audit_logs.resource_id,
			audit_logs.old_value,
			audit_logs.new_value,
			COALESCE(audit_logs.ip_address::text, ''),
			audit_logs.created_at
		FROM audit_logs
		LEFT JOIN users ON users.id = audit_logs.user_id
		WHERE %s
		ORDER BY audit_logs.created_at DESC
	`, whereClause), args...)
	if err != nil {
		return nil, fmt.Errorf("list audit logs for export: %w", err)
	}
	defer rows.Close()

	items := make([]LogRecord, 0)
	for rows.Next() {
		item, err := scanLogRecord(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, rows.Err()
}

func (r *Repository) Summary(ctx context.Context) (Summary, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var summary Summary
	if err := r.db.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE created_at >= date_trunc('day', NOW()))::bigint AS total_today,
			COUNT(*) FILTER (WHERE created_at >= date_trunc('week', NOW()))::bigint AS total_week
		FROM audit_logs
	`).Scan(&summary.TotalToday, &summary.TotalWeek); err != nil {
		return Summary{}, fmt.Errorf("load audit log totals: %w", err)
	}

	byModuleRows, err := r.db.Query(ctx, `
		SELECT module, COUNT(*)::bigint
		FROM audit_logs
		GROUP BY module
		ORDER BY COUNT(*) DESC, module ASC
	`)
	if err != nil {
		return Summary{}, fmt.Errorf("load audit log module summary: %w", err)
	}
	defer byModuleRows.Close()

	summary.ByModule = make([]SummaryBucket, 0)
	for byModuleRows.Next() {
		var bucket SummaryBucket
		if err := byModuleRows.Scan(&bucket.Key, &bucket.Count); err != nil {
			return Summary{}, fmt.Errorf("scan audit log module summary: %w", err)
		}
		summary.ByModule = append(summary.ByModule, bucket)
	}
	if err := byModuleRows.Err(); err != nil {
		return Summary{}, fmt.Errorf("iterate audit log module summary: %w", err)
	}

	byActionRows, err := r.db.Query(ctx, `
		SELECT action, COUNT(*)::bigint
		FROM audit_logs
		GROUP BY action
		ORDER BY COUNT(*) DESC, action ASC
	`)
	if err != nil {
		return Summary{}, fmt.Errorf("load audit log action summary: %w", err)
	}
	defer byActionRows.Close()

	summary.ByAction = make([]SummaryBucket, 0)
	for byActionRows.Next() {
		var bucket SummaryBucket
		if err := byActionRows.Scan(&bucket.Key, &bucket.Count); err != nil {
			return Summary{}, fmt.Errorf("scan audit log action summary: %w", err)
		}
		summary.ByAction = append(summary.ByAction, bucket)
	}
	if err := byActionRows.Err(); err != nil {
		return Summary{}, fmt.Errorf("iterate audit log action summary: %w", err)
	}

	topUserRows, err := r.db.Query(ctx, `
		SELECT
			audit_logs.user_id::text,
			COALESCE(users.full_name, 'System'),
			COALESCE(users.email, ''),
			COUNT(*)::bigint AS total_logs
		FROM audit_logs
		LEFT JOIN users ON users.id = audit_logs.user_id
		GROUP BY audit_logs.user_id, users.full_name, users.email
		ORDER BY total_logs DESC, COALESCE(users.full_name, 'System') ASC
		LIMIT 5
	`)
	if err != nil {
		return Summary{}, fmt.Errorf("load audit log top users: %w", err)
	}
	defer topUserRows.Close()

	summary.TopUsers = make([]UserActivityCount, 0)
	for topUserRows.Next() {
		var userID sql.NullString
		var item UserActivityCount
		if err := topUserRows.Scan(&userID, &item.UserName, &item.UserEmail, &item.Count); err != nil {
			return Summary{}, fmt.Errorf("scan audit log top user: %w", err)
		}
		if userID.Valid {
			value := userID.String
			item.UserID = &value
		}
		summary.TopUsers = append(summary.TopUsers, item)
	}
	if err := topUserRows.Err(); err != nil {
		return Summary{}, fmt.Errorf("iterate audit log top users: %w", err)
	}

	return summary, nil
}

func (r *Repository) ListActors(ctx context.Context, search string) ([]ActorOption, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	args := make([]any, 0)
	where := ""
	if trimmed := strings.TrimSpace(search); trimmed != "" {
		args = append(args, "%"+trimmed+"%")
		where = "AND (users.full_name ILIKE $1 OR users.email ILIKE $1)"
	}

	rows, err := r.db.Query(ctx, fmt.Sprintf(`
		SELECT DISTINCT users.id::text, users.full_name, users.email, users.avatar_url
		FROM audit_logs
		INNER JOIN users ON users.id = audit_logs.user_id
		WHERE audit_logs.user_id IS NOT NULL %s
		ORDER BY users.full_name ASC, users.email ASC
	`, where), args...)
	if err != nil {
		return nil, fmt.Errorf("list audit log actors: %w", err)
	}
	defer rows.Close()

	items := make([]ActorOption, 0)
	for rows.Next() {
		var item ActorOption
		if err := rows.Scan(&item.UserID, &item.UserName, &item.UserEmail, &item.UserAvatarURL); err != nil {
			return nil, fmt.Errorf("scan audit log actor: %w", err)
		}
		items = append(items, item)
	}

	return items, rows.Err()
}

func buildWhereClause(params ListParams) (string, []any, int) {
	filters := []string{"1=1"}
	args := make([]any, 0)
	argIndex := 1

	if module := strings.TrimSpace(params.Module); module != "" {
		filters = append(filters, fmt.Sprintf("audit_logs.module = $%d", argIndex))
		args = append(args, module)
		argIndex++
	}
	if action := strings.TrimSpace(params.Action); action != "" {
		filters = append(filters, fmt.Sprintf("audit_logs.action = $%d", argIndex))
		args = append(args, action)
		argIndex++
	}
	if userID := strings.TrimSpace(params.UserID); userID != "" {
		filters = append(filters, fmt.Sprintf("audit_logs.user_id = $%d::uuid", argIndex))
		args = append(args, userID)
		argIndex++
	}
	if resource := strings.TrimSpace(params.Resource); resource != "" {
		filters = append(filters, fmt.Sprintf("audit_logs.resource = $%d", argIndex))
		args = append(args, resource)
		argIndex++
	}
	if resourceID := strings.TrimSpace(params.ResourceID); resourceID != "" {
		filters = append(filters, fmt.Sprintf("audit_logs.resource_id = $%d", argIndex))
		args = append(args, resourceID)
		argIndex++
	}
	if params.DateFrom != nil {
		filters = append(filters, fmt.Sprintf("audit_logs.created_at >= $%d", argIndex))
		args = append(args, params.DateFrom.UTC())
		argIndex++
	}
	if params.DateTo != nil {
		filters = append(filters, fmt.Sprintf("audit_logs.created_at < $%d", argIndex))
		args = append(args, params.DateTo.UTC())
		argIndex++
	}
	if search := strings.TrimSpace(params.Search); search != "" {
		filters = append(filters, fmt.Sprintf(`(
			audit_logs.action ILIKE $%d OR
			audit_logs.resource ILIKE $%d OR
			audit_logs.resource_id ILIKE $%d OR
			audit_logs.old_value::text ILIKE $%d OR
			audit_logs.new_value::text ILIKE $%d OR
			COALESCE(users.full_name, '') ILIKE $%d OR
			COALESCE(users.email, '') ILIKE $%d
		)`, argIndex, argIndex, argIndex, argIndex, argIndex, argIndex, argIndex))
		args = append(args, "%"+search+"%")
		argIndex++
	}

	return strings.Join(filters, " AND "), args, argIndex
}

type logScanner interface {
	Scan(dest ...any) error
}

func scanLogRecord(scanner logScanner) (LogRecord, error) {
	var item LogRecord
	var userID sql.NullString
	var avatarURL sql.NullString
	var oldValue []byte
	var newValue []byte

	if err := scanner.Scan(
		&item.ID,
		&userID,
		&item.UserName,
		&item.UserEmail,
		&avatarURL,
		&item.Action,
		&item.Module,
		&item.Resource,
		&item.ResourceID,
		&oldValue,
		&newValue,
		&item.IPAddress,
		&item.CreatedAt,
	); err != nil {
		return LogRecord{}, fmt.Errorf("scan audit log: %w", err)
	}

	if userID.Valid {
		value := userID.String
		item.UserID = &value
	}
	if avatarURL.Valid {
		value := avatarURL.String
		item.UserAvatarURL = &value
	}
	if len(oldValue) > 0 {
		item.OldValue = json.RawMessage(oldValue)
	}
	if len(newValue) > 0 {
		item.NewValue = json.RawMessage(newValue)
	}

	return item, nil
}

func marshalNullableJSON(v interface{}) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	return json.Marshal(v)
}
