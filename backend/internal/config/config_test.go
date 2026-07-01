package config

import (
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
		"CODEGYM_CORS_ALLOWED_ORIGINS",
	} {
		t.Setenv(key, "")
	}
}
