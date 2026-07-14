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

	t.Setenv("CODEGYM_MEMORY_WORKER_DISABLED", "true")
	t.Setenv("CODEGYM_MEMORY_WORKER_INTERVAL", "15m")

	cfg = Load()
	if !cfg.MemoryWorker.Disabled {
		t.Fatal("MemoryWorker.Disabled = false, want true")
	}
	if cfg.MemoryWorker.Interval != 15*time.Minute {
		t.Fatalf("MemoryWorker.Interval = %s, want 15m", cfg.MemoryWorker.Interval)
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
		"CODEGYM_DEV_TENANT_ID",
		"CODEGYM_HOST",
		"CODEGYM_PORT",
		"PORT",
		"CODEGYM_CORS_ALLOWED_ORIGINS",
		"CODEGYM_MEMORY_WORKER_DISABLED",
		"CODEGYM_MEMORY_WORKER_INTERVAL",
		"CODEGYM_GENAI_BASE_URL",
		"CODEGYM_GENAI_API_KEY",
		"AI_GATEWAY_API_KEY",
		"CODEGYM_GENAI_MODEL",
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
	if cfg.GenAI.BaseURL != "https://ai-gateway.vercel.sh/v1" {
		t.Fatalf("BaseURL = %q", cfg.GenAI.BaseURL)
	}
	if cfg.GenAI.Model == "" {
		t.Fatal("Model default is empty")
	}

	t.Setenv("AI_GATEWAY_API_KEY", "vercel-key")
	cfg = Load()
	if !cfg.GenAI.Enabled() || cfg.GenAI.APIKey != "vercel-key" {
		t.Fatalf("GenAI = %#v, want AI_GATEWAY_API_KEY alias honored", cfg.GenAI)
	}

	t.Setenv("CODEGYM_GENAI_API_KEY", "codegym-key")
	t.Setenv("CODEGYM_GENAI_BASE_URL", "https://example.test/v1")
	t.Setenv("CODEGYM_GENAI_MODEL", "custom-model")
	cfg = Load()
	if cfg.GenAI.APIKey != "codegym-key" {
		t.Fatalf("APIKey = %q, want CODEGYM_GENAI_API_KEY to win", cfg.GenAI.APIKey)
	}
	if cfg.GenAI.BaseURL != "https://example.test/v1" || cfg.GenAI.Model != "custom-model" {
		t.Fatalf("GenAI = %#v", cfg.GenAI)
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
