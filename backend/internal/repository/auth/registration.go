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

const registrationSettingDescription = "Konfigurasi self-registration: kode, domain allowlist, rotasi"

type RegistrationSettings struct {
	Enabled              bool       `json:"enabled"`
	CodeEncrypted        string     `json:"code_encrypted"`
	CodeExpiresAt        *time.Time `json:"code_expires_at"`
	LastRolledBy         *string    `json:"last_rolled_by"`
	LastRolledAt         *time.Time `json:"last_rolled_at"`
	RotationIntervalDays int        `json:"rotation_interval_days"`
	AllowedEmailDomains  []string   `json:"allowed_email_domains"`
}

type RegistrationSettingsView struct {
	Enabled              bool       `json:"enabled"`
	HasCode              bool       `json:"has_code"`
	Code                 *string    `json:"code,omitempty"`
	CodeExpiresAt        *time.Time `json:"code_expires_at"`
	LastRolledBy         *string    `json:"last_rolled_by"`
	LastRolledByName     *string    `json:"last_rolled_by_name"`
	LastRolledAt         *time.Time `json:"last_rolled_at"`
	RotationIntervalDays int        `json:"rotation_interval_days"`
	AllowedEmailDomains  []string   `json:"allowed_email_domains"`
}

func defaultRegistrationSettings() RegistrationSettings {
	return RegistrationSettings{
		Enabled:              false,
		CodeEncrypted:        "",
		CodeExpiresAt:        nil,
		LastRolledBy:         nil,
		LastRolledAt:         nil,
		RotationIntervalDays: 7,
		AllowedEmailDomains:  []string{},
	}
}

func normalizeRegistrationSettings(s RegistrationSettings) RegistrationSettings {
	n := s
	n.CodeEncrypted = strings.TrimSpace(n.CodeEncrypted)
	if n.RotationIntervalDays < 1 {
		n.RotationIntervalDays = 7
	}
	if n.RotationIntervalDays > 90 {
		n.RotationIntervalDays = 90
	}

	if n.AllowedEmailDomains == nil {
		n.AllowedEmailDomains = []string{}
	}
	cleaned := make([]string, 0, len(n.AllowedEmailDomains))
	seen := make(map[string]struct{}, len(n.AllowedEmailDomains))
	for _, d := range n.AllowedEmailDomains {
		domain := strings.ToLower(strings.TrimSpace(d))
		domain = strings.TrimPrefix(domain, "@")
		if domain == "" {
			continue
		}
		if _, ok := seen[domain]; ok {
			continue
		}
		seen[domain] = struct{}{}
		cleaned = append(cleaned, domain)
	}
	n.AllowedEmailDomains = cleaned

	return n
}

func (s RegistrationSettings) View() RegistrationSettingsView {
	return RegistrationSettingsView{
		Enabled:              s.Enabled,
		HasCode:              strings.TrimSpace(s.CodeEncrypted) != "",
		Code:                 nil,
		CodeExpiresAt:        s.CodeExpiresAt,
		LastRolledBy:         s.LastRolledBy,
		LastRolledByName:     nil,
		LastRolledAt:         s.LastRolledAt,
		RotationIntervalDays: s.RotationIntervalDays,
		AllowedEmailDomains:  s.AllowedEmailDomains,
	}
}

// ResolveUserFullName returns the user's full_name for the given UUID within
// the current tenant context. Used to enrich registration settings view.
func (r *Repository) ResolveUserFullName(ctx context.Context, userID string) (string, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var fullName string
	err := repository.DB(ctx, r.db).QueryRow(ctx, `SELECT full_name FROM users WHERE id = $1::uuid`, userID).Scan(&fullName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	return fullName, nil
}

func (r *Repository) GetRegistrationSettings(ctx context.Context) (RegistrationSettings, error) {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	var raw []byte
	err := repository.DB(ctx, r.db).QueryRow(ctx, `SELECT value FROM system_settings WHERE key = 'registration'`).Scan(&raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return defaultRegistrationSettings(), nil
		}
		return RegistrationSettings{}, err
	}

	setting := defaultRegistrationSettings()
	if err := json.Unmarshal(raw, &setting); err != nil {
		return RegistrationSettings{}, err
	}
	return normalizeRegistrationSettings(setting), nil
}

func (r *Repository) UpdateRegistrationSettings(ctx context.Context, updatedBy string, setting RegistrationSettings) error {
	ctx, cancel := repository.QueryContext(ctx)
	defer cancel()

	normalized := normalizeRegistrationSettings(setting)
	raw, err := json.Marshal(normalized)
	if err != nil {
		return err
	}

	_, err = repository.DB(ctx, r.db).Exec(ctx, `
		INSERT INTO system_settings (key, value, description, updated_by, updated_at)
		VALUES ('registration', $1::jsonb, $2, NULLIF($3, '')::uuid, NOW())
		ON CONFLICT (tenant_id, key) DO UPDATE
		SET value = EXCLUDED.value,
		    description = EXCLUDED.description,
		    updated_by = EXCLUDED.updated_by,
		    updated_at = NOW()
	`, string(raw), registrationSettingDescription, updatedBy)
	return err
}

