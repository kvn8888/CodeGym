package api

import (
	"context"
	"errors"
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
	"github.com/kvn8888/codegym/backend/internal/submission"
)

type capturingRunner struct {
	spec execution.RunSpec
}

type failingEventStore struct {
	*memory.InMemoryStore
}

func (s *failingEventStore) AppendEvent(context.Context, memory.Event) error {
	return errors.New("event store unavailable")
}

func (r *capturingRunner) Run(_ context.Context, spec execution.RunSpec) (execution.RunOutcome, error) {
	r.spec = spec
	exitCode := 0
	return execution.RunOutcome{
		ExitCode: 0,
		Result: execution.JudgeResult{
			Schema: execution.JudgeSchema, Status: execution.JudgeStatusPassed,
			ExitCode: &exitCode, DurationMs: 125, Stdout: "learner debug\n",
			Cases: []execution.CaseResult{
				{Name: "basic", Status: "pass", DurationMs: 3},
				{Name: "duplicates", Status: "pass", DurationMs: 4},
			},
		},
		Stdout:   "learner debug\n",
		Output:   "learner debug\n",
		Duration: 125 * time.Millisecond,
	}, nil
}

func TestRouterTwoSumSubmissionAndSessionCompletion(t *testing.T) {
	problemService := problems.NewService(problems.NewInMemoryStore())
	if err := problemService.EnsureSeed(context.Background()); err != nil {
		t.Fatalf("seed problem: %v", err)
	}
	sessionService := session.NewService(session.NewInMemoryStore(), nil)
	runner := &capturingRunner{}
	executionService := execution.NewService(execution.NewInMemoryStore(), runner, nil)
	memoryService := memory.NewService(memory.NewInMemoryStore(), nil)
	submissionService := submission.NewService(problemService, executionService, sessionService, memoryService, nil)

	router := NewRouter(Dependencies{
		Authenticator: auth.NewDevAuthenticator(auth.DevAuthenticatorConfig{}),
		Identity:      identity.NewService(identity.NewInMemoryStore()),
		Memory:        memoryService,
		Sessions:      sessionService,
		Execution:     executionService,
		Problems:      problemService,
		Submissions:   submissionService,
	})

	problemResponse := authedRequest(t, router, http.MethodGet, "/api/v1/problems/two-sum", "")
	if problemResponse.Code != http.StatusOK {
		t.Fatalf("problem status = %d: %s", problemResponse.Code, problemResponse.Body.String())
	}
	skeletonResponse := authedRequest(t, router, http.MethodGet, "/api/v1/problems/two-sum/skeleton", "")
	if skeletonResponse.Code != http.StatusOK {
		t.Fatalf("skeleton status = %d: %s", skeletonResponse.Code, skeletonResponse.Body.String())
	}
	for _, body := range []string{problemResponse.Body.String(), skeletonResponse.Body.String()} {
		if strings.Contains(body, "CODEGYM_RESULT") ||
			strings.Contains(body, "seen[complement]") ||
			strings.Contains(body, "test_solution.py") {
			t.Fatalf("public problem endpoint leaked a hidden artifact: %s", body)
		}
	}

	createSession := authedRequest(
		t,
		router,
		http.MethodPost,
		"/api/v1/sessions",
		`{"kind":"workspace","title":"Two Sum","problem_id":"two-sum","state":{"schema_version":1,"hints_revealed":0}}`,
	)
	if createSession.Code != http.StatusCreated {
		t.Fatalf("create session status = %d: %s", createSession.Code, createSession.Body.String())
	}
	created := decodeEnvelopeData[session.Session](t, createSession)

	submit := authedRequest(
		t,
		router,
		http.MethodPost,
		"/api/v1/submissions",
		`{"problem_id":"two-sum","session_id":"`+created.ID+`","files":[{"path":"solution.py","content":"def two_sum(nums, target):\n    return [0, 1]\n"}]}`,
	)
	if submit.Code != http.StatusAccepted {
		t.Fatalf("submit status = %d: %s", submit.Code, submit.Body.String())
	}
	accepted := decodeEnvelopeData[submission.Accepted](t, submit)

	foundHiddenTest := false
	for _, file := range runner.spec.Files {
		if file.Path == "test_solution.py" && strings.Contains(file.Content, "cases.jsonl") && strings.Contains(file.Content, "verdict.json") {
			foundHiddenTest = true
			if !strings.Contains(file.Content, "handles duplicate values") {
				t.Fatal("default submission did not receive the full hidden suite")
			}
		}
		if strings.Contains(file.Content, "seen[complement]") {
			t.Fatal("reference solution was assembled into the user execution")
		}
	}
	if !foundHiddenTest || runner.spec.Entrypoint != "test_solution.py" {
		t.Fatalf("captured run spec = %#v", runner.spec)
	}
	if runner.spec.Limits != (execution.Limits{TimeoutSeconds: 30, MemoryMB: 256, NetworkMode: execution.NetworkModeBlockAll}) {
		t.Fatalf("problem runtime did not reach the runner: %#v", runner.spec.Limits)
	}

	getSubmission := authedRequest(t, router, http.MethodGet, "/api/v1/submissions/"+accepted.SubmissionID, "")
	if getSubmission.Code != http.StatusOK {
		t.Fatalf("get submission status = %d: %s", getSubmission.Code, getSubmission.Body.String())
	}
	view := decodeEnvelopeData[submission.View](t, getSubmission)
	if view.Status != submission.StatusCompleted || view.Result == nil ||
		view.Result.Status != "pass" || view.Result.Passed != 2 || view.Stdout != "learner debug\n" ||
		view.Mode != submission.ModeSubmit || view.ExecutedCount != 2 {
		t.Fatalf("submission view = %#v", view)
	}

	getSession := authedRequest(t, router, http.MethodGet, "/api/v1/sessions/"+created.ID, "")
	updatedSession := decodeEnvelopeData[session.Session](t, getSession)
	if updatedSession.Status != session.StatusCompleted ||
		len(updatedSession.Files) != 1 ||
		updatedSession.Files[0].Path != "solution.py" ||
		!strings.Contains(string(updatedSession.State), `"last_submission_id":"`+accepted.SubmissionID+`"`) {
		t.Fatalf("updated session = %#v", updatedSession)
	}

	getExecution := authedRequest(t, router, http.MethodGet, "/api/v1/executions/"+accepted.SubmissionID, "")
	if strings.Contains(getExecution.Body.String(), `"files"`) ||
		strings.Contains(getExecution.Body.String(), "seen[complement]") {
		t.Fatalf("execution endpoint leaked stored run files: %s", getExecution.Body.String())
	}

	eventsResponse := authedRequest(t, router, http.MethodGet, "/api/v1/memory/events", "")
	for _, eventType := range []string{"attempt_started", "attempt_submitted", "tests_run", "attempt_solved"} {
		if !strings.Contains(eventsResponse.Body.String(), eventType) {
			t.Fatalf("memory events missing %s: %s", eventType, eventsResponse.Body.String())
		}
	}
	if !strings.Contains(eventsResponse.Body.String(), `"tags"`) {
		t.Fatalf("memory events missing governed problem tags: %s", eventsResponse.Body.String())
	}
	if strings.Contains(eventsResponse.Body.String(), "def two_sum") ||
		strings.Contains(eventsResponse.Body.String(), "CODEGYM_RESULT") {
		t.Fatalf("memory event leaked code or hidden tests: %s", eventsResponse.Body.String())
	}
}

