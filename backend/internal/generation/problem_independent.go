package generation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync"

	"github.com/kvn8888/codegym/backend/internal/problems"
)

const problemSpecMaxTokens = 4096

const problemArtifactMaxTokens = 6144

// GeneratedProblemSpec is the alignment contract shared with the otherwise
// isolated tests and reference calls. It deliberately contains no executable
// test cases or reference implementation.
type GeneratedProblemSpec struct {
	Language             string                `json:"language"`
	Strategy             problems.TestStrategy `json:"strategy"`
	Title                string                `json:"title"`
	Description          string                `json:"description"`
	Category             string                `json:"category"`
	Subcategory          string                `json:"subcategory"`
	Tags                 []string              `json:"tags"`
	Difficulty           int                   `json:"difficulty"`
	EstimatedMinutes     int                   `json:"estimated_minutes"`
	Hints                []string              `json:"hints"`
	Entrypoint           string                `json:"entrypoint"`
	Signature            string                `json:"signature"`
	IOContract           string                `json:"io_contract"`
	Comparator           problems.Comparator   `json:"comparator"`
	AmbiguityResolutions []string              `json:"ambiguity_resolutions"`
	FunctionName         string                `json:"function_name,omitempty"`
	Parameters           []ProblemParameter    `json:"parameters,omitempty"`
	ReturnType           string                `json:"return_type,omitempty"`
}

// GeneratedProblemTests is produced by the tests role. It has no field in
// which a reference implementation could be supplied or returned.
type GeneratedProblemTests struct {
	Checker       string              `json:"checker,omitempty"`
	TestCases     []ProblemTestCase   `json:"test_cases,omitempty"`
	HTTPTestCases []problems.HTTPCase `json:"http_test_cases,omitempty"`
}

// GeneratedProblemReference is produced by the reference role. It has no
// field in which generated cases or checker source could be supplied or
// returned.
type GeneratedProblemReference struct {
	ReferenceSolution string `json:"reference_solution"`
	StarterCode       string `json:"starter_code,omitempty"`
}

// ReconciliationRecord is the auditable attribution emitted for every
// post-disagreement correction.
type ReconciliationRecord struct {
	Round          int    `json:"round"`
	EditedArtifact string `json:"edited_artifact"`
	Reason         string `json:"reason"`
}

type reconciledProblemArtifacts struct {
	EditedArtifact    string              `json:"edited_artifact"`
	Reason            string              `json:"reason"`
	ReferenceSolution string              `json:"reference_solution"`
	StarterCode       string              `json:"starter_code,omitempty"`
	Checker           string              `json:"checker,omitempty"`
	TestCases         []ProblemTestCase   `json:"test_cases,omitempty"`
	HTTPTestCases     []problems.HTTPCase `json:"http_test_cases,omitempty"`
}

const problemSpecSystemPrompt = `You are the specification agent for CodeGym.
Return one precise coding-problem contract as JSON. Do not write tests, a checker,
starter code, or a reference solution.

The spec is the only alignment artifact two isolated implementation calls will
share. Make it sufficiently exact that they can independently produce compatible
tests and code. In particular:
- State the complete user-visible problem in description.
- State entrypoint and signature in the required canonical form, including all types.
- State the complete I/O contract for the selected unit or http strategy.
- Choose and explicitly include an acceptance comparator. Never omit it.
- ambiguity_resolutions is load-bearing and must be non-empty. Resolve edge cases,
  empty inputs, tie-breaking or multiple valid answers, ordering, numeric tolerance,
  and concrete JSON field types wherever relevant. Never leave those choices implicit.
- Use comparator checker when multiple structurally different answers are valid;
  explain exactly what makes an answer valid in the I/O contract and ambiguity resolutions.
- difficulty is 1..3, estimated_minutes is 10..90, hints contains 1..3 progressive hints.

Language and strategy rules:
- Python supports unit only. Its canonical signature is
  "def name(param: type, ...) -> return_type" and entrypoint is the function name.
  Allowed types: int, str, bool, list[int], list[str].
- Go supports unit and http. A unit canonical signature is
  "func Name(param type, ...) return_type" and entrypoint is the function name.
  Allowed unit types are int, float64, bool, string, slices and nested slices of
  those, and maps keyed by string or int.
- Go http uses entrypoint "main.go" and signature "func main()". Its I/O contract
  must name every method, path, status, request-body shape, response-body shape,
  and the JSON type of every field, plus at least one worked request/response.
  checker is unsupported for http.`

