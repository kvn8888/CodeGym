package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

type fixedRunner struct {
	outcome execution.RunOutcome
	err     error
}

func (r fixedRunner) Run(context.Context, execution.RunSpec) (execution.RunOutcome, error) {
	return r.outcome, r.err
}

type coachingUsageFake struct {
	used map[string]bool
	err  map[string]error
}

func (f coachingUsageFake) CodingCoachUsed(_ context.Context, sessionID string, _ time.Time) (bool, error) {
	return f.used[sessionID], f.err[sessionID]
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
	assertCodingAttemptEvents(t, decodeEnvelopeData[[]memory.Event](t, eventsResponse), accepted.SubmissionID, memory.TypeAttemptSolved)
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

func TestSubmissionReportsFailedMemoryWhenCodingNoteCandidateIsInvalid(t *testing.T) {
	problemService := problems.NewService(problems.NewInMemoryStore())
	if err := problemService.EnsureSeed(context.Background()); err != nil {
		t.Fatal(err)
	}
	sessionService := session.NewService(session.NewInMemoryStore(), nil)
	executionService := execution.NewService(execution.NewInMemoryStore(), &capturingRunner{}, nil)
	memoryService := memory.NewService(memory.NewInMemoryStore(), nil)
	generator := &sequencedGenerator{payloads: []string{
		`{"actions":[{"op":"create","note":{"id":"note_queue","title":"Queues","summary":"Missing required coding source.","tags":["queues"],"action":"review"}}]}`,
	}}
	profiles := generation.NewProfileSynthesizer(generation.NewOrchestrator(memoryService, generator), memoryService)
	submissionService := submission.NewService(problemService, executionService, sessionService, memoryService, profiles)
	router := NewRouter(Dependencies{
		Authenticator: auth.NewDevAuthenticator(auth.DevAuthenticatorConfig{}),
		Identity:      identity.NewService(identity.NewInMemoryStore()), Memory: memoryService,
		Sessions: sessionService, Execution: executionService, Problems: problemService,
		Submissions: submissionService, MemoryProfiles: profiles,
	})
	created := decodeEnvelopeData[session.Session](t, authedRequest(t, router, http.MethodPost, "/api/v1/sessions",
		`{"kind":"workspace","problem_id":"two-sum","state":{"schema_version":1,"hints_revealed":0}}`))
	accepted := decodeEnvelopeData[submission.Accepted](t, authedRequest(t, router, http.MethodPost, "/api/v1/submissions",
		`{"problem_id":"two-sum","session_id":"`+created.ID+`","mode":"submit","files":[{"path":"solution.py","content":"def two_sum(nums,target): return [0,1]"}]}`))
	if accepted.MemoryUpdateStatus != "failed" {
		t.Fatalf("accepted=%#v", accepted)
	}
	updated := decodeEnvelopeData[session.Session](t, authedRequest(t, router, http.MethodGet, "/api/v1/sessions/"+created.ID, ""))
	if !strings.Contains(string(updated.State), `"memory_update_status":"failed"`) {
		t.Fatalf("session state=%s", updated.State)
	}
	if len(generator.requests) != 1 || generator.requests[0].Kind != generation.KindNotes {
		t.Fatalf("requests=%#v", generator.requests)
	}
}

func TestGradedFailedSubmissionEmitsStableCompletionEvidence(t *testing.T) {
	problemService := problems.NewService(problems.NewInMemoryStore())
	if err := problemService.EnsureSeed(context.Background()); err != nil {
		t.Fatal(err)
	}
	exitCode := 1
	runner := fixedRunner{outcome: execution.RunOutcome{
		ExitCode: 1,
		Result: execution.JudgeResult{
			Schema: execution.JudgeSchema, Status: execution.JudgeStatusFailed,
			ExitCode: &exitCode, DurationMs: 25,
			Cases: []execution.CaseResult{{Name: "basic", Status: "fail", DurationMs: 2}},
		},
	}}
	sessionService := session.NewService(session.NewInMemoryStore(), nil)
	memoryService := memory.NewService(memory.NewInMemoryStore(), nil)
	executionService := execution.NewService(execution.NewInMemoryStore(), runner, nil)
	submissionService := submission.NewService(problemService, executionService, sessionService, memoryService, nil)
	router := NewRouter(Dependencies{
		Authenticator: auth.NewDevAuthenticator(auth.DevAuthenticatorConfig{}),
		Identity:      identity.NewService(identity.NewInMemoryStore()), Memory: memoryService,
		Sessions: sessionService, Execution: executionService, Problems: problemService, Submissions: submissionService,
	})
	created := decodeEnvelopeData[session.Session](t, authedRequest(t, router, http.MethodPost, "/api/v1/sessions",
		`{"kind":"workspace","problem_id":"two-sum","state":{"schema_version":1,"hints_revealed":0}}`))
	accepted := decodeEnvelopeData[submission.Accepted](t, authedRequest(t, router, http.MethodPost, "/api/v1/submissions",
		`{"problem_id":"two-sum","session_id":"`+created.ID+`","mode":"submit","files":[{"path":"solution.py","content":"def two_sum(nums,target): return []"}]}`))

	stored := decodeEnvelopeData[session.Session](t, authedRequest(t, router, http.MethodGet, "/api/v1/sessions/"+created.ID, ""))
	if stored.Status != session.StatusActive {
		t.Fatalf("failed graded session status = %s", stored.Status)
	}
	events := decodeEnvelopeData[[]memory.Event](t, authedRequest(t, router, http.MethodGet, "/api/v1/memory/events", ""))
	assertCodingAttemptEvents(t, events, accepted.SubmissionID, memory.TypeAttemptFailed)
}

func TestUngradedTerminalSubmissionsDoNotEmitLearningEvidence(t *testing.T) {
	tests := []struct {
		name    string
		outcome execution.RunOutcome
		err     error
	}{
		{name: "timeout", outcome: terminalJudgeOutcome(execution.JudgeStatusTimeout)},
		{name: "out_of_memory", outcome: terminalJudgeOutcome(execution.JudgeStatusOutOfMemory)},
		{name: "crashed", outcome: terminalJudgeOutcome(execution.JudgeStatusCrashed)},
		{name: "platform_error", err: errors.New("sandbox provider unavailable")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			problemService := problems.NewService(problems.NewInMemoryStore())
			if err := problemService.EnsureSeed(context.Background()); err != nil {
				t.Fatal(err)
			}
			sessionService := session.NewService(session.NewInMemoryStore(), nil)
			memoryService := memory.NewService(memory.NewInMemoryStore(), nil)
			executionService := execution.NewService(execution.NewInMemoryStore(), fixedRunner{outcome: test.outcome, err: test.err}, nil)
			submissionService := submission.NewService(problemService, executionService, sessionService, memoryService, nil)
			router := NewRouter(Dependencies{
				Authenticator: auth.NewDevAuthenticator(auth.DevAuthenticatorConfig{}),
				Identity:      identity.NewService(identity.NewInMemoryStore()), Memory: memoryService,
				Sessions: sessionService, Execution: executionService, Problems: problemService, Submissions: submissionService,
			})
			created := decodeEnvelopeData[session.Session](t, authedRequest(t, router, http.MethodPost, "/api/v1/sessions",
				`{"kind":"workspace","problem_id":"two-sum","state":{"schema_version":1,"hints_revealed":0}}`))
			accepted := decodeEnvelopeData[submission.Accepted](t, authedRequest(t, router, http.MethodPost, "/api/v1/submissions",
				`{"problem_id":"two-sum","session_id":"`+created.ID+`","mode":"submit","files":[{"path":"solution.py","content":"pass"}]}`))
			if accepted.MemoryUpdateStatus != "" {
				t.Fatalf("memory status = %q", accepted.MemoryUpdateStatus)
			}
			events := decodeEnvelopeData[[]memory.Event](t, authedRequest(t, router, http.MethodGet, "/api/v1/memory/events", ""))
			if len(events) != 0 {
				t.Fatalf("ungraded execution emitted learning evidence: %#v", events)
			}
			stored := decodeEnvelopeData[session.Session](t, authedRequest(t, router, http.MethodGet, "/api/v1/sessions/"+created.ID, ""))
			if strings.Contains(string(stored.State), "memory_update_status") || strings.Contains(string(stored.State), "last_submission_id") {
				t.Fatalf("ungraded execution triggered completion refresh state: %s", stored.State)
			}
		})
	}
}

