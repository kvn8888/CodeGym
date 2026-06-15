package config

import "os"

type Config struct {
	Host         string
	Port         string
	DevAuthToken string
	DevUserID    string
	DevTenantID  string
}

func Load() Config {
	return Config{
		Host:         env("CODEGYM_HOST", "127.0.0.1"),
		Port:         env("CODEGYM_PORT", "8080"),
		DevAuthToken: os.Getenv("CODEGYM_DEV_AUTH_TOKEN"),
		DevUserID:    env("CODEGYM_DEV_USER_ID", "dev-user"),
		DevTenantID:  env("CODEGYM_DEV_TENANT_ID", "personal-dev"),
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
