package auth

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

const mailDeliverySettingDescription = "Konfigurasi pengiriman email tenant"

func defaultMailDeliverySettingRecord() MailDeliverySettingRecord {
	return MailDeliverySettingRecord{
		Enabled:                    false,
		Provider:                   "resend",
		SenderName:                 "",
		SenderEmail:                "",
		ReplyToEmail:               nil,
		APIKeyEncrypted:            "",
		PasswordResetEnabled:       false,
		PasswordResetExpiryMinutes: 30,
		NotificationEnabled:        false,
	}
}

func normalizeMailDeliverySettingRecord(setting MailDeliverySettingRecord) MailDeliverySettingRecord {
	normalized := setting
	normalized.Provider = strings.ToLower(strings.TrimSpace(normalized.Provider))
	if normalized.Provider == "" {
		normalized.Provider = "resend"
	}
	normalized.SenderName = strings.TrimSpace(normalized.SenderName)
	normalized.SenderEmail = strings.ToLower(strings.TrimSpace(normalized.SenderEmail))
	normalized.APIKeyEncrypted = strings.TrimSpace(normalized.APIKeyEncrypted)

	if normalized.ReplyToEmail != nil {
		trimmed := strings.ToLower(strings.TrimSpace(*normalized.ReplyToEmail))
		if trimmed == "" {
			normalized.ReplyToEmail = nil
		} else {
			normalized.ReplyToEmail = &trimmed
		}
	}

	if normalized.PasswordResetExpiryMinutes < 5 {
		normalized.PasswordResetExpiryMinutes = 30
	}

	return normalized
}

func (setting MailDeliverySettingRecord) publicView() MailDeliverySetting {
	normalized := normalizeMailDeliverySettingRecord(setting)
	return MailDeliverySetting{
		Enabled:                    normalized.Enabled,
		Provider:                   normalized.Provider,
		SenderName:                 normalized.SenderName,
		SenderEmail:                normalized.SenderEmail,
		ReplyToEmail:               normalized.ReplyToEmail,
		HasAPIKey:                  normalized.APIKeyEncrypted != "",
		PasswordResetEnabled:       normalized.PasswordResetEnabled,
		PasswordResetExpiryMinutes: normalized.PasswordResetExpiryMinutes,
		NotificationEnabled:        normalized.NotificationEnabled,
	}
}

func (setting MailDeliverySettingRecord) ForgotPasswordEnabled() bool {
	normalized := normalizeMailDeliverySettingRecord(setting)
	return normalized.Enabled &&
		normalized.Provider == "resend" &&
		normalized.PasswordResetEnabled &&
		normalized.PasswordResetExpiryMinutes >= 5 &&
		normalized.SenderEmail != "" &&
		normalized.APIKeyEncrypted != ""
}

func (setting MailDeliverySettingRecord) NotificationEmailsEnabled() bool {
	normalized := normalizeMailDeliverySettingRecord(setting)
	return normalized.Enabled &&
		normalized.Provider == "resend" &&
		normalized.NotificationEnabled &&
		normalized.SenderEmail != "" &&
		normalized.APIKeyEncrypted != ""
}

func (r *Repository) GetMailDeliveryRecord(ctx context.Context) (MailDeliverySettingRecord, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var raw []byte
	err := repository.DB(ctx, r.db).QueryRow(ctx, `SELECT value FROM system_settings WHERE key = 'mail_delivery'`).Scan(&raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return defaultMailDeliverySettingRecord(), nil
		}
		return MailDeliverySettingRecord{}, err
	}

	setting := defaultMailDeliverySettingRecord()
	if err := json.Unmarshal(raw, &setting); err != nil {
		return MailDeliverySettingRecord{}, err
	}

	return normalizeMailDeliverySettingRecord(setting), nil
}

func (r *Repository) UpdateMailDelivery(ctx context.Context, updatedBy string, setting MailDeliverySettingRecord) error {
	normalized := normalizeMailDeliverySettingRecord(setting)
	raw, err := json.Marshal(normalized)
	if err != nil {
		return err
	}

	_, err = repository.DB(ctx, r.db).Exec(ctx, `
		INSERT INTO system_settings (key, value, description, updated_by, updated_at)
		VALUES ('mail_delivery', $1::jsonb, $2, NULLIF($3, '')::uuid, NOW())
		ON CONFLICT (tenant_id, key) DO UPDATE
		SET value = EXCLUDED.value,
		    description = EXCLUDED.description,
		    updated_by = EXCLUDED.updated_by,
		    updated_at = NOW()
	`, string(raw), mailDeliverySettingDescription, updatedBy)
	return err
}

func (r *Repository) GetPublicAuthOptions(ctx context.Context) (PublicAuthOptions, error) {
	setting, err := r.GetMailDeliveryRecord(ctx)
	if err != nil {
		return PublicAuthOptions{}, err
	}

	registration, err := r.GetRegistrationSettings(ctx)
	if err != nil {
		return PublicAuthOptions{}, err
	}

	// Registration requires both the feature toggle and a live (non-expired) code
	// so the UI never shows a register form the backend will reject outright.
	registrationReady := registration.Enabled &&
		strings.TrimSpace(registration.CodeEncrypted) != "" &&
		registration.CodeExpiresAt != nil &&
		registration.CodeExpiresAt.After(time.Now().UTC())

	return PublicAuthOptions{
		ForgotPasswordEnabled: setting.ForgotPasswordEnabled(),
		RegistrationEnabled:   registrationReady,
	}, nil
}

func (r *Repository) GetTenantPrimaryDomain(ctx context.Context) (string, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var domain string
	err := repository.DB(ctx, r.db).QueryRow(ctx, `
		SELECT domain
		FROM tenant_domains
		WHERE tenant_id = current_setting('app.current_tenant', true)::uuid
		ORDER BY is_primary DESC, created_at ASC
		LIMIT 1
	`).Scan(&domain)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}

	return strings.TrimSpace(domain), nil
}
