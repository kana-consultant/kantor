package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/dto"
	authrepo "github.com/kana-consultant/kantor/backend/internal/repository/auth"
)

const (
	registrationCodeByteLen   = 18 // 24 base64 chars
	registrationMinRotationDs = 1
	registrationMaxRotationDs = 90
)

var (
	ErrRegistrationDisabled    = errors.New("registrasi mandiri tidak diaktifkan")
	ErrRegistrationCodeMissing = errors.New("kode registrasi wajib diisi")
	ErrRegistrationCodeExpired = errors.New("kode registrasi sudah kedaluwarsa, hubungi admin")
	ErrRegistrationCodeInvalid = errors.New("kode registrasi tidak valid")
	ErrRegistrationDomainDeny  = errors.New("domain email tidak diizinkan untuk registrasi")
	errRegistrationNoEncrypter = errors.New("encrypter for registration code is not configured")
)

func (s *Service) GetRegistrationSettings(ctx context.Context) (authrepo.RegistrationSettingsView, error) {
	setting, err := s.repo.GetRegistrationSettings(ctx)
	if err != nil {
		return authrepo.RegistrationSettingsView{}, err
	}
	return s.enrichRegistrationView(ctx, setting), nil
}

// enrichRegistrationView decorates the raw settings with derived fields —
// the plaintext code (decrypted) and the rolled-by user full name.
func (s *Service) enrichRegistrationView(ctx context.Context, setting authrepo.RegistrationSettings) authrepo.RegistrationSettingsView {
	view := setting.View()

	if view.LastRolledBy != nil {
		id := strings.TrimSpace(*view.LastRolledBy)
		if id != "" {
			if name, err := s.repo.ResolveUserFullName(ctx, id); err == nil {
				trimmed := strings.TrimSpace(name)
				if trimmed != "" {
					view.LastRolledByName = &trimmed
				}
			}
		}
	}

	if plaintext, err := s.decryptRegistrationCode(setting.CodeEncrypted); err == nil && plaintext != "" {
		view.Code = &plaintext
	}

	return view
}

func (s *Service) UpdateRegistrationSettings(ctx context.Context, updatedBy string, input dto.UpdateRegistrationSettingsRequest) (authrepo.RegistrationSettingsView, error) {
	existing, err := s.repo.GetRegistrationSettings(ctx)
	if err != nil {
		return authrepo.RegistrationSettingsView{}, err
	}

	updated := existing
	updated.Enabled = input.Enabled
	if input.RotationIntervalDays > 0 {
		updated.RotationIntervalDays = input.RotationIntervalDays
	}
	updated.AllowedEmailDomains = input.AllowedEmailDomains

	if err := s.repo.UpdateRegistrationSettings(ctx, updatedBy, updated); err != nil {
		return authrepo.RegistrationSettingsView{}, err
	}

	refreshed, err := s.repo.GetRegistrationSettings(ctx)
	if err != nil {
		return authrepo.RegistrationSettingsView{}, err
	}
	return s.enrichRegistrationView(ctx, refreshed), nil
}

func (s *Service) RollRegistrationCode(ctx context.Context, rolledBy string) (string, authrepo.RegistrationSettingsView, error) {
	if s.encrypter == nil {
		return "", authrepo.RegistrationSettingsView{}, errRegistrationNoEncrypter
	}

	existing, err := s.repo.GetRegistrationSettings(ctx)
	if err != nil {
		return "", authrepo.RegistrationSettingsView{}, err
	}

	rawCode, err := generateRegistrationCode()
	if err != nil {
		return "", authrepo.RegistrationSettingsView{}, err
	}

	ciphertext, err := s.encrypter.EncryptString(rawCode)
	if err != nil {
		return "", authrepo.RegistrationSettingsView{}, err
	}

	now := time.Now().UTC()
	interval := clampRotationInterval(existing.RotationIntervalDays)
	expires := now.Add(time.Duration(interval) * 24 * time.Hour)

	updated := existing
	updated.CodeEncrypted = ciphertext
	updated.CodeExpiresAt = &expires
	updated.LastRolledAt = &now
	trimmedBy := strings.TrimSpace(rolledBy)
	if trimmedBy != "" {
		updated.LastRolledBy = &trimmedBy
	} else {
		updated.LastRolledBy = nil
	}
	updated.RotationIntervalDays = interval

	if err := s.repo.UpdateRegistrationSettings(ctx, trimmedBy, updated); err != nil {
		return "", authrepo.RegistrationSettingsView{}, err
	}

	refreshed, err := s.repo.GetRegistrationSettings(ctx)
	if err != nil {
		return "", authrepo.RegistrationSettingsView{}, err
	}
	return rawCode, s.enrichRegistrationView(ctx, refreshed), nil
}

