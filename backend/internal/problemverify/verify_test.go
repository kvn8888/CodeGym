package problemverify

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/execution"
	"github.com/kvn8888/codegym/backend/internal/generation"
	"github.com/kvn8888/codegym/backend/internal/memory"
	"github.com/kvn8888/codegym/backend/internal/workspace"
)

type fakeRunner struct {
	outcomes []execution.RunOutcome
	calls    int
	lastSpec execution.RunSpec
	err      error
}

func (f *fakeRunner) Run(_ context.Context, spec execution.RunSpec) (execution.RunOutcome, error) {
	f.lastSpec = spec
	if f.err != nil {
		return execution.RunOutcome{}, f.err
	}
	index := f.calls
	f.calls++
	if index >= len(f.outcomes) {
		index = len(f.outcomes) - 1
	}
	return f.outcomes[index], nil
}

type sequenceGenerator struct {
	payloads []string
	requests []generation.GenerateRequest
}

func (g *sequenceGenerator) Generate(_ context.Context, request generation.GenerateRequest) (generation.GenerateResult, error) {
	g.requests = append(g.requests, request)
	index := len(g.requests) - 1
	if index >= len(g.payloads) {
		index = len(g.payloads) - 1
	}
	return generation.GenerateResult{Object: json.RawMessage(g.payloads[index]), Provider: "test", Model: "model"}, nil
}

func testContext() context.Context {
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{
		UserID: "user-1", DefaultWorkspaceID: "workspace-1",
		WorkspaceIDs: []string{"workspace-1"},
	})
	return workspace.WithScope(ctx, workspace.Scope{WorkspaceID: "workspace-1"})
}

func sampleGenerated() generation.GeneratedProblem {
	return generation.GeneratedProblem{
		Title: "Pair Difference", Description: "Return whether two numbers differ by k.",
		Category: "algorithms", Subcategory: "arrays", Tags: []string{"arrays"},
		Difficulty: 2, EstimatedMinutes: 25, FunctionName: "has_pair_difference",
		Parameters: []generation.ProblemParameter{
			{Name: "nums", Type: "list[int]"}, {Name: "k", Type: "int"},
		},
		ReturnType: "bool",
		Hints:      []string{"Track values already seen.", "Check both directions.", "Edge zeros."},
		ReferenceSolution: `def has_pair_difference(nums: list[int], k: int) -> bool:
    seen = set()
    for value in nums:
        if value-k in seen or value+k in seen:
            return True
        seen.add(value)
    return False`,
		TestCases: []generation.ProblemTestCase{
			{Name: "basic", Args: raws([]any{[]int{1, 5, 3}, 2}), Expected: raw(true)},
			{Name: "missing", Args: raws([]any{[]int{1, 2, 3}, 8}), Expected: raw(false)},
			{Name: "negative", Args: raws([]any{[]int{-2, 4, 1}, 3}), Expected: raw(true)},
			{Name: "duplicate", Args: raws([]any{[]int{2, 2}, 0}), Expected: raw(true)},
		},
	}
}

func raw(value any) json.RawMessage {
	out, _ := json.Marshal(value)
	return out
}

func raws(values []any) []json.RawMessage {
	out := make([]json.RawMessage, 0, len(values))
	for _, value := range values {
		out = append(out, raw(value))
	}
	return out
}

func passOutput() string {
	return `CODEGYM_RESULT {"tests":[{"name":"basic","status":"pass","duration_ms":1,"error":null},{"name":"missing","status":"pass","duration_ms":1,"error":null},{"name":"negative","status":"pass","duration_ms":1,"error":null},{"name":"duplicate","status":"pass","duration_ms":1,"error":null}],"compile_error":null}`
}

func failOneOutput() string {
	return `CODEGYM_RESULT {"tests":[{"name":"basic","status":"pass","duration_ms":1,"error":null},{"name":"missing","status":"fail","duration_ms":1,"error":"expected false, got true"},{"name":"negative","status":"pass","duration_ms":1,"error":null},{"name":"duplicate","status":"pass","duration_ms":1,"error":null}],"compile_error":null}`
}

