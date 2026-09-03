package api

import (
	"context"
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
	"github.com/kvn8888/codegym/backend/internal/workflow"
	"github.com/kvn8888/codegym/backend/internal/workspace"
)

const problemWorkflowPayload = `{
	"title":"Reverse Words","description":"Reverse the words.","category":"algorithms",
	"subcategory":"strings","tags":["strings"],"difficulty":1,"estimated_minutes":15,
	"function_name":"reverse_words","parameters":[{"name":"value","type":"str"}],
	"return_type":"str","hints":["Split first."],
		"reference_solution":"def reverse_words(value: str) -> str:\n    return ' '.join(reversed(value.split()))",
		"test_cases":[
			{"name":"one","kind":"example","hidden":false,"args":["hello world"],"expected":"world hello"},
			{"name":"single","kind":"functional","hidden":false,"args":["hello"],"expected":"hello"},
			{"name":"spaces","kind":"hidden","hidden":true,"args":["a b c"],"expected":"c b a"},
			{"name":"empty","kind":"edge","hidden":true,"args":[""],"expected":""}
	]
}`

// failingVerifyRunner returns all-fail sandbox results so verification cannot
// converge; the static generator cannot satisfy the adjudicator either.
type failingVerifyRunner struct{}

func (failingVerifyRunner) Run(_ context.Context, _ execution.RunSpec) (execution.RunOutcome, error) {
	return execution.RunOutcome{
		ExitCode: 0,
		Output:   `CODEGYM_RESULT {"tests":[{"name":"one","status":"fail","duration_ms":1,"error":"expected \"world hello\", got \"hello\""},{"name":"single","status":"fail","duration_ms":1,"error":"boom"},{"name":"spaces","status":"fail","duration_ms":1,"error":"boom"},{"name":"empty","status":"fail","duration_ms":1,"error":"boom"}],"compile_error":null}`,
		Duration: time.Millisecond,
	}, nil
}

func newProblemWorkflowTestRouter(t *testing.T, generator generation.Generator, runner execution.Runner) (http.Handler, *workflow.Service) {
	t.Helper()
	memoryService := memory.NewService(memory.NewInMemoryStore(), nil)
	orchestrator := generation.NewOrchestrator(memoryService, generator)
	problemService := problems.NewService(problems.NewInMemoryStore())
	if err := problemService.EnsureSeed(context.Background()); err != nil {
		t.Fatal(err)
	}
	workflowService := workflow.NewService(workflow.NewInMemoryStore(), nil)
	router := NewRouter(Dependencies{
		Authenticator:   auth.NewDevAuthenticator(auth.DevAuthenticatorConfig{}),
		Identity:        identity.NewService(identity.NewInMemoryStore()),
		Memory:          memoryService,
		Sessions:        session.NewService(session.NewInMemoryStore(), nil),
		Generation:      orchestrator,
		Problems:        problemService,
		ExecutionRunner: runner,
		Workflow:        workflowService,
	})
	return router, workflowService
}

func problemWorkflowServiceContext() context.Context {
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{
		UserID:             "kevin",
		DefaultWorkspaceID: "personal-kevin",
		WorkspaceIDs:       []string{"personal-kevin"},
	})
	return workspace.WithScope(ctx, workspace.Scope{WorkspaceID: "personal-kevin"})
}

func latestWorkflowState(events []workflow.Event) map[string]workflow.Event {
	latest := make(map[string]workflow.Event, len(events))
	for _, event := range events {
		latest[event.StepID] = event
	}
	return latest
}

func createProblemWorkflowOperation(t *testing.T, router http.Handler) string {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/workflow-operations", strings.NewReader(`{"kind":"problem_generation"}`))
	request.Header.Set("Authorization", "Bearer dev:kevin:personal-kevin")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	return decodeEnvelopeData[workflow.CreateResult](t, recorder).Operation.ID
}

