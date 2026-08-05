package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/identity"
	"github.com/kvn8888/codegym/backend/internal/settings"
)

func TestRouterRuntimeSettingsAreAuthenticatedAndWorkspaceScoped(t *testing.T) {
	service := settings.NewService(settings.NewInMemoryStore(), nil)
	router := NewRouter(Dependencies{
		Authenticator: auth.NewDevAuthenticator(auth.DevAuthenticatorConfig{}),
		Identity:      identity.NewService(identity.NewInMemoryStore()),
		Settings:      service,
	})

	unauthenticated := httptest.NewRecorder()
	router.ServeHTTP(unauthenticated, httptest.NewRequest(
		http.MethodGet, "/api/v1/settings/"+settings.HedgeCountKey, nil,
	))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d body=%s", unauthenticated.Code, unauthenticated.Body.String())
	}

	update := httptest.NewRecorder()
	updateRequest := httptest.NewRequest(
		http.MethodPut, "/api/v1/settings/"+settings.HedgeCountKey,
		strings.NewReader(`{"value":9}`),
	)
	updateRequest.Header.Set("Authorization", "Bearer dev:kevin:personal-kevin")
	router.ServeHTTP(update, updateRequest)
	if update.Code != http.StatusOK || !strings.Contains(update.Body.String(), `"value":3`) {
		t.Fatalf("update status=%d body=%s", update.Code, update.Body.String())
	}

	readOwn := httptest.NewRecorder()
	readOwnRequest := httptest.NewRequest(http.MethodGet, "/api/v1/settings/"+settings.HedgeCountKey, nil)
	readOwnRequest.Header.Set("Authorization", "Bearer dev:kevin:personal-kevin")
	router.ServeHTTP(readOwn, readOwnRequest)
	if readOwn.Code != http.StatusOK || !strings.Contains(readOwn.Body.String(), `"value":3`) {
		t.Fatalf("own read status=%d body=%s", readOwn.Code, readOwn.Body.String())
	}

	readOther := httptest.NewRecorder()
	readOtherRequest := httptest.NewRequest(http.MethodGet, "/api/v1/settings/"+settings.HedgeCountKey, nil)
	readOtherRequest.Header.Set("Authorization", "Bearer dev:alec:personal-alec")
	router.ServeHTTP(readOther, readOtherRequest)
	if readOther.Code != http.StatusOK || !strings.Contains(readOther.Body.String(), `"value":1`) ||
		!strings.Contains(readOther.Body.String(), `"source":"default"`) {
		t.Fatalf("other workspace read status=%d body=%s", readOther.Code, readOther.Body.String())
	}
}
