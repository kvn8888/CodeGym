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
	"github.com/kvn8888/codegym/backend/internal/session"
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
		"/api/v1/me":                     {http.MethodGet, http.MethodPatch},
		"/api/v1/memory/profile":         {http.MethodGet},
		"/api/v1/memory/profile/refresh": {http.MethodPost},
		"/api/v1/memory/events":          {http.MethodGet, http.MethodPost},
		"/api/v1/sessions":               {http.MethodGet, http.MethodPost},
		"/api/v1/sessions/{id}":          {http.MethodGet, http.MethodPatch},
		"/api/v1/sessions/{id}/files":    {http.MethodPut},
		"/api/v1/generate":               {http.MethodPost},
		"/api/v1/memory/notes/maintain":  {http.MethodPost},
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

func TestRouterUserProfileLifecycle(t *testing.T) {
	router := NewRouter(Dependencies{
		Authenticator: auth.NewDevAuthenticator(auth.DevAuthenticatorConfig{}),
		Identity:      identity.NewService(identity.NewInMemoryStore()),
		Memory:        memory.NewService(memory.NewInMemoryStore(), nil),
		Sessions:      session.NewService(session.NewInMemoryStore(), nil),
	})

	getInitial := httptest.NewRecorder()
	getInitialReq := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	getInitialReq.Header.Set("Authorization", "Bearer dev:kevin:personal-kevin")
	router.ServeHTTP(getInitial, getInitialReq)

	if getInitial.Code != http.StatusOK {
		t.Fatalf("initial profile status = %d: %s", getInitial.Code, getInitial.Body.String())
	}
	initial := decodeEnvelopeData[identity.UserProfile](t, getInitial)
	if initial.UserID != "kevin" || initial.DisplayName != "kevin" || initial.DisplayNameSource != "fallback" {
		t.Fatalf("initial profile = %#v", initial)
	}

	patch := httptest.NewRecorder()
	patchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/me", strings.NewReader(`{"display_name":"Kevin Chen"}`))
	patchReq.Header.Set("Authorization", "Bearer dev:kevin:personal-kevin")
	router.ServeHTTP(patch, patchReq)

	if patch.Code != http.StatusOK {
		t.Fatalf("patch profile status = %d: %s", patch.Code, patch.Body.String())
	}
	updated := decodeEnvelopeData[identity.UserProfile](t, patch)
	if updated.DisplayName != "Kevin Chen" || updated.DisplayNameSource != "user" {
		t.Fatalf("updated profile = %#v", updated)
	}

	bad := httptest.NewRecorder()
	badReq := httptest.NewRequest(http.MethodPatch, "/api/v1/me", strings.NewReader(`{"display_name":"   "}`))
	badReq.Header.Set("Authorization", "Bearer dev:kevin:personal-kevin")
	router.ServeHTTP(bad, badReq)

	if bad.Code != http.StatusBadRequest {
		t.Fatalf("blank display name status = %d: %s", bad.Code, bad.Body.String())
	}
}