// AutoRollRegistrationCodeIfExpired rotates the code when enabled and either
// unset or expired. Returns (rolled, settingsView, err). No-op otherwise.
func (s *Service) AutoRollRegistrationCodeIfExpired(ctx context.Context, now time.Time) (bool, authrepo.RegistrationSettingsView, error) {
	existing, err := s.repo.GetRegistrationSettings(ctx)
	if err != nil {
		return false, authrepo.RegistrationSettingsView{}, err
	}
	if !existing.Enabled {
		return false, s.enrichRegistrationView(ctx, existing), nil
	}

	hasCode := strings.TrimSpace(existing.CodeEncrypted) != ""
	expired := existing.CodeExpiresAt == nil || !existing.CodeExpiresAt.After(now.UTC())
	if hasCode && !expired {
		return false, s.enrichRegistrationView(ctx, existing), nil
	}

	if s.encrypter == nil {
		return false, s.enrichRegistrationView(ctx, existing), errRegistrationNoEncrypter
	}

	rawCode, err := generateRegistrationCode()
	if err != nil {
		return false, authrepo.RegistrationSettingsView{}, err
	}
	ciphertext, err := s.encrypter.EncryptString(rawCode)
	if err != nil {
		return false, authrepo.RegistrationSettingsView{}, err
	}

	interval := clampRotationInterval(existing.RotationIntervalDays)
	nowUTC := now.UTC()
	expires := nowUTC.Add(time.Duration(interval) * 24 * time.Hour)

	updated := existing
	updated.CodeEncrypted = ciphertext
	updated.CodeExpiresAt = &expires
	updated.LastRolledAt = &nowUTC
	updated.LastRolledBy = nil
	updated.RotationIntervalDays = interval

	if err := s.repo.UpdateRegistrationSettings(ctx, "", updated); err != nil {
		return false, authrepo.RegistrationSettingsView{}, err
	}

	refreshed, err := s.repo.GetRegistrationSettings(ctx)
	if err != nil {
		return false, authrepo.RegistrationSettingsView{}, err
	}
	return true, s.enrichRegistrationView(ctx, refreshed), nil
}

func (s *Service) validateRegistrationPolicy(ctx context.Context, email string, rawCode string) error {
	setting, err := s.repo.GetRegistrationSettings(ctx)
	if err != nil {
		return err
	}

	if !setting.Enabled {
		return ErrRegistrationDisabled
	}

	trimmedCode := strings.TrimSpace(rawCode)
	if trimmedCode == "" {
		return ErrRegistrationCodeMissing
	}

	if strings.TrimSpace(setting.CodeEncrypted) == "" {
		return ErrRegistrationCodeInvalid
	}
	if setting.CodeExpiresAt == nil || !setting.CodeExpiresAt.After(time.Now().UTC()) {
		return ErrRegistrationCodeExpired
	}

	storedPlain, err := s.decryptRegistrationCode(setting.CodeEncrypted)
	if err != nil || storedPlain == "" {
		return ErrRegistrationCodeInvalid
	}
	if subtle.ConstantTimeCompare([]byte(storedPlain), []byte(trimmedCode)) != 1 {
		return ErrRegistrationCodeInvalid
	}

	if len(setting.AllowedEmailDomains) > 0 {
		domain := extractEmailDomain(email)
		if domain == "" || !domainAllowed(domain, setting.AllowedEmailDomains) {
			return ErrRegistrationDomainDeny
		}
	}

	return nil
}

func (s *Service) decryptRegistrationCode(ciphertext string) (string, error) {
	if strings.TrimSpace(ciphertext) == "" {
		return "", nil
	}
	if s.encrypter == nil {
		return "", errRegistrationNoEncrypter
	}
	return s.encrypter.DecryptString(ciphertext)
}

func clampRotationInterval(days int) int {
	if days < registrationMinRotationDs {
		return 7
	}
	if days > registrationMaxRotationDs {
		return registrationMaxRotationDs
	}
	return days
}

func generateRegistrationCode() (string, error) {
	entropy := make([]byte, registrationCodeByteLen)
	if _, err := rand.Read(entropy); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(entropy), nil
}

func extractEmailDomain(email string) string {
	at := strings.LastIndex(email, "@")
	if at < 0 || at == len(email)-1 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(email[at+1:]))
}

func domainAllowed(domain string, allowed []string) bool {
	domain = strings.ToLower(strings.TrimSpace(domain))
	for _, d := range allowed {
		if strings.EqualFold(strings.TrimSpace(d), domain) {
			return true
		}
	}
	return false
}
