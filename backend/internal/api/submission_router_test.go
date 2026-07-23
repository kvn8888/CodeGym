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
	"github.com/kvn8888/codegym/backend/internal/identity"
	"github.com/kvn8888/codegym/backend/internal/memory"
	"github.com/kvn8888/codegym/backend/internal/problems"
	"github.com/kvn8888/codegym/backend/internal/session"
	"github.com/kvn8888/codegym/backend/internal/submission"
)

type capturingRunner struct {
	spec execution.RunSpec
}

func (r *capturingRunner) Run(_ context.Context, spec execution.RunSpec) (execution.RunOutcome, error) {
	r.spec = spec
	return execution.RunOutcome{
		ExitCode: 0,
		Output:   `CODEGYM_RESULT {"tests":[{"name":"basic","status":"pass","duration_ms":3,"error":null},{"name":"duplicates","status":"pass","duration_ms":4,"error":null}],"compile_error":null}`,
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
	submissionService := submission.NewService(problemService, executionService, sessionService)

	router := NewRouter(Dependencies{
		Authenticator: auth.NewDevAuthenticator(auth.DevAuthenticatorConfig{}),
		Identity:      identity.NewService(identity.NewInMemoryStore()),
		Memory:        memory.NewService(memory.NewInMemoryStore(), nil),
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
		if file.Path == "test_solution.py" && strings.Contains(file.Content, "CODEGYM_RESULT") {
			foundHiddenTest = true
		}
		if strings.Contains(file.Content, "seen[complement]") {
			t.Fatal("reference solution was assembled into the user execution")
		}
	}
	if !foundHiddenTest || runner.spec.Entrypoint != "test_solution.py" {
		t.Fatalf("captured run spec = %#v", runner.spec)
	}

	getSubmission := authedRequest(t, router, http.MethodGet, "/api/v1/submissions/"+accepted.SubmissionID, "")
	if getSubmission.Code != http.StatusOK {
		t.Fatalf("get submission status = %d: %s", getSubmission.Code, getSubmission.Body.String())
	}
	view := decodeEnvelopeData[submission.View](t, getSubmission)
	if view.Status != submission.StatusCompleted || view.Result == nil ||
		view.Result.Status != "pass" || view.Result.Passed != 2 {
		t.Fatalf("submission view = %#v", view)
	}

	getSession := authedRequest(t, router, http.MethodGet, "/api/v1/sessions/"+created.ID, "")
	updatedSession := decodeEnvelopeData[session.Session](t, getSession)
	if updatedSession.Status != session.StatusCompleted ||
		len(updatedSession.Files) != 1 ||
		updatedSession.Files[0].Path != "solution.py" {
		t.Fatalf("updated session = %#v", updatedSession)
	}

	getExecution := authedRequest(t, router, http.MethodGet, "/api/v1/executions/"+accepted.SubmissionID, "")
	if strings.Contains(getExecution.Body.String(), `"files"`) ||
		strings.Contains(getExecution.Body.String(), "seen[complement]") {
		t.Fatalf("execution endpoint leaked stored run files: %s", getExecution.Body.String())
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