func TestRouterSessionsLifecycle(t *testing.T) {
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	router := NewRouter(Dependencies{
		Authenticator: auth.NewDevAuthenticator(auth.DevAuthenticatorConfig{}),
		Identity:      identity.NewService(identity.NewInMemoryStore()),
		Memory:        memory.NewService(memory.NewInMemoryStore(), nil),
		Sessions:      session.NewService(session.NewInMemoryStore(), func() time.Time { return now }),
	})

	create := httptest.NewRecorder()
	createReq := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/sessions",
		strings.NewReader(`{"kind":"workspace","title":"Graph traversal","problem_id":"prob_graph","state":{"schema_version":1,"language":"go"}}`),
	)
	createReq.Header.Set("Authorization", "Bearer dev:kevin:personal-kevin")
	router.ServeHTTP(create, createReq)

	if create.Code != http.StatusCreated {
		t.Fatalf("create session status = %d: %s", create.Code, create.Body.String())
	}
	created := decodeEnvelopeData[session.Session](t, create)
	if created.ID == "" {
		t.Fatal("created session id is empty")
	}
	if len(created.State) == 0 {
		t.Fatal("created session state is empty")
	}

	list := httptest.NewRecorder()
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/sessions?status=active&limit=5", nil)
	listReq.Header.Set("Authorization", "Bearer dev:kevin:personal-kevin")
	router.ServeHTTP(list, listReq)

	if list.Code != http.StatusOK {
		t.Fatalf("list sessions status = %d: %s", list.Code, list.Body.String())
	}
	if strings.Contains(list.Body.String(), `"state"`) || strings.Contains(list.Body.String(), `"files"`) {
		t.Fatalf("list response included heavy fields: %s", list.Body.String())
	}
	summaries := decodeEnvelopeData[[]session.Summary](t, list)
	if len(summaries) != 1 || summaries[0].ID != created.ID {
		t.Fatalf("summaries = %#v", summaries)
	}

	detail := httptest.NewRecorder()
	detailReq := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+created.ID, nil)
	detailReq.Header.Set("Authorization", "Bearer dev:kevin:personal-kevin")
	router.ServeHTTP(detail, detailReq)

	if detail.Code != http.StatusOK {
		t.Fatalf("detail status = %d: %s", detail.Code, detail.Body.String())
	}
	detailed := decodeEnvelopeData[session.Session](t, detail)
	if string(detailed.State) == "" {
		t.Fatalf("detail state is empty: %#v", detailed)
	}

	patch := httptest.NewRecorder()
	patchReq := httptest.NewRequest(
		http.MethodPatch,
		"/api/v1/sessions/"+created.ID,
		strings.NewReader(`{"title":"Completed graph traversal","status":"completed","state":{"schema_version":1,"language":"go","last_run":{"passed":3,"total":3}}}`),
	)
	patchReq.Header.Set("Authorization", "Bearer dev:kevin:personal-kevin")
	router.ServeHTTP(patch, patchReq)

	if patch.Code != http.StatusOK {
		t.Fatalf("patch status = %d: %s", patch.Code, patch.Body.String())
	}
	updated := decodeEnvelopeData[session.Session](t, patch)
	if updated.Status != session.StatusCompleted || updated.CompletedAt == nil {
		t.Fatalf("updated session = %#v", updated)
	}

	files := httptest.NewRecorder()
	filesReq := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/sessions/"+created.ID+"/files",
		strings.NewReader(`{"files":[{"file_path":"main.go","content":"package main\n"}]}`),
	)
	filesReq.Header.Set("Authorization", "Bearer dev:kevin:personal-kevin")
	router.ServeHTTP(files, filesReq)

	if files.Code != http.StatusOK {
		t.Fatalf("upsert files status = %d: %s", files.Code, files.Body.String())
	}
	withFiles := decodeEnvelopeData[session.Session](t, files)
	if len(withFiles.Files) != 1 || withFiles.Files[0].Path != "main.go" {
		t.Fatalf("files = %#v", withFiles.Files)
	}

	crossScope := httptest.NewRecorder()
	crossScopeReq := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+created.ID, nil)
	crossScopeReq.Header.Set("Authorization", "Bearer dev:alec:personal-alec")
	router.ServeHTTP(crossScope, crossScopeReq)

	if crossScope.Code != http.StatusNotFound {
		t.Fatalf("cross-scope status = %d: %s", crossScope.Code, crossScope.Body.String())
	}
}

func decodeEnvelopeData[T any](t *testing.T, recorder *httptest.ResponseRecorder) T {
	t.Helper()

	var envelope struct {
		Data  T           `json:"data"`
		Error interface{} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode envelope: %v; body=%s", err, recorder.Body.String())
	}
	if envelope.Error != nil {
		t.Fatalf("unexpected envelope error: %#v", envelope.Error)
	}
	return envelope.Data
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

func (s *recordingIdentityStore) GetUserProfile(_ context.Context, userID string) (identity.UserProfile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, record := range s.records {
		if record.UserID == userID {
			return identity.UserProfile{
				UserID:            record.UserID,
				Email:             record.Email,
				DisplayName:       record.DisplayName,
				DisplayNameSource: record.DisplayNameSource,
				DefaultTenantID:   record.TenantID,
			}, nil
		}
	}
	return identity.UserProfile{}, identity.ErrUserNotFound
}

func (s *recordingIdentityStore) UpdateDisplayName(_ context.Context, userID, displayName string) (identity.UserProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for key, record := range s.records {
		if record.UserID == userID {
			record.DisplayName = displayName
			record.DisplayNameSource = "user"
			s.records[key] = record
			return identity.UserProfile{
				UserID:            record.UserID,
				Email:             record.Email,
				DisplayName:       record.DisplayName,
				DisplayNameSource: record.DisplayNameSource,
				DefaultTenantID:   record.TenantID,
			}, nil
		}
	}
	return identity.UserProfile{}, identity.ErrUserNotFound
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