func TestProblemGenerationWithOperationReportsSucceededTerminal(t *testing.T) {
	router, workflowService := newProblemWorkflowTestRouter(t,
		staticGenerator{payload: problemWorkflowPayload}, passingVerifyRunner{})
	operationID := createProblemWorkflowOperation(t, router)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/generate", strings.NewReader(
		`{"kind":"problem","spec":{"topic":"strings","difficulty":"easy"},"operation_id":"`+operationID+`"}`))
	request.Header.Set("Authorization", "Bearer dev:kevin:personal-kevin")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	events, err := workflowService.Events(problemWorkflowServiceContext(), operationID, 0)
	if err != nil {
		t.Fatal(err)
	}
	latest := latestWorkflowState(events)
	for _, stepID := range []string{"load_context", "generate_problem", "verify_solution", "save_problem", "problem_ready"} {
		event, ok := latest[stepID]
		if !ok || event.Status != workflow.StatusSucceeded {
			t.Fatalf("latest %s=%#v, want succeeded", stepID, event)
		}
	}
	terminalCount := 0
	for _, event := range events {
		if terminal, ok := event.Metadata["terminal"]; ok && terminal == true {
			terminalCount++
			if event.StepID != "problem_ready" || event.Status != workflow.StatusSucceeded {
				t.Fatalf("terminal event=%#v, want problem_ready/succeeded", event)
			}
		}
	}
	if terminalCount != 1 {
		t.Fatalf("terminal events=%d, want 1", terminalCount)
	}
	operation, err := workflowService.Get(problemWorkflowServiceContext(), operationID)
	if err != nil {
		t.Fatal(err)
	}
	if operation.Status != workflow.StatusSucceeded {
		t.Fatalf("operation status=%q, want succeeded", operation.Status)
	}
}

func TestProblemGenerationVerifyFailureOwnsTerminal(t *testing.T) {
	router, workflowService := newProblemWorkflowTestRouter(t,
		staticGenerator{payload: problemWorkflowPayload}, failingVerifyRunner{})
	operationID := createProblemWorkflowOperation(t, router)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/generate", strings.NewReader(
		`{"kind":"problem","spec":{"topic":"strings"},"operation_id":"`+operationID+`"}`))
	request.Header.Set("Authorization", "Bearer dev:kevin:personal-kevin")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadGateway || !strings.Contains(recorder.Body.String(), "verification_failed") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	svcCtx := problemWorkflowServiceContext()
	events, err := workflowService.Events(svcCtx, operationID, 0)
	if err != nil {
		t.Fatal(err)
	}
	latest := latestWorkflowState(events)
	verify, ok := latest["verify_solution"]
	if !ok || verify.Status != workflow.StatusFailed {
		t.Fatalf("latest verify_solution=%#v, want failed", verify)
	}
	if terminal, ok := verify.Metadata["terminal"]; !ok || terminal != true {
		t.Fatalf("verify_solution does not own the terminal failure: %#v", verify)
	}
	// The seeded queued problem_ready event still exists, but the step must
	// never transition to succeeded or carry the terminal marker.
	for _, event := range events {
		if event.StepID != "problem_ready" {
			continue
		}
		if event.Status == workflow.StatusSucceeded {
			t.Fatalf("problem_ready reached succeeded after verify failure: %#v", event)
		}
		if terminal, ok := event.Metadata["terminal"]; ok && terminal == true {
			t.Fatalf("problem_ready carries terminal after verify failure: %#v", event)
		}
	}
	operation, err := workflowService.Get(svcCtx, operationID)
	if err != nil {
		t.Fatal(err)
	}
	if operation.Status != workflow.StatusFailed {
		t.Fatalf("operation status=%q, want failed", operation.Status)
	}
}

func TestProblemGenerationRejectsMismatchedWorkflowKind(t *testing.T) {
	router, _ := newProblemWorkflowTestRouter(t,
		staticGenerator{payload: problemWorkflowPayload}, passingVerifyRunner{})

	mcqOp := httptest.NewRecorder()
	mcqRequest := httptest.NewRequest(http.MethodPost, "/api/v1/workflow-operations", strings.NewReader(`{"kind":"mcq_generation"}`))
	mcqRequest.Header.Set("Authorization", "Bearer dev:kevin:personal-kevin")
	router.ServeHTTP(mcqOp, mcqRequest)
	if mcqOp.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", mcqOp.Code, mcqOp.Body.String())
	}
	operationID := decodeEnvelopeData[workflow.CreateResult](t, mcqOp).Operation.ID

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/generate", strings.NewReader(
		`{"kind":"problem","spec":{"topic":"strings"},"operation_id":"`+operationID+`"}`))
	request.Header.Set("Authorization", "Bearer dev:kevin:personal-kevin")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "workflow_mismatch") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
