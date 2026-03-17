package notifications

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kana-consultant/kantor/backend/internal/model"
)

var ErrNotificationNotFound = errors.New("notification not found")

type Repository struct {
	db *pgxpool.Pool
}

type ListParams struct {
	UserID string
	Read   *bool
	Limit  int
	Offset int
}

type CreateParams struct {
	UserID        string
	Type          string
	Title         string
	Message       string
	ReferenceType *string
	ReferenceID   *string
}

func New(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, params CreateParams) (model.Notification, error) {
	query := `
		INSERT INTO notifications (user_id, type, title, message, reference_type, reference_id)
		VALUES ($1::uuid, $2, $3, $4, NULLIF($5, ''), $6::uuid)
		RETURNING id::text, user_id::text, type, title, message, is_read, reference_type, reference_id::text, created_at
	`

	var item model.Notification
	err := r.db.QueryRow(
		ctx,
		query,
		params.UserID,
		strings.TrimSpace(params.Type),
		strings.TrimSpace(params.Title),
		strings.TrimSpace(params.Message),
		nullableString(params.ReferenceType),
		nullableUUID(params.ReferenceID),
	).Scan(
		&item.ID,
		&item.UserID,
		&item.Type,
		&item.Title,
		&item.Message,
		&item.IsRead,
		&item.ReferenceType,
		&item.ReferenceID,
		&item.CreatedAt,
	)
	return item, err
}

func (r *Repository) CreateMany(ctx context.Context, params []CreateParams) error {
	if len(params) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	query := `
		INSERT INTO notifications (user_id, type, title, message, reference_type, reference_id)
		VALUES ($1::uuid, $2, $3, $4, NULLIF($5, ''), $6::uuid)
	`

	for _, item := range params {
		batch.Queue(
			query,
			item.UserID,
			strings.TrimSpace(item.Type),
			strings.TrimSpace(item.Title),
			strings.TrimSpace(item.Message),
			nullableString(item.ReferenceType),
			nullableUUID(item.ReferenceID),
		)
	}

	results := r.db.SendBatch(ctx, batch)
	defer results.Close()

	for range params {
		if _, err := results.Exec(); err != nil {
			return err
		}
	}

	return nil
}

func (r *Repository) List(ctx context.Context, params ListParams) ([]model.Notification, int64, error) {
	filters := []string{"user_id = $1::uuid"}
	args := []interface{}{params.UserID}

	if params.Read != nil {
		filters = append(filters, "is_read = $2")
		args = append(args, *params.Read)
	}

	whereClause := strings.Join(filters, " AND ")

	var total int64
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM notifications WHERE `+whereClause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limitIndex := len(args) + 1
	offsetIndex := len(args) + 2
	query := `
		SELECT id::text, user_id::text, type, title, message, is_read, reference_type, reference_id::text, created_at
		FROM notifications
		WHERE ` + whereClause + `
		ORDER BY created_at DESC
		LIMIT $` + strconv.Itoa(limitIndex) + ` OFFSET $` + strconv.Itoa(offsetIndex)
	args = append(args, params.Limit, params.Offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := make([]model.Notification, 0)
	for rows.Next() {
		var item model.Notification
		if err := rows.Scan(
			&item.ID,
			&item.UserID,
			&item.Type,
			&item.Title,
			&item.Message,
			&item.IsRead,
			&item.ReferenceType,
			&item.ReferenceID,
			&item.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

func (r *Repository) MarkRead(ctx context.Context, notificationID string, userID string) error {
	tag, err := r.db.Exec(
		ctx,
		`UPDATE notifications SET is_read = TRUE WHERE id = $1::uuid AND user_id = $2::uuid`,
		notificationID,
		userID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotificationNotFound
	}
	return nil
}

func (r *Repository) MarkAllRead(ctx context.Context, userID string) error {
	_, err := r.db.Exec(ctx, `UPDATE notifications SET is_read = TRUE WHERE user_id = $1::uuid AND is_read = FALSE`, userID)
	return err
}

func nullableString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func nullableUUID(value *string) interface{} {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil
	}
	return strings.TrimSpace(*value)
}
