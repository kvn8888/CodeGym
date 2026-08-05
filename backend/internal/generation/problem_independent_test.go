package generation

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/kvn8888/codegym/backend/internal/memory"
)

const independentPythonTestsArtifact = `{
  "test_cases":[
    {"name":"case-public-one","kind":"example","hidden":false,"args":[[1,2,1]],"expected":[1,2]},
    {"name":"case-public-empty","kind":"edge","hidden":false,"args":[[]],"expected":[]},
    {"name":"case-hidden-repeat","kind":"hidden","hidden":true,"args":[[3,3,2,3]],"expected":[3,2]},
    {"name":"case-hidden-order","kind":"stress","hidden":true,"args":[[2,1,2,1]],"expected":[2,1]}
  ]
}`

const independentPythonReferenceArtifact = `{
  "reference_solution":"def stable_unique(values: list[int]) -> list[int]:\n    # REFERENCE_ONLY_MARKER\n    return list(dict.fromkeys(values))"
}`

type isolatedRoleGenerator struct {
	mu               sync.Mutex
	requests         []GenerateRequest
	testsStarted     chan struct{}
	referenceStarted chan struct{}
}

func newIsolatedRoleGenerator() *isolatedRoleGenerator {
	return &isolatedRoleGenerator{testsStarted: make(chan struct{}), referenceStarted: make(chan struct{})}
}

func (g *isolatedRoleGenerator) Generate(ctx context.Context, request GenerateRequest) (GenerateResult, error) {
	g.mu.Lock()
	g.requests = append(g.requests, request)
	g.mu.Unlock()
	switch request.Kind {
	case KindProblemSpec:
		return GenerateResult{Object: json.RawMessage(validIndependentPythonSpec), Provider: "test", Model: "model"}, nil
	case KindProblemTests:
		close(g.testsStarted)
		select {
		case <-g.referenceStarted:
		case <-ctx.Done():
			return GenerateResult{}, ctx.Err()
		}
		return GenerateResult{Object: json.RawMessage(independentPythonTestsArtifact), Provider: "test", Model: "model"}, nil
	case KindProblemReference:
		close(g.referenceStarted)
		select {
		case <-g.testsStarted:
		case <-ctx.Done():
			return GenerateResult{}, ctx.Err()
		}
		return GenerateResult{Object: json.RawMessage(independentPythonReferenceArtifact), Provider: "test", Model: "model"}, nil
	default:
		return GenerateResult{}, &InvalidOutputError{Reason: "unexpected role " + string(request.Kind)}
	}
}

func (g *isolatedRoleGenerator) recordedRequests() []GenerateRequest {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]GenerateRequest(nil), g.requests...)
}

const validIndependentPythonSpec = `{
  "language":"python","strategy":"unit","title":"Stable Unique Values",
  "description":"Return unique integers in first-seen order.","category":"algorithms",
  "subcategory":"arrays","tags":["arrays"],"difficulty":1,"estimated_minutes":15,
  "hints":["Track seen values."],"entrypoint":"stable_unique",
  "signature":"def stable_unique(values: list[int]) -> list[int]",
  "io_contract":"The function accepts one list of integers and returns a list of integers.",
  "comparator":{"kind":"exact"},
  "ambiguity_resolutions":["Empty input returns [].","Output preserves first occurrence order."],
  "function_name":"stable_unique","parameters":[{"name":"values","type":"list[int]"}],
  "return_type":"list[int]"
}`

const validIndependentGoHTTPSpec = `{
  "language":"go","strategy":"http","title":"Greeting API",
  "description":"Implement GET /greet/{name}; the JSON response field greeting is a string.",
  "category":"backend","subcategory":"http","tags":["go","http"],"difficulty":1,
  "estimated_minutes":20,"hints":["Use net/http."],"entrypoint":"main.go",
  "signature":"func main()","io_contract":"GET /greet/{name} returns status 200 and JSON {greeting: string}.",
  "comparator":{"kind":"exact"},
  "ambiguity_resolutions":["The greeting field is always a JSON string.","Missing names return status 404."]
}`

func TestValidateGeneratedProblemSpecAcceptsUnitAndHTTP(t *testing.T) {
	for _, test := range []struct {
		name, language, payload string
	}{
		{name: "python unit", language: "python", payload: validIndependentPythonSpec},
		{name: "go http", language: "go", payload: validIndependentGoHTTPSpec},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ValidateGeneratedProblemSpecForLanguage(json.RawMessage(test.payload), test.language); err != nil {
				t.Fatalf("ValidateGeneratedProblemSpecForLanguage: %v", err)
			}
		})
	}
}