func TestCodingCompletionEvidencePreservesAssistanceCertaintyAndScope(t *testing.T) {
	problemService := problems.NewService(problems.NewInMemoryStore())
	if err := problemService.EnsureSeed(context.Background()); err != nil {
		t.Fatal(err)
	}
	sessionService := session.NewService(session.NewInMemoryStore(), nil)
	memoryService := memory.NewService(memory.NewInMemoryStore(), nil)
	executionService := execution.NewService(execution.NewInMemoryStore(), &capturingRunner{}, nil)
	lookup := coachingUsageFake{used: map[string]bool{}, err: map[string]error{}}
	submissionService := submission.NewService(problemService, executionService, sessionService, memoryService, nil).
		WithCoachingUsageLookup(lookup)
	router := NewRouter(Dependencies{
		Authenticator: auth.NewDevAuthenticator(auth.DevAuthenticatorConfig{}),
		Identity:      identity.NewService(identity.NewInMemoryStore()), Memory: memoryService,
		Sessions: sessionService, Execution: executionService, Problems: problemService, Submissions: submissionService,
	})

	states := []string{
		`{"schema_version":1,"hints_revealed":1}`,
		`{"schema_version":1,"hints_revealed":0}`,
		`{"schema_version":1}`,
		`{"schema_version":1,"hints_revealed":0}`,
	}
	sessionIDs := make([]string, 0, len(states))
	for index, state := range states {
		created := decodeEnvelopeData[session.Session](t, authedRequest(t, router, http.MethodPost, "/api/v1/sessions",
			`{"kind":"workspace","problem_id":"two-sum","state":`+state+`}`))
		sessionIDs = append(sessionIDs, created.ID)
		if index == 3 {
			lookup.used[created.ID] = true
		}
		response := authedRequest(t, router, http.MethodPost, "/api/v1/submissions",
			`{"problem_id":"two-sum","session_id":"`+created.ID+`","mode":"submit","files":[{"path":"solution.py","content":"def two_sum(nums,target): return [0,1]"}]}`)
		if response.Code != http.StatusAccepted {
			t.Fatalf("submit status=%d body=%s", response.Code, response.Body.String())
		}
	}

	events := decodeEnvelopeData[[]memory.Event](t, authedRequest(t, router, http.MethodGet, "/api/v1/memory/events", ""))
	want := map[string]string{
		sessionIDs[0]: "used", sessionIDs[1]: "not_used",
		sessionIDs[2]: "unknown", sessionIDs[3]: "used",
	}
	seen := map[string]bool{}
	for _, event := range events {
		if event.Type != memory.TypeAttemptSolved {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		sessionID, _ := payload["session_id"].(string)
		seen[sessionID] = true
		if got, _ := payload["assistance_status"].(string); got != want[sessionID] {
			t.Fatalf("session %s assistance=%q payload=%#v", sessionID, got, payload)
		}
		if want[sessionID] == "unknown" {
			if _, exists := payload["used_help"]; exists {
				t.Fatalf("unknown assistance asserted used_help: %#v", payload)
			}
		}
		if sessionID == sessionIDs[3] && payload["coaching_used"] != true {
			t.Fatalf("coaching assistance was not preserved: %#v", payload)
		}
	}
	for sessionID := range want {
		if !seen[sessionID] {
			t.Fatalf("missing completion evidence for session %s", sessionID)
		}
	}

	other := httptest.NewRequest(http.MethodGet, "/api/v1/memory/events", nil)
	other.Header.Set("Authorization", "Bearer dev:other:personal-other")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, other)
	otherEvents := decodeEnvelopeData[[]memory.Event](t, recorder)
	if len(otherEvents) != 0 {
		t.Fatalf("cross-user evidence leak: %#v", otherEvents)
	}
}

