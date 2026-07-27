package operational

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// ErrDiscordReminderDisabled indicates the Discord reminder push is disabled
// for this tenant.
var ErrDiscordReminderDisabled = errors.New("discord reminder push is disabled for this tenant")

const discordReminderSecretHeader = "X-Discord-Bot-Secret"

// DiscordReminderTaskPayload is a single task entry in the digest payload.
type DiscordReminderTaskPayload struct {
	Title    string `json:"title"`
	Project  string `json:"project"`
	Column   string `json:"column"`
	Status   string `json:"status"`
	DueDate  string `json:"due_date"`
	Priority string `json:"priority"`
}

// DiscordReminderEmployeePayload groups tasks for a single employee.
type DiscordReminderEmployeePayload struct {
	KantorUserID string                       `json:"kantor_user_id"`
	Name         string                       `json:"name"`
	Tasks        []DiscordReminderTaskPayload `json:"tasks"`
}

// DiscordReminderDigestPayload is the JSON body posted to the external
// Discord bot. Field names and shape must match the bot contract exactly.
type DiscordReminderDigestPayload struct {
	Job       string                           `json:"job"`
	Date      string                           `json:"date"`
	Employees []DiscordReminderEmployeePayload `json:"employees"`
}

type discordReminderClientConfig struct {
	WebhookURL   string
	SharedSecret string
	Enabled      bool
}

// DiscordReminderClient pushes the task digest payload to the tenant's
// configured external Discord bot webhook.
type DiscordReminderClient struct {
	cfg        discordReminderClientConfig
	httpClient *http.Client
}

func newDiscordReminderClient(cfg discordReminderClientConfig) *DiscordReminderClient {
	return &DiscordReminderClient{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			// Wrap the default transport so each outbound push is captured
			// as a child span and the W3C traceparent header is injected
			// for the receiving bot.
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		},
	}
}

// NewDiscordReminderClientFromConfig creates a DiscordReminderClient from the
// per-tenant DB config.
func NewDiscordReminderClientFromConfig(webhookURL string, sharedSecret string, enabled bool) *DiscordReminderClient {
	return newDiscordReminderClient(discordReminderClientConfig{
		WebhookURL:   webhookURL,
		SharedSecret: sharedSecret,
		Enabled:      enabled,
	})
}

// Push sends the digest payload to the configured webhook.
func (c *DiscordReminderClient) Push(payload DiscordReminderDigestPayload) error {
	if !c.cfg.Enabled {
		return ErrDiscordReminderDisabled
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	resp, err := c.doRequest(c.cfg.WebhookURL, body)
	if err != nil {
		return fmt.Errorf("push discord reminder digest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("push discord reminder digest failed (%d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func (c *DiscordReminderClient) doRequest(url string, body []byte) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(discordReminderSecretHeader, c.cfg.SharedSecret)

	return c.httpClient.Do(req)
}
