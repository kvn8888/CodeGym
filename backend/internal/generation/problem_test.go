package generation

import (
	"context"
	"encoding/json"
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
	if !strings.Contains(runner, `PREFIX = "CODEGYM_RESULT "`) || !strings.Contains(runner, "base64.b64decode") {
		t.Fatalf("runner is not controlled: %s", runner)
	}
	if strings.Contains(runner, "Track values already seen") {
		t.Fatal("runner unexpectedly contains public hint text")
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
