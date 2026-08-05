package problemverify

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/execution"
	"github.com/kvn8888/codegym/backend/internal/generation"
	"github.com/kvn8888/codegym/backend/internal/memory"
	"github.com/kvn8888/codegym/backend/internal/problems"
	"github.com/kvn8888/codegym/backend/internal/workspace"
)

type fakeRunner struct {
	outcomes []execution.RunOutcome
	calls    int
	lastSpec execution.RunSpec
	err      error
}

type localPythonRunner struct {
	t *testing.T
}

func (r localPythonRunner) Run(_ context.Context, spec execution.RunSpec) (execution.RunOutcome, error) {
	r.t.Helper()
	root := r.t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".codegym"), 0o700); err != nil {
		return execution.RunOutcome{}, err
	}
	for _, file := range spec.Files {
		path := filepath.Join(root, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return execution.RunOutcome{}, err
		}
		if err := os.WriteFile(path, []byte(file.Content), 0o600); err != nil {
			return execution.RunOutcome{}, err
		}
	}
	commandArgs := spec.Language.ChildCommand(spec.Entrypoint)
	command := exec.Command(commandArgs[0], commandArgs[1:]...)
	command.Dir = root
	output, runErr := command.CombinedOutput()
	exitCode := 0
	if runErr != nil {
		var exitError *exec.ExitError
		if !errors.As(runErr, &exitError) {
			return execution.RunOutcome{}, runErr
		}
		exitCode = exitError.ExitCode()
	}
	verdictData, err := os.ReadFile(filepath.Join(root, ".codegym", "verdict.json"))
	if err != nil {
		return execution.RunOutcome{}, errors.Join(runErr, err)
	}
	var verdict execution.HarnessVerdict
	if err := json.Unmarshal(verdictData, &verdict); err != nil {
		return execution.RunOutcome{}, err
	}
	return execution.RunOutcome{
		ExitCode: exitCode,
		Result: execution.JudgeResult{
			Schema: verdict.Schema, Status: verdict.Status, Cases: verdict.Cases,
			CompileError: verdict.CompileError, ExitCode: &exitCode,
		},
		Output: string(output),
	}, nil
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
			{CaseMetadata: problems.CaseMetadata{Kind: problems.CaseKindExample}, Name: "basic", Args: raws([]any{[]int{1, 5, 3}, 2}), Expected: raw(true)},
			{CaseMetadata: problems.CaseMetadata{Kind: problems.CaseKindFunctional}, Name: "missing", Args: raws([]any{[]int{1, 2, 3}, 8}), Expected: raw(false)},
			{CaseMetadata: problems.CaseMetadata{Kind: problems.CaseKindEdge, Hidden: true}, Name: "negative", Args: raws([]any{[]int{-2, 4, 1}, 3}), Expected: raw(true)},
			{CaseMetadata: problems.CaseMetadata{Kind: problems.CaseKindHidden, Hidden: true}, Name: "duplicate", Args: raws([]any{[]int{2, 2}, 0}), Expected: raw(true)},
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

func TestVerifyAcceptsDifferentValidAnswerWithChecker(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is required for comparator verification regression")
	}
	_ = python

	generated := multiAnswerGenerated(problems.Comparator{Kind: problems.ComparatorChecker})
	rawGenerated, err := json.Marshal(generated)
	if err != nil {
		t.Fatalf("marshal generated problem: %v", err)
	}
	generated, err = generation.ValidateGeneratedProblem(rawGenerated)
	if err != nil {
		t.Fatalf("ValidateGeneratedProblem: %v", err)
	}
	definition, err := generation.BuildProblemDefinition(generated)
	if err != nil {
		t.Fatalf("BuildProblemDefinition: %v", err)
	}
	if err := VerifyAgreement(testContext(), localPythonRunner{t: t}, definition); err != nil {
		t.Fatalf("checker verification rejected a different valid pair: %v", err)
	}

	exact := multiAnswerGenerated(problems.Comparator{Kind: problems.ComparatorExact})
	exact.Checker = ""
	exactDefinition, err := generation.BuildProblemDefinition(exact)
	if err != nil {
		t.Fatalf("BuildProblemDefinition exact: %v", err)
	}
	if err := VerifyAgreement(testContext(), localPythonRunner{t: t}, exactDefinition); !errors.Is(err, ErrRejected) {
		t.Fatalf("exact verification err = %v, want ErrRejected", err)
	}
}

func multiAnswerGenerated(comparator problems.Comparator) generation.GeneratedProblem {
	return generation.GeneratedProblem{
		Title: "Any Pair", Description: "Return the indices of any distinct pair whose values sum to target.",
		Category: "algorithms", Subcategory: "arrays", Tags: []string{"arrays"},
		Difficulty: 2, EstimatedMinutes: 20, FunctionName: "any_pair",
		Parameters: []generation.ProblemParameter{
			{Name: "nums", Type: "list[int]"}, {Name: "target", Type: "int"},
		},
		ReturnType: "list[int]", Hints: []string{"Track complements."}, Comparator: comparator,
		ReferenceSolution: `def any_pair(nums: list[int], target: int) -> list[int]:
    for right in range(len(nums) - 1, -1, -1):
        for left in range(right - 1, -1, -1):
            if nums[left] + nums[right] == target:
                return [left, right]
    return []`,
		Checker: `def check(args, actual, expected):
    nums, target = args
    if not isinstance(actual, list) or len(actual) != 2:
        return False, "answer must contain two indices"
    left, right = actual
    if not isinstance(left, int) or isinstance(left, bool) or not isinstance(right, int) or isinstance(right, bool):
        return False, "indices must be integers"
    if left == right or left < 0 or right < 0 or left >= len(nums) or right >= len(nums):
        return False, "indices must be distinct and in range"
    return nums[left] + nums[right] == target, "selected values do not sum to target"`,
		TestCases: []generation.ProblemTestCase{
			{CaseMetadata: problems.CaseMetadata{Kind: problems.CaseKindExample}, Name: "first", Args: raws([]any{[]int{1, 4, 2, 3}, 5}), Expected: raw([]int{0, 1})},
			{CaseMetadata: problems.CaseMetadata{Kind: problems.CaseKindFunctional}, Name: "second", Args: raws([]any{[]int{2, 6, 3, 5}, 8}), Expected: raw([]int{0, 1})},
			{CaseMetadata: problems.CaseMetadata{Kind: problems.CaseKindEdge, Hidden: true}, Name: "negative", Args: raws([]any{[]int{-1, 5, 1, 3}, 4}), Expected: raw([]int{0, 1})},
			{CaseMetadata: problems.CaseMetadata{Kind: problems.CaseKindHidden, Hidden: true}, Name: "zero", Args: raws([]any{[]int{0, 10, 4, 6}, 10}), Expected: raw([]int{0, 1})},
		},
	}
}
