package generation

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/memory"
	"github.com/kvn8888/codegym/backend/internal/workspace"
)

const validProblemPayload = `{
  "title":"Pair Difference",
  "description":"Return whether two numbers differ by k.",
  "category":"algorithms",
  "subcategory":"arrays-and-hash-maps",
  "tags":["arrays","hash-map"],
  "difficulty":2,
  "estimated_minutes":25,
  "function_name":"has_pair_difference",
  "parameters":[{"name":"nums","type":"list[int]"},{"name":"k","type":"int"}],
  "return_type":"bool",
  "hints":["Track values already seen.","Check both directions."],
  "reference_solution":"def has_pair_difference(nums: list[int], k: int) -> bool:\n    seen = set()\n    for value in nums:\n        if value-k in seen or value+k in seen:\n            return True\n        seen.add(value)\n    return False",
  "test_cases":[
    {"name":"basic","args":[[1,5,3],2],"expected":true},
    {"name":"missing","args":[[1,2,3],8],"expected":false},
    {"name":"negative","args":[[-2,4,1],3],"expected":true},
    {"name":"duplicate","args":[[2,2],0],"expected":true}
  ]
}`

type problemSequenceGenerator struct {
	payloads []string
	requests []GenerateRequest
}

func generationTestContext() context.Context {
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{
		UserID: "user-1", DefaultWorkspaceID: "workspace-1",
		WorkspaceIDs: []string{"workspace-1"},
	})
	return workspace.WithScope(ctx, workspace.Scope{WorkspaceID: "workspace-1"})
}

func (g *problemSequenceGenerator) Generate(_ context.Context, request GenerateRequest) (GenerateResult, error) {
	g.requests = append(g.requests, request)
	index := len(g.requests) - 1
	if index >= len(g.payloads) {
		index = len(g.payloads) - 1
	}
	return GenerateResult{Object: json.RawMessage(g.payloads[index]), Provider: "test", Model: "model"}, nil
}

func TestGenerateProblemRepairsOnceAndBuildsControlledHarness(t *testing.T) {
	generator := &problemSequenceGenerator{payloads: []string{`{"title":"broken"}`, validProblemPayload}}
	orchestrator := NewOrchestrator(memory.NewService(memory.NewInMemoryStore(), nil), generator)
	ctx := generationTestContext()

	definition, _, _, err := GenerateProblem(ctx, orchestrator, ProblemSpec{Topic: "hash maps"})
	if err != nil {
		t.Fatalf("GenerateProblem: %v", err)
	}
	if len(generator.requests) != 2 || !strings.Contains(generator.requests[1].Instructions, "previous output was rejected") {
		t.Fatalf("requests = %#v", generator.requests)
	}
	if definition.Entrypoint != "test_solution.py" || len(definition.HiddenTestFiles) != 1 {
		t.Fatalf("definition missing controlled runner: %#v", definition)
	}
	runner := definition.HiddenTestFiles[0].Content
	for _, protocol := range []string{"cases.jsonl", "verdict.json", "signal.setitimer", "base64.b64decode"} {
		if !strings.Contains(runner, protocol) {
			t.Fatalf("runner is missing %q: %s", protocol, runner)
		}
	}
	if strings.Contains(runner, "CODEGYM_RESULT") {
		t.Fatalf("runner is not controlled: %s", runner)
	}
	if strings.Contains(runner, "Track values already seen") {
		t.Fatal("runner unexpectedly contains public hint text")
	}
}

func TestGeneratedHarnessWritesIncrementalCasesAndFinalVerdict(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is required to execute the generated harness")
	}
	generated, err := ValidateGeneratedProblem(json.RawMessage(validProblemPayload))
	if err != nil {
		t.Fatalf("ValidateGeneratedProblem: %v", err)
	}
	definition, err := BuildProblemDefinition(generated)
	if err != nil {
		t.Fatalf("BuildProblemDefinition: %v", err)
	}
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".codegym"), 0o700); err != nil {
		t.Fatalf("mkdir protocol dir: %v", err)
	}
	for _, file := range []struct{ name, content string }{
		{"solution.py", definition.ReferenceSolution},
		{"test_solution.py", definition.HiddenTestFiles[0].Content},
	} {
		if err := os.WriteFile(filepath.Join(root, file.name), []byte(file.content), 0o600); err != nil {
			t.Fatalf("write %s: %v", file.name, err)
		}
	}
	command := exec.Command(python, "test_solution.py")
	command.Dir = root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("generated harness failed: %v\n%s", err, output)
	}
	progress, err := os.ReadFile(filepath.Join(root, ".codegym", "cases.jsonl"))
	if err != nil {
		t.Fatalf("read cases.jsonl: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(progress)), "\n")
	if len(lines) != len(generated.TestCases)*2 || !strings.Contains(lines[0], `"event":"case_start"`) ||
		!strings.Contains(lines[1], `"event":"case_result"`) {
		t.Fatalf("unexpected progress log: %s", progress)
	}
	verdict, err := os.ReadFile(filepath.Join(root, ".codegym", "verdict.json"))
	if err != nil {
		t.Fatalf("read verdict.json: %v", err)
	}
	if !strings.Contains(string(verdict), `"schema":1`) || !strings.Contains(string(verdict), `"status":"passed"`) ||
		strings.Contains(string(verdict), "CODEGYM_RESULT") {
		t.Fatalf("unexpected verdict: %s", verdict)
	}
}

func TestValidateGeneratedProblemRejectsFreeFormRunnerTokens(t *testing.T) {
	payload := strings.Replace(validProblemPayload,
		`"reference_solution":"def has_pair_difference`,
		`"reference_solution":"import subprocess\ndef has_pair_difference`, 1)
	if _, err := ValidateGeneratedProblem(json.RawMessage(payload)); err == nil {
		t.Fatal("expected forbidden reference solution to fail")
	}
}

func TestValidateGeneratedProblemRejectsMismatchedTypedCases(t *testing.T) {
	payload := strings.Replace(validProblemPayload, `"args":[[1,5,3],2]`, `"args":["not-an-int-list",2]`, 1)
	if _, err := ValidateGeneratedProblem(json.RawMessage(payload)); err == nil {
		t.Fatal("expected mismatched typed case to fail")
	}
}

func TestGenerateProblemStopsAfterOneRepair(t *testing.T) {
	generator := &problemSequenceGenerator{payloads: []string{`{}`, `{}`}}
	orchestrator := NewOrchestrator(memory.NewService(memory.NewInMemoryStore(), nil), generator)
	_, _, _, err := GenerateProblem(generationTestContext(), orchestrator, ProblemSpec{})
	if err == nil || len(generator.requests) != 2 {
		t.Fatalf("err=%v requests=%d", err, len(generator.requests))
	}
}
