package config

import (
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config contains process configuration loaded from environment variables.
type Config struct {
	Host           string
	Port           string
	DatabaseURL    string
	AuthMode       string
	Auth0Domain    string
	Auth0IssuerURL string
	Auth0Audience  string
	Auth0ClockSkew time.Duration
	DevAuthToken   string
	DevUserID      string
	DevWorkspaceID string
	// Daytona sandbox credentials for the execution runner; both come from
	// Doppler (codegym/dev). Empty API key disables code execution.
	DaytonaAPIKey      string
	DaytonaAPIURL      string
	SeedDemo           bool
	CORSAllowedOrigins []string
	MemoryWorker       WorkerConfig
	SandboxSweeper     SweeperConfig
	// GenAI is the legacy single-provider view (Gemini / CODEGYM_GENAI_*).
	// Prefer GenAIProviders for multi-provider routing.
	GenAI          GenAIConfig
	GenAIProviders []GenAIProviderConfig
	// GenAIProviderOrder is the configured priority list (may include names
	// that are not currently enabled).
	GenAIProviderOrder []string
	Relay              RelayConfig
}

// RelayConfig configures the sandbox-facing single-operation model relay.
type RelayConfig struct {
	TokenSecret      string
	TokenTTL         time.Duration
	MaxTotalTokens   int64
	MaxCostUSDMicros int64
	MaxWallClock     time.Duration
	PublicModel      string
}

// Enabled reports whether relay token signing is configured.
func (r RelayConfig) Enabled() bool {
	return strings.TrimSpace(r.TokenSecret) != ""
}

type WorkerConfig struct {
	Disabled bool
	Interval time.Duration
	Trigger  string
}

// SweeperConfig controls conservative cleanup of orphaned Daytona sandboxes.
type SweeperConfig struct {
	Disabled bool
	Interval time.Duration
	MaxAge   time.Duration
}

const (
	MemoryRefreshDaily         = "daily"
	MemoryRefreshSetCompletion = "set-completion"
	MemoryRefreshBoth          = "both"
)

func (w WorkerConfig) DailyEnabled() bool {
	return !w.Disabled && (w.Trigger == MemoryRefreshDaily || w.Trigger == MemoryRefreshBoth)
}

func (w WorkerConfig) SetCompletionEnabled() bool {
	return w.Trigger == MemoryRefreshSetCompletion || w.Trigger == MemoryRefreshBoth
}

// GenAIConfig is the legacy single OpenAI-compatible provider (Gemini path).
type GenAIConfig struct {
	BaseURL string
	APIKey  string
	Model   string
}

// GenAIProviderConfig is one named backend in the multi-provider registry.
type GenAIProviderConfig struct {
	Name             string
	BaseURL          string
	APIKey           string
	Model            string
	AuthStyle        string // "bearer" (default) or "azure_api_key"
	APIVersion       string // Azure api-version query
	DefaultMaxTokens int
}

// Enabled reports whether generation is configured. Without an API key the
// server still runs; POST /api/v1/generate responds 503.
func (g GenAIConfig) Enabled() bool {
	return strings.TrimSpace(g.APIKey) != ""
}

// AnyGenAIEnabled is true when at least one multi-provider entry has a key.
func (c Config) AnyGenAIEnabled() bool {
	return len(c.GenAIProviders) > 0
}

func (g GenAIConfig) PairingWarning() string {
	return pairingWarning(g.BaseURL, g.Model)
}

func (p GenAIProviderConfig) PairingWarning() string {
	return pairingWarning(p.BaseURL, p.Model)
}

func pairingWarning(baseURL, model string) string {
	baseURL = strings.ToLower(strings.TrimSpace(baseURL))
	model = strings.TrimSpace(model)
	if baseURL == "" || model == "" {
		return ""
	}

	modelHasVendorPrefix := strings.Contains(model, "/")
	switch {
	case strings.Contains(baseURL, "generativelanguage.googleapis.com") && modelHasVendorPrefix:
		return "GenAI config mismatch: Google direct OpenAI-compatible base URL expects a bare Gemini model slug such as gemini-flash-latest or gemini-2.5-flash; current model includes a vendor prefix."
	case strings.Contains(baseURL, "ai-gateway.vercel.sh") && !modelHasVendorPrefix:
		return "GenAI config mismatch: Vercel AI Gateway base URL expects a vendor-prefixed model slug such as google/gemini-2.5-flash; current model has no vendor prefix."
	default:
		return ""
	}
}

// DefaultGenAIProviderOrder spends Meta and Azure credits before Gemini.
var DefaultGenAIProviderOrder = []string{"meta", "azure", "gemini"}

// Load reads environment variables and returns the effective runtime config.
// If both NEON_CONNECTION_STRING and DATABASE_URL are set, Neon is preferred.
func Load() Config {
	databaseURL := os.Getenv("NEON_CONNECTION_STRING")
	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
	}

	// CODEGYM_PORT wins, but honor the PORT most PaaS runtimes (Render,
	// Heroku-style) inject so hosted deploys work without extra config.
	port := firstEnv("CODEGYM_PORT", "PORT")
	if port == "" {
		port = "8080"
	}

	// Gemini hop (renamed from CODEGYM_GENAI_*). Legacy CODEGYM_GENAI_* /
	// AI_GATEWAY_API_KEY still accepted as fallbacks during transition.
	legacyGenAI := GenAIConfig{
		BaseURL: firstNonEmpty(
			strings.TrimSpace(os.Getenv("CODEGYM_GEMINI_BASE_URL")),
			env("CODEGYM_GENAI_BASE_URL", "https://generativelanguage.googleapis.com/v1beta/openai"),
		),
		APIKey: firstEnv(
			"CODEGYM_GEMINI_API_KEY",
			"CODEGYM_GENAI_GEMINI_API_KEY",
			"CODEGYM_GENAI_API_KEY",
			"AI_GATEWAY_API_KEY",
		),
		Model: firstNonEmpty(
			strings.TrimSpace(os.Getenv("CODEGYM_GEMINI_MODEL")),
			env("CODEGYM_GENAI_MODEL", "gemini-flash-latest"),
		),
	}

	order := csvEnv("CODEGYM_GENAI_PROVIDER_ORDER", DefaultGenAIProviderOrder)
	providers := loadGenAIProviders(order, legacyGenAI)

	return Config{
		Host:           env("CODEGYM_HOST", "127.0.0.1"),
		Port:           port,
		DatabaseURL:    databaseURL,
		AuthMode:       strings.ToLower(strings.TrimSpace(os.Getenv("CODEGYM_AUTH_MODE"))),
		Auth0Domain:    firstEnv("CODEGYM_AUTH0_DOMAIN", "AUTH0_DOMAIN"),
		Auth0IssuerURL: firstEnv("CODEGYM_AUTH0_ISSUER_URL", "AUTH0_ISSUER_URL"),
		Auth0Audience:  firstEnv("CODEGYM_AUTH0_AUDIENCE", "AUTH0_AUDIENCE"),
		Auth0ClockSkew: durationEnv("CODEGYM_AUTH0_CLOCK_SKEW", 0),
		DevAuthToken:   os.Getenv("CODEGYM_DEV_AUTH_TOKEN"),
		DevUserID:      env("CODEGYM_DEV_USER_ID", "dev-user"),
		// Prefer CODEGYM_DEV_WORKSPACE_ID; fall back to legacy CODEGYM_DEV_TENANT_ID.
		DevWorkspaceID: firstEnvOr("personal-dev", "CODEGYM_DEV_WORKSPACE_ID", "CODEGYM_DEV_TENANT_ID"),
		DaytonaAPIKey:  os.Getenv("DAYTONA_API_KEY"),
		DaytonaAPIURL:  os.Getenv("DAYTONA_API_URL"),
		SeedDemo:       boolEnv("CODEGYM_SEED_DEMO", false),
		CORSAllowedOrigins: csvEnv("CODEGYM_CORS_ALLOWED_ORIGINS", []string{
			"http://localhost:3000",
			"http://127.0.0.1:3000",
			"http://localhost:5173",
			"http://127.0.0.1:5173",
		}),
		MemoryWorker: WorkerConfig{
			Disabled: boolEnv("CODEGYM_MEMORY_WORKER_DISABLED", false),
			Interval: durationEnv(
				"CODEGYM_MEMORY_WORKER_INTERVAL",
				24*time.Hour,
			),
			Trigger: memoryRefreshTriggerEnv(),
		},
		SandboxSweeper: SweeperConfig{
			Disabled: boolEnv("CODEGYM_SANDBOX_SWEEPER_DISABLED", false),
			Interval: positiveDurationEnv(
				"CODEGYM_SANDBOX_SWEEPER_INTERVAL",
				5*time.Minute,
			),
			MaxAge: positiveDurationEnv(
				"CODEGYM_SANDBOX_SWEEPER_MAX_AGE",
				15*time.Minute,
			),
		},
		GenAI:              legacyGenAI,
		GenAIProviders:     providers,
		GenAIProviderOrder: order,
		Relay: RelayConfig{
			TokenSecret:      firstEnv("CODEGYM_RELAY_TOKEN_SECRET"),
			TokenTTL:         positiveDurationEnv("CODEGYM_RELAY_TOKEN_TTL", 15*time.Minute),
			MaxTotalTokens:   int64Env("CODEGYM_RELAY_MAX_TOTAL_TOKENS", 100_000),
			MaxCostUSDMicros: usdMicrosEnv("CODEGYM_RELAY_MAX_COST_USD", 5_000_000),
			MaxWallClock:     positiveDurationEnv("CODEGYM_RELAY_MAX_WALL_CLOCK", 10*time.Minute),
			PublicModel:      env("CODEGYM_RELAY_MODEL", "codegym-agent"),
		},
	}
}

