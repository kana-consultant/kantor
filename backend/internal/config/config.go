package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	AppEnv            string
	Port              string
	DatabaseURL       string
	JWTSecret         string
	DataEncryptionKey string
	UploadsDir        string
	JWTAccessExpiry   time.Duration
	JWTRefreshExpiry  time.Duration
	CORSOrigins       []string
	SeedSuperAdmin    SeedSuperAdminConfig
	SeedDemoUsers     SeedDemoUsersConfig
}

type SeedSuperAdminConfig struct {
	Enabled  bool
	Email    string
	Password string
	FullName string
}

type SeedDemoUsersConfig struct {
	Enabled bool
	Staff   SeedUserConfig
	Viewer  SeedUserConfig
}

type SeedUserConfig struct {
	Email      string
	Password   string
	FullName   string
	Department string
	Skills     []string
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

	jwtSecret := getEnv("JWT_SECRET", "change-me")
	dataEncryptionKey := getEnv("DATA_ENCRYPTION_KEY", jwtSecret)

	seedEnabled, err := parseBool("SEED_SUPERADMIN_ENABLED", appEnv != "production")
	if err != nil {
		return Config{}, err
	}

	demoUsersEnabled, err := parseBool("SEED_DEMO_USERS_ENABLED", appEnv != "production")
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		AppEnv:            appEnv,
		Port:              getEnv("PORT", "8080"),
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		JWTSecret:         jwtSecret,
		DataEncryptionKey: dataEncryptionKey,
		UploadsDir:        getEnv("UPLOADS_DIR", "uploads"),
		JWTAccessExpiry:   accessExpiry,
		JWTRefreshExpiry:  refreshExpiry,
		CORSOrigins:       splitCSV(getEnv("CORS_ORIGINS", "http://localhost:3000")),
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
		},
	}

	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
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

func getEnv(key string, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && strings.TrimSpace(value) != "" {
		return value
	}

	return fallback
}
