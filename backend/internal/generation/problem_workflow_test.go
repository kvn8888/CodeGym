package generation

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/kvn8888/codegym/backend/internal/workflow"
)

// TestGenerateProblemReportsOrderedObservableWorkflow mirrors the MCQ reporter
// test: one load_context sequence (single evidence snapshot) followed by the
// generate_problem attempt, all observable through the workflow reporter.
func TestGenerateProblemReportsOrderedObservableWorkflow(t *testing.T) {
	generator := &scriptedGenerator{payloads: []json.RawMessage{json.RawMessage(validProblemPayload)}}
	orchestrator := newTestOrchestrator(generator)
	progress := workflow.NewService(workflow.NewInMemoryStore(), nil)
	ctx := scopedContext()
	created, err := progress.Create(ctx, workflow.CreateInput{Kind: workflow.KindProblemGeneration})
	if err != nil {
		t.Fatal(err)
	}
	reporter, err := progress.Attach(ctx, created.Operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	ctx = workflow.WithReporter(ctx, reporter)

	if _, _, _, err := GenerateProblem(ctx, orchestrator, ProblemSpec{Topic: "hash maps"}); err != nil {
		t.Fatal(err)
	}
	if len(created.Events) == 0 {
		t.Fatal("expected seeded workflow events from Create")
	}
	cursor := created.Events[len(created.Events)-1].Sequence
	events, err := progress.Events(ctx, created.Operation.ID, cursor)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		step   string
		status workflow.Status
	}{
		{"load_context", workflow.StatusRunning},
		{"load_context", workflow.StatusSucceeded},
		{"generate_problem", workflow.StatusRunning},
		{"generate_problem", workflow.StatusSucceeded},
	}
	if len(events) != len(want) {
		t.Fatalf("events=%#v", events)
	}
	for index, expected := range want {
		if events[index].StepID != expected.step || events[index].Status != expected.status {
			t.Fatalf("event %d=%#v, want %s/%s", index, events[index], expected.step, expected.status)
		}
		if events[index].Sequence != cursor+int64(index+1) {
			t.Fatalf("event %d sequence=%d, want %d", index, events[index].Sequence, cursor+int64(index+1))
		}
	}
}

