package api

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
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

type streamedProgressEvent struct {
	OperationID string         `json:"operation_id"`
	Sequence    int64          `json:"sequence"`
	StepID      string         `json:"step_id"`
	Label       string         `json:"label"`
	Status      string         `json:"status"`
	Metadata    map[string]any `json:"metadata"`
}

func parseSSEProgressEvents(t *testing.T, body string) []streamedProgressEvent {
	t.Helper()
	var events []streamedProgressEvent
	for _, block := range strings.Split(body, "\n\n") {
		eventType := ""
		data := ""
		for _, line := range strings.Split(block, "\n") {
			line = strings.TrimSpace(line)
			if after, ok := strings.CutPrefix(line, "event:"); ok {
				eventType = strings.TrimSpace(after)
			}
			if after, ok := strings.CutPrefix(line, "data:"); ok {
				data += strings.TrimSpace(after)
			}
		}
		if eventType != "progress" || data == "" {
			continue
		}
		var event streamedProgressEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			t.Fatalf("decode SSE data %q: %v", data, err)
		}
		events = append(events, event)
	}
	return events
}

// TestProblemGenerationStreamsLiveSSEProgress proves the real HTTP SSE stream
// delivers problem_generation progress end to end. The SSE consumer connects
// before generation starts and reads frames from the live HTTP response while
// POST /api/v1/generate runs; assertions come from those streamed frames, not
// from storage reads after the fact.
func TestProblemGenerationStreamsLiveSSEProgress(t *testing.T) {
	router, _ := newProblemWorkflowTestRouter(t,
		staticGenerator{payload: problemWorkflowPayload}, passingVerifyRunner{})
	operationID := createProblemWorkflowOperation(t, router)

	type sseOutcome struct {
		code int
		body string
	}
	sseDone := make(chan sseOutcome, 1)
	go func() {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet,
			"/api/v1/workflow-operations/"+operationID+"/events", nil)
		request.Header.Set("Authorization", "Bearer dev:kevin:personal-kevin")
		router.ServeHTTP(recorder, request)
		sseDone <- sseOutcome{code: recorder.Code, body: recorder.Body.String()}
	}()

	generateDone := make(chan int, 1)
	go func() {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/v1/generate", strings.NewReader(
			`{"kind":"problem","spec":{"topic":"strings","difficulty":"easy"},"operation_id":"`+operationID+`"}`))
		request.Header.Set("Authorization", "Bearer dev:kevin:personal-kevin")
		router.ServeHTTP(recorder, request)
		generateDone <- recorder.Code
	}()

	timeout := time.After(30 * time.Second)
	var generateCode int
	select {
	case generateCode = <-generateDone:
	case <-timeout:
		t.Fatal("timed out waiting for POST /api/v1/generate")
	}
	if generateCode != http.StatusOK {
		t.Fatalf("generate status=%d", generateCode)
	}
	var outcome sseOutcome
	select {
	case outcome = <-sseDone:
	case <-timeout:
		t.Fatal("timed out waiting for the SSE stream to close")
	}
	if outcome.code != http.StatusOK {
		t.Fatalf("SSE status=%d body=%s", outcome.code, outcome.body)
	}
	events := parseSSEProgressEvents(t, outcome.body)
	if len(events) == 0 {
		t.Fatal("SSE stream delivered no progress events")
	}

	wantOrder := []string{"load_context", "generate_problem", "verify_solution", "save_problem", "problem_ready"}
	firstSeen := map[string]int{}
	for index, event := range events {
		if event.OperationID != operationID {
			t.Fatalf("event operation=%q, want %q", event.OperationID, operationID)
		}
		if event.Label == "" {
			t.Fatalf("event has no backend-owned label: %#v", event)
		}
		if _, ok := firstSeen[event.StepID]; !ok {
			firstSeen[event.StepID] = index
		}
	}
	if len(firstSeen) != len(wantOrder) {
		t.Fatalf("streamed steps=%v, want %v", firstSeen, wantOrder)
	}
	for position, stepID := range wantOrder {
		if position > 0 && firstSeen[stepID] <= firstSeen[wantOrder[position-1]] {
			t.Fatalf("step %q first appears out of order: %#v", stepID, events)
		}
	}

	terminalCount := 0
	for _, event := range events {
		if terminal, ok := event.Metadata["terminal"]; ok && terminal == true {
			terminalCount++
			if event.StepID != "problem_ready" || event.Status != "succeeded" {
				t.Fatalf("terminal SSE event=%#v, want problem_ready/succeeded", event)
			}
		}
	}
	if terminalCount != 1 {
		t.Fatalf("terminal SSE events=%d, want 1", terminalCount)
	}
}

