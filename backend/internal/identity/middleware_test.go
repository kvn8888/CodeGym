package identity

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kvn8888/codegym/backend/internal/auth"
)

func TestMiddlewareBootstrapsAuthenticatedPrincipal(t *testing.T) {
	store := NewInMemoryStore()
	middleware := Middleware(NewService(store))
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := auth.WithPrincipal(req.Context(), auth.Principal{
		UserID:             "kevin",
		DefaultWorkspaceID: "personal-kevin",
		WorkspaceIDs:       []string{"personal-kevin"},
	})

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req.WithContext(ctx))

	if res.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", res.Code)
	}

	store.mu.RLock()
	defer store.mu.RUnlock()
	if _, ok := store.memberships[membershipKey("personal-kevin", "kevin")]; !ok {
		t.Fatal("expected membership to be bootstrapped")
	}
}

func TestMiddlewareRejectsMissingPrincipal(t *testing.T) {
	middleware := Middleware(NewService(NewInMemoryStore()))
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("next handler should not be called")
	}))

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/", nil))

	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", res.Code)
	}
}