func TestRunModeUsesOnlyPublicCasesWithoutSubmissionSideEffects(t *testing.T) {
	problemService := problems.NewService(problems.NewInMemoryStore())
	if err := problemService.EnsureSeed(context.Background()); err != nil {
		t.Fatal(err)
	}
	sessionService := session.NewService(session.NewInMemoryStore(), nil)
	runner := &capturingRunner{}
	executionService := execution.NewService(execution.NewInMemoryStore(), runner, nil)
	memoryService := memory.NewService(memory.NewInMemoryStore(), nil)
	submissionService := submission.NewService(problemService, executionService, sessionService, memoryService, nil)
	router := NewRouter(Dependencies{
		Authenticator: auth.NewDevAuthenticator(auth.DevAuthenticatorConfig{}),
		Identity:      identity.NewService(identity.NewInMemoryStore()), Memory: memoryService,
		Sessions: sessionService, Execution: executionService, Problems: problemService, Submissions: submissionService,
	})

	create := authedRequest(t, router, http.MethodPost, "/api/v1/sessions",
		`{"kind":"workspace","title":"Two Sum run","problem_id":"two-sum","state":{"schema_version":1}}`)
	created := decodeEnvelopeData[session.Session](t, create)
	runResponse := authedRequest(t, router, http.MethodPost, "/api/v1/submissions",
		`{"problem_id":"two-sum","session_id":"`+created.ID+`","mode":"run","files":[{"path":"solution.py","content":"def two_sum(nums,target): return [0,1]"}]}`)
	if runResponse.Code != http.StatusAccepted {
		t.Fatalf("run status=%d body=%s", runResponse.Code, runResponse.Body.String())
	}
	accepted := decodeEnvelopeData[submission.Accepted](t, runResponse)
	if accepted.MemoryUpdateStatus != "" {
		t.Fatalf("run memory status = %q", accepted.MemoryUpdateStatus)
	}
	publicHarness := ""
	for _, file := range runner.spec.Files {
		if file.Path == "test_solution.py" {
			publicHarness = file.Content
		}
	}
	if publicHarness == "" || !strings.Contains(publicHarness, "finds a pair without relying on order") ||
		strings.Contains(publicHarness, "handles duplicate values") || strings.Contains(publicHarness, "handles negative values") {
		t.Fatalf("run received wrong unit suite: %s", publicHarness)
	}

	viewResponse := authedRequest(t, router, http.MethodGet, "/api/v1/submissions/"+accepted.SubmissionID, "")
	view := decodeEnvelopeData[submission.View](t, viewResponse)
	if view.Mode != submission.ModeRun || view.ExecutedCount != 2 || view.Result == nil || view.Result.Total != 2 {
		t.Fatalf("run view = %#v", view)
	}
	storedSession := decodeEnvelopeData[session.Session](t,
		authedRequest(t, router, http.MethodGet, "/api/v1/sessions/"+created.ID, ""))
	if storedSession.Status != session.StatusActive || len(storedSession.Files) != 0 || strings.Contains(string(storedSession.State), "last_submission_id") {
		t.Fatalf("run mutated session = %#v", storedSession)
	}
	events := authedRequest(t, router, http.MethodGet, "/api/v1/memory/events", "")
	for _, forbidden := range []string{"attempt_started", "attempt_submitted", "tests_run", "attempt_solved", "attempt_failed"} {
		if strings.Contains(events.Body.String(), forbidden) {
			t.Fatalf("run emitted %s: %s", forbidden, events.Body.String())
		}
	}

	httpRun := authedRequest(t, router, http.MethodPost, "/api/v1/submissions",
		`{"problem_id":"go-http-items","mode":"run","files":[{"path":"main.go","content":"package main\\nfunc main() {}\\n"}]}`)
	if httpRun.Code != http.StatusAccepted {
		t.Fatalf("HTTP run status=%d body=%s", httpRun.Code, httpRun.Body.String())
	}
	httpCases := ""
	for _, file := range runner.spec.Files {
		if file.Path == "codegym_http_cases.json" {
			httpCases = file.Content
		}
	}
	if runner.spec.Strategy != execution.TestStrategyHTTP || !strings.Contains(httpCases, "gets-the-created-item") ||
		strings.Contains(httpCases, "returns-not-found-for-an-unknown-item") || strings.Contains(httpCases, "increments-item-identifiers") {
		t.Fatalf("run received wrong HTTP suite: strategy=%s cases=%s", runner.spec.Strategy, httpCases)
	}
}

