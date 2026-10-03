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
	TrustedProxyCIDRs         []string
	TrackerRetentionDays      int
	AppURL                    string
	Tenants                   []TenantConfig
	WAHADefaults              WAHADefaultsConfig
	// RawAppEnv is APP_ENV exactly as set (trimmed), without the
	// "development" default AppEnv falls back to. Security-relevant dev
	// switches require it to be set explicitly.
	RawAppEnv string
	// DocumentMailDevSMTPAddr is the raw DOCUMENT_MAIL_DEV_SMTP_ADDR value.
	// Read it through DocumentMailCapture, which only captures with an
	// explicit APP_ENV=development.
	DocumentMailDevSMTPAddr string
	// Documents holds the document engine (DOCX render + LibreOffice PDF)
	// settings.
	Documents DocumentsConfig
	// DataMaintenance holds the opt-in, irreversible clean-ups of existing
	// rows. Both are off unless explicitly enabled.
	DataMaintenance DataMaintenanceConfig
}

// DataMaintenanceConfig switches on clean-ups that rewrite existing rows and
// cannot be undone. They are off by default so that starting a new release
// never changes existing data; enable them only after a verified backup
// (docs/deployment.md, "First deploy of the HR documents feature").
type DataMaintenanceConfig struct {
	// BankAccountClearPlaintext (BANK_ACCOUNT_CLEAR_PLAINTEXT) clears
	// employees.bank_account_number once the encrypted copy is stored, and
	// makes writes store the ciphertext only. Off: both columns are kept
	// and written.
	BankAccountClearPlaintext bool
	// AuditScrubExisting (AUDIT_SCRUB_EXISTING) redacts the denylisted keys
	// in audit_logs rows written before this release. New rows are always
	// redacted on insert, whatever this says.
	AuditScrubExisting bool
	// Invalid lists the variables above whose value is not a boolean; they
	// are treated as off (and logged at startup) instead of failing the
	// start.
	Invalid []string
}

// DocumentsConfig configures PDF conversion of generated documents (payslips,
// contracts). An empty SofficeBin disables PDF: documents can still be
// generated and downloaded as DOCX, but sending returns 409 unless
// AllowDocxSend is set.
type DocumentsConfig struct {
	// SofficeBin is the resolved LibreOffice soffice executable: SOFFICE_BIN
	// when it points at an executable file, otherwise auto-detected when
	// SOFFICE_BIN is unset. Empty when PDF is disabled.
	SofficeBin string
	// SofficeSource is how SofficeBin was resolved (SofficeSource*).
	SofficeSource string
	// SofficeRequested is the raw SOFFICE_BIN value, for the startup log.
	SofficeRequested string
	// SofficeInvalidReason explains SofficeSourceEnvInvalid.
	SofficeInvalidReason string
	// RenderTimeout bounds one soffice call (DOCUMENT_RENDER_TIMEOUT).
	RenderTimeout time.Duration
	// LOProfileDir is the persistent LibreOffice profile reused across jobs:
	// DOCUMENT_LO_PROFILE_DIR, or a per-user cache dir per port when unset.
	LOProfileDir string
	// LOProfileDirDefaulted is true when LOProfileDir is the default.
	LOProfileDirDefaulted bool
	// AllowDocxSend lets documents be emailed as DOCX when no PDF exists
	// (DOCUMENTS_ALLOW_DOCX_SEND, default false).
	AllowDocxSend bool
}