func TestValidateGeneratedProblemSpecRejectsMissingAlignmentFields(t *testing.T) {
	for _, field := range []string{"signature", "comparator", "ambiguity_resolutions"} {
		t.Run(field, func(t *testing.T) {
			var object map[string]any
			if err := json.Unmarshal([]byte(validIndependentPythonSpec), &object); err != nil {
				t.Fatal(err)
			}
			delete(object, field)
			raw, err := json.Marshal(object)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ValidateGeneratedProblemSpecForLanguage(raw, "python"); err == nil || !strings.Contains(err.Error(), field) {
				t.Fatalf("missing %s error = %v", field, err)
			}
		})
	}
}

func TestValidateGeneratedProblemSpecRejectsEmptyAmbiguityResolutions(t *testing.T) {
	payload := strings.Replace(validIndependentPythonSpec,
		`"ambiguity_resolutions":["Empty input returns [].","Output preserves first occurrence order."]`,
		`"ambiguity_resolutions":[]`, 1)
	if _, err := ValidateGeneratedProblemSpecForLanguage(json.RawMessage(payload), "python"); err == nil || !strings.Contains(err.Error(), "ambiguity_resolutions") {
		t.Fatalf("error = %v", err)
	}
}

func TestGenerateProblemSpecUsesDedicatedContractPrompt(t *testing.T) {
	generator := &problemSequenceGenerator{payloads: []string{validIndependentPythonSpec}}
	orchestrator := NewOrchestrator(memory.NewService(memory.NewInMemoryStore(), nil), generator)
	generated, _, err := GenerateProblemSpec(generationTestContext(), orchestrator, ProblemSpec{Topic: "arrays", Language: "python"})
	if err != nil {
		t.Fatalf("GenerateProblemSpec: %v", err)
	}
	if generated.Signature == "" || len(generator.requests) != 1 {
		t.Fatalf("generated=%#v requests=%d", generated, len(generator.requests))
	}
	request := generator.requests[0]
	if request.Kind != KindProblemSpec || !strings.Contains(request.Instructions, "ambiguity_resolutions") || strings.Contains(request.Instructions, "reference_solution contains") {
		t.Fatalf("unexpected spec request: kind=%q instructions=%q", request.Kind, request.Instructions)
	}
}

func TestGenerateIndependentProblemRunsIsolatedArtifactsInParallel(t *testing.T) {
	generator := newIsolatedRoleGenerator()
	orchestrator := NewOrchestrator(memory.NewService(memory.NewInMemoryStore(), nil), generator)
	definition, contract, generated, _, err := GenerateIndependentProblem(
		generationTestContext(), orchestrator, ProblemSpec{Topic: "arrays", Language: "python"},
	)
	if err != nil {
		t.Fatalf("GenerateIndependentProblem: %v", err)
	}
	if contract.Signature == "" || len(generated.TestCases) != 4 || !strings.Contains(definition.ReferenceSolution, "REFERENCE_ONLY_MARKER") {
		t.Fatalf("unexpected independent result: contract=%#v generated=%#v", contract, generated)
	}

	var testsRequest, referenceRequest *GenerateRequest
	requests := generator.recordedRequests()
	for index := range requests {
		switch requests[index].Kind {
		case KindProblemTests:
			testsRequest = &requests[index]
		case KindProblemReference:
			referenceRequest = &requests[index]
		}
	}
	if testsRequest == nil || referenceRequest == nil {
		t.Fatalf("missing isolated requests: %#v", requests)
	}
	testsPrompt := testsRequest.Instructions + "\n" + string(testsRequest.Spec)
	referencePrompt := referenceRequest.Instructions + "\n" + string(referenceRequest.Spec)
	if strings.Contains(testsPrompt, "REFERENCE_ONLY_MARKER") || strings.Contains(testsPrompt, "reference_solution\"") {
		t.Fatalf("tests prompt leaked reference artifact: %s", testsPrompt)
	}
	if strings.Contains(referencePrompt, "case-public-one") || strings.Contains(referencePrompt, "\"test_cases\"") {
		t.Fatalf("reference prompt leaked tests artifact: %s", referencePrompt)
	}
}

func TestAssembleIndependentProblemRejectsCrossRoleFields(t *testing.T) {
	contract, err := ValidateGeneratedProblemSpecForLanguage(json.RawMessage(validIndependentPythonSpec), "python")
	if err != nil {
		t.Fatal(err)
	}
	poisonedTests := strings.TrimSuffix(independentPythonTestsArtifact, "}") + `,"reference_solution":"leak"}`
	if _, err := AssembleIndependentProblem(contract, json.RawMessage(poisonedTests), json.RawMessage(independentPythonReferenceArtifact)); err == nil {
		t.Fatal("expected tests artifact with reference_solution to be rejected")
	}
}
