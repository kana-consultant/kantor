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
	AppEnv           string
	Port             string
	DatabaseURL      string
	JWTSecret        string
	JWTAccessExpiry  time.Duration
	JWTRefreshExpiry time.Duration
	CORSOrigins      []string
	SeedSuperAdmin   SeedSuperAdminConfig
}

type SeedSuperAdminConfig struct {
	Enabled  bool
	Email    string
	Password string
	FullName string
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

	seedEnabled, err := parseBool("SEED_SUPERADMIN_ENABLED", appEnv != "production")
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		AppEnv:           appEnv,
		Port:             getEnv("PORT", "8080"),
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		JWTSecret:        getEnv("JWT_SECRET", "change-me"),
		JWTAccessExpiry:  accessExpiry,
		JWTRefreshExpiry: refreshExpiry,
		CORSOrigins:      splitCSV(getEnv("CORS_ORIGINS", "http://localhost:3000")),
		SeedSuperAdmin: SeedSuperAdminConfig{
			Enabled:  seedEnabled,
			Email:    getEnv("SEED_SUPERADMIN_EMAIL", "superadmin@kantor.local"),
			Password: getEnv("SEED_SUPERADMIN_PASSWORD", "Password123!"),
			FullName: getEnv("SEED_SUPERADMIN_FULL_NAME", "Seeded Super Admin"),
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
