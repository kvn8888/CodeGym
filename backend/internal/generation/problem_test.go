package generation

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/memory"
	"github.com/kvn8888/codegym/backend/internal/problems"
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

const validGoProblemPayload = `{
  "title":"Summarize Buckets",
  "description":"Count nested values and labels when enabled.",
  "category":"algorithms",
  "subcategory":"collections",
  "tags":["slices","maps"],
  "difficulty":2,
  "estimated_minutes":25,
  "function_name":"summarizeBuckets",
  "parameters":[
    {"name":"values","type":"[][]float64"},
    {"name":"labels","type":"map[int][]string"},
    {"name":"enabled","type":"bool"}
  ],
  "return_type":"map[string]int",
  "hints":["Count each nested value.","Map keys arrive from JSON objects."],
  "reference_solution":"package main\n\nfunc summarizeBuckets(values [][]float64, labels map[int][]string, enabled bool) map[string]int {\n    result := map[string]int{\"values\": 0, \"labels\": 0}\n    if !enabled { return result }\n    for _, bucket := range values { result[\"values\"] += len(bucket) }\n    for _, bucketLabels := range labels { result[\"labels\"] += len(bucketLabels) }\n    return result\n}",
  "test_cases":[
    {"name":"basic","args":[[[1.5,2.5],[3.0]],{"1":["a"],"2":["b","c"]},true],"expected":{"values":3,"labels":3}},
    {"name":"disabled","args":[[[1.0]],{"1":["a"]},false],"expected":{"values":0,"labels":0}},
    {"name":"empty","args":[[],{},true],"expected":{"values":0,"labels":0}},
    {"name":"mixed empty","args":[[[],[4.0]],{"-1":[]},true],"expected":{"values":1,"labels":0}}
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
	if definition.Entrypoint != "test_solution.py" || len(definition.HiddenTestFiles) != 2 {
		t.Fatalf("definition missing controlled runner: %#v", definition)
	}
	runner := hiddenFileContent(t, definition, "test_solution.py")
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
	files := []struct{ name, content string }{{"solution.py", definition.ReferenceSolution}}
	for _, hidden := range definition.HiddenTestFiles {
		files = append(files, struct{ name, content string }{hidden.Path, hidden.Content})
	}
	for _, file := range files {
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

func TestLanguageStrategyKeepsLegacyPythonDefinition(t *testing.T) {
	generated, err := ValidateGeneratedProblem(json.RawMessage(validProblemPayload))
	if err != nil {
		t.Fatalf("ValidateGeneratedProblem: %v", err)
	}
	explicit := generated
	explicit.Language = "python"
	legacy := generated
	legacy.Language = ""
	explicitDefinition, err := BuildProblemDefinition(explicit)
	if err != nil {
		t.Fatalf("BuildProblemDefinition explicit Python: %v", err)
	}
	legacyDefinition, err := BuildProblemDefinition(legacy)
	if err != nil {
		t.Fatalf("BuildProblemDefinition legacy Python: %v", err)
	}
	if !reflect.DeepEqual(explicitDefinition, legacyDefinition) {
		t.Fatalf("explicit Python changed legacy output\nexplicit=%#v\nlegacy=%#v", explicitDefinition, legacyDefinition)
	}
	if explicitDefinition.SkeletonFiles[0].Content != "def has_pair_difference(nums: list[int], k: int) -> bool:\n    \"\"\"Implement the solution described in the problem.\"\"\"\n    # TODO: implement this function.\n    raise NotImplementedError\n" {
		t.Fatalf("Python skeleton changed: %q", explicitDefinition.SkeletonFiles[0].Content)
	}
}

func TestGoStrategyBuildsTypedRunnableDefinition(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is required for generated Go validation")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is required for the Go compile wrapper")
	}
	generated, err := ValidateGeneratedProblemForLanguage(json.RawMessage(validGoProblemPayload), "go")
	if err != nil {
		t.Fatalf("ValidateGeneratedProblemForLanguage: %v", err)
	}
	definition, err := BuildProblemDefinition(generated)
	if err != nil {
		t.Fatalf("BuildProblemDefinition: %v", err)
	}
	if definition.Language != "go" || definition.Entrypoint != ".codegym/compile_and_run.py" || definition.Files.Skeleton[0].Path != "solution.go" {
		t.Fatalf("Go definition = %#v", definition)
	}
	skeleton := definition.SkeletonFiles[0].Content
	for _, fragment := range []string{"package main", "func summarizeBuckets(values [][]float64, labels map[int][]string, enabled bool) map[string]int"} {
		if !strings.Contains(skeleton, fragment) {
			t.Fatalf("Go skeleton missing %q: %s", fragment, skeleton)
		}
	}

	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".codegym"), 0o700); err != nil {
		t.Fatalf("mkdir protocol directory: %v", err)
	}
	files := []problems.File{{Path: "solution.go", Content: definition.ReferenceSolution}}
	files = append(files, definition.HiddenTestFiles...)
	for _, file := range files {
		path := filepath.Join(root, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("mkdir %s: %v", file.Path, err)
		}
		if err := os.WriteFile(path, []byte(file.Content), 0o600); err != nil {
			t.Fatalf("write %s: %v", file.Path, err)
		}
	}
	command := exec.Command("python3", definition.Entrypoint)
	command.Dir = root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("run generated Go harness: %v\n%s", err, output)
	}
	verdict, err := os.ReadFile(filepath.Join(root, ".codegym", "verdict.json"))
	if err != nil || !strings.Contains(string(verdict), `"status":"passed"`) {
		t.Fatalf("Go verdict = %s, %v", verdict, err)
	}

	if err := os.WriteFile(filepath.Join(root, "solution.go"), []byte(skeleton), 0o600); err != nil {
		t.Fatalf("write Go skeleton: %v", err)
	}
	compileSkeleton := exec.Command("go", "test", "solution.go")
	compileSkeleton.Dir = root
	if output, err := compileSkeleton.CombinedOutput(); err != nil {
		t.Fatalf("generated Go skeleton does not compile: %v\n%s", err, output)
	}
}

func TestGoStrategyRejectsOutOfScopeTypes(t *testing.T) {
	for _, unsupported := range []string{"*int", "TreeNode", "[]*int", "map[float64]int", "struct{ Value int }", "chan int"} {
		payload := strings.Replace(validGoProblemPayload, `"type":"[][]float64"`, `"type":"`+unsupported+`"`, 1)
		_, err := ValidateGeneratedProblemForLanguage(json.RawMessage(payload), "go")
		if err == nil || !strings.Contains(err.Error(), "unsupported parameter type") {
			t.Fatalf("type %q err = %v, want clear unsupported-type rejection", unsupported, err)
		}
	}
}

func TestGenerateProblemSelectsGoStrategy(t *testing.T) {
	generator := &problemSequenceGenerator{payloads: []string{validGoProblemPayload}}
	orchestrator := NewOrchestrator(memory.NewService(memory.NewInMemoryStore(), nil), generator)
	definition, generated, _, err := GenerateProblem(generationTestContext(), orchestrator, ProblemSpec{Topic: "maps", Language: "go"})
	if err != nil {
		t.Fatalf("GenerateProblem: %v", err)
	}
	if definition.Language != "go" || generated.Language != "go" || len(generator.requests) != 1 || !strings.Contains(generator.requests[0].Instructions, "safe Go coding problem") {
		t.Fatalf("Go strategy result = %#v %#v requests=%#v", definition, generated, generator.requests)
	}
}

func TestValidateGeneratedProblemRejectsInvalidComparators(t *testing.T) {
	for _, replacement := range []string{
		`"comparator":{"kind":"approximately"},`,
		`"comparator":{"kind":"float","epsilon":0},`,
		`"comparator":{"kind":"exact","epsilon":0.1},`,
	} {
		payload := strings.Replace(validProblemPayload, `"reference_solution":`, replacement+`"reference_solution":`, 1)
		if _, err := ValidateGeneratedProblem(json.RawMessage(payload)); err == nil {
			t.Fatalf("expected invalid comparator to fail: %s", replacement)
		}
	}
}

func TestValidateGeneratedProblemRequiresCheckerSource(t *testing.T) {
	payload := strings.Replace(validProblemPayload, `"reference_solution":`, `"comparator":{"kind":"checker"},"reference_solution":`, 1)
	if _, err := ValidateGeneratedProblem(json.RawMessage(payload)); err == nil || !strings.Contains(err.Error(), "requires checker") {
		t.Fatalf("err = %v, want checker source rejection", err)
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

func hiddenFileContent(t *testing.T, definition problems.Definition, path string) string {
	t.Helper()
	for _, file := range definition.HiddenTestFiles {
		if file.Path == path {
			return file.Content
		}
	}
	t.Fatalf("hidden file %q not found", path)
	return ""
}
