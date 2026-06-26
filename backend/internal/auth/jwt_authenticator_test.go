package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	auth0validator "github.com/auth0/go-jwt-middleware/v3/validator"
)

const (
	testIssuer   = "https://codegym-test.us.auth0.com/"
	testAudience = "https://api.codegym.test"
)

func TestAuth0Authenticator(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	authn := newTestAuth0Authenticator(t, &privateKey.PublicKey)
	future := time.Now().Add(time.Hour).Unix()

	t.Run("empty token is rejected", func(t *testing.T) {
		if _, err := authn.Authenticate(context.Background(), " "); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("expected ErrUnauthenticated, got %v", err)
		}
	})

	t.Run("valid token maps auth0 subject to personal workspace principal", func(t *testing.T) {
		token := mintRS256(t, privateKey, map[string]any{
			"iss": testIssuer,
			"aud": testAudience,
			"sub": "auth0|user_123",
			"exp": future,
		})

		principal, err := authn.Authenticate(context.Background(), token)
		if err != nil {
			t.Fatalf("expected valid token, got %v", err)
		}
		if principal.UserID != "auth0|user_123" {
			t.Fatalf("UserID = %q, want auth0|user_123", principal.UserID)
		}
		if principal.DefaultTenantID != "personal-auth0-user-123" {
			t.Fatalf("DefaultTenantID = %q, want personal-auth0-user-123", principal.DefaultTenantID)
		}
		if !principal.HasTenant("personal-auth0-user-123") || principal.HasTenant("shared-claimed") {
			t.Fatalf("expected only personal-auth0-user-123 in TenantIDs: %#v", principal.TenantIDs)
		}
	})

	t.Run("app workspace claims do not grant extra workspace access", func(t *testing.T) {
		token := mintRS256(t, privateKey, map[string]any{
			"iss":                                    testIssuer,
			"aud":                                    testAudience,
			"sub":                                    "auth0|user_123",
			"exp":                                    future,
			"https://codegym.dev/default_workspace":  "personal-claimed",
			"https://codegym.dev/allowed_workspaces": []string{"personal-claimed", "shared-claimed"},
		})

		principal, err := authn.Authenticate(context.Background(), token)
		if err != nil {
			t.Fatalf("expected valid token, got %v", err)
		}
		if principal.DefaultTenantID != "personal-auth0-user-123" {
			t.Fatalf("DefaultTenantID = %q, want personal-auth0-user-123", principal.DefaultTenantID)
		}
		if principal.HasTenant("personal-claimed") || principal.HasTenant("shared-claimed") {
			t.Fatalf("unexpected app-claimed workspace access: %#v", principal.TenantIDs)
		}
	})

	t.Run("tampered token is rejected", func(t *testing.T) {
		token := mintRS256(t, privateKey, map[string]any{
			"iss": testIssuer,
			"aud": testAudience,
			"sub": "auth0|user_123",
			"exp": future,
		})

		if _, err := authn.Authenticate(context.Background(), token+"x"); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("expected ErrUnauthenticated, got %v", err)
		}
	})

	t.Run("wrong audience token is rejected", func(t *testing.T) {
		token := mintRS256(t, privateKey, map[string]any{
			"iss": testIssuer,
			"aud": "https://wrong-audience.test",
			"sub": "auth0|user_123",
			"exp": future,
		})

		if _, err := authn.Authenticate(context.Background(), token); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("expected ErrUnauthenticated, got %v", err)
		}
	})

	t.Run("wrong issuer token is rejected", func(t *testing.T) {
		token := mintRS256(t, privateKey, map[string]any{
			"iss": "https://other-tenant.us.auth0.com/",
			"aud": testAudience,
			"sub": "auth0|user_123",
			"exp": future,
		})

		if _, err := authn.Authenticate(context.Background(), token); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("expected ErrUnauthenticated, got %v", err)
		}
	})

	t.Run("expired token is rejected", func(t *testing.T) {
		token := mintRS256(t, privateKey, map[string]any{
			"iss": testIssuer,
			"aud": testAudience,
			"sub": "auth0|user_123",
			"exp": time.Now().Add(-time.Hour).Unix(),
		})

		if _, err := authn.Authenticate(context.Background(), token); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("expected ErrUnauthenticated, got %v", err)
		}
	})

	t.Run("not-before token is rejected", func(t *testing.T) {
		token := mintRS256(t, privateKey, map[string]any{
			"iss": testIssuer,
			"aud": testAudience,
			"sub": "auth0|user_123",
			"exp": future,
			"nbf": time.Now().Add(time.Hour).Unix(),
		})

		if _, err := authn.Authenticate(context.Background(), token); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("expected ErrUnauthenticated, got %v", err)
		}
	})

	t.Run("missing subject is rejected", func(t *testing.T) {
		token := mintRS256(t, privateKey, map[string]any{
			"iss": testIssuer,
			"aud": testAudience,
			"exp": future,
		})

		if _, err := authn.Authenticate(context.Background(), token); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("expected ErrUnauthenticated, got %v", err)
		}
	})
}

func TestAuth0IssuerURL(t *testing.T) {
	t.Run("builds issuer from domain", func(t *testing.T) {
		issuerURL, err := auth0IssuerURL(Auth0AuthenticatorConfig{Domain: "codegym.us.auth0.com"})
		if err != nil {
			t.Fatalf("expected issuer URL, got %v", err)
		}
		if issuerURL.String() != "https://codegym.us.auth0.com/" {
			t.Fatalf("issuer URL = %q", issuerURL.String())
		}
	})

	t.Run("requires https issuer", func(t *testing.T) {
		if _, err := auth0IssuerURL(Auth0AuthenticatorConfig{IssuerURL: "http://codegym.us.auth0.com/"}); err == nil {
			t.Fatal("expected non-https issuer to fail")
		}
	})
}

func newTestAuth0Authenticator(t *testing.T, publicKey *rsa.PublicKey) *Auth0Authenticator {
	t.Helper()

	validator, err := auth0validator.New(
		auth0validator.WithKeyFunc(func(context.Context) (any, error) {
			return publicKey, nil
		}),
		auth0validator.WithAlgorithm(auth0validator.RS256),
		auth0validator.WithIssuer(testIssuer),
		auth0validator.WithAudience(testAudience),
	)
	if err != nil {
		t.Fatalf("create validator: %v", err)
	}

	return newAuth0Authenticator(validator, Auth0AuthenticatorConfig{})
}

func mintRS256(t *testing.T, privateKey *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()

	enc := func(v any) string {
		payload, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return base64.RawURLEncoding.EncodeToString(payload)
	}

	signingInput := enc(map[string]any{
		"alg": "RS256",
		"typ": "JWT",
		"kid": "test-key",
	}) + "." + enc(claims)

	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)
}