func memoryRefreshTriggerEnv() string {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("CODEGYM_MEMORY_REFRESH_TRIGGER")))
	switch value {
	case MemoryRefreshDaily, MemoryRefreshSetCompletion, MemoryRefreshBoth:
		return value
	default:
		return MemoryRefreshBoth
	}
}

func loadGenAIProviders(order []string, legacy GenAIConfig) []GenAIProviderConfig {
	// Candidate configs keyed by name (may lack API keys).
	candidates := map[string]GenAIProviderConfig{
		"meta": {
			Name:             "meta",
			BaseURL:          env("CODEGYM_GENAI_META_BASE_URL", "https://api.meta.ai/v1"),
			APIKey:           firstEnv("CODEGYM_GENAI_META_API_KEY", "META_MUSE_SPARK_API"),
			Model:            env("CODEGYM_GENAI_META_MODEL", "muse-spark-1.1"),
			AuthStyle:        "bearer",
			DefaultMaxTokens: intEnv("CODEGYM_GENAI_META_MAX_TOKENS", 4096),
		},
		"azure": {
			Name:             "azure",
			BaseURL:          strings.TrimSpace(os.Getenv("CODEGYM_GENAI_AZURE_BASE_URL")),
			APIKey:           strings.TrimSpace(os.Getenv("CODEGYM_GENAI_AZURE_API_KEY")),
			Model:            env("CODEGYM_GENAI_AZURE_MODEL", ""),
			AuthStyle:        "azure_api_key",
			APIVersion:       env("CODEGYM_GENAI_AZURE_API_VERSION", "2024-10-21-preview"),
			DefaultMaxTokens: intEnv("CODEGYM_GENAI_AZURE_MAX_TOKENS", 4096),
		},
		"gemini": {
			Name: "gemini",
			BaseURL: firstNonEmpty(
				strings.TrimSpace(os.Getenv("CODEGYM_GEMINI_BASE_URL")),
				strings.TrimSpace(os.Getenv("CODEGYM_GENAI_GEMINI_BASE_URL")),
				legacy.BaseURL,
			),
			APIKey: firstNonEmpty(
				firstEnv(
					"CODEGYM_GEMINI_API_KEY",
					"CODEGYM_GENAI_GEMINI_API_KEY",
					"CODEGYM_GENAI_API_KEY",
					"AI_GATEWAY_API_KEY",
				),
				legacy.APIKey,
			),
			Model: firstNonEmpty(
				strings.TrimSpace(os.Getenv("CODEGYM_GEMINI_MODEL")),
				strings.TrimSpace(os.Getenv("CODEGYM_GENAI_GEMINI_MODEL")),
				legacy.Model,
			),
			AuthStyle:        "bearer",
			DefaultMaxTokens: intEnv("CODEGYM_GEMINI_MAX_TOKENS", 2048),
		},
	}

	// Azure model defaults to last path segment of deployment URL when unset.
	if azure := candidates["azure"]; azure.APIKey != "" && azure.Model == "" && azure.BaseURL != "" {
		azure.Model = lastPathSegment(azure.BaseURL)
		candidates["azure"] = azure
	}

	out := make([]GenAIProviderConfig, 0, len(order))
	seen := map[string]bool{}
	for _, name := range order {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		candidate, ok := candidates[name]
		if !ok {
			continue
		}
		if strings.TrimSpace(candidate.APIKey) == "" {
			continue
		}
		if strings.TrimSpace(candidate.BaseURL) == "" || strings.TrimSpace(candidate.Model) == "" {
			continue
		}
		out = append(out, candidate)
	}
	return out
}

func lastPathSegment(raw string) string {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		return ""
	}
	if idx := strings.LastIndex(raw, "/"); idx >= 0 && idx+1 < len(raw) {
		return raw[idx+1:]
	}
	return raw
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func intEnv(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}

func int64Env(key string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func usdMicrosEnv(key string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || parsed <= 0 || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return fallback
	}
	return int64(parsed*1_000_000 + 0.5)
}

func positiveDurationEnv(key string, fallback time.Duration) time.Duration {
	duration := durationEnv(key, fallback)
	if duration <= 0 {
		return fallback
	}
	return duration
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

func firstEnvOr(fallback string, keys ...string) string {
	if value := firstEnv(keys...); value != "" {
		return value
	}
	return fallback
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

func boolEnv(key string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch value {
	case "":
		return fallback
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
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
