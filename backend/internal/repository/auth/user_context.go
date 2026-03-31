package auth

import (
	"context"
	"database/sql"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/model"
	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

const userSelectColumns = `
	id::text,
	email,
	password_hash,
	full_name,
	avatar_url,
	department,
	skills,
	is_active,
	is_super_admin,
	failed_login_attempts,
	locked_until,
	browser_timezone,
	browser_timezone_offset_minutes,
	browser_locale,
	tracker_extension_version,
	tracker_extension_reported_at,
	created_at,
	updated_at
`

type userScanner interface {
	Scan(dest ...any) error
}

type UpdateUserClientContextParams struct {
	Timezone                *string
	TimezoneOffsetMinutes   *int
	Locale                  *string
	TrackerExtensionVersion *string
	ReportedAt              *time.Time
}

func scanUser(scanner userScanner, user *model.User) error {
	var lockedUntil sql.NullTime
	var browserTimezone sql.NullString
	var browserTimezoneOffset sql.NullInt32
	var browserLocale sql.NullString
	var trackerExtensionVersion sql.NullString
	var trackerExtensionReportedAt sql.NullTime

	if err := scanner.Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.FullName,
		&user.AvatarURL,
		&user.Department,
		&user.Skills,
		&user.IsActive,
		&user.IsSuperAdmin,
		&user.FailedLoginAttempts,
		&lockedUntil,
		&browserTimezone,
		&browserTimezoneOffset,
		&browserLocale,
		&trackerExtensionVersion,
		&trackerExtensionReportedAt,
		&user.CreatedAt,
		&user.UpdatedAt,
	); err != nil {
		return err
	}

	user.LockedUntil = authNullTimePointer(lockedUntil)
	user.BrowserTimezone = authNullStringPointer(browserTimezone)
	if browserTimezoneOffset.Valid {
		value := int(browserTimezoneOffset.Int32)
		user.BrowserTimezoneOffsetMinutes = &value
	} else {
		user.BrowserTimezoneOffsetMinutes = nil
	}
	user.BrowserLocale = authNullStringPointer(browserLocale)
	user.TrackerExtensionVersion = authNullStringPointer(trackerExtensionVersion)
	user.TrackerExtensionReportedAt = authNullTimePointer(trackerExtensionReportedAt)

	return nil
}

func (r *Repository) UpdateUserClientContext(ctx context.Context, userID string, params UpdateUserClientContextParams) error {
	_, err := repository.DB(ctx, r.db).Exec(ctx, `
		UPDATE users
		SET
			browser_timezone = COALESCE(NULLIF($2, ''), browser_timezone),
			browser_timezone_offset_minutes = COALESCE($3, browser_timezone_offset_minutes),
			browser_locale = COALESCE(NULLIF($4, ''), browser_locale),
			tracker_extension_version = COALESCE(NULLIF($5, ''), tracker_extension_version),
			tracker_extension_reported_at = CASE
				WHEN NULLIF($5, '') IS NOT NULL THEN COALESCE($6, NOW())
				ELSE tracker_extension_reported_at
			END
		WHERE id = $1::uuid
	`,
		userID,
		nullableText(params.Timezone),
		params.TimezoneOffsetMinutes,
		nullableText(params.Locale),
		nullableText(params.TrackerExtensionVersion),
		params.ReportedAt,
	)
	return err
}

func authNullTimePointer(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	v := value.Time
	return &v
}

func authNullStringPointer(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	v := value.String
	return &v
}