func terminalJudgeOutcome(status execution.JudgeStatus) execution.RunOutcome {
	exitCode := 1
	return execution.RunOutcome{ExitCode: 1, Result: execution.JudgeResult{
		Schema: execution.JudgeSchema, Status: status, ExitCode: &exitCode, DurationMs: 20,
	}}
}

func assertCodingAttemptEvents(t *testing.T, events []memory.Event, submissionID, outcomeType string) {
	t.Helper()
	want := map[string]bool{
		memory.TypeAttemptStarted: false, memory.TypeAttemptSubmitted: false,
		memory.TypeTestsRun: false, outcomeType: false,
	}
	for _, event := range events {
		if _, expected := want[event.Type]; !expected {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if payload["submission_id"] != submissionID {
			t.Fatalf("%s submission_id=%v want %s", event.Type, payload["submission_id"], submissionID)
		}
		if event.Type == outcomeType && payload["graded"] != true {
			t.Fatalf("%s missing graded completion marker: %#v", event.Type, payload)
		}
		if event.WorkspaceID == "" || event.UserID == "" || payload["problem_id"] != "two-sum" || payload["session_id"] == "" {
			t.Fatalf("%s missing scoped references: event=%#v payload=%#v", event.Type, event, payload)
		}
		want[event.Type] = true
	}
	for eventType, found := range want {
		if !found {
			t.Fatalf("missing %s in %#v", eventType, events)
		}
	}
}

type countingRunner struct {
	calls int
}

func (r *countingRunner) Run(context.Context, execution.RunSpec) (execution.RunOutcome, error) {
	r.calls++
	exitCode := 0
	return execution.RunOutcome{
		ExitCode: 0,
		Result: execution.JudgeResult{
			Schema: execution.JudgeSchema, Status: execution.JudgeStatusPassed,
			ExitCode: &exitCode, DurationMs: 5,
			Cases: []execution.CaseResult{{Name: "basic", Status: "pass", DurationMs: 1}},
		},
		Output: "", Duration: 5 * time.Millisecond,
	}, nil
}

func TestSubmitRequestTokenReplaysOneLogicalSubmission(t *testing.T) {
	problemService := problems.NewService(problems.NewInMemoryStore())
	if err := problemService.EnsureSeed(context.Background()); err != nil {
		t.Fatal(err)
	}
	sessionService := session.NewService(session.NewInMemoryStore(), nil)
	runner := &countingRunner{}
	executionService := execution.NewService(execution.NewInMemoryStore(), runner, nil)
	memoryService := memory.NewService(memory.NewInMemoryStore(), nil)
	generator := &sequencedGenerator{payloads: []string{
		`{"actions":[{"op":"create","note":{"id":"note_two_sum","problem_id":"two-sum","title":"Two Sum","summary":"Practice two-sum edge cases.","tags":["dsa"],"action":"review"}}]}`,
		`{"summary":"Two Sum practice is progressing.","strengths":["Two Pointers"],"growth_edges":["Arrays"],"skills":[{"id":"two-sum","label":"Two Sum","area":"DSA","level":2,"confidence":60,"trend":"up"}],"notes":[]}`,
	}}
	profiles := generation.NewProfileSynthesizer(generation.NewOrchestrator(memoryService, generator), memoryService)
	submissionService := submission.NewService(problemService, executionService, sessionService, memoryService, profiles)
	router := NewRouter(Dependencies{
		Authenticator: auth.NewDevAuthenticator(auth.DevAuthenticatorConfig{}),
		Identity:      identity.NewService(identity.NewInMemoryStore()), Memory: memoryService,
		Sessions: sessionService, Execution: executionService, Problems: problemService,
		Submissions: submissionService, MemoryProfiles: profiles,
	})
	created := decodeEnvelopeData[session.Session](t, authedRequest(t, router, http.MethodPost, "/api/v1/sessions",
		`{"kind":"workspace","problem_id":"two-sum","state":{"schema_version":1,"hints_revealed":0}}`))

	submit := func(token, content string) *httptest.ResponseRecorder {
		body := `{"problem_id":"two-sum","session_id":"` + created.ID + `","mode":"submit","client_token":"` + token + `","files":[{"path":"solution.py","content":"` + content + `"}]}`
		return authedRequest(t, router, http.MethodPost, "/api/v1/submissions", body)
	}

	first := submit("tok-phase4-replay", "def two_sum(nums, target): return [0, 1]")
	firstAccepted := decodeEnvelopeData[submission.Accepted](t, first)
	if first.Code != http.StatusAccepted || firstAccepted.SubmissionID == "" {
		t.Fatalf("first submit status=%d accepted=%#v body=%s", first.Code, firstAccepted, first.Body.String())
	}
	if runner.calls != 1 {
		t.Fatalf("first submit executions=%d", runner.calls)
	}
	second := submit("tok-phase4-replay", "def two_sum(nums, target): return [0, 1]")
	secondAccepted := decodeEnvelopeData[submission.Accepted](t, second)
	if second.Code != http.StatusAccepted || secondAccepted.SubmissionID != firstAccepted.SubmissionID {
		t.Fatalf("replayed submit status=%d first=%q second=%q body=%s",
			second.Code, firstAccepted.SubmissionID, secondAccepted.SubmissionID, second.Body.String())
	}
	if runner.calls != 1 {
		t.Fatalf("uncertain retry created a duplicate execution: calls=%d", runner.calls)
	}
	profileBefore := decodeEnvelopeData[memory.Profile](t, authedRequest(t, router, http.MethodGet, "/api/v1/memory/profile", ""))
	noteAuditBefore := countNoteAuditEvents(t, router)
	if len(generator.requests) != 2 {
		t.Fatalf("first submit model calls=%d, want notes and profile stages", len(generator.requests))
	}

	conflict := submit("tok-phase4-replay", "def two_sum(nums, target): return [0, 2]")
	if conflict.Code != http.StatusConflict || !strings.Contains(conflict.Body.String(), "submission_token_conflict") {
		t.Fatalf("conflicting token reuse status=%d body=%s", conflict.Code, conflict.Body.String())
	}

	profileAfter := decodeEnvelopeData[memory.Profile](t, authedRequest(t, router, http.MethodGet, "/api/v1/memory/profile", ""))
	if profileAfter.Version != profileBefore.Version || len(profileAfter.Notes) != len(profileBefore.Notes) {
		t.Fatalf("replay changed the profile: before=%#v after=%#v", profileBefore, profileAfter)
	}
	if noteAuditAfter := countNoteAuditEvents(t, router); noteAuditAfter != noteAuditBefore {
		t.Fatalf("replay duplicated note audit events: before=%d after=%d", noteAuditBefore, noteAuditAfter)
	}
	if len(generator.requests) != 2 {
		t.Fatalf("replay regenerated notes/profile: model calls=%d", len(generator.requests))
	}
}

func TestMemoryMaintenanceRetryUsesExactCompletionIdentity(t *testing.T) {
	problemService := problems.NewService(problems.NewInMemoryStore())
	if err := problemService.EnsureSeed(context.Background()); err != nil {
		t.Fatal(err)
	}
	sessionService := session.NewService(session.NewInMemoryStore(), nil)
	runner := &countingRunner{}
	executionService := execution.NewService(execution.NewInMemoryStore(), runner, nil)
	memoryService := memory.NewService(memory.NewInMemoryStore(), nil)
	generator := &sequencedGenerator{payloads: []string{
		`{"actions":[{"op":"create","note":{"id":"note_two_sum","problem_id":"two-sum","title":"Two Sum","summary":"Practice two-sum edge cases.","tags":["dsa"],"action":"review"}}]}`,
		`{"summary":"Two Sum practice is progressing.","strengths":["Two Pointers"],"growth_edges":["Arrays"],"skills":[],"notes":[]}`,
	}}
	profiles := generation.NewProfileSynthesizer(generation.NewOrchestrator(memoryService, generator), memoryService)
	submissionService := submission.NewService(problemService, executionService, sessionService, memoryService, profiles)
	router := NewRouter(Dependencies{
		Authenticator: auth.NewDevAuthenticator(auth.DevAuthenticatorConfig{}),
		Identity:      identity.NewService(identity.NewInMemoryStore()), Memory: memoryService,
		Sessions: sessionService, Execution: executionService, Problems: problemService,
		Submissions: submissionService, MemoryProfiles: profiles,
	})
	created := decodeEnvelopeData[session.Session](t, authedRequest(t, router, http.MethodPost, "/api/v1/sessions",
		`{"kind":"workspace","problem_id":"two-sum","state":{"schema_version":1,"hints_revealed":0}}`))
	accepted := decodeEnvelopeData[submission.Accepted](t, authedRequest(t, router, http.MethodPost, "/api/v1/submissions",
		`{"problem_id":"two-sum","session_id":"`+created.ID+`","mode":"submit","files":[{"path":"solution.py","content":"def two_sum(nums, target): return [0, 1]"}]}`))
	if accepted.MemoryUpdateStatus != "synced" {
		t.Fatalf("graded submit memory status = %q", accepted.MemoryUpdateStatus)
	}
	profileBefore := decodeEnvelopeData[memory.Profile](t, authedRequest(t, router, http.MethodGet, "/api/v1/memory/profile", ""))
	modelCallsBefore := len(generator.requests)

	maintain := func(submissionID string) *httptest.ResponseRecorder {
		body := `{"session_id":"` + created.ID + `","problem_id":"two-sum","submission_id":"` + submissionID + `"}`
		return authedRequest(t, router, http.MethodPost, "/api/v1/memory/profile/maintain", body)
	}

	for attempt := 1; attempt <= 2; attempt++ {
		retry := maintain(accepted.SubmissionID)
		if retry.Code != http.StatusOK {
			t.Fatalf("maintenance retry %d status=%d body=%s", attempt, retry.Code, retry.Body.String())
		}
	}
	if runner.calls != 1 {
		t.Fatalf("maintenance retry re-ran learner code: executions=%d", runner.calls)
	}
	if len(generator.requests) != modelCallsBefore {
		t.Fatalf("maintenance retry regenerated notes: calls=%d want=%d", len(generator.requests), modelCallsBefore)
	}
	profileAfter := decodeEnvelopeData[memory.Profile](t, authedRequest(t, router, http.MethodGet, "/api/v1/memory/profile", ""))
	if profileAfter.Version != profileBefore.Version || len(profileAfter.Notes) != len(profileBefore.Notes) {
		t.Fatalf("maintenance retry changed the profile: before=%#v after=%#v", profileBefore, profileAfter)
	}

	stale := maintain("exec-stale-completion")
	if stale.Code != http.StatusConflict || !strings.Contains(stale.Body.String(), "memory_completion_evidence_missing") {
		t.Fatalf("stale completion retry status=%d body=%s", stale.Code, stale.Body.String())
	}
	stored := decodeEnvelopeData[session.Session](t, authedRequest(t, router, http.MethodGet, "/api/v1/sessions/"+created.ID, ""))
	var state map[string]any
	if err := json.Unmarshal(stored.State, &state); err != nil {
		t.Fatalf("decode session state: %v", err)
	}
	if state["memory_update_status"] != "synced" || state["last_submission_id"] != accepted.SubmissionID {
		t.Fatalf("stale completion cleared newer status: %#v", state)
	}
}

type blockingRunner struct {
	started chan struct{}
	release chan struct{}
	mu      sync.Mutex
	calls   int
}

func (r *blockingRunner) Run(ctx context.Context, _ execution.RunSpec) (execution.RunOutcome, error) {
	r.mu.Lock()
	r.calls++
	first := r.calls == 1
	r.mu.Unlock()
	if first {
		close(r.started)
	}
	select {
	case <-r.release:
	case <-ctx.Done():
		return execution.RunOutcome{}, ctx.Err()
	}
	exitCode := 0
	return execution.RunOutcome{
		ExitCode: 0,
		Result: execution.JudgeResult{
			Schema: execution.JudgeSchema, Status: execution.JudgeStatusPassed,
			ExitCode: &exitCode, DurationMs: 5,
			Cases: []execution.CaseResult{{Name: "basic", Status: "pass", DurationMs: 1}},
		},
		Output: "", Duration: 5 * time.Millisecond,
	}, nil
}

func TestUnboundSubmitTokenReturnsRetryableConflict(t *testing.T) {
	problemService := problems.NewService(problems.NewInMemoryStore())
	if err := problemService.EnsureSeed(context.Background()); err != nil {
		t.Fatal(err)
	}
	sessionService := session.NewService(session.NewInMemoryStore(), nil)
	runner := &blockingRunner{started: make(chan struct{}), release: make(chan struct{})}
	executionService := execution.NewService(execution.NewInMemoryStore(), runner, nil)
	memoryService := memory.NewService(memory.NewInMemoryStore(), nil)
	submissionService := submission.NewService(problemService, executionService, sessionService, memoryService, nil).
		WithSubmissionRequestBindTimeout(20 * time.Millisecond)
	router := NewRouter(Dependencies{
		Authenticator: auth.NewDevAuthenticator(auth.DevAuthenticatorConfig{}),
		Identity:      identity.NewService(identity.NewInMemoryStore()), Memory: memoryService,
		Sessions: sessionService, Execution: executionService, Problems: problemService,
		Submissions: submissionService,
	})
	created := decodeEnvelopeData[session.Session](t, authedRequest(t, router, http.MethodPost, "/api/v1/sessions",
		`{"kind":"workspace","problem_id":"two-sum","state":{"schema_version":1,"hints_revealed":0}}`))
	body := `{"problem_id":"two-sum","session_id":"` + created.ID + `","mode":"submit","client_token":"tok-unbound","files":[{"path":"solution.py","content":"def two_sum(nums, target): return [0, 1]"}]}`

	firstDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		firstDone <- authedRequest(t, router, http.MethodPost, "/api/v1/submissions", body)
	}()
	<-runner.started

	duplicate := authedRequest(t, router, http.MethodPost, "/api/v1/submissions", body)
	if duplicate.Code != http.StatusConflict || !strings.Contains(duplicate.Body.String(), "submission_in_progress") {
		t.Fatalf("unbound duplicate status=%d body=%s", duplicate.Code, duplicate.Body.String())
	}
	if runner.calls != 1 {
		t.Fatalf("unbound duplicate started another execution: calls=%d", runner.calls)
	}

	close(runner.release)
	first := <-firstDone
	if first.Code != http.StatusAccepted {
		t.Fatalf("original submit status=%d body=%s", first.Code, first.Body.String())
	}
	if runner.calls != 1 {
		t.Fatalf("duplicate retries started another execution: calls=%d", runner.calls)
	}
}

