package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// mintHS256 builds a signed JWT for tests. This is the "issuer" side — your
// JWTAuthenticator.Authenticate is the "verifier" side. Read it to see exactly
// what bytes you will be verifying, then leave it alone.
func mintHS256(t *testing.T, secret string, claims map[string]any) string {
	t.Helper()
	enc := func(v any) string {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return base64.RawURLEncoding.EncodeToString(b)
	}
	signingInput := enc(map[string]any{"alg": "HS256", "typ": "JWT"}) + "." + enc(claims)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signingInput))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return signingInput + "." + sig
}

// TestJWTAuthenticator is your TDD harness. Implement Authenticate, delete the
// t.Skip line, and run:  go test ./internal/auth/ -run TestJWTAuthenticator -v
func TestJWTAuthenticator(t *testing.T) {
	t.Skip("Remove this skip once you implement JWTAuthenticator.Authenticate.")

	const secret = "test-secret"
	authn := NewJWTAuthenticator(secret)
	future := time.Now().Add(time.Hour).Unix()

	t.Run("valid token maps claims to principal", func(t *testing.T) {
		token := mintHS256(t, secret, map[string]any{
			"sub":            "user_123",
			"default_tenant": "t_personal",
			"tenants":        []string{"t_personal"},
			"exp":            future,
		})
		p, err := authn.Authenticate(context.Background(), token)
		if err != nil {
			t.Fatalf("expected valid token, got %v", err)
		}
		if p.UserID != "user_123" {
			t.Fatalf("UserID = %q, want user_123", p.UserID)
		}
		if !p.HasTenant("t_personal") {
			t.Fatal("expected principal to have access to t_personal")
		}
	})

	t.Run("tampered token is rejected", func(t *testing.T) {
		token := mintHS256(t, secret, map[string]any{"sub": "user_123", "exp": future})
		if _, err := authn.Authenticate(context.Background(), token+"x"); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("expected ErrUnauthenticated, got %v", err)
		}
	})

	t.Run("token signed with the wrong secret is rejected", func(t *testing.T) {
		token := mintHS256(t, "attacker-secret", map[string]any{"sub": "user_123", "exp": future})
		if _, err := authn.Authenticate(context.Background(), token); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("expected ErrUnauthenticated, got %v", err)
		}
	})

	t.Run("expired token is rejected", func(t *testing.T) {
		token := mintHS256(t, secret, map[string]any{
			"sub": "user_123",
			"exp": time.Now().Add(-time.Hour).Unix(),
		})
		if _, err := authn.Authenticate(context.Background(), token); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("expected ErrUnauthenticated, got %v", err)
		}
	})
}
