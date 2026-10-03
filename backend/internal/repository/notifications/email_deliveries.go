package notifications

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/kana-consultant/kantor/backend/internal/model"
	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

var (
	ErrEmailDeliveryNotFound = errors.New("email delivery not found")
	// ErrEmailDeliveryInFlight is returned when a queued/sending row already
	// exists for the same document (uq_email_deliveries_in_flight).
	ErrEmailDeliveryInFlight = errors.New("email delivery already in progress for this document")
)

// EmailDeliveriesRepository persists email_deliveries rows. It is shared by
// the admin test email and the payslip / contract senders.
type EmailDeliveriesRepository struct {
	db repository.DBTX
}

type CreateEmailDeliveryParams struct {
	Kind             string
	ReferenceType    *string
	ReferenceID      *string
	Recipient        string
	RecipientSource  string
	Cc               []string
	Subject          string
	AttachmentNames  []string
	AttachmentSHA256 []string
	// Status is queued (batch, sent later) or sending (sent right away,
	// which also counts the first attempt).
	Status      string
	BatchID     *string
	RequestedBy *string
}

func NewEmailDeliveriesRepository(db repository.DBTX) *EmailDeliveriesRepository {
	return &EmailDeliveriesRepository{db: db}
}

const emailDeliveryColumns = `
	id::text, kind, reference_type, reference_id::text, recipient, recipient_source,
	cc, subject, attachment_names, attachment_sha256, status, error, attempts,
	batch_id::text, requested_by::text, created_at, updated_at, sent_at
`

func (r *EmailDeliveriesRepository) Create(ctx context.Context, params CreateEmailDeliveryParams) (model.EmailDelivery, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	status := strings.TrimSpace(params.Status)
	if status == "" {
		status = model.EmailDeliveryStatusQueued
	}
	attempts := 0
	if status == model.EmailDeliveryStatusSending {
		attempts = 1
	}

	row := repository.DB(ctx, r.db).QueryRow(ctx, `
		INSERT INTO email_deliveries (
			kind, reference_type, reference_id, recipient, recipient_source, cc, subject,
			attachment_names, attachment_sha256, status, attempts, batch_id, requested_by
		)
		VALUES ($1, NULLIF($2, ''), $3::uuid, $4, $5, $6, $7, $8, $9, $10, $11, $12::uuid, $13::uuid)
		RETURNING `+emailDeliveryColumns,
		params.Kind,
		nullableString(params.ReferenceType),
		nullableUUID(params.ReferenceID),
		strings.TrimSpace(params.Recipient),
		params.RecipientSource,
		nonNilStrings(params.Cc),
		strings.TrimSpace(params.Subject),
		nonNilStrings(params.AttachmentNames),
		nonNilStrings(params.AttachmentSHA256),
		status,
		attempts,
		nullableUUID(params.BatchID),
		nullableUUID(params.RequestedBy),
	)
	item, err := scanEmailDelivery(row)
	if err != nil {
		if isInFlightViolation(err) {
			return model.EmailDelivery{}, ErrEmailDeliveryInFlight
		}
		return model.EmailDelivery{}, err
	}
	return item, nil
}

// MarkSending moves a queued row (created by Enqueue) to sending and counts
// the attempt. Failed rows are never reopened: a retry is a new row, so each
// attempt keeps the recipient and digests it was actually sent with.
func (r *EmailDeliveriesRepository) MarkSending(ctx context.Context, id string) (model.EmailDelivery, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	row := repository.DB(ctx, r.db).QueryRow(ctx, `
		UPDATE email_deliveries
		SET status = 'sending', attempts = attempts + 1, error = NULL, updated_at = NOW()
		WHERE id = $1::uuid AND status = 'queued'
		RETURNING `+emailDeliveryColumns, id)
	item, err := scanEmailDelivery(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.EmailDelivery{}, ErrEmailDeliveryNotFound
	}
	if err != nil && isInFlightViolation(err) {
		return model.EmailDelivery{}, ErrEmailDeliveryInFlight
	}
	return item, err
}

func (r *EmailDeliveriesRepository) MarkSent(ctx context.Context, id string, sentAt time.Time) (model.EmailDelivery, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	row := repository.DB(ctx, r.db).QueryRow(ctx, `
		UPDATE email_deliveries
		SET status = 'sent', error = NULL, sent_at = $2, updated_at = NOW()
		WHERE id = $1::uuid
		RETURNING `+emailDeliveryColumns, id, sentAt.UTC())
	item, err := scanEmailDelivery(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.EmailDelivery{}, ErrEmailDeliveryNotFound
	}
	return item, err
}

// MarkFailed stores the fixed category message (never raw SMTP text).
func (r *EmailDeliveriesRepository) MarkFailed(ctx context.Context, id string, message string) (model.EmailDelivery, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	row := repository.DB(ctx, r.db).QueryRow(ctx, `
		UPDATE email_deliveries
		SET status = 'failed', error = NULLIF($2, ''), updated_at = NOW()
		WHERE id = $1::uuid
		RETURNING `+emailDeliveryColumns, id, strings.TrimSpace(message))
	item, err := scanEmailDelivery(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.EmailDelivery{}, ErrEmailDeliveryNotFound
	}
	return item, err
}