// gatedGenerator blocks the first model call until released, so a test can
// observe the memory status while the owner request is mid-refresh.
type gatedGenerator struct {
	mu       sync.Mutex
	requests []generation.GenerateRequest
	started  chan struct{}
	release  chan struct{}
	payloads []string
}

func (g *gatedGenerator) Generate(_ context.Context, request generation.GenerateRequest) (generation.GenerateResult, error) {
	g.mu.Lock()
	g.requests = append(g.requests, request)
	index := len(g.requests) - 1
	g.mu.Unlock()
	if index == 0 {
		close(g.started)
		<-g.release
	}
	if index >= len(g.payloads) {
		index = len(g.payloads) - 1
	}
	return generation.GenerateResult{
		Object:   []byte(g.payloads[index]),
		Provider: "gated",
		Model:    "fake-model",
	}, nil
}

const codingNotesPayload = `{"actions":[{"op":"create","note":{"id":"note_two_sum","problem_id":"two-sum","title":"Two Sum","summary":"Practice two-sum edge cases.","tags":["dsa"],"action":"review"}}]}`
const codingProfilePayload = `{"summary":"Two Sum practice is progressing.","strengths":["Two Pointers"],"growth_edges":["Arrays"],"skills":[],"notes":[]}`

func TestJoiningSubmitReportsNoMemoryStatusBeforeOwnerRefreshCompletes(t *testing.T) {
	problemService := problems.NewService(problems.NewInMemoryStore())
	if err := problemService.EnsureSeed(context.Background()); err != nil {
		t.Fatal(err)
	}
	sessionService := session.NewService(session.NewInMemoryStore(), nil)
	executionService := execution.NewService(execution.NewInMemoryStore(), &countingRunner{}, nil)
	memoryService := memory.NewService(memory.NewInMemoryStore(), nil)
	generator := &gatedGenerator{
		started:  make(chan struct{}),
		release:  make(chan struct{}),
		payloads: []string{codingNotesPayload, codingProfilePayload},
	}
	profiles := generation.NewProfileSynthesizer(generation.NewOrchestrator(memoryService, generator), memoryService)
	submissionService := submission.NewService(problemService, executionService, sessionService, memoryService, profiles)
	router := NewRouter(Dependencies{
		Authenticator: auth.NewDevAuthenticator(auth.DevAuthenticatorConfig{}),
		Identity:      identity.NewService(identity.NewInMemoryStore()), Memory: memoryService,
		Sessions: sessionService, Execution: executionService, Problems: problemService,
		Submissions: submissionService, MemoryProfiles: profiles,
	})
	created := decodeEnvelopeData[session.Session](t, authedRequest(t, router, http.MethodPost, "/api/v1/sessions",
		`{"kind":"workspace","problem_id":"two-sum","state":{"schema_version":1,"hints_revealed":0}}`))
	body := `{"problem_id":"two-sum","session_id":"` + created.ID + `","mode":"submit","client_token":"tok-join-status","files":[{"path":"solution.py","content":"def two_sum(nums, target): return [0, 1]"}]}`

	ownerDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		ownerDone <- authedRequest(t, router, http.MethodPost, "/api/v1/submissions", body)
	}()
	<-generator.started

	readState := func() map[string]any {
		stored := decodeEnvelopeData[session.Session](t, authedRequest(t, router, http.MethodGet, "/api/v1/sessions/"+created.ID, ""))
		state := map[string]any{}
		if err := json.Unmarshal(stored.State, &state); err != nil {
			t.Fatalf("decode session state: %v", err)
		}
		return state
	}
	if state := readState(); state["memory_update_status"] != "pending" {
		t.Fatalf("owner refresh did not claim pending status: %#v", state)
	}

	joined := authedRequest(t, router, http.MethodPost, "/api/v1/submissions", body)
	joinedAccepted := decodeEnvelopeData[submission.Accepted](t, joined)
	if joined.Code != http.StatusAccepted {
		t.Fatalf("joined submit status=%d body=%s", joined.Code, joined.Body.String())
	}
	if joinedAccepted.MemoryUpdateStatus != "" {
		t.Fatalf("joined submit reported %q while refresh was still running", joinedAccepted.MemoryUpdateStatus)
	}
	if state := readState(); state["memory_update_status"] != "pending" {
		t.Fatalf("joined submit overwrote in-progress status: %#v", state)
	}

	close(generator.release)
	owner := <-ownerDone
	ownerAccepted := decodeEnvelopeData[submission.Accepted](t, owner)
	if owner.Code != http.StatusAccepted || ownerAccepted.MemoryUpdateStatus != "synced" {
		t.Fatalf("owner submit status=%d accepted=%#v body=%s", owner.Code, ownerAccepted, owner.Body.String())
	}
	if state := readState(); state["memory_update_status"] != "synced" {
		t.Fatalf("owner refresh did not settle status: %#v", state)
	}
	if len(generator.requests) != 2 {
		t.Fatalf("joined submit triggered extra model calls: %d", len(generator.requests))
	}
}

