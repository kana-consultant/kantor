package auth

import "testing"

func TestNormalizeMailDeliverySettingRecord(t *testing.T) {
	t.Parallel()

	replyTo := "  SUPPORT@SENTINELHUB.AI  "
	normalized := normalizeMailDeliverySettingRecord(MailDeliverySettingRecord{
		Provider:                   "  RESEND ",
		SenderName:                 "  Kantor  ",
		SenderEmail:                "  NO-REPLY@SENTINELHUB.AI ",
		ReplyToEmail:               &replyTo,
		APIKeyEncrypted:            "  encrypted-key  ",
		PasswordResetExpiryMinutes: 1,
	})

	if normalized.Provider != "resend" {
		t.Fatalf("Provider = %q, want %q", normalized.Provider, "resend")
	}
	if normalized.SenderName != "Kantor" {
		t.Fatalf("SenderName = %q, want %q", normalized.SenderName, "Kantor")
	}
	if normalized.SenderEmail != "no-reply@sentinelhub.ai" {
		t.Fatalf("SenderEmail = %q, want lowercase trimmed email", normalized.SenderEmail)
	}
	if normalized.ReplyToEmail == nil || *normalized.ReplyToEmail != "support@sentinelhub.ai" {
		t.Fatalf("ReplyToEmail = %v, want normalized reply email", normalized.ReplyToEmail)
	}
	if normalized.APIKeyEncrypted != "encrypted-key" {
		t.Fatalf("APIKeyEncrypted = %q, want trimmed key", normalized.APIKeyEncrypted)
	}
	if normalized.PasswordResetExpiryMinutes != 30 {
		t.Fatalf("PasswordResetExpiryMinutes = %d, want 30 fallback", normalized.PasswordResetExpiryMinutes)
	}
}

func TestMailDeliverySettingPredicates(t *testing.T) {
	t.Parallel()

	record := MailDeliverySettingRecord{
		Enabled:                    true,
		Provider:                   "resend",
		SenderEmail:                "no-reply@sentinelhub.ai",
		APIKeyEncrypted:            "secret",
		PasswordResetEnabled:       true,
		PasswordResetExpiryMinutes: 30,
		NotificationEnabled:        true,
	}

	if !record.ForgotPasswordEnabled() {
		t.Fatal("expected forgot password to be enabled")
	}
	if !record.NotificationEmailsEnabled() {
		t.Fatal("expected notification emails to be enabled")
	}
}

func TestMailDeliveryPublicViewMasksAPIKey(t *testing.T) {
	t.Parallel()

	public := (MailDeliverySettingRecord{
		Enabled:                    true,
		Provider:                   "resend",
		SenderName:                 "Kantor",
		SenderEmail:                "no-reply@sentinelhub.ai",
		APIKeyEncrypted:            "secret",
		PasswordResetEnabled:       true,
		PasswordResetExpiryMinutes: 30,
		NotificationEnabled:        true,
	}).publicView()

	if !public.HasAPIKey {
		t.Fatal("expected public view to expose has_api_key=true")
	}
	if public.SenderEmail != "no-reply@sentinelhub.ai" {
		t.Fatalf("SenderEmail = %q, want sender email", public.SenderEmail)
	}
}
