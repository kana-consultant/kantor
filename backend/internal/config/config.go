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
	SeedSuperAdmin            SeedSuperAdminConfig
	SeedDemoUsers             SeedDemoUsersConfig
	AppURL                    string
	Tenants                   []TenantConfig
}

type SeedSuperAdminConfig struct {
	Enabled  bool
	Email    string
	Password string
	FullName string
}

type SeedDemoUsersConfig struct {
	Enabled         bool
	Staff           SeedUserConfig
	Viewer          SeedUserConfig
	MarketingStaff  SeedUserConfig
	MarketingViewer SeedUserConfig
}

type SeedUserConfig struct {
	Email      string
	Password   string
	FullName   string
	Department string
	Skills     []string
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

	seedEnabled, err := parseBool("SEED_SUPERADMIN_ENABLED", false)
	if err != nil {
		return Config{}, err
	}

	demoUsersEnabled, err := parseBool("SEED_DEMO_USERS_ENABLED", false)
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
		SeedSuperAdmin: SeedSuperAdminConfig{
			Enabled:  seedEnabled,
			Email:    getEnv("SEED_SUPERADMIN_EMAIL", "superadmin@kantor.local"),
			Password: getEnv("SEED_SUPERADMIN_PASSWORD", "Password123!"),
			FullName: getEnv("SEED_SUPERADMIN_FULL_NAME", "Seeded Super Admin"),
		},
		SeedDemoUsers: SeedDemoUsersConfig{
			Enabled: demoUsersEnabled,
			Staff: SeedUserConfig{
				Email:      getEnv("SEED_STAFF_EMAIL", "staff.ops@kantor.local"),
				Password:   getEnv("SEED_STAFF_PASSWORD", "Password123!"),
				FullName:   getEnv("SEED_STAFF_FULL_NAME", "Operational Staff"),
				Department: getEnv("SEED_STAFF_DEPARTMENT", "engineering"),
				Skills:     splitCSV(getEnv("SEED_STAFF_SKILLS", "frontend,kanban")),
			},
			Viewer: SeedUserConfig{
				Email:      getEnv("SEED_VIEWER_EMAIL", "viewer.ops@kantor.local"),
				Password:   getEnv("SEED_VIEWER_PASSWORD", "Password123!"),
				FullName:   getEnv("SEED_VIEWER_FULL_NAME", "Operational Viewer"),
				Department: getEnv("SEED_VIEWER_DEPARTMENT", "finance"),
				Skills:     splitCSV(getEnv("SEED_VIEWER_SKILLS", "qa,reporting")),
			},
			MarketingStaff: SeedUserConfig{
				Email:      getEnv("SEED_MARKETING_STAFF_EMAIL", "staff.marketing@kantor.local"),
				Password:   getEnv("SEED_MARKETING_STAFF_PASSWORD", "Password123!"),
				FullName:   getEnv("SEED_MARKETING_STAFF_FULL_NAME", "Marketing Staff"),
				Department: getEnv("SEED_MARKETING_STAFF_DEPARTMENT", "marketing"),
				Skills:     splitCSV(getEnv("SEED_MARKETING_STAFF_SKILLS", "copywriting,ads,crm")),
			},
			MarketingViewer: SeedUserConfig{
				Email:      getEnv("SEED_MARKETING_VIEWER_EMAIL", "viewer.marketing@kantor.local"),
				Password:   getEnv("SEED_MARKETING_VIEWER_PASSWORD", "Password123!"),
				FullName:   getEnv("SEED_MARKETING_VIEWER_FULL_NAME", "Marketing Viewer"),
				Department: getEnv("SEED_MARKETING_VIEWER_DEPARTMENT", "marketing"),
				Skills:     splitCSV(getEnv("SEED_MARKETING_VIEWER_SKILLS", "reporting")),
			},
		},
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

	if cfg.SeedSuperAdmin.Enabled {
		if strings.TrimSpace(cfg.SeedSuperAdmin.Email) == "" {
			return Config{}, errors.New("SEED_SUPERADMIN_EMAIL is required when seed is enabled")
		}

		if strings.TrimSpace(cfg.SeedSuperAdmin.Password) == "" {
			return Config{}, errors.New("SEED_SUPERADMIN_PASSWORD is required when seed is enabled")
		}

		if strings.TrimSpace(cfg.SeedSuperAdmin.FullName) == "" {
			return Config{}, errors.New("SEED_SUPERADMIN_FULL_NAME is required when seed is enabled")
		}
	}

	if cfg.SeedDemoUsers.Enabled {
		if strings.TrimSpace(cfg.SeedDemoUsers.Staff.Email) == "" || strings.TrimSpace(cfg.SeedDemoUsers.Staff.Password) == "" || strings.TrimSpace(cfg.SeedDemoUsers.Staff.FullName) == "" {
			return Config{}, errors.New("SEED_STAFF_EMAIL, SEED_STAFF_PASSWORD, and SEED_STAFF_FULL_NAME are required when demo seeds are enabled")
		}

		if strings.TrimSpace(cfg.SeedDemoUsers.Viewer.Email) == "" || strings.TrimSpace(cfg.SeedDemoUsers.Viewer.Password) == "" || strings.TrimSpace(cfg.SeedDemoUsers.Viewer.FullName) == "" {
			return Config{}, errors.New("SEED_VIEWER_EMAIL, SEED_VIEWER_PASSWORD, and SEED_VIEWER_FULL_NAME are required when demo seeds are enabled")
		}

		if strings.TrimSpace(cfg.SeedDemoUsers.MarketingStaff.Email) == "" || strings.TrimSpace(cfg.SeedDemoUsers.MarketingStaff.Password) == "" || strings.TrimSpace(cfg.SeedDemoUsers.MarketingStaff.FullName) == "" {
			return Config{}, errors.New("SEED_MARKETING_STAFF_EMAIL, SEED_MARKETING_STAFF_PASSWORD, and SEED_MARKETING_STAFF_FULL_NAME are required when demo seeds are enabled")
		}

		if strings.TrimSpace(cfg.SeedDemoUsers.MarketingViewer.Email) == "" || strings.TrimSpace(cfg.SeedDemoUsers.MarketingViewer.Password) == "" || strings.TrimSpace(cfg.SeedDemoUsers.MarketingViewer.FullName) == "" {
			return Config{}, errors.New("SEED_MARKETING_VIEWER_EMAIL, SEED_MARKETING_VIEWER_PASSWORD, and SEED_MARKETING_VIEWER_FULL_NAME are required when demo seeds are enabled")
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
