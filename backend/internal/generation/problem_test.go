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
	    {"name":"basic","kind":"example","hidden":false,"rationale":"Shows an ordinary match.","args":[[1,5,3],2],"expected":true},
	    {"name":"missing","kind":"functional","hidden":false,"rationale":"Shows the no-match result.","args":[[1,2,3],8],"expected":false},
	    {"name":"negative","kind":"edge","hidden":true,"rationale":"Checks negative values.","args":[[-2,4,1],3],"expected":true},
	    {"name":"duplicate","kind":"hidden","hidden":true,"rationale":"Checks equal values with zero difference.","args":[[2,2],0],"expected":true}
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
	    {"name":"basic","kind":"example","hidden":false,"rationale":"Shows enabled aggregation.","args":[[[1.5,2.5],[3.0]],{"1":["a"],"2":["b","c"]},true],"expected":{"values":3,"labels":3}},
	    {"name":"disabled","kind":"functional","hidden":false,"rationale":"Shows disabled behavior.","args":[[[1.0]],{"1":["a"]},false],"expected":{"values":0,"labels":0}},
	    {"name":"empty","kind":"edge","hidden":true,"rationale":"Checks empty collections.","args":[[],{},true],"expected":{"values":0,"labels":0}},
	    {"name":"mixed empty","kind":"hidden","hidden":true,"rationale":"Checks nested empty values.","args":[[[],[4.0]],{"-1":[]},true],"expected":{"values":1,"labels":0}}
  ]
}`

const validGoHTTPProblemPayload = `{
  "language":"go",
  "strategy":"http",
  "title":"Build an Item API",
  "description":"Implement a small in-memory item API with health, create, and fetch routes.",
  "category":"backend",
  "subcategory":"http-servers",
  "tags":["go","net-http","json"],
  "difficulty":2,
  "estimated_minutes":35,
  "hints":["Use an http.ServeMux.","Protect shared state with a mutex."],
  "entrypoint":"main.go",
  "starter_code":"package main\n\nimport (\n    \"log\"\n    \"net/http\"\n    \"os\"\n)\n\nfunc main() {\n    port := os.Getenv(\"PORT\")\n    mux := http.NewServeMux()\n    mux.HandleFunc(\"GET /health\", func(w http.ResponseWriter, r *http.Request) {\n        // TODO: return the health response.\n        http.Error(w, \"not implemented\", http.StatusNotImplemented)\n    })\n    mux.HandleFunc(\"POST /items\", func(w http.ResponseWriter, r *http.Request) {\n        // TODO: create an item.\n        http.Error(w, \"not implemented\", http.StatusNotImplemented)\n    })\n    mux.HandleFunc(\"GET /items/{id}\", func(w http.ResponseWriter, r *http.Request) {\n        // TODO: fetch an item.\n        http.Error(w, \"not implemented\", http.StatusNotImplemented)\n    })\n    if err := http.ListenAndServe(\":\"+port, mux); err != nil { log.Fatal(err) }\n}",
  "reference_solution":"package main\n\nimport (\n    \"encoding/json\"\n    \"log\"\n    \"net/http\"\n    \"os\"\n    \"sync\"\n)\n\nfunc main() {\n    port := os.Getenv(\"PORT\")\n    mux := http.NewServeMux()\n    var mu sync.Mutex\n    items := map[string]map[string]string{}\n    mux.HandleFunc(\"GET /health\", func(w http.ResponseWriter, r *http.Request) {\n        w.Header().Set(\"Content-Type\", \"application/json\")\n        _ = json.NewEncoder(w).Encode(map[string]string{\"status\": \"ok\"})\n    })\n    mux.HandleFunc(\"POST /items\", func(w http.ResponseWriter, r *http.Request) {\n        input := map[string]string{}\n        if json.NewDecoder(r.Body).Decode(&input) != nil { http.Error(w, \"bad request\", 400); return }\n        created := map[string]string{\"id\": \"1\", \"name\": input[\"name\"]}\n        mu.Lock(); items[created[\"id\"]] = created; mu.Unlock()\n        w.Header().Set(\"Content-Type\", \"application/json\")\n        w.WriteHeader(http.StatusCreated)\n        _ = json.NewEncoder(w).Encode(created)\n    })\n    mux.HandleFunc(\"GET /items/{id}\", func(w http.ResponseWriter, r *http.Request) {\n        mu.Lock(); found, ok := items[r.PathValue(\"id\")]; mu.Unlock()\n        if !ok { http.NotFound(w, r); return }\n        w.Header().Set(\"Content-Type\", \"application/json\")\n        _ = json.NewEncoder(w).Encode(found)\n    })\n    if err := http.ListenAndServe(\":\"+port, mux); err != nil { log.Fatal(err) }\n}",
  "comparator":{"kind":"exact"},
  "http_test_cases":[
	    {"name":"health","kind":"example","hidden":false,"rationale":"Shows the health response.","request":{"method":"GET","path":"/health"},"expect":{"status":200,"json":{"status":"ok"},"headers":{"content-type":"application/json"}}},
	    {"name":"creates-item","kind":"example","hidden":false,"rationale":"Shows item creation.","request":{"method":"POST","path":"/items","headers":{"content-type":"application/json"},"body":{"name":"book"}},"expect":{"status":201,"json":{"id":"1","name":"book"}}},
	    {"name":"gets-item","kind":"functional","hidden":true,"rationale":"Checks persisted state.","request":{"method":"GET","path":"/items/1"},"expect":{"status":200,"json":{"name":"book","id":"1"}}},
	    {"name":"missing-item","kind":"edge","hidden":true,"rationale":"Checks missing resources.","request":{"method":"GET","path":"/items/404"},"expect":{"status":404,"body":"404 page not found\n"}}
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
	if definition.Entrypoint != "test_solution.py" || len(definition.HiddenTestFiles) != 2 || len(definition.PublicTestFiles) != 2 || len(definition.PublicCases) != 2 {
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
	if generated.Strategy != problems.TestStrategyUnit || generated.Entrypoint != "" || generated.StarterCode != "" || len(generated.HTTPTestCases) != 0 {
		t.Fatalf("legacy Python generation shape changed: %#v", generated)
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

func TestGoHTTPStrategyBuildsAndRunsServerDefinition(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is required for generated Go HTTP validation")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is required for the Go HTTP compile wrapper")
	}
	generated, err := ValidateGeneratedProblemForLanguage(json.RawMessage(validGoHTTPProblemPayload), "go")
	if err != nil {
		t.Fatalf("ValidateGeneratedProblemForLanguage: %v", err)
	}
	definition, err := BuildProblemDefinition(generated)
	if err != nil {
		t.Fatalf("BuildProblemDefinition: %v", err)
	}
	if definition.Language != "go" || definition.Framework != "net/http" || definition.TestConfig.Strategy != problems.TestStrategyHTTP {
		t.Fatalf("Go HTTP definition = %#v", definition)
	}
	if definition.Entrypoint != goHTTPLauncherPath || definition.Files.Skeleton[0].Path != "main.go" ||
		definition.TestConfig.ReadinessTimeoutSeconds != 10 || definition.Runtime.TimeoutSeconds != 60 {
		t.Fatalf("Go HTTP wiring = %#v", definition)
	}
	if len(definition.HiddenTestFiles) != 4 || len(definition.PublicTestFiles) != 4 ||
		!strings.Contains(hiddenFileContent(t, definition, goHTTPCasesPath), "gets-item") ||
		strings.Contains(publicFileContent(t, definition, goHTTPCasesPath), "gets-item") {
		t.Fatalf("Go HTTP hidden files = %#v", definition.HiddenTestFiles)
	}
	public, err := json.Marshal(definition.Problem)
	if err != nil {
		t.Fatalf("marshal public problem: %v", err)
	}
	if !strings.Contains(string(public), "creates-item") || strings.Contains(string(public), "gets-item") || strings.Contains(string(public), "missing-item") {
		t.Fatalf("public problem projection is incorrect: %s", public)
	}

	root := t.TempDir()
	files := []problems.File{{Path: "main.go", Content: definition.ReferenceSolution}}
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
		t.Fatalf("run generated Go HTTP harness: %v\n%s", err, output)
	}
	verdict, err := os.ReadFile(filepath.Join(root, ".codegym", "verdict.json"))
	if err != nil || !strings.Contains(string(verdict), `"status":"passed"`) {
		t.Fatalf("Go HTTP verdict = %s, %v", verdict, err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte(definition.SkeletonFiles[0].Content), 0o600); err != nil {
		t.Fatalf("write Go HTTP skeleton: %v", err)
	}
	compileSkeleton := exec.Command("go", "build", "-o", filepath.Join(root, ".codegym", "skeleton"), "main.go")
	compileSkeleton.Dir = root
	if output, err := compileSkeleton.CombinedOutput(); err != nil {
		t.Fatalf("generated Go HTTP skeleton does not compile: %v\n%s", err, output)
	}
}

func TestGoHTTPStrategyRejectsEntrypointThatDoesNotStartServer(t *testing.T) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(validGoHTTPProblemPayload), &payload); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	payload["starter_code"] = "package main\n\nimport (\"net/http\"; \"os\")\n\nfunc main() { _ = http.MethodGet; _ = os.Getenv }\n"
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode fixture: %v", err)
	}
	_, err = ValidateGeneratedProblemForLanguage(raw, "go")
	if err == nil || !strings.Contains(err.Error(), "starter_code entrypoint must read PORT and start an HTTP server") {
		t.Fatalf("error = %v", err)
	}
}