func TestStaleCompletionCannotOverwriteNewerPendingStatus(t *testing.T) {
	problemService := problems.NewService(problems.NewInMemoryStore())
	if err := problemService.EnsureSeed(context.Background()); err != nil {
		t.Fatal(err)
	}
	sessionStore := session.NewInMemoryStore()
	sessionService := session.NewService(sessionStore, nil)
	runner := &countingRunner{}
	executionService := execution.NewService(execution.NewInMemoryStore(), runner, nil)
	memoryService := memory.NewService(memory.NewInMemoryStore(), nil)
	generator := &sequencedGenerator{payloads: []string{
		`{}`,
		codingNotesPayload,
		codingProfilePayload,
		codingNotesPayload,
		codingProfilePayload,
	}}
	profiles := generation.NewProfileSynthesizer(generation.NewOrchestrator(memoryService, generator), memoryService)
	submissionService := submission.NewService(problemService, executionService, sessionService, memoryService, profiles)
	router := NewRouter(Dependencies{
		Authenticator: auth.NewDevAuthenticator(auth.DevAuthenticatorConfig{}),
		Identity:      identity.NewService(identity.NewInMemoryStore()), Memory: memoryService,
		Sessions: sessionService, Execution: executionService, Problems: problemService,
		Submissions: submissionService, MemoryProfiles: profiles,
	})
	created := decodeEnvelopeData[session.Session](t, authedRequest(t, router, http.MethodPost, "/api/v1/sessions",
		`{"kind":"workspace","problem_id":"two-sum","state":{"schema_version":1,"hints_revealed":0}}`))

	submit := func(content string) submission.Accepted {
		body := `{"problem_id":"two-sum","session_id":"` + created.ID + `","mode":"submit","files":[{"path":"solution.py","content":"` + content + `"}]}`
		recorder := authedRequest(t, router, http.MethodPost, "/api/v1/submissions", body)
		if recorder.Code != http.StatusAccepted {
			t.Fatalf("submit status=%d body=%s", recorder.Code, recorder.Body.String())
		}
		return decodeEnvelopeData[submission.Accepted](t, recorder)
	}

	older := submit("def two_sum(nums, target): return [0, 1]")
	if older.MemoryUpdateStatus != "failed" {
		t.Fatalf("older submit memory status=%q, want failed", older.MemoryUpdateStatus)
	}
	newer := submit("def two_sum(nums, target): return [1, 0]")
	if newer.MemoryUpdateStatus != "synced" {
		t.Fatalf("newer submit memory status=%q, want synced", newer.MemoryUpdateStatus)
	}

	storeCtx := context.Background()
	if _, err := sessionStore.SetMemoryUpdateStatus(storeCtx, "personal-kevin", "kevin", created.ID, newer.SubmissionID, "pending"); err != nil {
		t.Fatalf("mark newer completion pending: %v", err)
	}

	maintain := func(submissionID string) *httptest.ResponseRecorder {
		body := `{"session_id":"` + created.ID + `","problem_id":"two-sum","submission_id":"` + submissionID + `"}`
		return authedRequest(t, router, http.MethodPost, "/api/v1/memory/profile/maintain", body)
	}
	olderRetry := maintain(older.SubmissionID)
	if olderRetry.Code != http.StatusOK {
		t.Fatalf("older completion retry status=%d body=%s", olderRetry.Code, olderRetry.Body.String())
	}
	readState := func() map[string]any {
		stored := decodeEnvelopeData[session.Session](t, authedRequest(t, router, http.MethodGet, "/api/v1/sessions/"+created.ID, ""))
		state := map[string]any{}
		if err := json.Unmarshal(stored.State, &state); err != nil {
			t.Fatalf("decode session state: %v", err)
		}
		return state
	}
	if state := readState(); state["memory_update_status"] != "pending" || state["last_submission_id"] != newer.SubmissionID {
		t.Fatalf("older retry overwrote newer pending status: %#v", state)
	}

	stale := maintain("exec-stale-completion")
	if stale.Code != http.StatusConflict {
		t.Fatalf("unknown completion retry status=%d body=%s", stale.Code, stale.Body.String())
	}
	if state := readState(); state["memory_update_status"] != "pending" || state["last_submission_id"] != newer.SubmissionID {
		t.Fatalf("failed older retry overwrote newer pending status: %#v", state)
	}
}