func TestVerifyAllPass(t *testing.T) {
	generated := sampleGenerated()
	definition, err := generation.BuildProblemDefinition(generated)
	if err != nil {
		t.Fatalf("BuildProblemDefinition: %v", err)
	}
	runner := &fakeRunner{outcomes: []execution.RunOutcome{{ExitCode: 0, Output: passOutput(), Duration: time.Millisecond}}}
	got, _, err := Verify(testContext(), nil, runner, definition, generated)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got.Entrypoint != "test_solution.py" {
		t.Fatalf("entrypoint = %q", got.Entrypoint)
	}
	if runner.calls != 1 {
		t.Fatalf("runner calls = %d", runner.calls)
	}
	foundSolution := false
	for _, file := range runner.lastSpec.Files {
		if file.Path == "solution.py" && strings.Contains(file.Content, "has_pair_difference") {
			foundSolution = true
		}
	}
	if !foundSolution {
		t.Fatalf("runner did not receive reference solution: %#v", runner.lastSpec.Files)
	}
}

func TestVerifyRepairsThenPasses(t *testing.T) {
	generated := sampleGenerated()
	definition, err := generation.BuildProblemDefinition(generated)
	if err != nil {
		t.Fatalf("BuildProblemDefinition: %v", err)
	}
	runner := &fakeRunner{outcomes: []execution.RunOutcome{
		{ExitCode: 1, Output: failOneOutput(), Duration: time.Millisecond},
		{ExitCode: 0, Output: passOutput(), Duration: time.Millisecond},
	}}
	repairPayload := `{
  "verdict":"repaired",
  "reason":"missing expected was wrong",
  "repaired_cases":[{"name":"missing","args":[[1,2,3],8],"expected":false}]
}`
	generator := &sequenceGenerator{payloads: []string{repairPayload}}
	orchestrator := generation.NewOrchestrator(memory.NewService(memory.NewInMemoryStore(), nil), generator)

	got, repaired, err := Verify(testContext(), orchestrator, runner, definition, generated)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if runner.calls != 2 {
		t.Fatalf("runner calls = %d, want 2", runner.calls)
	}
	if len(generator.requests) != 1 || generator.requests[0].Kind != generation.KindProblemVerification {
		t.Fatalf("requests = %#v", generator.requests)
	}
	if got.Entrypoint != "test_solution.py" || len(repaired.TestCases) != 4 {
		t.Fatalf("unexpected repair result: %#v %#v", got, repaired)
	}
}

func TestVerifyRejectsRegenerate(t *testing.T) {
	generated := sampleGenerated()
	definition, err := generation.BuildProblemDefinition(generated)
	if err != nil {
		t.Fatalf("BuildProblemDefinition: %v", err)
	}
	runner := &fakeRunner{outcomes: []execution.RunOutcome{
		{ExitCode: 1, Output: failOneOutput(), Duration: time.Millisecond},
	}}
	generator := &sequenceGenerator{payloads: []string{`{"verdict":"regenerate","reason":"reference is wrong","regenerate_reason":"reference returns wrong answers"}`}}
	orchestrator := generation.NewOrchestrator(memory.NewService(memory.NewInMemoryStore(), nil), generator)

	_, _, err = Verify(testContext(), orchestrator, runner, definition, generated)
	if !errors.Is(err, ErrRejected) {
		t.Fatalf("err = %v, want ErrRejected", err)
	}
}

func TestVerifyUnavailableWithoutRunner(t *testing.T) {
	generated := sampleGenerated()
	definition, err := generation.BuildProblemDefinition(generated)
	if err != nil {
		t.Fatalf("BuildProblemDefinition: %v", err)
	}
	_, _, err = Verify(testContext(), nil, nil, definition, generated)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}
