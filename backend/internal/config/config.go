package config

import "os"

type Config struct {
	Host         string
	Port         string
	DatabaseURL  string
	DevAuthToken string
	DevUserID    string
	DevTenantID  string
	// Daytona sandbox credentials for the execution runner; both live in
	// Doppler (codegym/dev). Empty API key disables code execution.
	DaytonaAPIKey string
	DaytonaAPIURL string
}

func Load() Config {
	databaseURL := os.Getenv("NEON_CONNECTION_STRING")
	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
	}

	return Config{
		Host:          env("CODEGYM_HOST", "127.0.0.1"),
		Port:          env("CODEGYM_PORT", "8080"),
		DatabaseURL:   databaseURL,
		DevAuthToken:  os.Getenv("CODEGYM_DEV_AUTH_TOKEN"),
		DevUserID:     env("CODEGYM_DEV_USER_ID", "dev-user"),
		DevTenantID:   env("CODEGYM_DEV_TENANT_ID", "personal-dev"),
		DaytonaAPIKey: os.Getenv("DAYTONA_API_KEY"),
		DaytonaAPIURL: os.Getenv("DAYTONA_API_URL"),
	}
}

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