func TestCodingMemoryRefreshExcludesRawCodeHiddenTestsSecretsAndCoachingText(t *testing.T) {
	const (
		rawCodeSentinel = "SENTINEL_RAW_CODE_7f3a"
		hiddenSentinel  = "SENTINEL_HIDDEN_TEST_91bc"
		secretSentinel  = "SENTINEL_SESSION_SECRET_44de"
		coachSentinel   = "SENTINEL_COACH_TEXT_2c9a"
	)
	problemStore := problems.NewInMemoryStore()
	problemService := problems.NewService(problemStore)
	if err := problemService.EnsureSeed(context.Background()); err != nil {
		t.Fatal(err)
	}
	definition, err := problemService.GetDefinition(context.Background(), "two-sum")
	if err != nil {
		t.Fatal(err)
	}
	if len(definition.HiddenTestFiles) == 0 {
		t.Fatal("seeded problem has no hidden test files")
	}
	definition.HiddenTestFiles[0].Content += "\n# " + hiddenSentinel + "\n"
	if err := problemStore.Upsert(context.Background(), definition); err != nil {
		t.Fatal(err)
	}

	sessionService := session.NewService(session.NewInMemoryStore(), nil)
	executionService := execution.NewService(execution.NewInMemoryStore(), &countingRunner{}, nil)
	memoryService := memory.NewService(memory.NewInMemoryStore(), nil)
	generator := &sequencedGenerator{payloads: []string{codingNotesPayload, codingProfilePayload}}
	profiles := generation.NewProfileSynthesizer(generation.NewOrchestrator(memoryService, generator), memoryService)
	submissionService := submission.NewService(problemService, executionService, sessionService, memoryService, profiles)
	router := NewRouter(Dependencies{
		Authenticator: auth.NewDevAuthenticator(auth.DevAuthenticatorConfig{}),
		Identity:      identity.NewService(identity.NewInMemoryStore()), Memory: memoryService,
		Sessions: sessionService, Execution: executionService, Problems: problemService,
		Submissions: submissionService, MemoryProfiles: profiles,
	})

	created := decodeEnvelopeData[session.Session](t, authedRequest(t, router, http.MethodPost, "/api/v1/sessions",
		`{"kind":"workspace","problem_id":"two-sum","state":{"schema_version":1,"hints_revealed":0,"access_token":"`+secretSentinel+`"}}`))
	coaching := authedRequest(t, router, http.MethodPost, "/api/v1/memory/events",
		`{"source":"chat","type":"message_sent","summary":"Coach said: `+coachSentinel+`","payload":{"thread_id":"thread-sentinel","body":"`+coachSentinel+`"}}`)
	if coaching.Code != http.StatusCreated {
		t.Fatalf("record coaching event status=%d body=%s", coaching.Code, coaching.Body.String())
	}

	accepted := decodeEnvelopeData[submission.Accepted](t, authedRequest(t, router, http.MethodPost, "/api/v1/submissions",
		`{"problem_id":"two-sum","session_id":"`+created.ID+`","mode":"submit","files":[{"path":"solution.py","content":"def two_sum(nums, target): return [0, 1]  # `+rawCodeSentinel+`"}]}`))
	if accepted.MemoryUpdateStatus != "synced" {
		t.Fatalf("graded submit memory status=%q", accepted.MemoryUpdateStatus)
	}

	kinds := map[generation.Kind]bool{}
	for _, request := range generator.requests {
		kinds[request.Kind] = true
		encoded, err := json.Marshal(request)
		if err != nil {
			t.Fatalf("marshal model request: %v", err)
		}
		for name, sentinel := range map[string]string{
			"raw code": rawCodeSentinel, "hidden test": hiddenSentinel,
			"session secret": secretSentinel, "coaching text": coachSentinel,
		} {
			if strings.Contains(string(encoded), sentinel) {
				t.Fatalf("%s sentinel leaked into %s model input: %s", name, request.Kind, encoded)
			}
		}
	}
	if !kinds[generation.KindNotes] || !kinds[generation.KindProfile] {
		t.Fatalf("missing notes/profile stages: %#v", kinds)
	}

	profileBody := authedRequest(t, router, http.MethodGet, "/api/v1/memory/profile", "").Body.String()
	for name, sentinel := range map[string]string{
		"raw code": rawCodeSentinel, "hidden test": hiddenSentinel,
		"session secret": secretSentinel, "coaching text": coachSentinel,
	} {
		if strings.Contains(profileBody, sentinel) {
			t.Fatalf("%s sentinel leaked into saved profile/provenance: %s", name, profileBody)
		}
	}
	if !strings.Contains(profileBody, "note_two_sum") || !strings.Contains(profileBody, "\"maintained_notes_snapshot\"") {
		t.Fatalf("profile body does not contain the maintained-note snapshot: %s", profileBody)
	}

	eventsBody := authedRequest(t, router, http.MethodGet, "/api/v1/memory/events", "").Body.String()
	for name, sentinel := range map[string]string{
		"raw code": rawCodeSentinel, "hidden test": hiddenSentinel, "session secret": secretSentinel,
	} {
		if strings.Contains(eventsBody, sentinel) {
			t.Fatalf("%s sentinel leaked into memory events: %s", name, eventsBody)
		}
	}
}

