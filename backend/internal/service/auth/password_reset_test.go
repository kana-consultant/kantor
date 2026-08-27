package auth

import (
	"context"
	"strings"
	"testing"

	"github.com/kana-consultant/kantor/backend/internal/model"
	authrepo "github.com/kana-consultant/kantor/backend/internal/repository/auth"
	"github.com/kana-consultant/kantor/backend/internal/security"
)

const testEncryptionKey = "test-data-encryption-key-0123456789abcdef"

type stubPasswordResetRepo struct {
	authRepository
	user          model.User
	mailRecord    authrepo.MailDeliverySettingRecord
	primaryDomain string
	domainErr     error
	createdToken  bool
}

func (s *stubPasswordResetRepo) GetUserByEmail(ctx context.Context, email string) (model.User, error) {
	return s.user, nil
}

func (s *stubPasswordResetRepo) GetMailDeliveryRecord(ctx context.Context) (authrepo.MailDeliverySettingRecord, error) {
	return s.mailRecord, nil
}

func (s *stubPasswordResetRepo) CreatePasswordResetToken(ctx context.Context, params authrepo.CreatePasswordResetTokenParams) error {
	s.createdToken = true
	return nil
}

func (s *stubPasswordResetRepo) GetTenantPrimaryDomain(ctx context.Context) (string, error) {
	return s.primaryDomain, s.domainErr
}

type stubPasswordResetMailer struct {
	messages []passwordResetEmail
}

func (m *stubPasswordResetMailer) SendPasswordReset(ctx context.Context, config resendMailConfig, message passwordResetEmail) error {
	m.messages = append(m.messages, message)
	return nil
}

func newPasswordResetTestService(repo *stubPasswordResetRepo, mailer *stubPasswordResetMailer, appURL string) *Service {
	enc, err := security.NewEncrypter(testEncryptionKey)
	if err != nil {
		panic(err)
	}
	record := repo.mailRecord
	record.Enabled = true
	record.Provider = "resend"
	record.SenderName = "Kantor"
	record.SenderEmail = "noreply@example.com"
	record.PasswordResetEnabled = true
	record.PasswordResetExpiryMinutes = 30
	record.APIKeyEncrypted, err = enc.EncryptString("resend-test-key")
	if err != nil {
		panic(err)
	}
	repo.mailRecord = record

	return &Service{
		repo:           repo,
		passwordMailer: mailer,
		encrypter:      enc,
		fallbackAppURL: strings.TrimRight(strings.TrimSpace(appURL), "/"),
	}
}

func TestRequestPasswordReset_PrefersCanonicalTenantDomainOverHostBaseURL(t *testing.T) {
	repo := &stubPasswordResetRepo{
		user:          model.User{ID: "u1", Email: "user@example.com", FullName: "User", IsActive: true},
		primaryDomain: "tenant.example.com",
	}
	mailer := &stubPasswordResetMailer{}
	svc := newPasswordResetTestService(repo, mailer, "")

	err := svc.RequestPasswordReset(context.Background(), "user@example.com", PasswordResetRequestMeta{
		PublicBaseURL: "https://evil.example",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mailer.messages) != 1 {
		t.Fatalf("expected 1 email, got %d", len(mailer.messages))
	}
	wantPrefix := "https://tenant.example.com/reset-password?token="
	if !strings.HasPrefix(mailer.messages[0].ResetURL, wantPrefix) {
		t.Fatalf("reset link must use the canonical tenant domain, got %q, want prefix %q", mailer.messages[0].ResetURL, wantPrefix)
	}
}

func TestRequestPasswordReset_FallsBackToHostBaseURLWhenNoCanonicalURLConfigured(t *testing.T) {
	repo := &stubPasswordResetRepo{
		user: model.User{ID: "u1", Email: "user@example.com", FullName: "User", IsActive: true},
	}
	mailer := &stubPasswordResetMailer{}
	svc := newPasswordResetTestService(repo, mailer, "")

	err := svc.RequestPasswordReset(context.Background(), "user@example.com", PasswordResetRequestMeta{
		PublicBaseURL: "http://localhost:5173",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantPrefix := "http://localhost:5173/reset-password?token="
	if !strings.HasPrefix(mailer.messages[0].ResetURL, wantPrefix) {
		t.Fatalf("expected host fallback, got %q, want prefix %q", mailer.messages[0].ResetURL, wantPrefix)
	}
}

func TestRequestPasswordReset_UsesAppURLWhenNoCanonicalURLOrHostConfigured(t *testing.T) {
	repo := &stubPasswordResetRepo{
		user: model.User{ID: "u1", Email: "user@example.com", FullName: "User", IsActive: true},
	}
	mailer := &stubPasswordResetMailer{}
	svc := newPasswordResetTestService(repo, mailer, "https://app.example.com")

	err := svc.RequestPasswordReset(context.Background(), "user@example.com", PasswordResetRequestMeta{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantPrefix := "https://app.example.com/reset-password?token="
	if !strings.HasPrefix(mailer.messages[0].ResetURL, wantPrefix) {
		t.Fatalf("expected APP_URL fallback, got %q, want prefix %q", mailer.messages[0].ResetURL, wantPrefix)
	}
}

func TestRequestPasswordReset_FailsClosedWhenCanonicalDomainLookupErrors(t *testing.T) {
	repo := &stubPasswordResetRepo{
		user:      model.User{ID: "u1", Email: "user@example.com", FullName: "User", IsActive: true},
		domainErr: errTestDomainLookup,
	}
	mailer := &stubPasswordResetMailer{}
	svc := newPasswordResetTestService(repo, mailer, "")

	err := svc.RequestPasswordReset(context.Background(), "user@example.com", PasswordResetRequestMeta{
		PublicBaseURL: "https://evil.example",
	})
	if err == nil {
		t.Fatal("expected error when canonical domain lookup fails, got nil")
	}
	if len(mailer.messages) != 0 {
		t.Fatalf("must not send an email with a Host-derived URL when canonical lookup fails, got %d message(s)", len(mailer.messages))
	}
}

func TestRequestPasswordReset_InactiveUserSendsNoEmail(t *testing.T) {
	repo := &stubPasswordResetRepo{
		user:          model.User{ID: "u1", Email: "user@example.com", FullName: "User", IsActive: false},
		primaryDomain: "tenant.example.com",
	}
	mailer := &stubPasswordResetMailer{}
	svc := newPasswordResetTestService(repo, mailer, "")

	err := svc.RequestPasswordReset(context.Background(), "user@example.com", PasswordResetRequestMeta{
		PublicBaseURL: "https://evil.example",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mailer.messages) != 0 {
		t.Fatalf("inactive user must not receive a reset email, got %d message(s)", len(mailer.messages))
	}
	if repo.createdToken {
		t.Fatal("inactive user must not get a reset token created")
	}
}

var errTestDomainLookup = context.DeadlineExceeded
