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

const (
	companyProfileSettingKey         = "company_profile"
	companyProfileSettingDescription = "Profil perusahaan untuk dokumen (slip gaji, kontrak kerja)"

	DefaultCompanyPaydayDay       = 25
	DefaultCompanyAnnualLeaveDays = 12
)

// CompanyProfileRecord is the stored system_settings value used by generated
// documents (payslips, contracts). The tenant logo itself lives on disk
// (UPLOADS_DIR/branding/<tenant_id>/logo.png); LogoUpdatedAt only records
// when it last changed.
type CompanyProfileRecord struct {
	LegalName       string     `json:"legal_name"`
	Address         string     `json:"address"`
	BusinessType    string     `json:"business_type"`
	City            string     `json:"city"`
	SignerName      string     `json:"signer_name"`
	SignerTitle     string     `json:"signer_title"`
	HRContactEmail  string     `json:"hr_contact_email"`
	DocCode         string     `json:"doc_code"`
	PaydayDay       int        `json:"payday_day"`
	AnnualLeaveDays int        `json:"annual_leave_days"`
	LogoUpdatedAt   *time.Time `json:"logo_updated_at"`
}

// DefaultCompanyProfileRecord mirrors the row seeded by rbac.seedSettings.
func DefaultCompanyProfileRecord() CompanyProfileRecord {
	return CompanyProfileRecord{
		PaydayDay:       DefaultCompanyPaydayDay,
		AnnualLeaveDays: DefaultCompanyAnnualLeaveDays,
	}
}

// NormalizeCompanyProfileRecord trims every text field, lower-cases the HR
// e-mail, upper-cases the document code and falls back to the defaults for
// out-of-range numbers (e.g. a hand-edited row).
func NormalizeCompanyProfileRecord(record CompanyProfileRecord) CompanyProfileRecord {
	normalized := record
	normalized.LegalName = strings.TrimSpace(normalized.LegalName)
	normalized.Address = strings.TrimSpace(normalized.Address)
	normalized.BusinessType = strings.TrimSpace(normalized.BusinessType)
	normalized.City = strings.TrimSpace(normalized.City)
	normalized.SignerName = strings.TrimSpace(normalized.SignerName)
	normalized.SignerTitle = strings.TrimSpace(normalized.SignerTitle)
	normalized.HRContactEmail = strings.ToLower(strings.TrimSpace(normalized.HRContactEmail))
	normalized.DocCode = strings.ToUpper(strings.TrimSpace(normalized.DocCode))
	if normalized.PaydayDay < 1 || normalized.PaydayDay > 31 {
		normalized.PaydayDay = DefaultCompanyPaydayDay
	}
	if normalized.AnnualLeaveDays < 0 || normalized.AnnualLeaveDays > 365 {
		normalized.AnnualLeaveDays = DefaultCompanyAnnualLeaveDays
	}
	if normalized.LogoUpdatedAt != nil {
		at := normalized.LogoUpdatedAt.UTC()
		normalized.LogoUpdatedAt = &at
	}
	return normalized
}

func (r *Repository) GetCompanyProfileRecord(ctx context.Context) (CompanyProfileRecord, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var raw []byte
	err := repository.DB(ctx, r.db).QueryRow(ctx, `SELECT value FROM system_settings WHERE key = $1`, companyProfileSettingKey).Scan(&raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DefaultCompanyProfileRecord(), nil
		}
		return CompanyProfileRecord{}, err
	}

	record := DefaultCompanyProfileRecord()
	if err := json.Unmarshal(raw, &record); err != nil {
		return CompanyProfileRecord{}, err
	}
	return NormalizeCompanyProfileRecord(record), nil
}

// UpdateCompanyProfile stores the profile fields. logo_updated_at is owned by
// SetCompanyLogoUpdatedAt: an existing row keeps its stored value, so a profile
// save racing a logo upload cannot roll the logo timestamp back.
func (r *Repository) UpdateCompanyProfile(ctx context.Context, updatedBy string, record CompanyProfileRecord) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	raw, err := json.Marshal(NormalizeCompanyProfileRecord(record))
	if err != nil {
		return err
	}

	_, err = repository.DB(ctx, r.db).Exec(ctx, `
		INSERT INTO system_settings (key, value, description, updated_by, updated_at)
		VALUES ($1, $2::jsonb, $3, NULLIF($4, '')::uuid, NOW())
		ON CONFLICT (tenant_id, key) DO UPDATE
		SET value = EXCLUDED.value || jsonb_build_object(
		        'logo_updated_at', COALESCE(system_settings.value -> 'logo_updated_at', 'null'::jsonb)
		    ),
		    description = EXCLUDED.description,
		    updated_by = EXCLUDED.updated_by,
		    updated_at = NOW()
	`, companyProfileSettingKey, string(raw), companyProfileSettingDescription, updatedBy)
	return err
}

// SetCompanyLogoUpdatedAt records when the tenant logo was uploaded (nil when
// it was removed) without touching the other profile fields.
func (r *Repository) SetCompanyLogoUpdatedAt(ctx context.Context, updatedBy string, at *time.Time) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var stamp any
	if at != nil {
		stamp = at.UTC().Format(time.RFC3339Nano)
	}
	defaults := DefaultCompanyProfileRecord()
	if at != nil {
		utc := at.UTC()
		defaults.LogoUpdatedAt = &utc
	}
	raw, err := json.Marshal(defaults)
	if err != nil {
		return err
	}

	_, err = repository.DB(ctx, r.db).Exec(ctx, `
		INSERT INTO system_settings (key, value, description, updated_by, updated_at)
		VALUES ($1, $2::jsonb, $3, NULLIF($4, '')::uuid, NOW())
		ON CONFLICT (tenant_id, key) DO UPDATE
		SET value = jsonb_set(system_settings.value, '{logo_updated_at}', COALESCE(to_jsonb($5::text), 'null'::jsonb), true),
		    updated_by = EXCLUDED.updated_by,
		    updated_at = NOW()
	`, companyProfileSettingKey, string(raw), companyProfileSettingDescription, updatedBy, stamp)
	return err
}
