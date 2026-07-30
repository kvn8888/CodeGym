package workflow

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/workspace"
)

func TestCreateSeedsOrderedServerOwnedSteps(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	service := NewService(NewInMemoryStore(), func() time.Time {
		now = now.Add(time.Second)
		return now
	})
	result, err := service.Create(testContext("workspace-a", "user-a"), CreateInput{
		Kind: KindMCQGeneration,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Operation.WorkspaceID != "workspace-a" || result.Operation.UserID != "user-a" {
		t.Fatalf("unexpected scope: %#v", result.Operation)
	}
	if len(result.Events) != 5 {
		t.Fatalf("got %d seeded events, want 5", len(result.Events))
	}
	for index, event := range result.Events {
		if event.Sequence != int64(index+1) || event.Status != StatusQueued {
			t.Fatalf("event %d = %#v", index, event)
		}
		if event.Label == "" {
			t.Fatalf("event %d has no server-owned label", index)
		}
	}
}

func TestReporterAllocatesMonotonicSequenceAndTerminalState(t *testing.T) {
	service := NewService(NewInMemoryStore(), nil)
	ctx := testContext("workspace-a", "user-a")
	result, err := service.Create(ctx, CreateInput{Kind: KindMemoryReflection})
	if err != nil {
		t.Fatal(err)
	}
	reporter, err := service.Attach(ctx, result.Operation.ID, KindMemoryReflection)
	if err != nil {
		t.Fatal(err)
	}
	if err := reporter.Report(ctx, "load_evidence", StatusRunning, nil, false); err != nil {
		t.Fatal(err)
	}
	if err := reporter.Report(ctx, "load_evidence", StatusSucceeded, map[string]any{"event_count": 3}, false); err != nil {
		t.Fatal(err)
	}
	if err := reporter.Report(ctx, "memory_ready", StatusSucceeded, map[string]any{"changed": true}, true); err != nil {
		t.Fatal(err)
	}
	events, err := service.Events(ctx, result.Operation.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	for index, event := range events {
		if event.Sequence != int64(index+1) {
			t.Fatalf("event %d sequence=%d", index, event.Sequence)
		}
	}
	operation, err := service.Get(ctx, result.Operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if operation.Status != StatusSucceeded || operation.CompletedAt == nil {
		t.Fatalf("operation not terminal: %#v", operation)
	}
	if err := reporter.Report(ctx, "memory_ready", StatusSucceeded, nil, true); !errors.Is(err, ErrTerminal) {
		t.Fatalf("terminal append error=%v, want ErrTerminal", err)
	}
}

func TestOperationScopePreventsCrossWorkspaceAndUserReads(t *testing.T) {
	service := NewService(NewInMemoryStore(), nil)
	owner := testContext("workspace-a", "user-a")
	result, err := service.Create(owner, CreateInput{Kind: KindMCQGeneration})
	if err != nil {
		t.Fatal(err)
	}
	for _, other := range []context.Context{
		testContext("workspace-b", "user-a"),
		testContext("workspace-a", "user-b"),
	} {
		if _, err := service.Get(other, result.Operation.ID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("cross-scope get error=%v, want ErrNotFound", err)
		}
		if _, err := service.Events(other, result.Operation.ID, 0); !errors.Is(err, ErrNotFound) {
			t.Fatalf("cross-scope events error=%v, want ErrNotFound", err)
		}
	}
}

func TestMetadataRejectsPrivateOrUnboundedFields(t *testing.T) {
	service := NewService(NewInMemoryStore(), nil)
	ctx := testContext("workspace-a", "user-a")
	result, err := service.Create(ctx, CreateInput{Kind: KindMCQGeneration})
	if err != nil {
		t.Fatal(err)
	}
	reporter, err := service.Attach(ctx, result.Operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]any{
		"prompt": "private prompt",
		"stack":  "private stack",
		"token":  "secret",
		"record": map[string]any{"private": true},
	} {
		if err := reporter.Report(ctx, "generate_questions", StatusRunning, map[string]any{key: value}, false); err == nil {
			t.Fatalf("metadata key %q was accepted", key)
		}
	}
}

func TestConcurrentAppendsRemainStrictlyOrdered(t *testing.T) {
	service := NewService(NewInMemoryStore(), nil)
	ctx := testContext("workspace-a", "user-a")
	result, err := service.Create(ctx, CreateInput{Kind: KindMCQGeneration})
	if err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	for range 20 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			reporter, attachErr := service.Attach(ctx, result.Operation.ID)
			if attachErr == nil {
				_ = reporter.Report(ctx, "generate_questions", StatusRunning, nil, false)
			}
		}()
	}
	wait.Wait()
	events, err := service.Events(ctx, result.Operation.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	for index, event := range events {
		if event.Sequence != int64(index+1) {
			t.Fatalf("event %d sequence=%d", index, event.Sequence)
		}
	}
}

func TestCancelTerminatesCurrentStep(t *testing.T) {
	service := NewService(NewInMemoryStore(), nil)
	ctx := testContext("workspace-a", "user-a")
	result, err := service.Create(ctx, CreateInput{Kind: KindMCQGeneration})
	if err != nil {
		t.Fatal(err)
	}
	reporter, err := service.Attach(ctx, result.Operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := reporter.Report(ctx, "load_context", StatusRunning, nil, false); err != nil {
		t.Fatal(err)
	}
	operation, event, err := service.Cancel(ctx, result.Operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if operation.Status != StatusFailed || event.StepID != "load_context" ||
		event.Metadata["reason_code"] != "cancelled" {
		t.Fatalf("unexpected cancellation: operation=%#v event=%#v", operation, event)
	}
}

func TestNextRoundRequiresSuccessfulMemorySteps(t *testing.T) {
	service := NewService(NewInMemoryStore(), nil)
	ctx := testContext("workspace-a", "user-a")
	result, err := service.Create(ctx, CreateInput{Kind: KindMCQNextRound})
	if err != nil {
		t.Fatal(err)
	}
	required := []string{"load_evidence", "synthesize_profile", "validate_profile", "save_profile"}
	if err := service.RequireSucceeded(ctx, result.Operation.ID, required...); !errors.Is(err, ErrPrerequisite) {
		t.Fatalf("initial prerequisite error=%v", err)
	}
	reporter, err := service.Attach(ctx, result.Operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, stepID := range required {
		if err := reporter.Report(ctx, stepID, StatusSucceeded, nil, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := service.RequireSucceeded(ctx, result.Operation.ID, required...); err != nil {
		t.Fatalf("completed prerequisite error=%v", err)
	}
}

func testContext(workspaceID, userID string) context.Context {
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{
		UserID: userID, DefaultWorkspaceID: workspaceID, WorkspaceIDs: []string{workspaceID},
	})
	return workspace.WithScope(ctx, workspace.Scope{WorkspaceID: workspaceID})
}
