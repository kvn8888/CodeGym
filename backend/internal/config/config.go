package config

import (
	"os"
	"strings"
	"time"
)

// Config contains process configuration loaded from environment variables.
type Config struct {
	Host               string
	Port               string
	DatabaseURL        string
	AuthMode           string
	Auth0Domain        string
	Auth0IssuerURL     string
	Auth0Audience      string
	Auth0ClockSkew     time.Duration
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
		Host:           env("CODEGYM_HOST", "127.0.0.1"),
		Port:           env("CODEGYM_PORT", "8080"),
		DatabaseURL:    databaseURL,
		AuthMode:       strings.ToLower(strings.TrimSpace(os.Getenv("CODEGYM_AUTH_MODE"))),
		Auth0Domain:    firstEnv("CODEGYM_AUTH0_DOMAIN", "AUTH0_DOMAIN"),
		Auth0IssuerURL: firstEnv("CODEGYM_AUTH0_ISSUER_URL", "AUTH0_ISSUER_URL"),
		Auth0Audience:  firstEnv("CODEGYM_AUTH0_AUDIENCE", "AUTH0_AUDIENCE"),
		Auth0ClockSkew: durationEnv("CODEGYM_AUTH0_CLOCK_SKEW", 0),
		DevAuthToken:   os.Getenv("CODEGYM_DEV_AUTH_TOKEN"),
		DevUserID:      env("CODEGYM_DEV_USER_ID", "dev-user"),
		DevTenantID:    env("CODEGYM_DEV_TENANT_ID", "personal-dev"),
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

func (c Config) UseAuth0() bool {
	if c.AuthMode == "auth0" {
		return true
	}
	return c.AuthMode == "" && c.Auth0Audience != "" && (c.Auth0Domain != "" || c.Auth0IssuerURL != "")
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		value := strings.TrimSpace(os.Getenv(key))
		if value != "" {
			return value
		}
	}
	return ""
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return duration
}

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
