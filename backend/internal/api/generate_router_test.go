package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/generation"
	"github.com/kvn8888/codegym/backend/internal/identity"
	"github.com/kvn8888/codegym/backend/internal/memory"
	"github.com/kvn8888/codegym/backend/internal/session"
)

// staticGenerator returns a fixed payload, satisfying generation.Generator.
type staticGenerator struct {
	payload string
}

func (s staticGenerator) Generate(_ context.Context, _ generation.GenerateRequest) (generation.GenerateResult, error) {
	return generation.GenerateResult{
		Object:   []byte(s.payload),
		Provider: "static",
		Model:    "fake-model",
	}, nil
}

func newGenerateTestRouter(t *testing.T, orchestrator *generation.Orchestrator, memoryService *memory.Service) http.Handler {
	t.Helper()
	return NewRouter(Dependencies{
		Authenticator: auth.NewDevAuthenticator(auth.DevAuthenticatorConfig{}),
		Identity:      identity.NewService(identity.NewInMemoryStore()),
		Memory:        memoryService,
		Sessions:      session.NewService(session.NewInMemoryStore(), nil),
		Generation:    orchestrator,
	})
}

func TestGenerateRouteRespondsServiceUnavailableWhenUnconfigured(t *testing.T) {
	memoryService := memory.NewService(memory.NewInMemoryStore(), nil)
	router := newGenerateTestRouter(t, nil, memoryService)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/generate", strings.NewReader(`{"kind":"mcq","spec":{"count":2}}`))
	request.Header.Set("Authorization", "Bearer dev:kevin:personal-kevin")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "generation_unconfigured") {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestGenerateRouteRequiresAuth(t *testing.T) {
	memoryService := memory.NewService(memory.NewInMemoryStore(), nil)
	router := newGenerateTestRouter(t, nil, memoryService)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/generate", strings.NewReader(`{"kind":"mcq"}`))
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", recorder.Code)
	}
}

func TestGenerateRouteReturnsMCQSetAndRecordsMemoryEvent(t *testing.T) {
	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	memoryService := memory.NewService(memory.NewInMemoryStore(), func() time.Time { return now })
	orchestrator := generation.NewOrchestrator(memoryService, staticGenerator{payload: `[
		{"id":"mq1","text":"What is FIFO?","options":["Queue","Stack","Heap","Trie"],"correctIndex":0,"concept":"Queues","helpContent":"First in, first out."},
		{"id":"mq2","text":"BFS order?","options":["Depth","Level","Post","In"],"correctIndex":1,"concept":"BFS","helpContent":"Level by level."}
	]`})
	router := newGenerateTestRouter(t, orchestrator, memoryService)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/generate", strings.NewReader(`{"kind":"mcq","spec":{"topic":"queues","count":2}}`))
	request.Header.Set("Authorization", "Bearer dev:kevin:personal-kevin")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `"questions"`) || !strings.Contains(body, `"correctIndex"`) {
		t.Fatalf("body = %s", body)
	}
	if !strings.Contains(body, `"provider":"static"`) {
		t.Fatalf("body missing provider: %s", body)
	}

	// The generate flow should append a generate.mcq_set_generated event.
	events := httptest.NewRecorder()
	eventsRequest := httptest.NewRequest(http.MethodGet, "/api/v1/memory/events", nil)
	eventsRequest.Header.Set("Authorization", "Bearer dev:kevin:personal-kevin")
	router.ServeHTTP(events, eventsRequest)

	if events.Code != http.StatusOK {
		t.Fatalf("events status = %d", events.Code)
	}
	if !strings.Contains(events.Body.String(), "mcq_set_generated") {
		t.Fatalf("memory events missing mcq_set_generated: %s", events.Body.String())
	}
}

func TestGenerateRouteRejectsUnknownKind(t *testing.T) {
	memoryService := memory.NewService(memory.NewInMemoryStore(), nil)
	orchestrator := generation.NewOrchestrator(memoryService, staticGenerator{payload: `[]`})
	router := newGenerateTestRouter(t, orchestrator, memoryService)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/generate", strings.NewReader(`{"kind":"poem"}`))
	request.Header.Set("Authorization", "Bearer dev:kevin:personal-kevin")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", recorder.Code, recorder.Body.String())
	}
}

func TestGenerateRouteNotImplementedKinds(t *testing.T) {
	memoryService := memory.NewService(memory.NewInMemoryStore(), nil)
	orchestrator := generation.NewOrchestrator(memoryService, staticGenerator{payload: `[]`})
	router := newGenerateTestRouter(t, orchestrator, memoryService)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/generate", strings.NewReader(`{"kind":"problem"}`))
	request.Header.Set("Authorization", "Bearer dev:kevin:personal-kevin")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501: %s", recorder.Code, recorder.Body.String())
	}
}
