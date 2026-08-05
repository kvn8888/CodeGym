package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/execution"
	"github.com/kvn8888/codegym/backend/internal/generation"
	"github.com/kvn8888/codegym/backend/internal/identity"
	"github.com/kvn8888/codegym/backend/internal/memory"
	"github.com/kvn8888/codegym/backend/internal/problems"
	"github.com/kvn8888/codegym/backend/internal/session"
)

// staticGenerator returns a fixed payload, satisfying generation.Generator.
type staticGenerator struct {
	payload string
}

type independentProblemGenerator struct{}

func (independentProblemGenerator) Generate(_ context.Context, request generation.GenerateRequest) (generation.GenerateResult, error) {
	var payload string
	switch request.Kind {
	case generation.KindProblemSpec:
		payload = `{
			"language":"python","strategy":"unit","title":"Reverse Words",
			"description":"Reverse the words.","category":"algorithms","subcategory":"strings",
			"tags":["strings"],"difficulty":1,"estimated_minutes":15,"hints":["Split first."],
			"entrypoint":"reverse_words","signature":"def reverse_words(value: str) -> str",
			"io_contract":"Accept one string and return one string containing its words in reverse order.",
			"comparator":{"kind":"exact"},
			"ambiguity_resolutions":["Empty input returns the empty string.","Runs of whitespace are normalized to one space."],
			"function_name":"reverse_words","parameters":[{"name":"value","type":"str"}],"return_type":"str"
		}`
	case generation.KindProblemTests:
		payload = `{"test_cases":[
			{"name":"one","kind":"example","hidden":false,"args":["hello world"],"expected":"world hello"},
			{"name":"single","kind":"functional","hidden":false,"args":["hello"],"expected":"hello"},
			{"name":"spaces","kind":"hidden","hidden":true,"args":["a b c"],"expected":"c b a"},
			{"name":"empty","kind":"edge","hidden":true,"args":[""],"expected":""}
		]}`
	case generation.KindProblemReference:
		payload = `{"reference_solution":"def reverse_words(value: str) -> str:\n    return ' '.join(reversed(value.split()))"}`
	default:
		payload = `{}`
	}
	return generation.GenerateResult{Object: []byte(payload), Provider: "static", Model: "fake-model"}, nil
}

func (s staticGenerator) Generate(_ context.Context, _ generation.GenerateRequest) (generation.GenerateResult, error) {
	return generation.GenerateResult{
		Object:   []byte(s.payload),
		Provider: "static",
		Model:    "fake-model",
	}, nil
}

type passingVerifyRunner struct{}

func (passingVerifyRunner) Run(_ context.Context, _ execution.RunSpec) (execution.RunOutcome, error) {
	return execution.RunOutcome{
		ExitCode: 0,
		Output:   `CODEGYM_RESULT {"tests":[{"name":"one","status":"pass","duration_ms":1,"error":null},{"name":"single","status":"pass","duration_ms":1,"error":null},{"name":"spaces","status":"pass","duration_ms":1,"error":null},{"name":"empty","status":"pass","duration_ms":1,"error":null}],"compile_error":null}`,
		Duration: time.Millisecond,
	}, nil
}