func TestProblemLanguageSchemasAreValidJSON(t *testing.T) {
	for name, schema := range map[string]json.RawMessage{"python": problemJSONSchema, "go": goProblemJSONSchema} {
		if !json.Valid(schema) {
			t.Fatalf("%s problem schema is invalid JSON", name)
		}
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

func TestGeneratedProblemsRequireBalancedPublicAndHiddenCases(t *testing.T) {
	for name, fixture := range map[string]string{
		"python unit": validProblemPayload,
		"go unit":     validGoProblemPayload,
		"go http":     validGoHTTPProblemPayload,
	} {
		language := "python"
		if strings.HasPrefix(name, "go ") {
			language = "go"
		}
		generated, err := ValidateGeneratedProblemForLanguage(json.RawMessage(fixture), language)
		if err != nil {
			t.Fatalf("%s fixture: %v", name, err)
		}
		counts := problems.CountUnitCaseVisibility(generated.TestCases)
		if generated.Strategy == problems.TestStrategyHTTP {
			counts = problems.CountHTTPCaseVisibility(generated.HTTPTestCases)
		}
		if counts.Public != 2 || counts.Hidden != 2 {
			t.Fatalf("%s counts = %#v", name, counts)
		}
		definition, err := BuildProblemDefinition(generated)
		if err != nil {
			t.Fatalf("build %s: %v", name, err)
		}
		if len(definition.PublicCases) != counts.Public || len(definition.PublicTestFiles) == 0 || len(definition.HiddenTestFiles) == 0 {
			t.Fatalf("%s definition visibility bundles = %#v", name, definition)
		}
	}

	allHidden := strings.ReplaceAll(validProblemPayload, `"hidden":false`, `"hidden":true`)
	if _, err := ValidateGeneratedProblem(json.RawMessage(allHidden)); err == nil || !strings.Contains(err.Error(), "public") {
		t.Fatalf("zero-public unit cases err = %v", err)
	}
	allPublicHTTP := strings.ReplaceAll(validGoHTTPProblemPayload, `"hidden":true`, `"hidden":false`)
	if _, err := ValidateGeneratedProblemForLanguage(json.RawMessage(allPublicHTTP), "go"); err == nil || !strings.Contains(err.Error(), "hidden") {
		t.Fatalf("zero-hidden HTTP cases err = %v", err)
	}
	missingHidden := strings.Replace(validProblemPayload, `,"hidden":false`, "", 1)
	if _, err := ValidateGeneratedProblem(json.RawMessage(missingHidden)); err == nil || !strings.Contains(err.Error(), "must include hidden") {
		t.Fatalf("missing hidden field err = %v", err)
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

func publicFileContent(t *testing.T, definition problems.Definition, path string) string {
	t.Helper()
	for _, file := range definition.PublicTestFiles {
		if file.Path == path {
			return file.Content
		}
	}
	t.Fatalf("public test file %q not found", path)
	return ""
}
