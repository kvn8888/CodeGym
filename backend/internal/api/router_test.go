package api

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/identity"
	"github.com/kvn8888/codegym/backend/internal/memory"
	"github.com/kvn8888/codegym/backend/internal/tenant"
	"gopkg.in/yaml.v3"
)

func TestOpenAPIContractCoversRouterRoutes(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file")
	}

	openAPIPath := filepath.Join(filepath.Dir(currentFile), "..", "..", "..", "api", "openapi.yaml")
	openAPIBytes, err := os.ReadFile(openAPIPath)
	if err != nil {
		t.Fatalf("read OpenAPI contract: %v", err)
	}

	var contract struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(openAPIBytes, &contract); err != nil {
		t.Fatalf("parse OpenAPI contract: %v", err)
	}

	required := map[string][]string{
		"/health":                        {http.MethodGet},
		"/ready":                         {http.MethodGet},
		"/api/v1/memory/profile":         {http.MethodGet},
		"/api/v1/memory/profile/refresh": {http.MethodPost},
		"/api/v1/memory/events":          {http.MethodGet, http.MethodPost},
	}

	for path, methods := range required {
		operations, ok := contract.Paths[path]
		if !ok {
			t.Fatalf("OpenAPI contract is missing route %s", path)
		}
		for _, method := range methods {
			if _, ok := operations[strings.ToLower(method)]; !ok {
				t.Fatalf("OpenAPI contract is missing %s %s", method, path)
			}
		}
	}
}

func TestRouterMapsAuth0UserIntoIdentityBootstrap(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	var issuer string
	jwksJSON := testJWKS(t, &privateKey.PublicKey)
	jwksServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]string{
				"issuer":   issuer,
				"jwks_uri": jwksServerURL(r) + "/.well-known/jwks.json",
			})
		case "/.well-known/jwks.json":
			_, _ = w.Write([]byte(jwksJSON))
		default:
			http.NotFound(w, r)
		}
	}))
	defer jwksServer.Close()
	issuer = jwksServer.URL + "/"

	authenticator, err := auth.NewAuth0Authenticator(auth.Auth0AuthenticatorConfig{
		IssuerURL:  issuer,
		Audience:   "https://api.codegym.test",
		HTTPClient: jwksServer.Client(),
	})
	if err != nil {
		t.Fatalf("build auth0 authenticator: %v", err)
	}

	identityStore := newRecordingIdentityStore()
	router := NewRouter(Dependencies{
		Authenticator: authenticator,
		Identity:      identity.NewService(identityStore),
		Memory:        memory.NewService(memory.NewInMemoryStore(), nil),
	})

	token := mintRouterTestRS256(t, privateKey, map[string]any{
		"iss":   issuer,
		"aud":   "https://api.codegym.test",
		"sub":   "auth0|user_123",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"email": "kevin@example.com",
		"name":  "Kevin Chen",
	})

	for i := 0; i < 2; i++ {
		res := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/memory/profile", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		router.ServeHTTP(res, req)

		if res.Code != http.StatusOK {
			t.Fatalf("request %d expected 200, got %d: %s", i+1, res.Code, res.Body.String())
		}
	}

	bootstrapped := identityStore.get("personal-auth0-user-123", "auth0|user_123")
	if bootstrapped.UserID != "auth0|user_123" {
		t.Fatalf("UserID = %q, want auth0|user_123", bootstrapped.UserID)
	}
	if bootstrapped.TenantID != "personal-auth0-user-123" {
		t.Fatalf("TenantID = %q, want personal-auth0-user-123", bootstrapped.TenantID)
	}
	if bootstrapped.Email != "kevin@example.com" {
		t.Fatalf("Email = %q, want kevin@example.com", bootstrapped.Email)
	}
	if bootstrapped.DisplayName != "Kevin Chen" {
		t.Fatalf("DisplayName = %q, want Kevin Chen", bootstrapped.DisplayName)
	}
	if identityStore.count() != 1 {
		t.Fatalf("expected idempotent single membership, got %d", identityStore.count())
	}

	forbidden := httptest.NewRecorder()
	forbiddenReq := httptest.NewRequest(http.MethodGet, "/api/v1/memory/profile", nil)
	forbiddenReq.Header.Set("Authorization", "Bearer "+token)
	forbiddenReq.Header.Set(tenant.HeaderTenantID, "shared-claimed")

	router.ServeHTTP(forbidden, forbiddenReq)

	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden override to return 403, got %d: %s", forbidden.Code, forbidden.Body.String())
	}
}

type recordingIdentityStore struct {
	mu      sync.RWMutex
	records map[string]identity.PersonalTenant
}

func newRecordingIdentityStore() *recordingIdentityStore {
	return &recordingIdentityStore{records: map[string]identity.PersonalTenant{}}
}

func (s *recordingIdentityStore) EnsurePersonalTenant(_ context.Context, personalTenant identity.PersonalTenant) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.records[personalTenant.TenantID+"\x00"+personalTenant.UserID] = personalTenant
	return nil
}

func (s *recordingIdentityStore) get(tenantID, userID string) identity.PersonalTenant {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.records[tenantID+"\x00"+userID]
}

func (s *recordingIdentityStore) count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.records)
}

func testJWKS(t *testing.T, publicKey *rsa.PublicKey) string {
	t.Helper()

	payload, err := json.Marshal(map[string]any{
		"keys": []map[string]string{
			{
				"kty": "RSA",
				"use": "sig",
				"kid": "test-key",
				"alg": "RS256",
				"n":   base64.RawURLEncoding.EncodeToString(publicKey.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(bigEndianInt(publicKey.E)),
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal jwks: %v", err)
	}
	return string(payload)
}

func bigEndianInt(value int) []byte {
	if value == 0 {
		return []byte{0}
	}

	var bytes []byte
	for value > 0 {
		bytes = append([]byte{byte(value & 0xff)}, bytes...)
		value >>= 8
	}
	return bytes
}

func jwksServerURL(r *http.Request) string {
	return "https://" + r.Host
}

func mintRouterTestRS256(t *testing.T, privateKey *rsa.PrivateKey, claims map[string]any) string {
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