var problemSpecJSONSchema = json.RawMessage(`{
  "type":"object",
  "required":["language","strategy","title","description","category","subcategory","tags","difficulty","estimated_minutes","hints","entrypoint","signature","io_contract","comparator","ambiguity_resolutions"],
  "properties":{
    "language":{"type":"string","enum":["python","go"]},
    "strategy":{"type":"string","enum":["unit","http"]},
    "title":{"type":"string"},
    "description":{"type":"string"},
    "category":{"type":"string"},
    "subcategory":{"type":"string"},
    "tags":{"type":"array","minItems":1,"maxItems":8,"items":{"type":"string"}},
    "difficulty":{"type":"integer","minimum":1,"maximum":3},
    "estimated_minutes":{"type":"integer","minimum":10,"maximum":90},
    "hints":{"type":"array","minItems":1,"maxItems":3,"items":{"type":"string"}},
    "entrypoint":{"type":"string"},
    "signature":{"type":"string"},
    "io_contract":{"type":"string"},
    "comparator":{"type":"object","required":["kind"],"properties":{"kind":{"type":"string","enum":["exact","set","multiset","sorted","float","checker"]},"epsilon":{"type":"number","exclusiveMinimum":0}}},
    "ambiguity_resolutions":{"type":"array","minItems":1,"items":{"type":"string"}},
    "function_name":{"type":"string"},
    "parameters":{"type":"array","minItems":1,"maxItems":5,"items":{"type":"object","required":["name","type"],"properties":{"name":{"type":"string"},"type":{"type":"string"}}}},
    "return_type":{"type":"string"}
  },
  "oneOf":[
    {"required":["function_name","parameters","return_type"],"properties":{"strategy":{"const":"unit"}}},
    {"properties":{"language":{"const":"go"},"strategy":{"const":"http"},"entrypoint":{"const":"main.go"},"signature":{"const":"func main()"}}}
  ]
}`)

const problemTestsSystemPrompt = `You are the tests agent for CodeGym.
Return only the tests artifact requested by the supplied schema. The supplied
problem spec is authoritative and is your entire problem context.

For unit strategy:
- Produce 4..12 deterministic typed cases with a roughly even split and at least
  two public and two hidden cases. Every case includes name, kind, hidden, args,
  expected, and an optional short rationale or comparator override.
- The top-level comparator is fixed by the spec. When it is checker, include a
  safe checker implementation matching the selected language contract.
For http strategy:
- Produce 4..12 deterministic request/expect cases with at least two public and
  two hidden. Each case includes name, kind, hidden, request, expect, and an
  optional rationale or comparator override.
- request.body is JSON; expect.headers is a subset; expect.json and expect.body
  are mutually exclusive. Do not use checker.

Never write an implementation, starter code, runner, shell command,
CODEGYM_RESULT, filesystem access, subprocess, socket, or external network call.`

const problemReferenceSystemPrompt = `You are the reference implementation agent for CodeGym.
Return only the reference artifact requested by the supplied schema. The supplied
problem spec is authoritative and is your entire problem context.

For Python unit, reference_solution contains only the target function and safe
helpers. For Go unit, it is a complete package main source file defining the
exact specified function. For Go http, reference_solution and starter_code are
complete package main files; both read PORT and start a net/http server, while
starter_code leaves the exercise behavior as clear TODOs without embedding
expected answers.

Never write a runner, shell command, CODEGYM_RESULT, hidden-test import,
filesystem access, os/exec, unsafe, cgo, or external network call.`

var unitTestsJSONSchema = json.RawMessage(`{
  "type":"object",
  "required":["test_cases"],
  "properties":{
    "checker":{"type":"string"},
    "test_cases":{"type":"array","minItems":4,"maxItems":12,"items":{"type":"object","required":["name","kind","hidden","args","expected"],"properties":{"name":{"type":"string"},"kind":{"type":"string","enum":["example","functional","edge","stress","hidden"]},"hidden":{"type":"boolean"},"rationale":{"type":"string","maxLength":300},"args":{"type":"array"},"expected":{},"comparator":{"type":"object","required":["kind"],"properties":{"kind":{"type":"string","enum":["exact","set","multiset","sorted","float","checker"]},"epsilon":{"type":"number","exclusiveMinimum":0}}}}}}
  }
}`)

var checkerUnitTestsJSONSchema = json.RawMessage(`{
  "type":"object",
  "required":["checker","test_cases"],
  "properties":{
    "checker":{"type":"string"},
    "test_cases":{"type":"array","minItems":4,"maxItems":12,"items":{"type":"object","required":["name","kind","hidden","args","expected"],"properties":{"name":{"type":"string"},"kind":{"type":"string","enum":["example","functional","edge","stress","hidden"]},"hidden":{"type":"boolean"},"rationale":{"type":"string","maxLength":300},"args":{"type":"array"},"expected":{},"comparator":{"type":"object","required":["kind"],"properties":{"kind":{"type":"string","enum":["exact","set","multiset","sorted","float","checker"]},"epsilon":{"type":"number","exclusiveMinimum":0}}}}}}
  }
}`)

