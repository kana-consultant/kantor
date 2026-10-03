package auth

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	repository "github.com/kana-consultant/kantor/backend/internal/repository"
)

const (
	documentMailSettingKey         = "document_mail"
	documentMailSettingDescription = "Konfigurasi Gmail untuk pengiriman dokumen (slip gaji, kontrak)"
	// DefaultDocumentMailPort is submission with STARTTLS.
	DefaultDocumentMailPort = 587
)

// DocumentMailSettingRecord is the stored system_settings value for the
// documents-only Gmail path. The host is not stored: it is the constant
// smtp.gmail.com in the mail package. The app password is only ever held as
// security.Encrypter ciphertext and is never returned by the API.
type DocumentMailSettingRecord struct {
	Enabled               bool   `json:"enabled"`
	SMTPUsername          string `json:"smtp_username"`
	SMTPPasswordEncrypted string `json:"smtp_password_encrypted,omitempty"`
	SMTPPort              int    `json:"smtp_port"`
	SenderName            string `json:"sender_name"`
}

// HasPassword reports whether an encrypted app password is stored.
func (setting DocumentMailSettingRecord) HasPassword() bool {
	return strings.TrimSpace(setting.SMTPPasswordEncrypted) != ""
}

func defaultDocumentMailSettingRecord() DocumentMailSettingRecord {
	return DocumentMailSettingRecord{SMTPPort: DefaultDocumentMailPort}
}

func NormalizeDocumentMailSettingRecord(setting DocumentMailSettingRecord) DocumentMailSettingRecord {
	normalized := setting
	normalized.SMTPUsername = strings.ToLower(strings.TrimSpace(normalized.SMTPUsername))
	normalized.SMTPPasswordEncrypted = strings.TrimSpace(normalized.SMTPPasswordEncrypted)
	normalized.SenderName = strings.TrimSpace(normalized.SenderName)
	if normalized.SMTPPort != 587 && normalized.SMTPPort != 465 {
		normalized.SMTPPort = DefaultDocumentMailPort
	}
	return normalized
}

func (r *Repository) GetDocumentMailRecord(ctx context.Context) (DocumentMailSettingRecord, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var raw []byte
	err := repository.DB(ctx, r.db).QueryRow(ctx, `SELECT value FROM system_settings WHERE key = $1`, documentMailSettingKey).Scan(&raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return defaultDocumentMailSettingRecord(), nil
		}
		return DocumentMailSettingRecord{}, err
	}

	setting := defaultDocumentMailSettingRecord()
	if err := json.Unmarshal(raw, &setting); err != nil {
		return DocumentMailSettingRecord{}, err
	}
	return NormalizeDocumentMailSettingRecord(setting), nil
}

func (r *Repository) UpdateDocumentMail(ctx context.Context, updatedBy string, setting DocumentMailSettingRecord) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	raw, err := json.Marshal(NormalizeDocumentMailSettingRecord(setting))
	if err != nil {
		return err
	}

	_, err = repository.DB(ctx, r.db).Exec(ctx, `
		INSERT INTO system_settings (key, value, description, updated_by, updated_at)
		VALUES ($1, $2::jsonb, $3, NULLIF($4, '')::uuid, NOW())
		ON CONFLICT (tenant_id, key) DO UPDATE
		SET value = EXCLUDED.value,
		    description = EXCLUDED.description,
		    updated_by = EXCLUDED.updated_by,
		    updated_at = NOW()
	`, documentMailSettingKey, string(raw), documentMailSettingDescription, updatedBy)
	return err
}