func TestSubmissionRejectsUnknownMode(t *testing.T) {
	problemService := problems.NewService(problems.NewInMemoryStore())
	if err := problemService.EnsureSeed(context.Background()); err != nil {
		t.Fatal(err)
	}
	executionService := execution.NewService(execution.NewInMemoryStore(), &capturingRunner{}, nil)
	router := NewRouter(Dependencies{
		Authenticator: auth.NewDevAuthenticator(auth.DevAuthenticatorConfig{}),
		Identity:      identity.NewService(identity.NewInMemoryStore()), Problems: problemService,
		Execution: executionService, Submissions: submission.NewService(problemService, executionService, nil, nil, nil),
	})
	response := authedRequest(t, router, http.MethodPost, "/api/v1/submissions",
		`{"problem_id":"two-sum","mode":"preview","files":[{"path":"solution.py","content":"pass"}]}`)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid mode status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestTerminalSubmissionSurvivesMemoryPersistenceFailure(t *testing.T) {
	problemService := problems.NewService(problems.NewInMemoryStore())
	if err := problemService.EnsureSeed(context.Background()); err != nil {
		t.Fatal(err)
	}
	sessionService := session.NewService(session.NewInMemoryStore(), nil)
	executionService := execution.NewService(execution.NewInMemoryStore(), &capturingRunner{}, nil)
	memoryService := memory.NewService(&failingEventStore{memory.NewInMemoryStore()}, nil)
	profiles := generation.NewProfileSynthesizer(nil, memoryService)
	submissionService := submission.NewService(problemService, executionService, sessionService, memoryService, profiles)
	router := NewRouter(Dependencies{
		Authenticator: auth.NewDevAuthenticator(auth.DevAuthenticatorConfig{}),
		Identity:      identity.NewService(identity.NewInMemoryStore()), Memory: memoryService,
		Sessions: sessionService, Execution: executionService, Problems: problemService,
		Submissions: submissionService, MemoryProfiles: profiles,
	})
	create := authedRequest(t, router, http.MethodPost, "/api/v1/sessions",
		`{"kind":"workspace","problem_id":"two-sum","state":{"schema_version":1}}`)
	created := decodeEnvelopeData[session.Session](t, create)
	submit := authedRequest(t, router, http.MethodPost, "/api/v1/submissions",
		`{"problem_id":"two-sum","session_id":"`+created.ID+`","files":[{"path":"solution.py","content":"def two_sum(nums,target): return [0,1]"}]}`)
	if submit.Code != http.StatusAccepted {
		t.Fatalf("submit status=%d body=%s", submit.Code, submit.Body.String())
	}
	accepted := decodeEnvelopeData[submission.Accepted](t, submit)
	if accepted.MemoryUpdateStatus != "failed" || accepted.SubmissionID == "" {
		t.Fatalf("accepted=%#v", accepted)
	}
	found := authedRequest(t, router, http.MethodGet, "/api/v1/sessions/"+created.ID, "")
	updated := decodeEnvelopeData[session.Session](t, found)
	if updated.Status != session.StatusCompleted || !strings.Contains(string(updated.State), `"memory_update_status":"failed"`) {
		t.Fatalf("terminal result/state not preserved: %#v", updated)
	}
}

func authedRequest(t *testing.T, handler http.Handler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer dev:kevin:personal-kevin")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}