// FailStaleInFlight marks queued/sending rows of one document that have not
// moved for longer than olderThan as failed, so a crashed send cannot block
// the double-send guard forever.
func (r *EmailDeliveriesRepository) FailStaleInFlight(ctx context.Context, referenceType string, referenceID string, olderThan time.Duration, message string) (int64, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	tag, err := repository.DB(ctx, r.db).Exec(ctx, `
		UPDATE email_deliveries
		SET status = 'failed', error = $4, updated_at = NOW()
		WHERE reference_type = $1
		  AND reference_id = $2::uuid
		  AND status IN ('queued', 'sending')
		  AND updated_at < NOW() - make_interval(secs => $3)
	`, referenceType, referenceID, olderThan.Seconds(), message)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// FailStaleDocuments fails every queued/sending row of a document (any
// reference) that has not moved for longer than olderThan: the sweep for
// batches cut off by a restart, whose rows nothing would otherwise touch.
func (r *EmailDeliveriesRepository) FailStaleDocuments(ctx context.Context, olderThan time.Duration, message string) (int64, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	tag, err := repository.DB(ctx, r.db).Exec(ctx, `
		UPDATE email_deliveries
		SET status = 'failed', error = $2, updated_at = NOW()
		WHERE reference_id IS NOT NULL
		  AND status IN ('queued', 'sending')
		  AND updated_at < NOW() - make_interval(secs => $1)
	`, olderThan.Seconds(), message)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// FailStaleUnreferenced does the same for rows of one kind that belong to no
// document (the admin test email), which the per-document sweep never sees.
func (r *EmailDeliveriesRepository) FailStaleUnreferenced(ctx context.Context, kind string, olderThan time.Duration, message string) (int64, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	tag, err := repository.DB(ctx, r.db).Exec(ctx, `
		UPDATE email_deliveries
		SET status = 'failed', error = $3, updated_at = NOW()
		WHERE kind = $1
		  AND reference_id IS NULL
		  AND status IN ('queued', 'sending')
		  AND updated_at < NOW() - make_interval(secs => $2)
	`, kind, olderThan.Seconds(), message)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (r *EmailDeliveriesRepository) GetByID(ctx context.Context, id string) (model.EmailDelivery, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	row := repository.DB(ctx, r.db).QueryRow(ctx, `SELECT `+emailDeliveryColumns+` FROM email_deliveries WHERE id = $1::uuid`, id)
	item, err := scanEmailDelivery(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.EmailDelivery{}, ErrEmailDeliveryNotFound
	}
	return item, err
}

// ListByReference returns every attempt for one document, newest first.
func (r *EmailDeliveriesRepository) ListByReference(ctx context.Context, referenceType string, referenceID string) ([]model.EmailDelivery, error) {
	return r.list(ctx, `WHERE reference_type = $1 AND reference_id = $2::uuid ORDER BY created_at DESC`, referenceType, referenceID)
}

// ListByBatch returns every row of one batch send, oldest first.
func (r *EmailDeliveriesRepository) ListByBatch(ctx context.Context, batchID string) ([]model.EmailDelivery, error) {
	return r.list(ctx, `WHERE batch_id = $1::uuid ORDER BY created_at ASC`, batchID)
}

// HasInFlight reports whether a queued or sending row exists for a document.
func (r *EmailDeliveriesRepository) HasInFlight(ctx context.Context, referenceType string, referenceID string) (bool, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var exists bool
	err := repository.DB(ctx, r.db).QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM email_deliveries
			WHERE reference_type = $1 AND reference_id = $2::uuid AND status IN ('queued', 'sending')
		)
	`, referenceType, referenceID).Scan(&exists)
	return exists, err
}

// LatestSent returns the most recent successful delivery for a document, or
// ErrEmailDeliveryNotFound.
func (r *EmailDeliveriesRepository) LatestSent(ctx context.Context, referenceType string, referenceID string) (model.EmailDelivery, error) {
	items, err := r.list(ctx, `
		WHERE reference_type = $1 AND reference_id = $2::uuid AND status = 'sent'
		ORDER BY sent_at DESC NULLS LAST, created_at DESC
		LIMIT 1`, referenceType, referenceID)
	if err != nil {
		return model.EmailDelivery{}, err
	}
	if len(items) == 0 {
		return model.EmailDelivery{}, ErrEmailDeliveryNotFound
	}
	return items[0], nil
}

func (r *EmailDeliveriesRepository) list(ctx context.Context, clause string, args ...any) ([]model.EmailDelivery, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	rows, err := repository.DB(ctx, r.db).Query(ctx, `SELECT `+emailDeliveryColumns+` FROM email_deliveries `+clause, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.EmailDelivery, 0)
	for rows.Next() {
		item, err := scanEmailDelivery(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanEmailDelivery(row pgx.Row) (model.EmailDelivery, error) {
	var item model.EmailDelivery
	err := row.Scan(
		&item.ID,
		&item.Kind,
		&item.ReferenceType,
		&item.ReferenceID,
		&item.Recipient,
		&item.RecipientSource,
		&item.Cc,
		&item.Subject,
		&item.AttachmentNames,
		&item.AttachmentSHA256,
		&item.Status,
		&item.Error,
		&item.Attempts,
		&item.BatchID,
		&item.RequestedBy,
		&item.CreatedAt,
		&item.UpdatedAt,
		&item.SentAt,
	)
	if err != nil {
		return model.EmailDelivery{}, err
	}
	item.Cc = nonNilStrings(item.Cc)
	item.AttachmentNames = nonNilStrings(item.AttachmentNames)
	item.AttachmentSHA256 = nonNilStrings(item.AttachmentSHA256)
	return item, nil
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func isInFlightViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_email_deliveries_in_flight"
}
