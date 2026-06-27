package config

import (
	"os"
	"strings"
)

// Config contains process configuration loaded from environment variables.
type Config struct {
	Host               string
	Port               string
	DatabaseURL        string
	DevAuthToken       string
	DevUserID          string
	DevTenantID        string
	CORSAllowedOrigins []string
}

// Load reads environment variables and returns the effective runtime config.
//
// If both NEON_CONNECTION_STRING and DATABASE_URL are set, Neon is preferred.
func Load() Config {
	databaseURL := os.Getenv("NEON_CONNECTION_STRING")
	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
	}

	return Config{
		Host:         env("CODEGYM_HOST", "127.0.0.1"),
		Port:         env("CODEGYM_PORT", "8080"),
		DatabaseURL:  databaseURL,
		DevAuthToken: os.Getenv("CODEGYM_DEV_AUTH_TOKEN"),
		DevUserID:    env("CODEGYM_DEV_USER_ID", "dev-user"),
		DevTenantID:  env("CODEGYM_DEV_TENANT_ID", "personal-dev"),
		CORSAllowedOrigins: csvEnv("CODEGYM_CORS_ALLOWED_ORIGINS", []string{
			"http://localhost:3000",
			"http://127.0.0.1:3000",
			"http://localhost:5173",
			"http://127.0.0.1:5173",
		}),
	}
}

// Addr returns the listen address in host:port form.
func (c Config) Addr() string {
	return c.Host + ":" + c.Port
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

// csvEnv parses a comma-separated env var into a trimmed string slice.
func csvEnv(key string, fallback []string) []string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}

	if len(out) == 0 {
		return fallback
	}
	return out
}
