package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/generation"
	"github.com/kvn8888/codegym/backend/internal/identity"
	"github.com/kvn8888/codegym/backend/internal/intake"
	"github.com/kvn8888/codegym/backend/internal/memory"
	"github.com/kvn8888/codegym/backend/internal/session"
)

type intakeRouteGenerator struct{}

func (intakeRouteGenerator) Generate(_ context.Context, _ generation.GenerateRequest) (generation.GenerateResult, error) {
	return generation.GenerateResult{
		Object: []byte(`[
			{"id":"q1","dimension":"exposure","text":"How familiar are you?","options":[{"id":"new","label":"New"},{"id":"some","label":"Some"}]},
			{"id":"q2","dimension":"application","text":"Where have you used it?","options":[{"id":"none","label":"Not yet"},{"id":"project","label":"A project"}]},
			{"id":"q3","dimension":"challenge","text":"What should today emphasize?","options":[{"id":"guided","label":"Guidance"},{"id":"stretch","label":"Stretch"}]}
		]`),
		Provider: "test",
		Model:    "test",
	}, nil
}

func newIntakeTestRouter() http.Handler {
	memoryService := memory.NewService(memory.NewInMemoryStore(), nil)
	orchestrator := generation.NewOrchestrator(memoryService, intakeRouteGenerator{})
	intakeService := intake.NewService(intake.NewInMemoryStore(), memoryService, orchestrator, nil)
	return NewRouter(Dependencies{
		Authenticator: auth.NewDevAuthenticator(auth.DevAuthenticatorConfig{}),
		Identity:      identity.NewService(identity.NewInMemoryStore()),
		Memory:        memoryService,
		Sessions:      session.NewService(session.NewInMemoryStore(), nil),
		Generation:    orchestrator,
		Intakes:       intakeService,
	})
}

func intakeRequest(router http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Authorization", token)
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestPracticeIntakeRoutesResumePartialAndEnforceScope(t *testing.T) {
	router := newIntakeTestRouter()
	ownerToken := "Bearer dev:kevin:personal-kevin"

	prepared := intakeRequest(router, http.MethodPost, "/api/v1/practice-intakes",
		`{"topic":"TypeScript","practice_seed":{"format":"mcq","difficulty":"medium"}}`, ownerToken)
	if prepared.Code != http.StatusOK {
		t.Fatalf("prepare status=%d body=%s", prepared.Code, prepared.Body.String())
	}
	var envelope struct {
		Data intake.Record `json:"data"`
	}
	if err := json.NewDecoder(prepared.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode prepare: %v", err)
	}
	if envelope.Data.ID == "" || len(envelope.Data.Questions) != 3 {
		t.Fatalf("unexpected prepared intake: %#v", envelope.Data)
	}

	updated := intakeRequest(router, http.MethodPatch, "/api/v1/practice-intakes/"+envelope.Data.ID,
		`{"answers":{"q1":"some"}}`, ownerToken)
	if updated.Code != http.StatusOK || !strings.Contains(updated.Body.String(), `"q1":"some"`) {
		t.Fatalf("update status=%d body=%s", updated.Code, updated.Body.String())
	}

	pending := intakeRequest(router, http.MethodGet, "/api/v1/practice-intakes?status=pending&limit=1", "", ownerToken)
	if pending.Code != http.StatusOK || !strings.Contains(pending.Body.String(), envelope.Data.ID) {
		t.Fatalf("pending status=%d body=%s", pending.Code, pending.Body.String())
	}

	crossScope := intakeRequest(router, http.MethodPatch, "/api/v1/practice-intakes/"+envelope.Data.ID,
		`{"answers":{"q1":"new"}}`, "Bearer dev:alec:personal-alec")
	if crossScope.Code != http.StatusNotFound {
		t.Fatalf("cross-scope status=%d body=%s", crossScope.Code, crossScope.Body.String())
	}
}
