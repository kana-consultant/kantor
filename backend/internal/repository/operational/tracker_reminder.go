package operational

import (
	"context"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/model"
	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

type TrackerReminderRepository struct {
	db repository.DBTX
}

func NewTrackerReminderRepository(db repository.DBTX) *TrackerReminderRepository {
	return &TrackerReminderRepository{db: db}
}

type UpdateTrackerReminderConfigParams struct {
	Enabled               bool
	StartHour             int
	EndHour               int
	WeekdaysOnly          bool
	Timezone              string
	HeartbeatStaleMinutes int
	NotifyInApp           bool
	NotifyWhatsapp        bool
}

func (r *TrackerReminderRepository) EnsureRow(ctx context.Context) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	_, err := repository.DB(ctx, r.db).Exec(ctx, `
		INSERT INTO tracker_reminder_configs (tenant_id)
		VALUES (current_setting('app.current_tenant')::uuid)
		ON CONFLICT (tenant_id) DO NOTHING
	`)
	return err
}

func (r *TrackerReminderRepository) Get(ctx context.Context) (model.TrackerReminderConfig, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var c model.TrackerReminderConfig
	err := repository.DB(ctx, r.db).QueryRow(ctx, `
		SELECT tenant_id::text, enabled, start_hour, end_hour, weekdays_only, timezone,
		       heartbeat_stale_minutes, notify_in_app, notify_whatsapp, created_at, updated_at
		FROM tracker_reminder_configs
		LIMIT 1
	`).Scan(
		&c.TenantID,
		&c.Enabled,
		&c.StartHour,
		&c.EndHour,
		&c.WeekdaysOnly,
		&c.Timezone,
		&c.HeartbeatStaleMinutes,
		&c.NotifyInApp,
		&c.NotifyWhatsapp,
		&c.CreatedAt,
		&c.UpdatedAt,
	)
	return c, err
}

func (r *TrackerReminderRepository) Update(ctx context.Context, p UpdateTrackerReminderConfigParams) (model.TrackerReminderConfig, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var c model.TrackerReminderConfig
	err := repository.DB(ctx, r.db).QueryRow(ctx, `
		UPDATE tracker_reminder_configs
		SET enabled = $1,
		    start_hour = $2,
		    end_hour = $3,
		    weekdays_only = $4,
		    timezone = $5,
		    heartbeat_stale_minutes = $6,
		    notify_in_app = $7,
		    notify_whatsapp = $8,
		    updated_at = NOW()
		RETURNING tenant_id::text, enabled, start_hour, end_hour, weekdays_only, timezone,
		          heartbeat_stale_minutes, notify_in_app, notify_whatsapp, created_at, updated_at
	`, p.Enabled, p.StartHour, p.EndHour, p.WeekdaysOnly, p.Timezone, p.HeartbeatStaleMinutes, p.NotifyInApp, p.NotifyWhatsapp).Scan(
		&c.TenantID,
		&c.Enabled,
		&c.StartHour,
		&c.EndHour,
		&c.WeekdaysOnly,
		&c.Timezone,
		&c.HeartbeatStaleMinutes,
		&c.NotifyInApp,
		&c.NotifyWhatsapp,
		&c.CreatedAt,
		&c.UpdatedAt,
	)
	return c, err
}

// ListCandidates returns active users with an employee record whose most recent
// heartbeat is older than staleCutoff (or who have never sent a heartbeat).
func (r *TrackerReminderRepository) ListCandidates(ctx context.Context, staleCutoff time.Time) ([]model.TrackerReminderCandidate, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT u.id::text, u.full_name, u.phone
		FROM users u
		INNER JOIN employees e ON e.user_id = u.id
		WHERE u.is_active = TRUE
		  AND (
		      SELECT COALESCE(MAX(ae.created_at), 'epoch'::timestamptz)
		      FROM activity_entries ae
		      WHERE ae.user_id = u.id
		  ) < $1
	`, staleCutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]model.TrackerReminderCandidate, 0)
	for rows.Next() {
		var c model.TrackerReminderCandidate
		if err := rows.Scan(&c.UserID, &c.FullName, &c.Phone); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// HasRecentReminder returns true if a tracker_reminder notification exists for
// userID created on or after "since". Used to dedupe dispatches across retries.
func (r *TrackerReminderRepository) HasRecentReminder(ctx context.Context, userID string, since time.Time) (bool, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var exists bool
	err := repository.DB(ctx, r.db).QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM notifications
			WHERE user_id = $1::uuid
			  AND type = 'tracker_reminder'
			  AND created_at >= $2
		)
	`, userID, since).Scan(&exists)
	return exists, err
}

// GetUserPhone returns the phone number stored on the users row for the given userID.
func (r *TrackerReminderRepository) GetUserPhone(ctx context.Context, userID string) (*string, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var phone *string
	err := repository.DB(ctx, r.db).QueryRow(ctx, `
		SELECT phone FROM users WHERE id = $1::uuid
	`, userID).Scan(&phone)
	return phone, err
}
