package auth

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/kana-consultant/kantor/backend/internal/model"
	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

const reimbursementReminderSettingDescription = "Konfigurasi reminder reimbursement tenant"

func defaultReimbursementReminderSetting() model.ReimbursementReminderSetting {
	return model.ReimbursementReminderSetting{
		Enabled: false,
		Review: model.ReimbursementReminderRule{
			Enabled: true,
			Cron:    "0 9 * * 1-5",
			Channels: model.ReminderChannels{
				InApp:    true,
				Email:    false,
				WhatsApp: false,
			},
		},
		Payment: model.ReimbursementReminderRule{
			Enabled: true,
			Cron:    "0 10 * * 1-5",
			Channels: model.ReminderChannels{
				InApp:    true,
				Email:    false,
				WhatsApp: false,
			},
		},
	}
}

func normalizeReminderCron(value string, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

func normalizeReimbursementReminderSetting(setting model.ReimbursementReminderSetting) model.ReimbursementReminderSetting {
	normalized := setting
	defaults := defaultReimbursementReminderSetting()

	normalized.Review.Cron = normalizeReminderCron(normalized.Review.Cron, defaults.Review.Cron)
	normalized.Payment.Cron = normalizeReminderCron(normalized.Payment.Cron, defaults.Payment.Cron)

	return normalized
}

func (r *Repository) GetReimbursementReminderSetting(ctx context.Context) (model.ReimbursementReminderSetting, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var raw []byte
	err := repository.DB(ctx, r.db).QueryRow(ctx, `SELECT value FROM system_settings WHERE key = 'reimbursement_reminder'`).Scan(&raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return defaultReimbursementReminderSetting(), nil
		}
		return model.ReimbursementReminderSetting{}, err
	}

	setting := defaultReimbursementReminderSetting()
	if err := json.Unmarshal(raw, &setting); err != nil {
		return model.ReimbursementReminderSetting{}, err
	}

	return normalizeReimbursementReminderSetting(setting), nil
}

func (r *Repository) UpdateReimbursementReminder(ctx context.Context, updatedBy string, setting model.ReimbursementReminderSetting) error {
	normalized := normalizeReimbursementReminderSetting(setting)
	raw, err := json.Marshal(normalized)
	if err != nil {
		return err
	}

	_, err = repository.DB(ctx, r.db).Exec(ctx, `
		INSERT INTO system_settings (key, value, description, updated_by, updated_at)
		VALUES ('reimbursement_reminder', $1::jsonb, $2, NULLIF($3, '')::uuid, NOW())
		ON CONFLICT (tenant_id, key) DO UPDATE
		SET value = EXCLUDED.value,
		    description = EXCLUDED.description,
		    updated_by = EXCLUDED.updated_by,
		    updated_at = NOW()
	`, string(raw), reimbursementReminderSettingDescription, updatedBy)
	return err
}

func (r *Repository) ListUserReminderRecipients(ctx context.Context, userIDs []string) ([]model.ReimbursementReminderRecipient, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	trimmedIDs := make([]string, 0, len(userIDs))
	for _, userID := range userIDs {
		trimmed := strings.TrimSpace(userID)
		if trimmed == "" {
			continue
		}
		trimmedIDs = append(trimmedIDs, trimmed)
	}
	if len(trimmedIDs) == 0 {
		return []model.ReimbursementReminderRecipient{}, nil
	}

	args := make([]any, 0, len(trimmedIDs))
	placeholders := make([]string, 0, len(trimmedIDs))
	for index, userID := range trimmedIDs {
		args = append(args, userID)
		placeholders = append(placeholders, "$"+strconv.Itoa(index+1)+"::uuid")
	}

	rows, err := repository.DB(ctx, r.db).Query(ctx, `
		SELECT id::text, full_name, email, phone
		FROM users
		WHERE is_active = TRUE AND id IN (`+strings.Join(placeholders, ", ")+`)
		ORDER BY full_name ASC, id ASC
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.ReimbursementReminderRecipient, 0, len(trimmedIDs))
	for rows.Next() {
		var item model.ReimbursementReminderRecipient
		if err := rows.Scan(&item.UserID, &item.UserName, &item.UserEmail, &item.Phone); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	return items, rows.Err()
}