// blockingProblemGenerator holds the provider call open until release is
// closed, so the test can observe SSE frames while generation is provably
// still in progress. It signals the first call on entered.
type blockingProblemGenerator struct {
	payload string
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func newBlockingProblemGenerator(payload string) *blockingProblemGenerator {
	return &blockingProblemGenerator{
		payload: payload,
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (g *blockingProblemGenerator) Generate(ctx context.Context, _ generation.GenerateRequest) (generation.GenerateResult, error) {
	g.once.Do(func() { close(g.entered) })
	select {
	case <-g.release:
		return generation.GenerateResult{Object: []byte(g.payload), Provider: "blocking", Model: "fake"}, nil
	case <-ctx.Done():
		return generation.GenerateResult{}, ctx.Err()
	}
}

// readSSEFrames parses event: progress frames incrementally from a live
// response body and forwards them as they arrive. It closes out on EOF or
// read error. Malformed frames are dropped; their absence then fails the
// ordering assertions downstream.
func readSSEFrames(body io.Reader, out chan<- streamedProgressEvent) {
	defer close(out)
	reader := bufio.NewReader(body)
	eventType := ""
	var data strings.Builder
	flush := func() {
		defer func() { eventType = ""; data.Reset() }()
		if eventType != "progress" || data.Len() == 0 {
			return
		}
		var event streamedProgressEvent
		if err := json.Unmarshal([]byte(data.String()), &event); err != nil {
			return
		}
		out <- event
	}
	for {
		line, err := reader.ReadString('\n')
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			flush()
		} else if after, ok := strings.CutPrefix(trimmed, "event:"); ok {
			eventType = strings.TrimSpace(after)
		} else if after, ok := strings.CutPrefix(trimmed, "data:"); ok {
			data.WriteString(strings.TrimSpace(after))
		}
		if err != nil {
			flush()
			return
		}
	}
}

// postAPIJSON issues a real HTTP POST and returns the status and body. It
// never touches the test handle so callers may use it from any goroutine and
// forward failures through channels to the main test goroutine.
func postAPIJSON(client *http.Client, url, auth, body string) (int, []byte, error) {
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", auth)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, err
	}
	return resp.StatusCode, raw, nil
}

// TestProblemGenerationStreamsLiveIncrementalProgress proves frames are
// flushed and readable while generation is still running. A controllable
// generator holds the provider call open; the test reads SSE frames from a
// real HTTP connection, asserts non-terminal progress arrives before
// generation completes, then releases generation and asserts the ordered
// terminal outcome.
func TestProblemGenerationStreamsLiveIncrementalProgress(t *testing.T) {
	generator := newBlockingProblemGenerator(problemWorkflowPayload)
	router, _ := newProblemWorkflowTestRouter(t, generator, passingVerifyRunner{})
	server := httptest.NewServer(router)
	defer server.Close()
	client := server.Client()
	authHeader := "Bearer dev:kevin:personal-kevin"
	deadline := time.After(30 * time.Second)

	status, raw, err := postAPIJSON(client,
		server.URL+"/api/v1/workflow-operations", authHeader, `{"kind":"problem_generation"}`)
	if err != nil {
		t.Fatalf("create request failed: %v", err)
	}
	if status != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", status, raw)
	}
	var created struct {
		Data workflow.CreateResult `json:"data"`
	}
	if err := json.Unmarshal(raw, &created); err != nil {
		t.Fatalf("decode create result: %v", err)
	}
	operationID := created.Data.Operation.ID
	if operationID == "" {
		t.Fatalf("empty operation id: %s", raw)
	}

	sseCtx, sseCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer sseCancel()
	sseReq, err := http.NewRequestWithContext(sseCtx, http.MethodGet,
		server.URL+"/api/v1/workflow-operations/"+operationID+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	sseReq.Header.Set("Authorization", authHeader)
	sseReq.Header.Set("Accept", "text/event-stream")
	sseResp, err := client.Do(sseReq)
	if err != nil {
		t.Fatal(err)
	}
	defer sseResp.Body.Close()
	if sseResp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(sseResp.Body)
		t.Fatalf("SSE status=%d body=%s", sseResp.StatusCode, raw)
	}
	streamCh := make(chan streamedProgressEvent, 64)
	go readSSEFrames(sseResp.Body, streamCh)

	type generateResult struct {
		code int
		err  error
	}
	generateDone := make(chan generateResult, 1)
	go func() {
		code, _, err := postAPIJSON(client,
			server.URL+"/api/v1/generate", authHeader,
			`{"kind":"problem","spec":{"topic":"strings","difficulty":"easy"},"operation_id":"`+operationID+`"}`)
		generateDone <- generateResult{code: code, err: err}
	}()

	// Generation must reach the provider call. Failures from the generate
	// goroutine arrive here in the main test goroutine.
	select {
	case <-generator.entered:
	case <-deadline:
		t.Fatal("timed out waiting for generation to reach the provider")
	}

	// Read frames until generate_problem/running is observed on the live
	// connection. That frame is emitted before the blocked provider call, so
	// observing it here proves mid-operation delivery.
	var streamed []streamedProgressEvent
	sawGenerateRunning := false
	for !sawGenerateRunning {
		select {
		case event, ok := <-streamCh:
			if !ok {
				t.Fatal("SSE stream closed before generate_problem running arrived")
			}
			streamed = append(streamed, event)
			if event.StepID == "generate_problem" && event.Status == "running" {
				sawGenerateRunning = true
			}
		case <-deadline:
			t.Fatal("timed out waiting for live generate_problem running frame")
		}
	}
	select {
	case <-generateDone:
		t.Fatal("generate completed before the client read live progress frames")
	default:
	}
	nonTerminal := 0
	for _, event := range streamed {
		if terminal, ok := event.Metadata["terminal"]; !ok || terminal != true {
			nonTerminal++
		}
	}
	if nonTerminal == 0 {
		t.Fatal("no non-terminal frames arrived while generation was blocked")
	}

	close(generator.release)
	select {
	case result := <-generateDone:
		if result.err != nil {
			t.Fatalf("generate request failed: %v", result.err)
		}
		if result.code != http.StatusOK {
			t.Fatalf("generate status=%d", result.code)
		}
	case <-deadline:
		t.Fatal("timed out waiting for generation to complete")
	}

	terminalSeen := false
	for !terminalSeen {
		select {
		case event, ok := <-streamCh:
			if !ok {
				t.Fatal("SSE stream closed without terminal problem_ready")
			}
			streamed = append(streamed, event)
			if event.StepID == "problem_ready" && event.Status == "succeeded" {
				if terminal, _ := event.Metadata["terminal"].(bool); terminal {
					terminalSeen = true
				}
			}
		case <-deadline:
			t.Fatal("timed out waiting for terminal problem_ready frame")
		}
	}

	wantOrder := []string{"load_context", "generate_problem", "verify_solution", "save_problem", "problem_ready"}
	firstSeen := map[string]int{}
	for index, event := range streamed {
		if event.OperationID != operationID {
			t.Fatalf("event operation=%q, want %q", event.OperationID, operationID)
		}
		if event.Label == "" {
			t.Fatalf("event has no backend-owned label: %#v", event)
		}
		if _, ok := firstSeen[event.StepID]; !ok {
			firstSeen[event.StepID] = index
		}
	}
	if len(firstSeen) != len(wantOrder) {
		t.Fatalf("streamed steps=%v, want %v", firstSeen, wantOrder)
	}
	for position, stepID := range wantOrder {
		if position > 0 && firstSeen[stepID] <= firstSeen[wantOrder[position-1]] {
			t.Fatalf("step %q arrived out of order: %#v", stepID, streamed)
		}
	}
	terminalCount := 0
	for _, event := range streamed {
		if terminal, ok := event.Metadata["terminal"]; ok && terminal == true {
			terminalCount++
			if event.StepID != "problem_ready" || event.Status != "succeeded" {
				t.Fatalf("terminal SSE event=%#v, want problem_ready/succeeded", event)
			}
		}
	}
	if terminalCount != 1 {
		t.Fatalf("terminal SSE events=%d, want 1", terminalCount)
	}
}