func newGenerateTestRouter(t *testing.T, orchestrator *generation.Orchestrator, memoryService *memory.Service) http.Handler {
	t.Helper()
	problemService := problems.NewService(problems.NewInMemoryStore())
	if err := problemService.EnsureSeed(context.Background()); err != nil {
		t.Fatal(err)
	}
	return NewRouter(Dependencies{
		Authenticator:   auth.NewDevAuthenticator(auth.DevAuthenticatorConfig{}),
		Identity:        identity.NewService(identity.NewInMemoryStore()),
		Memory:          memoryService,
		Sessions:        session.NewService(session.NewInMemoryStore(), nil),
		Generation:      orchestrator,
		Problems:        problemService,
		ExecutionRunner: passingVerifyRunner{},
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

// sequencedGenerator returns payloads in call order (last repeats).
type sequencedGenerator struct {
	payloads []string
	requests []generation.GenerateRequest
}

func (s *sequencedGenerator) Generate(_ context.Context, request generation.GenerateRequest) (generation.GenerateResult, error) {
	s.requests = append(s.requests, request)
	index := len(s.requests) - 1
	if index >= len(s.payloads) {
		index = len(s.payloads) - 1
	}
	return generation.GenerateResult{
		Object:   []byte(s.payloads[index]),
		Provider: "sequenced",
		Model:    "fake-model",
	}, nil
}

func TestEvaluateFreeResponseRoute(t *testing.T) {
	memoryService := memory.NewService(memory.NewInMemoryStore(), nil)
	generator := &sequencedGenerator{payloads: []string{`{"correct":true,"feedback":"The answer correctly explains FIFO ordering."}`}}
	router := newGenerateTestRouter(t, generation.NewOrchestrator(memoryService, generator), memoryService)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/mcq/evaluate", strings.NewReader(`{
		"question_id":"mq1","question":"How does a queue remove items?","concept":"Queues",
		"expected_answer":"First in, first out.","rubric":"Must identify FIFO ordering.",
		"answer":"It removes the oldest inserted item first."
	}`))
	request.Header.Set("Authorization", "Bearer dev:kevin:personal-kevin")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"correct":true`) {
		t.Fatalf("evaluation status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if len(generator.requests) != 1 || generator.requests[0].Kind != generation.KindMCQEvaluation {
		t.Fatalf("requests = %#v", generator.requests)
	}
}

func TestMaintainProfileRouteAppliesActionsAndAuditsEvents(t *testing.T) {
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	memoryService := memory.NewService(memory.NewInMemoryStore(), func() time.Time { return now })
	generator := &sequencedGenerator{payloads: []string{
		`{"summary":"SQL joins need another pass; two-pointer fundamentals are progressing.","strengths":["Two Pointers"],"growth_edges":["SQL Joins"],"skills":[{"id":"sql-joins","label":"SQL Joins","area":"Data Systems","level":2,"confidence":55,"trend":"down"},{"id":"two-pointers","label":"Two Pointers","area":"DSA","level":3,"confidence":70,"trend":"up"}],"notes":[{"id":"note_sql-joins","title":"SQL joins","summary":"Missed LEFT JOIN semantics.","tags":["sql"],"action":"review"}]}`,
	}}
	orchestrator := generation.NewOrchestrator(memoryService, generator)
	router := newGenerateTestRouter(t, orchestrator, memoryService)

	authed := func(method, path, body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer dev:kevin:personal-kevin")
		router.ServeHTTP(recorder, request)
		return recorder
	}

	// Seed one round of answer events (what the marathon emits per question).
	for _, event := range []string{
		`{"source":"mcq","type":"answer_incorrect","summary":"Missed a SQL Joins question.","payload":{"session_id":"mcq_r1","topic":"SQL Joins","correct":false}}`,
		`{"source":"mcq","type":"question_answered","summary":"Answered a Two Pointers question correctly.","payload":{"session_id":"mcq_r1","topic":"Two Pointers","correct":true}}`,
	} {
		if code := authed(http.MethodPost, "/api/v1/memory/events", event).Code; code != http.StatusCreated {
			t.Fatalf("seed event status = %d", code)
		}
	}

	maintain := authed(http.MethodPost, "/api/v1/memory/profile/maintain", `{"session_id":"mcq_r1"}`)
	if maintain.Code != http.StatusOK {
		t.Fatalf("maintain status = %d: %s", maintain.Code, maintain.Body.String())
	}
	if !strings.Contains(maintain.Body.String(), "note_sql-joins") {
		t.Fatalf("maintained profile missing note: %s", maintain.Body.String())
	}

	// Audit event recorded for the CRUD action.
	events := authed(http.MethodGet, "/api/v1/memory/events", "")
	if !strings.Contains(events.Body.String(), "note_created") {
		t.Fatalf("missing note_created audit event: %s", events.Body.String())
	}

	// The next generation call must see the updated note in its memory context.
	generator.payloads = append(generator.payloads, `[
		{"id":"mq1","text":"Which JOIN keeps unmatched left rows?","options":["INNER","LEFT","RIGHT","CROSS"],"correctIndex":1,"concept":"SQL Joins","helpContent":"LEFT JOIN keeps all left rows."},
		{"id":"mq2","text":"Q2","options":["a","b","c","d"],"correctIndex":0,"concept":"C","helpContent":"H"}
	]`)
	generate := authed(http.MethodPost, "/api/v1/generate", `{"kind":"mcq","spec":{"prompt":"drill my weak spots","count":2,"round":2}}`)
	if generate.Code != http.StatusOK {
		t.Fatalf("generate status = %d: %s", generate.Code, generate.Body.String())
	}
	lastRequest := generator.requests[len(generator.requests)-1]
	foundNote := false
	for _, note := range lastRequest.MemoryContext.Notes {
		if note.Title == "SQL joins" {
			foundNote = true
		}
	}
	if !foundNote {
		t.Fatalf("generation memory context missing maintained note: %+v", lastRequest.MemoryContext.Notes)
	}
	if !strings.Contains(string(lastRequest.Spec), "drill my weak spots") {
		t.Fatalf("spec missing user prompt: %s", lastRequest.Spec)
	}
}

func TestMaintainProfileCompatibilityRouteWorksWithoutOrchestrator(t *testing.T) {
	memoryService := memory.NewService(memory.NewInMemoryStore(), nil)
	router := newGenerateTestRouter(t, nil, memoryService)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/memory/notes/maintain", strings.NewReader(`{}`))
	request.Header.Set("Authorization", "Bearer dev:kevin:personal-kevin")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (refresh-only fallback): %s", recorder.Code, recorder.Body.String())
	}
}

func TestMemoryEventsRouteReturnsOccurrenceNewestFirstInUTC(t *testing.T) {
	now := time.Date(2026, 7, 15, 20, 0, 0, 0, time.UTC)
	memoryService := memory.NewService(memory.NewInMemoryStore(), func() time.Time { return now })
	router := newGenerateTestRouter(t, nil, memoryService)

	post := func(body string) {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/v1/memory/events", strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer dev:kevin:personal-kevin")
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusCreated {
			t.Fatalf("POST event status = %d: %s", recorder.Code, recorder.Body.String())
		}
	}
	post(`{"source":"mcq","type":"question_answered","summary":"older","occurred_at":"2026-07-15T13:00:00-04:00"}`)
	post(`{"source":"mcq","type":"question_skipped","summary":"newer","occurred_at":"2026-07-15T18:30:00Z"}`)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/memory/events", nil)
	request.Header.Set("Authorization", "Bearer dev:kevin:personal-kevin")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET events status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var envelope struct {
		Data []memory.Event `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode events: %v", err)
	}
	if len(envelope.Data) != 2 || envelope.Data[0].Summary != "newer" || envelope.Data[1].Summary != "older" {
		t.Fatalf("event order = %#v", envelope.Data)
	}
	if envelope.Data[1].OccurredAt.Location() != time.UTC || !strings.Contains(recorder.Body.String(), "2026-07-15T17:00:00Z") {
		t.Fatalf("timestamps were not normalized to UTC: %s", recorder.Body.String())
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
	request := httptest.NewRequest(http.MethodPost, "/api/v1/generate", strings.NewReader(`{"kind":"interview"}`))
	request.Header.Set("Authorization", "Bearer dev:kevin:personal-kevin")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501: %s", recorder.Code, recorder.Body.String())
	}
}

func TestGenerateRoutePersistsWorkspaceScopedProblem(t *testing.T) {
	memoryService := memory.NewService(memory.NewInMemoryStore(), nil)
	orchestrator := generation.NewOrchestrator(memoryService, independentProblemGenerator{})
	router := newGenerateTestRouter(t, orchestrator, memoryService)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/generate", strings.NewReader(`{"kind":"problem","spec":{"topic":"strings","difficulty":"easy"}}`))
	request.Header.Set("Authorization", "Bearer dev:kevin:personal-kevin")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var envelope struct {
		Data struct {
			ProblemID string `json:"problem_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil || envelope.Data.ProblemID == "" {
		t.Fatalf("response = %s err=%v", recorder.Body.String(), err)
	}
	get := httptest.NewRecorder()
	getRequest := httptest.NewRequest(http.MethodGet, "/api/v1/problems/"+envelope.Data.ProblemID, nil)
	getRequest.Header.Set("Authorization", "Bearer dev:kevin:personal-kevin")
	router.ServeHTTP(get, getRequest)
	if get.Code != http.StatusOK || strings.Contains(get.Body.String(), "reference_solution") ||
		strings.Contains(get.Body.String(), "CODEGYM_RESULT") || strings.Contains(get.Body.String(), `"spaces"`) ||
		!strings.Contains(get.Body.String(), `"one"`) {
		t.Fatalf("unsafe or missing public problem: %d %s", get.Code, get.Body.String())
	}
	otherScope := httptest.NewRecorder()
	otherRequest := httptest.NewRequest(http.MethodGet, "/api/v1/problems/"+envelope.Data.ProblemID, nil)
	otherRequest.Header.Set("Authorization", "Bearer dev:other:other-workspace")
	router.ServeHTTP(otherScope, otherRequest)
	if otherScope.Code != http.StatusNotFound {
		t.Fatalf("cross-workspace problem status=%d body=%s", otherScope.Code, otherScope.Body.String())
	}
}

func TestProblemGenerationRejectsExplicitlyStaleMemory(t *testing.T) {
	memoryService := memory.NewService(memory.NewInMemoryStore(), nil)
	orchestrator := generation.NewOrchestrator(memoryService, staticGenerator{payload: `{}`})
	router := newGenerateTestRouter(t, orchestrator, memoryService)
	create := httptest.NewRecorder()
	createRequest := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", strings.NewReader(
		`{"kind":"workspace","problem_id":"two-sum","state":{"schema_version":1,"memory_update_status":"failed"}}`,
	))
	createRequest.Header.Set("Authorization", "Bearer dev:kevin:personal-kevin")
	router.ServeHTTP(create, createRequest)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", create.Code, create.Body.String())
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/generate", strings.NewReader(`{"kind":"problem","spec":{"topic":"arrays"}}`))
	request.Header.Set("Authorization", "Bearer dev:kevin:personal-kevin")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "memory_update_required") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