func countNoteAuditEvents(t *testing.T, router http.Handler) int {
	t.Helper()
	events := decodeEnvelopeData[[]memory.Event](t, authedRequest(t, router, http.MethodGet, "/api/v1/memory/events", ""))
	count := 0
	for _, event := range events {
		switch event.Type {
		case memory.TypeNoteCreated, memory.TypeNoteUpdated, memory.TypeNotePruned:
			count++
		}
	}
	return count
}

type slowCountingRunner struct {
	mu    sync.Mutex
	calls int
	delay time.Duration
}

func (r *slowCountingRunner) Run(context.Context, execution.RunSpec) (execution.RunOutcome, error) {
	r.mu.Lock()
	r.calls++
	r.mu.Unlock()
	time.Sleep(r.delay)
	exitCode := 0
	return execution.RunOutcome{
		ExitCode: 0,
		Result: execution.JudgeResult{
			Schema: execution.JudgeSchema, Status: execution.JudgeStatusPassed,
			ExitCode: &exitCode, DurationMs: 5,
			Cases: []execution.CaseResult{{Name: "basic", Status: "pass", DurationMs: 1}},
		},
		Output: "", Duration: 5 * time.Millisecond,
	}, nil
}

func (r *slowCountingRunner) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func TestConcurrentSameSubmitTokenStartsOneExecution(t *testing.T) {
	problemService := problems.NewService(problems.NewInMemoryStore())
	if err := problemService.EnsureSeed(context.Background()); err != nil {
		t.Fatal(err)
	}
	sessionService := session.NewService(session.NewInMemoryStore(), nil)
	runner := &slowCountingRunner{delay: 75 * time.Millisecond}
	executionService := execution.NewService(execution.NewInMemoryStore(), runner, nil)
	memoryService := memory.NewService(memory.NewInMemoryStore(), nil)
	generator := &sequencedGenerator{payloads: []string{
		`{"actions":[{"op":"create","note":{"id":"note_two_sum","problem_id":"two-sum","title":"Two Sum","summary":"Practice two-sum edge cases.","tags":["dsa"],"action":"review"}}]}`,
		`{"summary":"Two Sum practice is progressing.","strengths":["Two Pointers"],"growth_edges":["Arrays"],"skills":[],"notes":[]}`,
	}}
	profiles := generation.NewProfileSynthesizer(generation.NewOrchestrator(memoryService, generator), memoryService)
	submissionService := submission.NewService(problemService, executionService, sessionService, memoryService, profiles)
	router := NewRouter(Dependencies{
		Authenticator: auth.NewDevAuthenticator(auth.DevAuthenticatorConfig{}),
		Identity:      identity.NewService(identity.NewInMemoryStore()), Memory: memoryService,
		Sessions: sessionService, Execution: executionService, Problems: problemService,
		Submissions: submissionService, MemoryProfiles: profiles,
	})
	created := decodeEnvelopeData[session.Session](t, authedRequest(t, router, http.MethodPost, "/api/v1/sessions",
		`{"kind":"workspace","problem_id":"two-sum","state":{"schema_version":1,"hints_revealed":0}}`))
	body := `{"problem_id":"two-sum","session_id":"` + created.ID + `","mode":"submit","client_token":"tok-concurrent","files":[{"path":"solution.py","content":"def two_sum(nums, target): return [0, 1]"}]}`

	responses := make([]*httptest.ResponseRecorder, 2)
	var wg sync.WaitGroup
	for index := range responses {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			responses[index] = authedRequest(t, router, http.MethodPost, "/api/v1/submissions", body)
		}(index)
	}
	wg.Wait()

	if got := runner.count(); got != 1 {
		t.Fatalf("same-token concurrent submits started %d executions, want 1", got)
	}
	ids := map[string]bool{}
	for index, recorder := range responses {
		if recorder.Code != http.StatusAccepted {
			t.Fatalf("response %d status=%d body=%s", index, recorder.Code, recorder.Body.String())
		}
		accepted := decodeEnvelopeData[submission.Accepted](t, recorder)
		ids[accepted.SubmissionID] = true
	}
	if len(ids) != 1 {
		t.Fatalf("concurrent same-token submits returned submission ids %#v", ids)
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
