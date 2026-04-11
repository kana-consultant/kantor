package notifications

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

type resendSender struct {
	client *http.Client
}

type resendEmail struct {
	ToEmail string
	Subject string
	HTML    string
	Text    string
}

type renderEmailLayoutParams struct {
	Eyebrow  string
	Title    string
	Intro    string
	BodyHTML string
	CTAURL   string
	CTALabel string
}

func newResendSender() *resendSender {
	return &resendSender{
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (s *resendSender) Send(ctx context.Context, cfg emailRuntimeConfig, tenantName string, message resendEmail) error {
	payload := map[string]any{
		"from":    formatNotificationFromAddress(cfg, tenantName),
		"to":      []string{strings.TrimSpace(message.ToEmail)},
		"subject": strings.TrimSpace(message.Subject),
		"html":    message.HTML,
		"text":    message.Text,
	}
	if cfg.ReplyToEmail != nil && strings.TrimSpace(*cfg.ReplyToEmail) != "" {
		payload["reply_to"] = strings.TrimSpace(*cfg.ReplyToEmail)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal resend payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create resend request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.APIKey))
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
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

func formatNotificationFromAddress(cfg emailRuntimeConfig, tenantName string) string {
	displayName := strings.TrimSpace(cfg.SenderName)
	if displayName == "" {
		displayName = strings.TrimSpace(tenantName)
	}
	if displayName == "" {
		return strings.TrimSpace(cfg.SenderEmail)
	}
	return fmt.Sprintf("%s <%s>", displayName, strings.TrimSpace(cfg.SenderEmail))
}

func renderEmailLayoutHTML(params renderEmailLayoutParams) string {
	introHTML := ""
	if strings.TrimSpace(params.Intro) != "" {
		introHTML = `<p style="margin:0 0 16px;font-size:15px;line-height:1.7;">` + html.EscapeString(strings.TrimSpace(params.Intro)) + `</p>`
	}

	ctaHTML := ""
	if strings.TrimSpace(params.CTAURL) != "" && strings.TrimSpace(params.CTALabel) != "" {
		safeURL := html.EscapeString(strings.TrimSpace(params.CTAURL))
		ctaHTML = fmt.Sprintf(`
			<p style="margin:0 0 28px;">
				<a href="%s" style="display:inline-block;background:#0065ff;color:#ffffff;text-decoration:none;font-weight:700;padding:14px 20px;border-radius:12px;">%s</a>
			</p>
			<p style="margin:0;font-size:13px;line-height:1.6;word-break:break-all;">
				<a href="%s" style="color:#0065ff;">%s</a>
			</p>`,
			safeURL,
			html.EscapeString(strings.TrimSpace(params.CTALabel)),
			safeURL,
			safeURL,
		)
	}

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
                <h1 style="margin:0 0 12px;font-size:28px;line-height:1.2;">%s</h1>
                %s
                %s
                %s
              </td>
            </tr>
          </table>
        </td>
      </tr>
    </table>
  </body>
</html>`,
		html.EscapeString(strings.TrimSpace(params.Eyebrow)),
		html.EscapeString(strings.TrimSpace(params.Title)),
		introHTML,
		params.BodyHTML,
		ctaHTML,
	)
}

func renderEmailText(title string, lines ...string) string {
	filtered := make([]string, 0, len(lines)+1)
	if strings.TrimSpace(title) != "" {
		filtered = append(filtered, strings.TrimSpace(title), "")
	}
	for _, line := range lines {
		filtered = append(filtered, line)
	}
	return strings.TrimSpace(strings.Join(filtered, "\n"))
}