// PDFEnabled reports whether a usable soffice binary was resolved.
func (d DocumentsConfig) PDFEnabled() bool {
	return strings.TrimSpace(d.SofficeBin) != ""
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

	port := getEnv("PORT", "8080")
	documents, err := loadDocumentsConfig(port, systemFileLookup())
	if err != nil {
		return Config{}, err
	}
	dataMaintenance := loadDataMaintenanceConfig()

	cfg := Config{
		AppEnv:                    appEnv,
		Port:                      port,
		DatabaseURL:               os.Getenv("DATABASE_URL"),
		JWTSecret:                 jwtSecret,
		DataEncryptionKey:         dataEncryptionKey,
		DataEncryptionKeyPrevious: dataEncryptionKeyPrevious,
		UploadsDir:                getEnv("UPLOADS_DIR", "uploads"),
		JWTAccessExpiry:           accessExpiry,
		JWTRefreshExpiry:          refreshExpiry,
		CORSOrigins:               splitCSV(getEnv("CORS_ORIGINS", "http://localhost:3000")),
		TrustedProxyCIDRs: splitCSV(getEnv("TRUSTED_PROXY_CIDRS",
			"127.0.0.1/32,::1/128,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,169.254.0.0/16,fc00::/7,fe80::/10")),
		TrackerRetentionDays: parseIntEnv("TRACKER_RETENTION_DAYS", 90),
		AppURL:               getEnv("APP_URL", "http://localhost:3000"),
		Tenants:              parseTenants(getEnv("TENANTS", "Default|default|localhost")),
		WAHADefaults:         wahaDefaults,

		RawAppEnv:               strings.TrimSpace(os.Getenv("APP_ENV")),
		DocumentMailDevSMTPAddr: strings.TrimSpace(os.Getenv("DOCUMENT_MAIL_DEV_SMTP_ADDR")),
		Documents:               documents,
		DataMaintenance:         dataMaintenance,
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

// DocumentMailCapture returns where document email goes. With APP_ENV
// explicitly set to development (an unset or blank APP_ENV does not count,
// even though AppEnv defaults to development) it is captured by a local SMTP
// server — DefaultDevSMTPAddr (Mailpit) unless DOCUMENT_MAIL_DEV_SMTP_ADDR
// names another host:port or switches capture off. In every other APP_ENV
// Addr is empty and documents go to the constant smtp.gmail.com.
func (c Config) DocumentMailCapture() MailCapture {
	return resolveMailCapture(c.RawAppEnv, c.DocumentMailDevSMTPAddr)
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

// loadDataMaintenanceConfig reads the opt-in clean-up switches. A value that
// is not a boolean counts as off: these switches only ever destroy data, so
// a typo must neither enable them nor stop the server from starting.
func loadDataMaintenanceConfig() DataMaintenanceConfig {
	var cfg DataMaintenanceConfig
	optIn := func(key string) bool {
		enabled, err := parseBool(key, false)
		if err != nil {
			cfg.Invalid = append(cfg.Invalid, key)
			return false
		}
		return enabled
	}
	cfg.BankAccountClearPlaintext = optIn("BANK_ACCOUNT_CLEAR_PLAINTEXT")
	cfg.AuditScrubExisting = optIn("AUDIT_SCRUB_EXISTING")
	return cfg
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

const defaultDocumentRenderTimeout = 90 * time.Second

func loadDocumentsConfig(port string, lookup fileLookup) (DocumentsConfig, error) {
	timeout, err := parseDuration("DOCUMENT_RENDER_TIMEOUT", defaultDocumentRenderTimeout.String())
	if err != nil {
		return DocumentsConfig{}, err
	}
	if timeout <= 0 {
		return DocumentsConfig{}, errors.New("DOCUMENT_RENDER_TIMEOUT must be greater than zero")
	}

	allowDocxSend, err := parseBool("DOCUMENTS_ALLOW_DOCX_SEND", false)
	if err != nil {
		return DocumentsConfig{}, err
	}

	soffice := resolveSoffice(os.Getenv("SOFFICE_BIN"), lookup)
	profileDir := strings.TrimSpace(os.Getenv("DOCUMENT_LO_PROFILE_DIR"))
	profileDefaulted := profileDir == ""
	if profileDefaulted {
		profileDir = defaultLOProfileDir(port, os.UserCacheDir, os.TempDir)
	}

	return DocumentsConfig{
		SofficeBin:            soffice.Bin,
		SofficeSource:         soffice.Source,
		SofficeRequested:      soffice.Requested,
		SofficeInvalidReason:  soffice.Reason,
		RenderTimeout:         timeout,
		LOProfileDir:          profileDir,
		LOProfileDirDefaulted: profileDefaulted,
		AllowDocxSend:         allowDocxSend,
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
