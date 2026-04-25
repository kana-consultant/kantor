package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	AppEnv                    string
	Port                      string
	DatabaseURL               string
	JWTSecret                 string
	DataEncryptionKey         string
	DataEncryptionKeyPrevious string
	UploadsDir                string
	JWTAccessExpiry           time.Duration
	JWTRefreshExpiry          time.Duration
	CORSOrigins               []string
	TrackerRetentionDays      int
	AppURL                    string
	Tenants                   []TenantConfig
	WAHADefaults              WAHADefaultsConfig
}

// WAHADefaultsConfig holds the WAHA (WhatsApp HTTP API) values used to seed a
// tenant_wa_configs row when a tenant is first created. Existing rows are
// untouched — operators tune live values via the in-app WhatsApp settings page.
type WAHADefaultsConfig struct {
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

// TenantConfig describes one tenant seeded at startup.
type TenantConfig struct {
	Name    string
	Slug    string
	Domains []string // first domain = primary
}

func Load() (Config, error) {
	loadDotEnv()
	appEnv := getEnv("APP_ENV", "development")

	accessExpiry, err := parseDuration("JWT_ACCESS_EXPIRY", "15m")
	if err != nil {
		return Config{}, err
	}

	refreshExpiry, err := parseDuration("JWT_REFRESH_EXPIRY", "168h")
	if err != nil {
		return Config{}, err
	}

	jwtSecret := strings.TrimSpace(os.Getenv("JWT_SECRET"))
	dataEncryptionKey := strings.TrimSpace(os.Getenv("DATA_ENCRYPTION_KEY"))
	dataEncryptionKeyPrevious := strings.TrimSpace(os.Getenv("DATA_ENCRYPTION_KEY_PREVIOUS"))

	wahaDefaults, err := loadWAHADefaults()
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		AppEnv:                    appEnv,
		Port:                      getEnv("PORT", "8080"),
		DatabaseURL:               os.Getenv("DATABASE_URL"),
		JWTSecret:                 jwtSecret,
		DataEncryptionKey:         dataEncryptionKey,
		DataEncryptionKeyPrevious: dataEncryptionKeyPrevious,
		UploadsDir:                getEnv("UPLOADS_DIR", "uploads"),
		JWTAccessExpiry:           accessExpiry,
		JWTRefreshExpiry:          refreshExpiry,
		CORSOrigins:               splitCSV(getEnv("CORS_ORIGINS", "http://localhost:3000")),
		TrackerRetentionDays:      parseIntEnv("TRACKER_RETENTION_DAYS", 90),
		AppURL:                    getEnv("APP_URL", "http://localhost:3000"),
		Tenants:                   parseTenants(getEnv("TENANTS", "Default|default|localhost")),
		WAHADefaults:              wahaDefaults,
	}

	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}

	if cfg.JWTSecret == "" {
		return Config{}, errors.New("JWT_SECRET is required")
	}

	if cfg.DataEncryptionKey == "" {
		return Config{}, errors.New("DATA_ENCRYPTION_KEY is required")
	}

	if cfg.AppEnv == "production" {
		if len(cfg.JWTSecret) < 32 {
			return Config{}, errors.New("JWT_SECRET must be at least 32 characters in production")
		}
	}

	return cfg, nil
}

func loadDotEnv() {
	candidates := []string{
		".env",
		filepath.Join("..", ".env"),
		filepath.Join("..", "..", ".env"),
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			_ = godotenv.Overload(candidate)
		}
	}
}

func parseDuration(key string, fallback string) (time.Duration, error) {
	value := getEnv(key, fallback)

	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid duration for %s: %w", key, err)
	}

	return duration, nil
}

func parseBool(key string, fallback bool) (bool, error) {
	value, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback, nil
	}

	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean for %s", key)
	}
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	return result
}

func loadWAHADefaults() (WAHADefaultsConfig, error) {
	enabled, err := parseBool("WAHA_ENABLED", false)
	if err != nil {
		return WAHADefaultsConfig{}, err
	}

	maxDaily, err := parseIntEnvStrict("WAHA_MAX_DAILY_MESSAGES", 50)
	if err != nil {
		return WAHADefaultsConfig{}, err
	}
	minDelay, err := parseIntEnvStrict("WAHA_MIN_DELAY_MS", 2000)
	if err != nil {
		return WAHADefaultsConfig{}, err
	}
	maxDelay, err := parseIntEnvStrict("WAHA_MAX_DELAY_MS", 5000)
	if err != nil {
		return WAHADefaultsConfig{}, err
	}

	if maxDaily <= 0 {
		return WAHADefaultsConfig{}, errors.New("WAHA_MAX_DAILY_MESSAGES must be greater than zero")
	}
	if minDelay < 0 || maxDelay < 0 {
		return WAHADefaultsConfig{}, errors.New("WAHA_MIN_DELAY_MS and WAHA_MAX_DELAY_MS must be non-negative")
	}
	if minDelay > maxDelay {
		return WAHADefaultsConfig{}, errors.New("WAHA_MIN_DELAY_MS must be less than or equal to WAHA_MAX_DELAY_MS")
	}

	return WAHADefaultsConfig{
		APIURL:           getEnv("WAHA_API_URL", "http://localhost:3000"),
		APIKey:           os.Getenv("WAHA_API_KEY"),
		SessionName:      getEnv("WAHA_SESSION", "default"),
		Enabled:          enabled,
		MaxDailyMessages: maxDaily,
		MinDelayMS:       minDelay,
		MaxDelayMS:       maxDelay,
		ReminderCron:     getEnv("WAHA_REMINDER_CRON", "0 8 * * 1-5"),
		WeeklyDigestCron: getEnv("WAHA_WEEKLY_DIGEST_CRON", "0 8 * * 1"),
	}, nil
}

func parseIntEnvStrict(key string, fallback int) (int, error) {
	value, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("invalid integer for %s: %w", key, err)
	}
	return parsed, nil
}

func parseIntEnv(key string, fallback int) int {
	value, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnv(key string, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && strings.TrimSpace(value) != "" {
		return value
	}

	return fallback
}

// parseTenants parses the TENANTS env var.
// Format: "name|slug|domain1,domain2;name2|slug2|domain3,domain4"
func parseTenants(raw string) []TenantConfig {
	var tenants []TenantConfig
	for _, entry := range strings.Split(raw, ";") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, "|", 3)
		if len(parts) != 3 {
			continue
		}
		tenants = append(tenants, TenantConfig{
			Name:    strings.TrimSpace(parts[0]),
			Slug:    strings.TrimSpace(parts[1]),
			Domains: splitCSV(parts[2]),
		})
	}
	return tenants
}