var httpTestsJSONSchema = json.RawMessage(`{
  "type":"object",
  "required":["http_test_cases"],
  "properties":{
    "http_test_cases":{"type":"array","minItems":4,"maxItems":12,"items":{"type":"object","required":["name","kind","hidden","request","expect"],"properties":{"name":{"type":"string"},"kind":{"type":"string","enum":["example","functional","edge","stress","hidden"]},"hidden":{"type":"boolean"},"rationale":{"type":"string","maxLength":300},"request":{"type":"object","required":["method","path"],"properties":{"method":{"type":"string"},"path":{"type":"string"},"headers":{"type":"object","additionalProperties":{"type":"string"}},"body":{}}},"expect":{"type":"object","required":["status"],"properties":{"status":{"type":"integer","minimum":100,"maximum":599},"json":{},"headers":{"type":"object","additionalProperties":{"type":"string"}},"body":{"type":"string"}}},"comparator":{"type":"object","required":["kind"],"properties":{"kind":{"type":"string","enum":["exact","set","multiset","sorted","float"]},"epsilon":{"type":"number","exclusiveMinimum":0}}}}}}
  }
}`)

var unitReferenceJSONSchema = json.RawMessage(`{
  "type":"object",
  "required":["reference_solution"],
  "properties":{"reference_solution":{"type":"string"}}
}`)

var httpReferenceJSONSchema = json.RawMessage(`{
  "type":"object",
  "required":["reference_solution","starter_code"],
  "properties":{"reference_solution":{"type":"string"},"starter_code":{"type":"string"}}
}`)

const problemReconcileSystemPrompt = problemReferenceSystemPrompt + `

The blind reference and tests artifacts disagreed when executed. Treat that as
evidence that the shared spec exposed an ambiguity or that one artifact did not
faithfully implement an explicit clause. You now receive the current tests,
current reference artifact, and execution result only for bounded reconciliation.

Return the complete corrected reference and tests artifacts. You may correct
your reference, the tests, or both, but the shared spec remains authoritative:
- edited_artifact must be exactly "reference", "tests", or "both" and must
  truthfully match the fields you changed.
- reason must name the controlling spec clause and explain why the edit is correct.
- Do not weaken an assertion merely to obtain agreement. Do not invent a new
  requirement absent from the spec. Preserve unchanged artifact fields exactly.`

var unitReconcileJSONSchema = json.RawMessage(`{
  "type":"object",
  "required":["edited_artifact","reason","reference_solution","test_cases"],
  "properties":{
    "edited_artifact":{"type":"string","enum":["reference","tests","both"]},
    "reason":{"type":"string"},
    "reference_solution":{"type":"string"},
    "checker":{"type":"string"},
    "test_cases":{"type":"array","minItems":4,"maxItems":12,"items":{"type":"object","required":["name","kind","hidden","args","expected"],"properties":{"name":{"type":"string"},"kind":{"type":"string","enum":["example","functional","edge","stress","hidden"]},"hidden":{"type":"boolean"},"rationale":{"type":"string","maxLength":300},"args":{"type":"array"},"expected":{},"comparator":{"type":"object","required":["kind"],"properties":{"kind":{"type":"string","enum":["exact","set","multiset","sorted","float","checker"]},"epsilon":{"type":"number","exclusiveMinimum":0}}}}}}
  }
}`)

var checkerUnitReconcileJSONSchema = json.RawMessage(`{
  "type":"object",
  "required":["edited_artifact","reason","reference_solution","checker","test_cases"],
  "properties":{
    "edited_artifact":{"type":"string","enum":["reference","tests","both"]},
    "reason":{"type":"string"},
    "reference_solution":{"type":"string"},
    "checker":{"type":"string"},
    "test_cases":{"type":"array","minItems":4,"maxItems":12,"items":{"type":"object","required":["name","kind","hidden","args","expected"],"properties":{"name":{"type":"string"},"kind":{"type":"string","enum":["example","functional","edge","stress","hidden"]},"hidden":{"type":"boolean"},"rationale":{"type":"string","maxLength":300},"args":{"type":"array"},"expected":{},"comparator":{"type":"object","required":["kind"],"properties":{"kind":{"type":"string","enum":["exact","set","multiset","sorted","float","checker"]},"epsilon":{"type":"number","exclusiveMinimum":0}}}}}}
  }
}`)

