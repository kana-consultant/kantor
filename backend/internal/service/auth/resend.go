package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
	"time"
)

type passwordResetEmail struct {
	ToEmail    string
	ToName     string
	TenantName string
	ResetURL   string
	ExpiresIn  time.Duration
}

type resendMailConfig struct {
	APIKey       string
	SenderName   string
	SenderEmail  string
	ReplyToEmail *string
}

type passwordResetMailer interface {
	SendPasswordReset(ctx context.Context, config resendMailConfig, message passwordResetEmail) error
}

type resendMailer struct {
	client *http.Client
}

func newResendMailer() passwordResetMailer {
	return &resendMailer{
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (m *resendMailer) SendPasswordReset(ctx context.Context, config resendMailConfig, message passwordResetEmail) error {
	payload := map[string]any{
		"from":    formatFromAddress(config, message.TenantName),
		"to":      []string{message.ToEmail},
		"subject": passwordResetSubject(message.TenantName),
		"html":    renderPasswordResetHTML(message),
		"text":    renderPasswordResetText(message),
	}

	if config.ReplyToEmail != nil && strings.TrimSpace(*config.ReplyToEmail) != "" {
		payload["reply_to"] = strings.TrimSpace(*config.ReplyToEmail)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal resend payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create resend request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(config.APIKey))
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("send resend request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("resend returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
}

func formatFromAddress(config resendMailConfig, tenantName string) string {
	displayName := strings.TrimSpace(config.SenderName)
	if displayName == "" {
		displayName = strings.TrimSpace(tenantName)
	}

	if displayName == "" {
		return strings.TrimSpace(config.SenderEmail)
	}

	return fmt.Sprintf("%s <%s>", displayName, strings.TrimSpace(config.SenderEmail))
}

func passwordResetSubject(tenantName string) string {
	if tenantName = strings.TrimSpace(tenantName); tenantName == "" {
		return "Atur ulang kata sandi akun Anda"
	}

	return fmt.Sprintf("Atur ulang kata sandi %s", tenantName)
}

func renderPasswordResetHTML(message passwordResetEmail) string {
	tenantName := html.EscapeString(strings.TrimSpace(message.TenantName))
	if tenantName == "" {
		tenantName = "Kantor"
	}

	toName := html.EscapeString(strings.TrimSpace(message.ToName))
	if toName == "" {
		toName = "Tim"
	}

	resetURL := html.EscapeString(message.ResetURL)
	expiresIn := html.EscapeString(formatPasswordResetDuration(message.ExpiresIn))

	return fmt.Sprintf(`
<!doctype html>
<html lang="id">
  <body style="margin:0;padding:24px;background:#f5f7fb;font-family:Arial,sans-serif;color:#172b4d;">
    <table role="presentation" width="100%%" cellspacing="0" cellpadding="0">
      <tr>
        <td align="center">
          <table role="presentation" width="100%%" style="max-width:560px;background:#ffffff;border:1px solid #dfe1e6;border-radius:16px;padding:32px;">
            <tr>
              <td>
                <p style="margin:0 0 12px;font-size:12px;letter-spacing:0.18em;text-transform:uppercase;color:#4c9aff;font-weight:700;">%s</p>
                <h1 style="margin:0 0 12px;font-size:28px;line-height:1.2;">Atur ulang kata sandi</h1>
                <p style="margin:0 0 16px;font-size:15px;line-height:1.7;">Halo %s, kami menerima permintaan untuk mengatur ulang kata sandi akun Anda.</p>
                <p style="margin:0 0 24px;font-size:15px;line-height:1.7;">Link ini berlaku selama <strong>%s</strong>. Jika Anda tidak meminta reset kata sandi, abaikan email ini.</p>
                <p style="margin:0 0 28px;">
                  <a href="%s" style="display:inline-block;background:#0065ff;color:#ffffff;text-decoration:none;font-weight:700;padding:14px 20px;border-radius:12px;">Buka halaman reset password</a>
                </p>
                <p style="margin:0 0 8px;font-size:13px;line-height:1.6;color:#5e6c84;">Jika tombol di atas tidak bisa dibuka, salin link ini ke browser:</p>
                <p style="margin:0;font-size:13px;line-height:1.6;word-break:break-all;"><a href="%s" style="color:#0065ff;">%s</a></p>
              </td>
            </tr>
          </table>
        </td>
      </tr>
    </table>
  </body>
</html>`, tenantName, toName, expiresIn, resetURL, resetURL, resetURL)
}

func renderPasswordResetText(message passwordResetEmail) string {
	tenantName := strings.TrimSpace(message.TenantName)
	if tenantName == "" {
		tenantName = "Kantor"
	}

	toName := strings.TrimSpace(message.ToName)
	if toName == "" {
		toName = "Tim"
	}

	return fmt.Sprintf(
		"Halo %s,\n\nKami menerima permintaan untuk mengatur ulang kata sandi akun %s.\n\nBuka link berikut untuk melanjutkan:\n%s\n\nLink ini berlaku selama %s.\nJika Anda tidak meminta reset kata sandi, abaikan email ini.",
		toName,
		tenantName,
		message.ResetURL,
		formatPasswordResetDuration(message.ExpiresIn),
	)
}

func formatPasswordResetDuration(duration time.Duration) string {
	minutes := int(duration.Minutes())
	if minutes < 1 {
		return "kurang dari 1 menit"
	}
	if minutes == 1 {
		return "1 menit"
	}
	return fmt.Sprintf("%d menit", minutes)
}