// TestGenerateProblemReusesOneProfileSnapshotAcrossRetries ensures the retry
// loop reuses a single personalization snapshot: load_context emits once even
// when the first model attempt fails validation.
func TestGenerateProblemReusesOneProfileSnapshotAcrossRetries(t *testing.T) {
	generator := &scriptedGenerator{payloads: []json.RawMessage{
		json.RawMessage(`{"title":"bad"}`),
		json.RawMessage(validProblemPayload),
	}}
	orchestrator := newTestOrchestrator(generator)
	progress := workflow.NewService(workflow.NewInMemoryStore(), nil)
	ctx := scopedContext()
	created, err := progress.Create(ctx, workflow.CreateInput{Kind: workflow.KindProblemGeneration})
	if err != nil {
		t.Fatal(err)
	}
	reporter, err := progress.Attach(ctx, created.Operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	ctx = workflow.WithReporter(ctx, reporter)

	if _, _, _, err := GenerateProblem(ctx, orchestrator, ProblemSpec{Topic: "hash maps"}); err != nil {
		t.Fatal(err)
	}
	events, err := progress.Events(ctx, created.Operation.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	loadContextRunning := 0
	for _, event := range events {
		if event.StepID == "load_context" && event.Status == workflow.StatusRunning {
			loadContextRunning++
		}
	}
	if loadContextRunning != 1 {
		t.Fatalf("load_context running events=%d, want 1: %#v", loadContextRunning, events)
	}
	if len(created.Events) == 0 {
		t.Fatal("expected seeded workflow events from Create")
	}
	dynamic := events[len(created.Events):]
	trueValue := true
	want := []struct {
		step      string
		status    workflow.Status
		attempt   int
		reason    string
		retryable *bool
	}{
		{"load_context", workflow.StatusRunning, 0, "", nil},
		{"load_context", workflow.StatusSucceeded, 0, "", nil},
		{"generate_problem", workflow.StatusRunning, 1, "", nil},
		{"generate_problem", workflow.StatusFailed, 1, "invalid_output", &trueValue},
		{"generate_problem", workflow.StatusRunning, 2, "", nil},
		{"generate_problem", workflow.StatusSucceeded, 2, "", nil},
	}
	if len(dynamic) != len(want) {
		t.Fatalf("dynamic events=%#v", dynamic)
	}
	for index, expected := range want {
		event := dynamic[index]
		if event.StepID != expected.step || event.Status != expected.status {
			t.Fatalf("event %d=%#v, want %s/%s", index, event, expected.step, expected.status)
		}
		if expected.attempt != 0 && event.Metadata["attempt"] != expected.attempt {
			t.Fatalf("event %d attempt=%#v, want %d", index, event.Metadata["attempt"], expected.attempt)
		}
		if expected.reason != "" && event.Metadata["reason_code"] != expected.reason {
			t.Fatalf("event %d reason=%#v, want %q", index, event.Metadata["reason_code"], expected.reason)
		}
		if expected.retryable != nil && event.Metadata["retryable"] != *expected.retryable {
			t.Fatalf("event %d retryable=%#v, want %v", index, event.Metadata["retryable"], *expected.retryable)
		}
		if terminal, ok := event.Metadata["terminal"]; ok && terminal == true {
			t.Fatalf("event %d unexpectedly carries terminal=true: %#v", index, event)
		}
	}
	latest := map[string]workflow.Status{}
	for _, event := range events {
		latest[event.StepID] = event.Status
	}
	if latest["generate_problem"] != workflow.StatusSucceeded {
		t.Fatalf("latest states=%#v", latest)
	}
}

// TestGenerateProblemTerminalOwnershipOnExhaustedInvalidOutput ensures the
// failing generate_problem step owns the terminal failure when the retry
// budget is spent, and that further appends are rejected.
func TestGenerateProblemTerminalOwnershipOnExhaustedInvalidOutput(t *testing.T) {
	generator := &scriptedGenerator{payloads: []json.RawMessage{
		json.RawMessage(`{"title":"bad"}`),
		json.RawMessage(`{"title":"still bad"}`),
	}}
	orchestrator := newTestOrchestrator(generator)
	progress := workflow.NewService(workflow.NewInMemoryStore(), nil)
	ctx := scopedContext()
	created, err := progress.Create(ctx, workflow.CreateInput{Kind: workflow.KindProblemGeneration})
	if err != nil {
		t.Fatal(err)
	}
	reporter, err := progress.Attach(ctx, created.Operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	ctx = workflow.WithReporter(ctx, reporter)

	if _, _, _, err := GenerateProblem(ctx, orchestrator, ProblemSpec{Topic: "hash maps"}); err == nil {
		t.Fatal("expected invalid-output error, got nil")
	}
	events, err := progress.Events(ctx, created.Operation.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	terminalCount := 0
	var terminal workflow.Event
	for _, event := range events {
		if terminalFlag, ok := event.Metadata["terminal"]; ok && terminalFlag == true {
			terminalCount++
			terminal = event
		}
	}
	if terminalCount != 1 {
		t.Fatalf("terminal events=%d, want 1: %#v", terminalCount, events)
	}
	if terminal.StepID != "generate_problem" || terminal.Status != workflow.StatusFailed {
		t.Fatalf("terminal event=%#v, want generate_problem/failed", terminal)
	}
	if terminal.Metadata["reason_code"] != "invalid_output" || terminal.Metadata["retryable"] != false {
		t.Fatalf("terminal metadata=%#v", terminal.Metadata)
	}
	operation, err := progress.Get(ctx, created.Operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if operation.Status != workflow.StatusFailed {
		t.Fatalf("operation status=%q, want failed", operation.Status)
	}
	if err := reporter.Report(ctx, "problem_ready", workflow.StatusFailed, map[string]any{
		"reason_code": "operation_failed", "retryable": true,
	}, true); !errors.Is(err, workflow.ErrTerminal) {
		t.Fatalf("terminal append error=%v, want ErrTerminal", err)
	}
}