var httpReconcileJSONSchema = json.RawMessage(`{
  "type":"object",
  "required":["edited_artifact","reason","reference_solution","starter_code","http_test_cases"],
  "properties":{
    "edited_artifact":{"type":"string","enum":["reference","tests","both"]},
    "reason":{"type":"string"},
    "reference_solution":{"type":"string"},
    "starter_code":{"type":"string"},
    "http_test_cases":{"type":"array","minItems":4,"maxItems":12,"items":{"type":"object","required":["name","kind","hidden","request","expect"],"properties":{"name":{"type":"string"},"kind":{"type":"string","enum":["example","functional","edge","stress","hidden"]},"hidden":{"type":"boolean"},"rationale":{"type":"string","maxLength":300},"request":{"type":"object","required":["method","path"],"properties":{"method":{"type":"string"},"path":{"type":"string"},"headers":{"type":"object","additionalProperties":{"type":"string"}},"body":{}}},"expect":{"type":"object","required":["status"],"properties":{"status":{"type":"integer","minimum":100,"maximum":599},"json":{},"headers":{"type":"object","additionalProperties":{"type":"string"}},"body":{"type":"string"}}},"comparator":{"type":"object","required":["kind"],"properties":{"kind":{"type":"string","enum":["exact","set","multiset","sorted","float"]},"epsilon":{"type":"number","exclusiveMinimum":0}}}}}}
  }
}`)

// GenerateProblemSpec makes the first, spec-only call in the independent
// generation pipeline. Invalid structured output gets one bounded correction,
// matching the other structured generation surfaces.
func GenerateProblemSpec(ctx context.Context, orchestrator *Orchestrator, request ProblemSpec) (GeneratedProblemSpec, GenerateResult, error) {
	request, err := NormalizeProblemSpec(request)
	if err != nil {
		return GeneratedProblemSpec{}, GenerateResult{}, err
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return GeneratedProblemSpec{}, GenerateResult{}, fmt.Errorf("encode requested problem: %w", err)
	}
	instructions := problemSpecSystemPrompt
	var lastErr error
	var lastRaw string
	for attempt := 1; attempt <= problemMaxAttempts; attempt++ {
		result, generateErr := orchestrator.Generate(ctx, GenerateInput{
			Kind:          KindProblemSpec,
			Spec:          payload,
			Schema:        Schema{Name: "problem_spec", Version: "1", JSONSchema: problemSpecJSONSchema},
			ModelPolicy:   ModelPolicy{MaxTokens: problemSpecMaxTokens},
			Instructions:  instructions,
			IntakeContext: request.IntakeContext,
		})
		if generateErr != nil {
			return GeneratedProblemSpec{}, GenerateResult{}, generateErr
		}
		generated, validateErr := ValidateGeneratedProblemSpecForLanguage(result.Object, request.Language)
		if validateErr == nil {
			return generated, result, nil
		}
		lastErr = validateErr
		lastRaw = string(result.Object)
		instructions = problemSpecSystemPrompt + "\n\nYour previous spec was rejected: " + validateErr.Error() + ". Return a complete corrected spec."
	}
	return GeneratedProblemSpec{}, GenerateResult{}, &InvalidOutputError{
		Reason:    "problem spec generation produced invalid output after 2 attempts",
		RawOutput: lastRaw,
		Err:       lastErr,
	}
}

// GenerateIndependentProblem creates the contract first, then starts the tests
// and reference calls concurrently. Each parallel request is constructed only
// from the validated contract, so neither artifact can enter the other role's
// model context.
func GenerateIndependentProblem(ctx context.Context, orchestrator *Orchestrator, request ProblemSpec) (problems.Definition, GeneratedProblemSpec, GeneratedProblem, GenerateResult, error) {
	contract, specResult, err := GenerateProblemSpec(ctx, orchestrator, request)
	if err != nil {
		return problems.Definition{}, GeneratedProblemSpec{}, GeneratedProblem{}, GenerateResult{}, err
	}
	contractJSON, err := json.Marshal(contract)
	if err != nil {
		return problems.Definition{}, GeneratedProblemSpec{}, GeneratedProblem{}, GenerateResult{}, fmt.Errorf("encode generated problem spec: %w", err)
	}

	testsSchema := unitTestsJSONSchema
	if contract.Comparator.Kind == problems.ComparatorChecker {
		testsSchema = checkerUnitTestsJSONSchema
	}
	if contract.Strategy == problems.TestStrategyHTTP {
		testsSchema = httpTestsJSONSchema
	}
	referenceSchema := unitReferenceJSONSchema
	if contract.Strategy == problems.TestStrategyHTTP {
		referenceSchema = httpReferenceJSONSchema
	}

	type artifactCall struct {
		role   string
		result GenerateResult
		err    error
	}
	results := make(chan artifactCall, 2)
	var group sync.WaitGroup
	group.Add(2)
	go func() {
		defer group.Done()
		result, callErr := orchestrator.Generate(ctx, GenerateInput{
			Kind:         KindProblemTests,
			Spec:         contractJSON,
			Schema:       Schema{Name: "problem_tests", Version: "1", JSONSchema: testsSchema},
			ModelPolicy:  ModelPolicy{MaxTokens: problemArtifactMaxTokens},
			Instructions: problemTestsSystemPrompt,
		})
		results <- artifactCall{role: "tests", result: result, err: callErr}
	}()
	go func() {
		defer group.Done()
		result, callErr := orchestrator.Generate(ctx, GenerateInput{
			Kind:         KindProblemReference,
			Spec:         contractJSON,
			Schema:       Schema{Name: "problem_reference", Version: "1", JSONSchema: referenceSchema},
			ModelPolicy:  ModelPolicy{MaxTokens: problemArtifactMaxTokens},
			Instructions: problemReferenceSystemPrompt,
		})
		results <- artifactCall{role: "reference", result: result, err: callErr}
	}()
	group.Wait()
	close(results)

	var testsResult, referenceResult GenerateResult
	for result := range results {
		if result.err != nil {
			return problems.Definition{}, GeneratedProblemSpec{}, GeneratedProblem{}, GenerateResult{}, fmt.Errorf("generate %s artifact: %w", result.role, result.err)
		}
		switch result.role {
		case "tests":
			testsResult = result.result
		case "reference":
			referenceResult = result.result
		}
	}

	generated, err := AssembleIndependentProblem(contract, testsResult.Object, referenceResult.Object)
	if err != nil {
		return problems.Definition{}, GeneratedProblemSpec{}, GeneratedProblem{}, GenerateResult{}, err
	}
	definition, err := BuildProblemDefinition(generated)
	if err != nil {
		return problems.Definition{}, GeneratedProblemSpec{}, GeneratedProblem{}, GenerateResult{}, err
	}
	return definition, contract, generated, aggregateProblemResults(specResult, testsResult, referenceResult, generated), nil
}

