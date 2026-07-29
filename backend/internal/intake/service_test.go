package intake

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/generation"
	"github.com/kvn8888/codegym/backend/internal/memory"
	"github.com/kvn8888/codegym/backend/internal/workspace"
)

type fixedGenerator struct {
	object json.RawMessage
	err    error
	calls  int
}

func (g *fixedGenerator) Generate(_ context.Context, _ generation.GenerateRequest) (generation.GenerateResult, error) {
	g.calls++
	return generation.GenerateResult{Object: g.object, Provider: "test", Model: "test"}, g.err
}

func testContext() context.Context {
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{
		UserID: "user-1", DefaultWorkspaceID: "workspace-1", WorkspaceIDs: []string{"workspace-1"},
	})
	return workspace.WithScope(ctx, workspace.Scope{WorkspaceID: "workspace-1"})
}

func validQuestions() json.RawMessage {
	return json.RawMessage(`[
		{"id":"q1","dimension":"exposure","text":"How familiar?","options":[{"id":"new","label":"New"},{"id":"some","label":"Some"}]},
		{"id":"q2","dimension":"application","text":"Used it?","options":[{"id":"never","label":"Never"},{"id":"built","label":"Built with it"}]},
		{"id":"q3","dimension":"challenge","text":"What pace?","options":[{"id":"guided","label":"Guided"},{"id":"stretch","label":"Stretch"}]}
	]`)
}

func newTestService(generator *fixedGenerator) (*Service, *memory.Service) {
	now := func() time.Time { return time.Date(2026, 7, 29, 18, 0, 0, 0, time.UTC) }
	memoryService := memory.NewService(memory.NewInMemoryStore(), now)
	orchestrator := generation.NewOrchestrator(memoryService, generator)
	return NewService(NewInMemoryStore(), memoryService, orchestrator, now), memoryService
}

func TestNormalizeTopicUsesExactAliasesWithoutSubstringMatches(t *testing.T) {
	if got := NormalizeTopic("  JS "); got != "javascript" {
		t.Fatalf("NormalizeTopic(JS) = %q", got)
	}
	if got := NormalizeTopic("Django"); got != "django" {
		t.Fatalf("NormalizeTopic(Django) = %q", got)
	}
	if NormalizeTopic("Django") == NormalizeTopic("Go") {
		t.Fatal("loose substring matching made Django equal Go")
	}
}

func TestPrepareCreatesQuestionsAndResumesPartialAnswers(t *testing.T) {
	generator := &fixedGenerator{object: validQuestions()}
	service, _ := newTestService(generator)
	ctx := testContext()

	record, err := service.Prepare(ctx, PrepareInput{Topic: "TypeScript", PracticeSeed: json.RawMessage(`{"format":"mcq"}`)})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if record.Status != StatusPending || len(record.Questions) != 3 {
		t.Fatalf("unexpected prepared record: %#v", record)
	}
	record, err = service.Update(ctx, record.ID, UpdateInput{Answers: map[string]string{"q1": "some"}})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	resumed, err := service.Prepare(ctx, PrepareInput{Topic: "ts"})
	if err != nil {
		t.Fatalf("resume Prepare: %v", err)
	}
	if resumed.ID != record.ID || resumed.Answers["q1"] != "some" || generator.calls != 1 {
		t.Fatalf("pending intake did not resume: %#v calls=%d", resumed, generator.calls)
	}
}

