package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadAuth0Config(t *testing.T) {
	clearConfigEnv(t)

	t.Setenv("AUTH0_DOMAIN", "generic.us.auth0.com")
	t.Setenv("AUTH0_AUDIENCE", "https://api.generic.test")
	t.Setenv("CODEGYM_AUTH0_DOMAIN", "codegym.us.auth0.com")
	t.Setenv("CODEGYM_AUTH0_AUDIENCE", "https://api.codegym.test")
	t.Setenv("CODEGYM_AUTH0_ISSUER_URL", "https://login.codegym.test/")
	t.Setenv("CODEGYM_AUTH0_CLOCK_SKEW", "30s")

	cfg := Load()

	if cfg.Auth0Domain != "codegym.us.auth0.com" {
		t.Fatalf("Auth0Domain = %q", cfg.Auth0Domain)
	}
	if cfg.Auth0Audience != "https://api.codegym.test" {
		t.Fatalf("Auth0Audience = %q", cfg.Auth0Audience)
	}
	if cfg.Auth0IssuerURL != "https://login.codegym.test/" {
		t.Fatalf("Auth0IssuerURL = %q", cfg.Auth0IssuerURL)
	}
	if cfg.Auth0ClockSkew != 30*time.Second {
		t.Fatalf("Auth0ClockSkew = %s", cfg.Auth0ClockSkew)
	}
}

func TestLoadFallsBackToPlainAuth0Names(t *testing.T) {
	clearConfigEnv(t)

	t.Setenv("AUTH0_DOMAIN", "plain.us.auth0.com")
	t.Setenv("AUTH0_AUDIENCE", "https://api.plain.test")
	t.Setenv("AUTH0_ISSUER_URL", "https://issuer.plain.test/")

	cfg := Load()

	if cfg.Auth0Domain != "plain.us.auth0.com" {
		t.Fatalf("Auth0Domain = %q", cfg.Auth0Domain)
	}
	if cfg.Auth0Audience != "https://api.plain.test" {
		t.Fatalf("Auth0Audience = %q", cfg.Auth0Audience)
	}
	if cfg.Auth0IssuerURL != "https://issuer.plain.test/" {
		t.Fatalf("Auth0IssuerURL = %q", cfg.Auth0IssuerURL)
	}
}