// AssembleIndependentProblem is the single join point for the isolated
// artifacts. Strict decoders reject attempts by either role to smuggle fields
// owned by the other role into its output.
func AssembleIndependentProblem(contract GeneratedProblemSpec, testsRaw, referenceRaw json.RawMessage) (GeneratedProblem, error) {
	var tests GeneratedProblemTests
	if err := strictProblemArtifactDecode(testsRaw, &tests); err != nil {
		return GeneratedProblem{}, &InvalidOutputError{Reason: "tests agent returned an invalid artifact", RawOutput: string(testsRaw), Err: err}
	}
	var reference GeneratedProblemReference
	if err := strictProblemArtifactDecode(referenceRaw, &reference); err != nil {
		return GeneratedProblem{}, &InvalidOutputError{Reason: "reference agent returned an invalid artifact", RawOutput: string(referenceRaw), Err: err}
	}
	var testFields, referenceFields map[string]json.RawMessage
	if err := json.Unmarshal(testsRaw, &testFields); err != nil {
		return GeneratedProblem{}, err
	}
	if err := json.Unmarshal(referenceRaw, &referenceFields); err != nil {
		return GeneratedProblem{}, err
	}
	if _, ok := referenceFields["reference_solution"]; !ok {
		return GeneratedProblem{}, errors.New("reference artifact must include reference_solution")
	}
	if contract.Strategy == problems.TestStrategyHTTP {
		if _, ok := testFields["http_test_cases"]; !ok {
			return GeneratedProblem{}, errors.New("http tests artifact must include http_test_cases")
		}
		if _, ok := referenceFields["starter_code"]; !ok {
			return GeneratedProblem{}, errors.New("http reference artifact must include starter_code")
		}
		if len(tests.TestCases) > 0 || tests.Checker != "" {
			return GeneratedProblem{}, errors.New("http tests artifact must not include unit cases or checker")
		}
	} else {
		if _, ok := testFields["test_cases"]; !ok {
			return GeneratedProblem{}, errors.New("unit tests artifact must include test_cases")
		}
		if len(tests.HTTPTestCases) > 0 {
			return GeneratedProblem{}, errors.New("unit tests artifact must not include http_test_cases")
		}
		if reference.StarterCode != "" {
			return GeneratedProblem{}, errors.New("unit reference artifact must not include starter_code")
		}
	}

	combined := GeneratedProblem{
		Language:          contract.Language,
		Strategy:          contract.Strategy,
		Title:             contract.Title,
		Description:       contract.Description,
		Category:          contract.Category,
		Subcategory:       contract.Subcategory,
		Tags:              append([]string(nil), contract.Tags...),
		Difficulty:        contract.Difficulty,
		EstimatedMinutes:  contract.EstimatedMinutes,
		FunctionName:      contract.FunctionName,
		Parameters:        append([]ProblemParameter(nil), contract.Parameters...),
		ReturnType:        contract.ReturnType,
		Hints:             append([]string(nil), contract.Hints...),
		ReferenceSolution: reference.ReferenceSolution,
		Comparator:        contract.Comparator,
		Checker:           tests.Checker,
		TestCases:         tests.TestCases,
		StarterCode:       reference.StarterCode,
		HTTPTestCases:     tests.HTTPTestCases,
	}
	if contract.Strategy == problems.TestStrategyHTTP {
		combined.Entrypoint = contract.Entrypoint
	}
	combinedRaw, err := json.Marshal(combined)
	if err != nil {
		return GeneratedProblem{}, err
	}
	validated, err := ValidateGeneratedProblemForLanguage(combinedRaw, contract.Language)
	if err != nil {
		return GeneratedProblem{}, &InvalidOutputError{Reason: "independent artifacts do not satisfy the problem spec", RawOutput: string(combinedRaw), Err: err}
	}
	validated.Specification = &contract
	return validated, nil
}