func TestCompletedAndSkippedSuppressUntilExplicitRestart(t *testing.T) {
	generator := &fixedGenerator{object: validQuestions()}
	service, _ := newTestService(generator)
	ctx := testContext()
	record, _ := service.Prepare(ctx, PrepareInput{Topic: "JavaScript"})
	completed := StatusCompleted
	record, err := service.Update(ctx, record.ID, UpdateInput{
		Answers: map[string]string{"q1": "new", "q2": "never", "q3": "guided"},
		Status:  &completed,
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	suppressed, _ := service.Prepare(ctx, PrepareInput{Topic: "js"})
	if suppressed.ID != record.ID || suppressed.Status != StatusCompleted || generator.calls != 1 {
		t.Fatalf("completed intake was not suppressed: %#v calls=%d", suppressed, generator.calls)
	}
	restarted, err := service.Prepare(ctx, PrepareInput{Topic: "javascript", Restart: true})
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	if restarted.ID == record.ID || restarted.Status != StatusPending || generator.calls != 2 {
		t.Fatalf("explicit restart did not create a new intake: %#v calls=%d", restarted, generator.calls)
	}
}

func TestDemonstratedTopicSuppressesButSubstringDoesNot(t *testing.T) {
	generator := &fixedGenerator{object: validQuestions()}
	service, memoryService := newTestService(generator)
	ctx := testContext()
	if _, err := memoryService.RecordEvent(ctx, memory.RecordEventInput{
		Source: "mcq", Type: memory.TypeQuestionAnswered,
		Payload: json.RawMessage(`{"topic":"Django","correct":true,"schema_version":1}`),
	}); err != nil {
		t.Fatalf("record Django event: %v", err)
	}
	goRecord, err := service.Prepare(ctx, PrepareInput{Topic: "Go"})
	if err != nil {
		t.Fatalf("prepare Go: %v", err)
	}
	if goRecord.Status != StatusPending {
		t.Fatalf("Django falsely suppressed Go: %#v", goRecord)
	}
	if _, err := memoryService.RecordEvent(ctx, memory.RecordEventInput{
		Source: "mcq", Type: memory.TypeQuestionAnswered,
		Payload: json.RawMessage(`{"topic":"Go","correct":true,"schema_version":1}`),
	}); err != nil {
		t.Fatalf("record Go event: %v", err)
	}
	known, err := service.Prepare(ctx, PrepareInput{Topic: "Golang", Restart: false})
	if err != nil {
		t.Fatalf("prepare known Go: %v", err)
	}
	// The existing pending Go intake resumes. A fresh alias demonstrates the
	// familiarity rule without overriding an unfinished baseline.
	if known.ID != goRecord.ID {
		t.Fatalf("pending intake should resume before familiarity suppression")
	}
	rustKnown, _ := memoryService.RecordEvent(ctx, memory.RecordEventInput{
		Source: "mcq", Type: memory.TypeQuestionAnswered,
		Payload: json.RawMessage(`{"topic":"Rust","correct":true,"schema_version":1}`),
	})
	if rustKnown.ID == "" {
		t.Fatal("expected Rust evidence")
	}
	suppressed, err := service.Prepare(ctx, PrepareInput{Topic: "Rust"})
	if err != nil {
		t.Fatalf("prepare Rust: %v", err)
	}
	if suppressed.Status != StatusSkipped || suppressed.SuppressionReason != "demonstrated_event" {
		t.Fatalf("demonstrated topic was not suppressed: %#v", suppressed)
	}
}

func TestGenerationFailurePersistsRetryablePendingRecord(t *testing.T) {
	generator := &fixedGenerator{object: json.RawMessage(`[]`)}
	service, _ := newTestService(generator)
	record, err := service.Prepare(testContext(), PrepareInput{Topic: "Graph theory"})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if record.Status != StatusPending || record.GenerationError == "" || len(record.Questions) != 0 {
		t.Fatalf("failure was not retryable: %#v", record)
	}
}

func TestContextExposesOnlyCompletedSelfReport(t *testing.T) {
	generator := &fixedGenerator{object: validQuestions()}
	service, _ := newTestService(generator)
	ctx := testContext()
	record, err := service.Prepare(ctx, PrepareInput{
		Topic:        "Graph traversal",
		PracticeSeed: json.RawMessage(`{"format":"coding","difficulty":"hard"}`),
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	pending, err := service.Context(ctx, record.ID)
	if err != nil || pending != nil {
		t.Fatalf("pending context = %#v, err=%v; want nil", pending, err)
	}
	completed := StatusCompleted
	record, err = service.Update(ctx, record.ID, UpdateInput{
		Answers: map[string]string{"q1": "some", "q2": "built", "q3": "stretch"},
		Status:  &completed,
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	baseline, err := service.Context(ctx, record.ID)
	if err != nil {
		t.Fatalf("Context: %v", err)
	}
	if baseline == nil || baseline.NormalizedTopic != "graph traversal" || len(baseline.Answers) != 3 {
		t.Fatalf("completed context = %#v", baseline)
	}
	if baseline.Answers[0].OptionLabel != "Some" || baseline.PracticeSeed != `{"format":"coding","difficulty":"hard"}` {
		t.Fatalf("context lost typed self-report: %#v", baseline)
	}
}
