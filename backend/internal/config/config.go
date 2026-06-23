package config

import (
	"os"
	"strings"
)

type Config struct {
	Host                 string
	Port                 string
	DevAuthToken         string
	DevUserID            string
	DevTenantID          string
	DatabaseURL          string
	NeonConnectionString string
	CORSAllowedOrigins   []string
}

func Load() Config {
	return Config{
		Host:                 env("CODEGYM_HOST", "127.0.0.1"),
		Port:                 env("CODEGYM_PORT", "8080"),
		DevAuthToken:         os.Getenv("CODEGYM_DEV_AUTH_TOKEN"),
		DevUserID:            env("CODEGYM_DEV_USER_ID", "dev-user"),
		DevTenantID:          env("CODEGYM_DEV_TENANT_ID", "personal-dev"),
		DatabaseURL:          os.Getenv("DATABASE_URL"),
		NeonConnectionString: os.Getenv("NEON_CONNECTION_STRING"),
		CORSAllowedOrigins: csvEnv("CODEGYM_CORS_ALLOWED_ORIGINS", []string{
			"http://localhost:3000",
			"http://127.0.0.1:3000",
			"http://localhost:5173",
			"http://127.0.0.1:5173",
		}),
	}
}

func (c Config) Addr() string {
	return c.Host + ":" + c.Port
}

func (c Config) ReadinessDBDSN() string {
	if c.NeonConnectionString != "" {
		return c.NeonConnectionString
	}
	return c.DatabaseURL
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
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