// ReconcileProblemArtifacts gives the reference role the test context only
// after a blind execution disagreement. It verifies the claimed edit
// attribution against the actual structured diff before returning.
func ReconcileProblemArtifacts(
	ctx context.Context,
	orchestrator *Orchestrator,
	contract GeneratedProblemSpec,
	current GeneratedProblem,
	executionResult any,
	round int,
) (GeneratedProblem, ReconciliationRecord, error) {
	tests := GeneratedProblemTests{
		Checker:       current.Checker,
		TestCases:     current.TestCases,
		HTTPTestCases: current.HTTPTestCases,
	}
	reference := GeneratedProblemReference{
		ReferenceSolution: current.ReferenceSolution,
		StarterCode:       current.StarterCode,
	}
	payload, err := json.Marshal(map[string]any{
		"round":             round,
		"spec":              contract,
		"current_tests":     tests,
		"current_reference": reference,
		"execution_result":  executionResult,
	})
	if err != nil {
		return GeneratedProblem{}, ReconciliationRecord{}, fmt.Errorf("encode reconciliation context: %w", err)
	}
	schema := unitReconcileJSONSchema
	if contract.Comparator.Kind == problems.ComparatorChecker {
		schema = checkerUnitReconcileJSONSchema
	}
	if contract.Strategy == problems.TestStrategyHTTP {
		schema = httpReconcileJSONSchema
	}
	result, err := orchestrator.Generate(ctx, GenerateInput{
		Kind:         KindProblemReference,
		Spec:         payload,
		Schema:       Schema{Name: "problem_reconciliation", Version: "1", JSONSchema: schema},
		ModelPolicy:  ModelPolicy{MaxTokens: problemArtifactMaxTokens},
		Instructions: problemReconcileSystemPrompt,
	})
	if err != nil {
		return GeneratedProblem{}, ReconciliationRecord{}, err
	}
	var reconciled reconciledProblemArtifacts
	if err := strictProblemArtifactDecode(result.Object, &reconciled); err != nil {
		return GeneratedProblem{}, ReconciliationRecord{}, &InvalidOutputError{
			Reason: "reconciliation returned an invalid artifact", RawOutput: string(result.Object), Err: err,
		}
	}
	reconciled.EditedArtifact = strings.ToLower(strings.TrimSpace(reconciled.EditedArtifact))
	reconciled.Reason = strings.TrimSpace(reconciled.Reason)
	if reconciled.Reason == "" {
		return GeneratedProblem{}, ReconciliationRecord{}, errors.New("reconciliation reason must not be empty")
	}

	testsRaw, err := json.Marshal(GeneratedProblemTests{
		Checker:       reconciled.Checker,
		TestCases:     reconciled.TestCases,
		HTTPTestCases: reconciled.HTTPTestCases,
	})
	if err != nil {
		return GeneratedProblem{}, ReconciliationRecord{}, err
	}
	referenceRaw, err := json.Marshal(GeneratedProblemReference{
		ReferenceSolution: reconciled.ReferenceSolution,
		StarterCode:       reconciled.StarterCode,
	})
	if err != nil {
		return GeneratedProblem{}, ReconciliationRecord{}, err
	}
	next, err := AssembleIndependentProblem(contract, testsRaw, referenceRaw)
	if err != nil {
		return GeneratedProblem{}, ReconciliationRecord{}, err
	}
	referenceChanged := current.ReferenceSolution != next.ReferenceSolution || current.StarterCode != next.StarterCode
	testsChanged := current.Checker != next.Checker || !reflect.DeepEqual(current.TestCases, next.TestCases) || !reflect.DeepEqual(current.HTTPTestCases, next.HTTPTestCases)
	actual := ""
	switch {
	case referenceChanged && testsChanged:
		actual = "both"
	case referenceChanged:
		actual = "reference"
	case testsChanged:
		actual = "tests"
	default:
		return GeneratedProblem{}, ReconciliationRecord{}, errors.New("reconciliation did not change either artifact")
	}
	if reconciled.EditedArtifact != actual {
		return GeneratedProblem{}, ReconciliationRecord{}, fmt.Errorf("reconciliation claimed edited_artifact %q but changed %q", reconciled.EditedArtifact, actual)
	}
	record := ReconciliationRecord{Round: round, EditedArtifact: actual, Reason: reconciled.Reason}
	next.Reconciliations = append(append([]ReconciliationRecord(nil), current.Reconciliations...), record)
	return next, record, nil
}

