package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMiddlewareRejectsMissingBearerToken(t *testing.T) {
	middleware := Middleware(NewDevAuthenticator(DevAuthenticatorConfig{}))
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called")
	}))

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/", nil))

	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", res.Code)
	}
}

func TestMiddlewareStoresPrincipal(t *testing.T) {
	middleware := Middleware(NewDevAuthenticator(DevAuthenticatorConfig{}))
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := PrincipalFromContext(r.Context())
		if !ok {
			t.Fatal("missing principal")
		}
		if principal.UserID != "kevin" {
			t.Fatalf("expected user kevin, got %q", principal.UserID)
		}
		if principal.DefaultTenantID != "rit" {
			t.Fatalf("expected tenant rit, got %q", principal.DefaultTenantID)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer dev:kevin:rit")

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", res.Code)
	}
}
