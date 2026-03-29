package whatsapp

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

type SessionStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type AccountInfo struct {
	ID       string `json:"id"`
	PushName string `json:"pushName"`
}

type QRResponse struct {
	Value string `json:"value"`
}

type BulkMessage struct {
	Phone   string
	Message string
}

type BulkResult struct {
	Sent    int
	Failed  int
	Skipped int
}

type DailyStats struct {
	SentToday  int
	DailyLimit int
}

type CheckExistsResponse struct {
	NumberExists bool   `json:"numberExists"`
	ChatID       string `json:"chatId"`
}

var ErrWAHADisabled = errors.New("whatsapp is disabled for this tenant")

type wahaClientConfig struct {
	APIURL           string
	APIKey           string
	Session          string
	Enabled          bool
	MaxDailyMessages int
	MinDelayMS       int
	MaxDelayMS       int
	ReminderCron     string
	WeeklyDigestCron string
}

type WAHAClient struct {
	cfg        wahaClientConfig
	httpClient *http.Client
}

func newWAHAClient(cfg wahaClientConfig) *WAHAClient {
	return &WAHAClient{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// NewWAHAClientFromDBConfig creates a WAHAClient from the per-tenant DB config.
func NewWAHAClientFromDBConfig(dbCfg WADBConfig) *WAHAClient {
	return newWAHAClient(wahaClientConfig{
		APIURL:           dbCfg.APIURL,
		APIKey:           dbCfg.APIKey,
		Session:          dbCfg.SessionName,
		Enabled:          dbCfg.Enabled,
		MaxDailyMessages: dbCfg.MaxDailyMessages,
		MinDelayMS:       dbCfg.MinDelayMS,
		MaxDelayMS:       dbCfg.MaxDelayMS,
		ReminderCron:     dbCfg.ReminderCron,
		WeeklyDigestCron: dbCfg.WeeklyDigestCron,
	})
}

// WADBConfig mirrors the per-tenant WA config stored in the database.
type WADBConfig struct {
	APIURL           string
	APIKey           string
	SessionName      string
	Enabled          bool
	MaxDailyMessages int
	MinDelayMS       int
	MaxDelayMS       int
	ReminderCron     string
	WeeklyDigestCron string
}

func (c *WAHAClient) IsEnabled() bool {
	return c.cfg.Enabled
}

func (c *WAHAClient) GetStatus() (*SessionStatus, error) {
	if !c.cfg.Enabled {
		return &SessionStatus{Name: c.cfg.Session, Status: "STOPPED"}, nil
	}

	url := fmt.Sprintf("%s/api/sessions/%s", c.cfg.APIURL, c.cfg.Session)
	resp, err := c.doRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Session doesn't exist yet
	if resp.StatusCode == 404 {
		return &SessionStatus{Name: c.cfg.Session, Status: "STOPPED"}, nil
	}

	var status SessionStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("decode status: %w", err)
	}
	return &status, nil
}

func (c *WAHAClient) GetQR() (string, error) {
	if !c.cfg.Enabled {
		return "", fmt.Errorf("WAHA is disabled")
	}

	// Request QR as base64 image
	url := fmt.Sprintf("%s/api/%s/auth/qr?format=image", c.cfg.APIURL, c.cfg.Session)
	resp, err := c.doRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("get QR failed (%d): %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read qr body: %w", err)
	}

	contentType := resp.Header.Get("Content-Type")

	// If response is an image, return as base64 data URI
	if strings.Contains(contentType, "image/") {
		encoded := base64.StdEncoding.EncodeToString(body)
		return "data:" + contentType + ";base64," + encoded, nil
	}

	// If JSON response (e.g. base64 field)
	if strings.Contains(contentType, "application/json") {
		var result map[string]string
		if err := json.Unmarshal(body, &result); err == nil {
			if v, ok := result["mimetype"]; ok {
				// WAHA returns {mimetype, data} for format=image
				return "data:" + v + ";base64," + result["data"], nil
			}
			if v, ok := result["value"]; ok {
				return v, nil
			}
		}
	}

	// Fallback: raw text
	return strings.TrimSpace(string(body)), nil
}