func strictProblemArtifactDecode(raw json.RawMessage, output any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(output); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("artifact must contain exactly one JSON object")
	}
	return nil
}

func aggregateProblemResults(spec, tests, reference GenerateResult, generated GeneratedProblem) GenerateResult {
	provider := spec.Provider
	model := spec.Model
	if tests.Provider != provider || reference.Provider != provider {
		provider = "mixed"
	}
	if tests.Model != model || reference.Model != model {
		model = "mixed"
	}
	object, _ := json.Marshal(generated)
	return GenerateResult{
		Object:    object,
		Provider:  provider,
		Model:     model,
		TokensIn:  spec.TokensIn + tests.TokensIn + reference.TokensIn,
		TokensOut: spec.TokensOut + tests.TokensOut + reference.TokensOut,
		CostUnits: spec.CostUnits + tests.CostUnits + reference.CostUnits,
	}
}

// ValidateGeneratedProblemSpecForLanguage validates the generated alignment
// contract and rejects implicit defaults for its load-bearing fields.
func ValidateGeneratedProblemSpecForLanguage(raw json.RawMessage, requestedLanguage string) (GeneratedProblemSpec, error) {
	strategy, err := problemStrategyFor(requestedLanguage)
	if err != nil {
		return GeneratedProblemSpec{}, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return GeneratedProblemSpec{}, fmt.Errorf("output is not a problem spec object: %w", err)
	}
	for _, required := range []string{"signature", "comparator", "ambiguity_resolutions"} {
		if _, ok := fields[required]; !ok {
			return GeneratedProblemSpec{}, fmt.Errorf("problem spec must explicitly include %s", required)
		}
	}
	var comparatorFields map[string]json.RawMessage
	if err := json.Unmarshal(fields["comparator"], &comparatorFields); err != nil {
		return GeneratedProblemSpec{}, errors.New("problem spec comparator must be an object")
	}
	if _, ok := comparatorFields["kind"]; !ok {
		return GeneratedProblemSpec{}, errors.New("problem spec comparator must explicitly include kind")
	}

	var output GeneratedProblemSpec
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&output); err != nil {
		return output, fmt.Errorf("output is not a problem spec object: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return output, errors.New("output must contain exactly one problem spec object")
	}

	output.Language = strings.ToLower(strings.TrimSpace(output.Language))
	output.Strategy = problems.TestStrategy(strings.ToLower(strings.TrimSpace(string(output.Strategy))))
	output.Title = strings.TrimSpace(output.Title)
	output.Description = strings.TrimSpace(output.Description)
	output.Category = strings.TrimSpace(output.Category)
	output.Subcategory = strings.TrimSpace(output.Subcategory)
	output.Entrypoint = strings.TrimSpace(output.Entrypoint)
	output.Signature = strings.TrimSpace(output.Signature)
	output.IOContract = strings.TrimSpace(output.IOContract)
	output.FunctionName = strings.TrimSpace(output.FunctionName)
	output.ReturnType = strings.TrimSpace(output.ReturnType)
	if output.Language != strategy.language {
		return output, fmt.Errorf("generated language %q does not match requested language %q", output.Language, strategy.language)
	}
	if output.Title == "" || output.Description == "" || output.Category == "" || output.Entrypoint == "" || output.Signature == "" || output.IOContract == "" {
		return output, errors.New("title, description, category, entrypoint, signature, and io_contract are required")
	}
	if len(output.Title) > 120 || len(output.Description) > 8000 || len(output.Category) > 80 || len(output.Subcategory) > 120 || len(output.IOContract) > 8000 {
		return output, errors.New("problem spec text exceeds allowed length")
	}
	if len(output.Tags) < 1 || len(output.Tags) > 8 {
		return output, errors.New("tags must contain 1..8 entries")
	}
	for _, tag := range output.Tags {
		if strings.TrimSpace(tag) == "" || len(tag) > 40 {
			return output, errors.New("tags must be non-empty and at most 40 characters")
		}
	}
	if output.Difficulty < 1 || output.Difficulty > 3 || output.EstimatedMinutes < 10 || output.EstimatedMinutes > 90 {
		return output, errors.New("difficulty must be 1..3 and estimated_minutes must be 10..90")
	}
	if len(output.Hints) < 1 || len(output.Hints) > 3 {
		return output, errors.New("hints must contain 1..3 entries")
	}
	for _, hint := range output.Hints {
		if strings.TrimSpace(hint) == "" {
			return output, errors.New("hints must not be empty")
		}
	}
	if len(output.AmbiguityResolutions) == 0 {
		return output, errors.New("ambiguity_resolutions must not be empty")
	}
	for index, resolution := range output.AmbiguityResolutions {
		resolution = strings.TrimSpace(resolution)
		if resolution == "" {
			return output, fmt.Errorf("ambiguity resolution %d must not be empty", index+1)
		}
		output.AmbiguityResolutions[index] = resolution
	}
	comparator, err := problems.NormalizeComparator(output.Comparator)
	if err != nil {
		return output, err
	}
	output.Comparator = comparator

	switch output.Strategy {
	case problems.TestStrategyUnit:
		if output.FunctionName == "" || output.Entrypoint != output.FunctionName {
			return output, errors.New("unit spec entrypoint must equal function_name")
		}
		if !strategy.validIdentifier(output.FunctionName) || len(output.Parameters) < 1 || len(output.Parameters) > 5 || !strategy.validType(output.ReturnType) {
			return output, errors.New("unit spec requires a valid function_name, 1..5 parameters, and return_type")
		}
		seen := make(map[string]bool, len(output.Parameters))
		for _, parameter := range output.Parameters {
			if !strategy.validIdentifier(parameter.Name) || seen[parameter.Name] || !strategy.validType(parameter.Type) {
				return output, errors.New("unit spec parameters must have unique valid names and supported types")
			}
			seen[parameter.Name] = true
		}
		expected := canonicalProblemSignature(output)
		if output.Signature != expected {
			return output, fmt.Errorf("unit spec signature must be exactly %q", expected)
		}
	case problems.TestStrategyHTTP:
		if strategy.language != "go" {
			return output, errors.New("http problem generation is currently supported only for Go")
		}
		if output.Entrypoint != "main.go" || output.Signature != "func main()" {
			return output, errors.New(`http spec requires entrypoint "main.go" and signature "func main()"`)
		}
		if output.FunctionName != "" || len(output.Parameters) != 0 || output.ReturnType != "" {
			return output, errors.New("http spec must not include unit function fields")
		}
		if output.Comparator.Kind == problems.ComparatorChecker {
			return output, errors.New("checker comparator is unsupported for http problems")
		}
	default:
		return output, fmt.Errorf("test strategy must be unit or http, got %q", output.Strategy)
	}
	return output, nil
}

