package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Host        string
	Port        int
	DBPath      string
	ProblemsDir string
	JWTSecret   string

	AnthropicAPIKey string

	MaxConcurrentExecutions int
	DefaultTimeoutSeconds   int
	DefaultMemoryMB         int
}

func Load() (*Config, error) {
	cfg := &Config{
		Host:                    envOrDefault("CODEGYM_HOST", "0.0.0.0"),
		Port:                    envIntOrDefault("CODEGYM_PORT", 8080),
		DBPath:                  envOrDefault("CODEGYM_DB_PATH", "./data/codegym.db"),
		ProblemsDir:             envOrDefault("CODEGYM_PROBLEMS_DIR", "./problems"),
		JWTSecret:               envOrDefault("CODEGYM_JWT_SECRET", ""),
		AnthropicAPIKey:         os.Getenv("ANTHROPIC_API_KEY"),
		MaxConcurrentExecutions: envIntOrDefault("CODEGYM_MAX_CONCURRENT_EXECUTIONS", 4),
		DefaultTimeoutSeconds:   envIntOrDefault("CODEGYM_DEFAULT_TIMEOUT_SECONDS", 30),
		DefaultMemoryMB:         envIntOrDefault("CODEGYM_DEFAULT_MEMORY_MB", 256),
	}

	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("CODEGYM_JWT_SECRET is required")
	}

	return cfg, nil
}

func (c *Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func envIntOrDefault(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return defaultVal
}