func (c *WAHAClient) StartSession() error {
	if !c.cfg.Enabled {
		return ErrWAHADisabled
	}

	// 1. Check current status — if FAILED, logout first to clear stale auth
	status, err := c.GetStatus()
	if err == nil && status.Status == "FAILED" {
		slog.Info("WAHA session FAILED, logging out to reset auth")
		c.logout()
	}

	// 2. Try to create the session (POST /api/sessions).
	//    If it already exists WAHA returns 422, which we ignore.
	createURL := fmt.Sprintf("%s/api/sessions", c.cfg.APIURL)
	createBody, _ := json.Marshal(map[string]interface{}{
		"name":   c.cfg.Session,
		"start":  true,
		"config": map[string]interface{}{},
	})
	createResp, err := c.doRequest("POST", createURL, createBody)
	if err != nil {
		return err
	}
	io.ReadAll(createResp.Body) //nolint:errcheck
	createResp.Body.Close()

	// 200/201 = created & started → done
	if createResp.StatusCode < 400 {
		return nil
	}

	// 3. Session already exists (409/422) → start it
	startURL := fmt.Sprintf("%s/api/sessions/%s/start", c.cfg.APIURL, c.cfg.Session)
	startResp, err := c.doRequest("POST", startURL, nil)
	if err != nil {
		return err
	}
	defer startResp.Body.Close()

	if startResp.StatusCode >= 400 {
		body, _ := io.ReadAll(startResp.Body)
		return fmt.Errorf("start session failed (%d): %s", startResp.StatusCode, string(body))
	}
	return nil
}

// logout clears stale authentication so a fresh QR scan can happen.
func (c *WAHAClient) logout() {
	url := fmt.Sprintf("%s/api/sessions/%s/logout", c.cfg.APIURL, c.cfg.Session)
	resp, err := c.doRequest("POST", url, nil)
	if err != nil {
		slog.Error("WAHA logout failed", "error", err)
		return
	}
	io.ReadAll(resp.Body) //nolint:errcheck
	resp.Body.Close()
}

func (c *WAHAClient) StopSession() error {
	if !c.cfg.Enabled {
		return ErrWAHADisabled
	}

	url := fmt.Sprintf("%s/api/sessions/%s/stop", c.cfg.APIURL, c.cfg.Session)
	resp, err := c.doRequest("POST", url, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("stop session failed (%d): %s", resp.StatusCode, string(body))
	}
	return nil
}

func (c *WAHAClient) GetAccountInfo() (*AccountInfo, error) {
	if !c.cfg.Enabled {
		return &AccountInfo{ID: "disabled", PushName: "WAHA Disabled"}, nil
	}

	url := fmt.Sprintf("%s/api/sessions/%s/me", c.cfg.APIURL, c.cfg.Session)
	resp, err := c.doRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var info AccountInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decode account info: %w", err)
	}
	return &info, nil
}

func (c *WAHAClient) SendMessage(phone string, message string) error {
	if !c.cfg.Enabled {
		slog.Info("WAHA disabled, message not sent", "phone", phone, "message_length", len(message))
		return ErrWAHADisabled
	}

	normalized := NormalizePhone(phone)
	payload := map[string]string{
		"session": c.cfg.Session,
		"chatId":  normalized + "@c.us",
		"text":    message,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	url := fmt.Sprintf("%s/api/sendText", c.cfg.APIURL)
	resp, err := c.doRequest("POST", url, body)
	if err != nil {
		return fmt.Errorf("send message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("send failed (%d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func (c *WAHAClient) SendBulk(messages []BulkMessage) (*BulkResult, error) {
	result := &BulkResult{}

	for _, msg := range messages {
		if err := c.SendMessage(msg.Phone, msg.Message); err != nil {
			slog.Error("bulk send failed", "phone", msg.Phone, "error", err)
			result.Failed++
			continue
		}
		result.Sent++

		delay := c.cfg.MinDelayMS + rand.Intn(c.cfg.MaxDelayMS-c.cfg.MinDelayMS+1)
		time.Sleep(time.Duration(delay) * time.Millisecond)
	}

	return result, nil
}

func (c *WAHAClient) CheckPhoneExists(phone string) (bool, string, error) {
	if !c.cfg.Enabled {
		return false, "", ErrWAHADisabled
	}

	normalized := NormalizePhone(phone)
	payload := map[string]string{
		"session": c.cfg.Session,
		"phone":   normalized,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return false, "", fmt.Errorf("marshal payload: %w", err)
	}

	url := fmt.Sprintf("%s/api/contacts/check-exists", c.cfg.APIURL)
	resp, err := c.doRequest("POST", url, body)
	if err != nil {
		return false, "", fmt.Errorf("check exists: %w", err)
	}
	defer resp.Body.Close()

	var result CheckExistsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, "", fmt.Errorf("decode check exists: %w", err)
	}

	return result.NumberExists, result.ChatID, nil
}

func (c *WAHAClient) GetDailyStats() *DailyStats {
	return &DailyStats{
		SentToday:  0,
		DailyLimit: c.cfg.MaxDailyMessages,
	}
}

func (c *WAHAClient) doRequest(method string, url string, body []byte) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.cfg.APIKey != "" {
		req.Header.Set("X-Api-Key", c.cfg.APIKey)
	}

	return c.httpClient.Do(req)
}