func canonicalProblemSignature(spec GeneratedProblemSpec) string {
	parameters := make([]string, 0, len(spec.Parameters))
	for _, parameter := range spec.Parameters {
		if spec.Language == "python" {
			parameters = append(parameters, parameter.Name+": "+parameter.Type)
		} else {
			parameters = append(parameters, parameter.Name+" "+parameter.Type)
		}
	}
	if spec.Language == "python" {
		return fmt.Sprintf("def %s(%s) -> %s", spec.FunctionName, strings.Join(parameters, ", "), spec.ReturnType)
	}
	return fmt.Sprintf("func %s(%s) %s", spec.FunctionName, strings.Join(parameters, ", "), spec.ReturnType)
}

// ProblemSpecFromGenerated supplies a compatibility contract for server-side
// packages assembled before independent generation existed. New generation
// always carries Specification and does not use this fallback.
func ProblemSpecFromGenerated(generated GeneratedProblem) GeneratedProblemSpec {
	strategy := generated.Strategy
	if strategy == "" {
		strategy = problems.TestStrategyUnit
	}
	contract := GeneratedProblemSpec{
		Language:             generated.Language,
		Strategy:             strategy,
		Title:                generated.Title,
		Description:          generated.Description,
		Category:             generated.Category,
		Subcategory:          generated.Subcategory,
		Tags:                 append([]string(nil), generated.Tags...),
		Difficulty:           generated.Difficulty,
		EstimatedMinutes:     generated.EstimatedMinutes,
		Hints:                append([]string(nil), generated.Hints...),
		IOContract:           generated.Description,
		Comparator:           generated.Comparator,
		AmbiguityResolutions: []string{"Follow the explicit behavior in the problem description."},
		FunctionName:         generated.FunctionName,
		Parameters:           append([]ProblemParameter(nil), generated.Parameters...),
		ReturnType:           generated.ReturnType,
	}
	if strategy == problems.TestStrategyHTTP {
		contract.Entrypoint = generated.Entrypoint
		contract.Signature = "func main()"
	} else {
		contract.Entrypoint = generated.FunctionName
		contract.Signature = canonicalProblemSignature(contract)
	}
	return contract
}