func TestUseAuth0(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want bool
	}{
		{
			name: "explicit auth0 mode requires auth0 authenticator",
			cfg:  Config{AuthMode: "auth0"},
			want: true,
		},
		{
			name: "implicit auth0 with audience and domain",
			cfg:  Config{Auth0Audience: "https://api.codegym.test", Auth0Domain: "codegym.us.auth0.com"},
			want: true,
		},
		{
			name: "implicit auth0 with audience and issuer",
			cfg:  Config{Auth0Audience: "https://api.codegym.test", Auth0IssuerURL: "https://login.codegym.test/"},
			want: true,
		},
		{
			name: "dev fallback without auth0 config",
			cfg:  Config{},
			want: false,
		},
		{
			name: "audience alone is not enough",
			cfg:  Config{Auth0Audience: "https://api.codegym.test"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.UseAuth0(); got != tt.want {
				t.Fatalf("UseAuth0() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoadMemoryWorkerConfig(t *testing.T) {
	clearConfigEnv(t)

	cfg := Load()
	if cfg.MemoryWorker.Disabled {
		t.Fatal("MemoryWorker.Disabled default = true, want false")
	}
	if cfg.MemoryWorker.Interval != 24*time.Hour {
		t.Fatalf("MemoryWorker.Interval default = %s, want 24h", cfg.MemoryWorker.Interval)
	}
	if cfg.MemoryWorker.Trigger != MemoryRefreshBoth {
		t.Fatalf("MemoryWorker.Trigger default = %q, want both", cfg.MemoryWorker.Trigger)
	}
	if !cfg.MemoryWorker.DailyEnabled() || !cfg.MemoryWorker.SetCompletionEnabled() {
		t.Fatal("default memory refresh trigger should enable daily and set completion")
	}

	t.Setenv("CODEGYM_MEMORY_WORKER_DISABLED", "true")
	t.Setenv("CODEGYM_MEMORY_WORKER_INTERVAL", "15m")
	t.Setenv("CODEGYM_MEMORY_REFRESH_TRIGGER", "set-completion")

	cfg = Load()
	if !cfg.MemoryWorker.Disabled {
		t.Fatal("MemoryWorker.Disabled = false, want true")
	}
	if cfg.MemoryWorker.Interval != 15*time.Minute {
		t.Fatalf("MemoryWorker.Interval = %s, want 15m", cfg.MemoryWorker.Interval)
	}
	if cfg.MemoryWorker.Trigger != MemoryRefreshSetCompletion || !cfg.MemoryWorker.SetCompletionEnabled() {
		t.Fatalf("MemoryWorker.Trigger = %q, want set-completion", cfg.MemoryWorker.Trigger)
	}
	if cfg.MemoryWorker.DailyEnabled() {
		t.Fatal("disabled worker must not enable daily refresh")
	}

	t.Setenv("CODEGYM_MEMORY_WORKER_DISABLED", "false")
	t.Setenv("CODEGYM_MEMORY_REFRESH_TRIGGER", "invalid")
	if got := Load().MemoryWorker.Trigger; got != MemoryRefreshBoth {
		t.Fatalf("invalid MemoryWorker.Trigger = %q, want both fallback", got)
	}
}

func TestLoadExecutionAndDemoConfig(t *testing.T) {
	clearConfigEnv(t)

	if cfg := Load(); cfg.SeedDemo || cfg.DaytonaAPIKey != "" || cfg.DaytonaAPIURL != "" {
		t.Fatalf("default execution/demo config = %#v", cfg)
	}

	t.Setenv("CODEGYM_SEED_DEMO", "true")
	t.Setenv("DAYTONA_API_KEY", "test-key")
	t.Setenv("DAYTONA_API_URL", "https://example.test")
	cfg := Load()
	if !cfg.SeedDemo || cfg.DaytonaAPIKey != "test-key" ||
		cfg.DaytonaAPIURL != "https://example.test" {
		t.Fatalf("execution/demo config = %#v", cfg)
	}
}

func TestLoadSandboxSweeperConfig(t *testing.T) {
	clearConfigEnv(t)

	cfg := Load()
	if cfg.SandboxSweeper.Disabled || cfg.SandboxSweeper.Interval != 5*time.Minute ||
		cfg.SandboxSweeper.MaxAge != 15*time.Minute {
		t.Fatalf("default sandbox sweeper config = %#v", cfg.SandboxSweeper)
	}

	t.Setenv("CODEGYM_SANDBOX_SWEEPER_DISABLED", "true")
	t.Setenv("CODEGYM_SANDBOX_SWEEPER_INTERVAL", "2m")
	t.Setenv("CODEGYM_SANDBOX_SWEEPER_MAX_AGE", "30m")
	cfg = Load()
	if !cfg.SandboxSweeper.Disabled || cfg.SandboxSweeper.Interval != 2*time.Minute ||
		cfg.SandboxSweeper.MaxAge != 30*time.Minute {
		t.Fatalf("configured sandbox sweeper = %#v", cfg.SandboxSweeper)
	}
}

func TestLoadRelayConfig(t *testing.T) {
	clearConfigEnv(t)

	cfg := Load()
	if cfg.Relay.Enabled() {
		t.Fatal("relay should be disabled without a token secret")
	}
	if cfg.Relay.TokenTTL != 15*time.Minute || cfg.Relay.MaxWallClock != 10*time.Minute ||
		cfg.Relay.MaxTotalTokens != 100_000 || cfg.Relay.MaxCostUSDMicros != 5_000_000 ||
		cfg.Relay.PublicModel != "codegym-agent" {
		t.Fatalf("unexpected relay defaults: %#v", cfg.Relay)
	}

	t.Setenv("CODEGYM_RELAY_TOKEN_SECRET", "test-secret")
	t.Setenv("CODEGYM_RELAY_TOKEN_TTL", "7m")
	t.Setenv("CODEGYM_RELAY_MAX_TOTAL_TOKENS", "12345")
	t.Setenv("CODEGYM_RELAY_MAX_COST_USD", "1.25")
	t.Setenv("CODEGYM_RELAY_MAX_WALL_CLOCK", "6m")
	t.Setenv("CODEGYM_RELAY_MODEL", "sandbox-model")
	cfg = Load()
	if !cfg.Relay.Enabled() || cfg.Relay.TokenSecret != "test-secret" ||
		cfg.Relay.TokenTTL != 7*time.Minute || cfg.Relay.MaxTotalTokens != 12_345 ||
		cfg.Relay.MaxCostUSDMicros != 1_250_000 || cfg.Relay.MaxWallClock != 6*time.Minute ||
		cfg.Relay.PublicModel != "sandbox-model" {
		t.Fatalf("unexpected relay config: %#v", cfg.Relay)
	}
}

func TestMemoryRefreshTriggerModes(t *testing.T) {
	tests := []struct {
		trigger        string
		wantDaily      bool
		wantCompletion bool
	}{
		{trigger: MemoryRefreshDaily, wantDaily: true},
		{trigger: MemoryRefreshSetCompletion, wantCompletion: true},
		{trigger: MemoryRefreshBoth, wantDaily: true, wantCompletion: true},
	}
	for _, test := range tests {
		t.Run(test.trigger, func(t *testing.T) {
			cfg := WorkerConfig{Trigger: test.trigger}
			if cfg.DailyEnabled() != test.wantDaily || cfg.SetCompletionEnabled() != test.wantCompletion {
				t.Fatalf("daily=%t completion=%t", cfg.DailyEnabled(), cfg.SetCompletionEnabled())
			}
		})
	}
}

func clearConfigEnv(t *testing.T) {
	t.Helper()

	for _, key := range []string{
		"NEON_CONNECTION_STRING",
		"DATABASE_URL",
		"CODEGYM_AUTH_MODE",
		"CODEGYM_AUTH0_DOMAIN",
		"AUTH0_DOMAIN",
		"CODEGYM_AUTH0_ISSUER_URL",
		"AUTH0_ISSUER_URL",
		"CODEGYM_AUTH0_AUDIENCE",
		"AUTH0_AUDIENCE",
		"CODEGYM_AUTH0_CLOCK_SKEW",
		"CODEGYM_DEV_AUTH_TOKEN",
		"CODEGYM_DEV_USER_ID",
		"CODEGYM_DEV_WORKSPACE_ID",
		"CODEGYM_HOST",
		"CODEGYM_PORT",
		"PORT",
		"CODEGYM_CORS_ALLOWED_ORIGINS",
		"CODEGYM_MEMORY_WORKER_DISABLED",
		"CODEGYM_MEMORY_WORKER_INTERVAL",
		"CODEGYM_MEMORY_REFRESH_TRIGGER",
		"CODEGYM_SEED_DEMO",
		"DAYTONA_API_KEY",
		"DAYTONA_API_URL",
		"CODEGYM_SANDBOX_SWEEPER_DISABLED",
		"CODEGYM_SANDBOX_SWEEPER_INTERVAL",
		"CODEGYM_SANDBOX_SWEEPER_MAX_AGE",
		"CODEGYM_GENAI_BASE_URL",
		"CODEGYM_GENAI_API_KEY",
		"AI_GATEWAY_API_KEY",
		"CODEGYM_GENAI_MODEL",
		"CODEGYM_GENAI_PROVIDER_ORDER",
		"CODEGYM_GENAI_META_API_KEY",
		"META_MUSE_SPARK_API",
		"CODEGYM_GENAI_META_BASE_URL",
		"CODEGYM_GENAI_META_MODEL",
		"CODEGYM_GENAI_META_MAX_TOKENS",
		"CODEGYM_GENAI_AZURE_API_KEY",
		"CODEGYM_GENAI_AZURE_BASE_URL",
		"CODEGYM_GENAI_AZURE_MODEL",
		"CODEGYM_GENAI_AZURE_API_VERSION",
		"CODEGYM_GEMINI_API_KEY",
		"CODEGYM_GEMINI_BASE_URL",
		"CODEGYM_GEMINI_MODEL",
		"CODEGYM_GENAI_GEMINI_API_KEY",
		"CODEGYM_GENAI_GEMINI_BASE_URL",
		"CODEGYM_GENAI_GEMINI_MODEL",
		"CODEGYM_RELAY_TOKEN_SECRET",
		"CODEGYM_RELAY_TOKEN_TTL",
		"CODEGYM_RELAY_MAX_TOTAL_TOKENS",
		"CODEGYM_RELAY_MAX_COST_USD",
		"CODEGYM_RELAY_MAX_WALL_CLOCK",
		"CODEGYM_RELAY_MODEL",
	} {
		t.Setenv(key, "")
	}
}

func TestLoadPortFallsBackToPlatformPort(t *testing.T) {
	clearConfigEnv(t)

	t.Setenv("PORT", "10000")
	if got := Load().Port; got != "10000" {
		t.Fatalf("Port = %q, want platform PORT 10000", got)
	}

	t.Setenv("CODEGYM_PORT", "9999")
	if got := Load().Port; got != "9999" {
		t.Fatalf("Port = %q, want CODEGYM_PORT to win", got)
	}
}

func TestLoadPortDefault(t *testing.T) {
	clearConfigEnv(t)

	if got := Load().Port; got != "8080" {
		t.Fatalf("Port = %q, want default 8080", got)
	}
}

func TestLoadGenAIConfig(t *testing.T) {
	clearConfigEnv(t)

	cfg := Load()
	if cfg.GenAI.Enabled() {
		t.Fatal("GenAI should be disabled without an API key")
	}
	if cfg.AnyGenAIEnabled() {
		t.Fatal("multi-provider registry should be empty without keys")
	}
	if cfg.GenAI.BaseURL != "https://generativelanguage.googleapis.com/v1beta/openai" {
		t.Fatalf("BaseURL = %q", cfg.GenAI.BaseURL)
	}
	if cfg.GenAI.Model != "gemini-flash-latest" {
		t.Fatalf("Model = %q, want gemini-flash-latest default", cfg.GenAI.Model)
	}

	t.Setenv("AI_GATEWAY_API_KEY", "vercel-key")
	cfg = Load()
	if !cfg.GenAI.Enabled() || cfg.GenAI.APIKey != "vercel-key" {
		t.Fatalf("GenAI = %#v, want AI_GATEWAY_API_KEY alias honored", cfg.GenAI)
	}
	if !cfg.AnyGenAIEnabled() || len(cfg.GenAIProviders) != 1 || cfg.GenAIProviders[0].Name != "gemini" {
		t.Fatalf("providers = %#v, want single gemini from legacy key", cfg.GenAIProviders)
	}

	t.Setenv("CODEGYM_GEMINI_API_KEY", "gemini-key")
	t.Setenv("CODEGYM_GEMINI_BASE_URL", "https://example.test/v1")
	t.Setenv("CODEGYM_GEMINI_MODEL", "custom-model")
	cfg = Load()
	if cfg.GenAI.APIKey != "gemini-key" {
		t.Fatalf("APIKey = %q, want CODEGYM_GEMINI_API_KEY to win", cfg.GenAI.APIKey)
	}
	if cfg.GenAI.BaseURL != "https://example.test/v1" || cfg.GenAI.Model != "custom-model" {
		t.Fatalf("GenAI = %#v", cfg.GenAI)
	}
}

func TestLoadMultiProviderOrderAndAliases(t *testing.T) {
	clearConfigEnv(t)

	t.Setenv("META_MUSE_SPARK_API", "meta-key")
	t.Setenv("CODEGYM_GENAI_AZURE_API_KEY", "azure-key")
	t.Setenv("CODEGYM_GENAI_AZURE_BASE_URL", "https://ex.openai.azure.com/openai/deployments/gpt-4o")
	t.Setenv("CODEGYM_GEMINI_API_KEY", "gemini-key")
	t.Setenv("CODEGYM_GEMINI_BASE_URL", "https://generativelanguage.googleapis.com/v1beta/openai")
	t.Setenv("CODEGYM_GEMINI_MODEL", "gemini-flash-latest")

	cfg := Load()
	if !cfg.AnyGenAIEnabled() {
		t.Fatal("expected providers")
	}
	if len(cfg.GenAIProviders) != 3 {
		t.Fatalf("providers = %#v, want meta,azure,gemini", cfg.GenAIProviders)
	}
	if cfg.GenAIProviders[0].Name != "meta" || cfg.GenAIProviders[0].APIKey != "meta-key" {
		t.Fatalf("meta = %#v", cfg.GenAIProviders[0])
	}
	if cfg.GenAIProviders[0].Model != "muse-spark-1.1" || cfg.GenAIProviders[0].DefaultMaxTokens != 4096 {
		t.Fatalf("meta defaults = %#v", cfg.GenAIProviders[0])
	}
	if cfg.GenAIProviders[1].Name != "azure" || cfg.GenAIProviders[1].AuthStyle != "azure_api_key" {
		t.Fatalf("azure = %#v", cfg.GenAIProviders[1])
	}
	if cfg.GenAIProviders[1].Model != "gpt-4o" {
		t.Fatalf("azure model = %q, want deployment path segment", cfg.GenAIProviders[1].Model)
	}
	if cfg.GenAIProviders[2].Name != "gemini" || cfg.GenAIProviders[2].APIKey != "gemini-key" {
		t.Fatalf("gemini = %#v", cfg.GenAIProviders[2])
	}

	// Azure without key is skipped; meta still first.
	clearConfigEnv(t)
	t.Setenv("META_MUSE_SPARK_API", "meta-key")
	t.Setenv("CODEGYM_GEMINI_API_KEY", "gemini-key")
	cfg = Load()
	if len(cfg.GenAIProviders) != 2 || cfg.GenAIProviders[0].Name != "meta" || cfg.GenAIProviders[1].Name != "gemini" {
		t.Fatalf("providers = %#v", cfg.GenAIProviders)
	}
}

func TestGenAIPairingWarning(t *testing.T) {
	cases := []struct {
		name    string
		cfg     GenAIConfig
		want    bool
		wantSub string
	}{
		{
			name: "google direct warns for gateway slug",
			cfg: GenAIConfig{
				BaseURL: "https://generativelanguage.googleapis.com/v1beta/openai",
				Model:   "google/gemini-2.5-flash",
			},
			want:    true,
			wantSub: "bare Gemini model slug",
		},
		{
			name: "gateway warns for bare slug",
			cfg: GenAIConfig{
				BaseURL: "https://ai-gateway.vercel.sh/v1",
				Model:   "gemini-flash-latest",
			},
			want:    true,
			wantSub: "vendor-prefixed model slug",
		},
		{
			name: "google direct accepts bare slug",
			cfg: GenAIConfig{
				BaseURL: "https://generativelanguage.googleapis.com/v1beta/openai",
				Model:   "gemini-flash-latest",
			},
			want: false,
		},
		{
			name: "gateway accepts vendor slug",
			cfg: GenAIConfig{
				BaseURL: "https://ai-gateway.vercel.sh/v1",
				Model:   "google/gemini-2.5-flash",
			},
			want: false,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := testCase.cfg.PairingWarning()
			if (got != "") != testCase.want {
				t.Fatalf("PairingWarning() = %q, want warning=%v", got, testCase.want)
			}
			if testCase.wantSub != "" && !strings.Contains(got, testCase.wantSub) {
				t.Fatalf("PairingWarning() = %q, want substring %q", got, testCase.wantSub)
			}
		})
	}
}
