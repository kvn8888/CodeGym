package generation

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/kvn8888/codegym/backend/internal/execution"
	"github.com/kvn8888/codegym/backend/internal/problems"
)

const (
	problemMaxAttempts      = 2
	problemDefaultMaxTokens = 6144
)

var pythonIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ProblemSpec is the caller-controlled part of a generated coding problem.
// IntakeContext is resolved by the server and never trusted from JSON.
type ProblemSpec struct {
	Topic         string                 `json:"topic"`
	Prompt        string                 `json:"prompt,omitempty"`
	Difficulty    string                 `json:"difficulty,omitempty"`
	Language      string                 `json:"language,omitempty"`
	IntakeContext *PracticeIntakeContext `json:"-"`
}

type GeneratedProblem struct {
	Language          string                `json:"language,omitempty"`
	Strategy          problems.TestStrategy `json:"strategy,omitempty"`
	Title             string                `json:"title"`
	Description       string                `json:"description"`
	Category          string                `json:"category"`
	Subcategory       string                `json:"subcategory"`
	Tags              []string              `json:"tags"`
	Difficulty        int                   `json:"difficulty"`
	EstimatedMinutes  int                   `json:"estimated_minutes"`
	FunctionName      string                `json:"function_name"`
	Parameters        []ProblemParameter    `json:"parameters"`
	ReturnType        string                `json:"return_type"`
	Hints             []string              `json:"hints"`
	ReferenceSolution string                `json:"reference_solution"`
	Comparator        problems.Comparator   `json:"comparator,omitempty"`
	Checker           string                `json:"checker,omitempty"`
	TestCases         []ProblemTestCase     `json:"test_cases"`
	Entrypoint        string                `json:"entrypoint,omitempty"`
	StarterCode       string                `json:"starter_code,omitempty"`
	HTTPTestCases     []problems.HTTPCase   `json:"http_test_cases,omitempty"`
}

type ProblemParameter struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type ProblemTestCase = problems.UnitCase

const pythonProblemSystemPrompt = `You generate one safe Python coding problem for CodeGym.
Return one JSON object matching the supplied schema. Do not return markdown.

Rules:
- Choose a focused problem that follows the learner's prompt and memory profile. Demonstrated evidence outranks the self-reported baseline.
- difficulty is 1, 2, or 3. estimated_minutes is 10..90.
- function_name and parameter names are valid Python identifiers.
- Allowed parameter and return types: int, str, bool, list[int], list[str].
- Include 1..3 progressive hints.
- Include 4..12 deterministic test cases. Each args array has exactly one value per parameter and expected matches return_type.
- comparator is part of the problem specification. Use exact (the default), set, multiset, sorted, float with an optional positive finite epsilon (default 1e-6), or checker. A test case may override the problem comparator.
- Use checker when several structurally different answers are valid. Then checker must contain a Python function check(args, actual, expected) returning bool or (bool, reason). CodeGym places it only in the hidden test file.
- reference_solution contains only the Python function definition and helpers it needs. It must define function_name.
- Never produce a test runner, imports of hidden tests, shell commands, CODEGYM_RESULT, eval, exec, open, subprocess, socket, or network/file access. CodeGym builds the hidden runner itself.`

var problemJSONSchema = json.RawMessage(`{
  "type":"object",
  "required":["title","description","category","subcategory","tags","difficulty","estimated_minutes","function_name","parameters","return_type","hints","reference_solution","test_cases"],
  "properties":{
    "title":{"type":"string"},
    "description":{"type":"string"},
    "category":{"type":"string"},
    "subcategory":{"type":"string"},
    "tags":{"type":"array","items":{"type":"string"}},
    "difficulty":{"type":"integer","minimum":1,"maximum":3},
    "estimated_minutes":{"type":"integer","minimum":10,"maximum":90},
    "function_name":{"type":"string"},
    "parameters":{"type":"array","minItems":1,"maxItems":5,"items":{"type":"object","required":["name","type"]}},
    "return_type":{"type":"string"},
    "hints":{"type":"array","minItems":1,"maxItems":3,"items":{"type":"string"}},
    "reference_solution":{"type":"string"},
    "comparator":{"type":"object","required":["kind"],"properties":{"kind":{"type":"string","enum":["exact","set","multiset","sorted","float","checker"]},"epsilon":{"type":"number","exclusiveMinimum":0}}},
    "checker":{"type":"string"},
    "test_cases":{"type":"array","minItems":4,"maxItems":12,"items":{"type":"object","required":["name","args","expected"],"properties":{"name":{"type":"string"},"args":{"type":"array"},"expected":{},"comparator":{"type":"object","required":["kind"],"properties":{"kind":{"type":"string","enum":["exact","set","multiset","sorted","float","checker"]},"epsilon":{"type":"number","exclusiveMinimum":0}}}}}}
  }
}`)

var allowedPythonProblemTypes = map[string]bool{
	"int": true, "str": true, "bool": true, "list[int]": true, "list[str]": true,
}

func NormalizeProblemSpec(spec ProblemSpec) (ProblemSpec, error) {
	spec.Topic = strings.TrimSpace(spec.Topic)
	spec.Prompt = strings.TrimSpace(spec.Prompt)
	if spec.Topic == "" {
		spec.Topic = spec.Prompt
	}
	if len(spec.Prompt) > 500 {
		spec.Prompt = spec.Prompt[:500]
	}
	spec.Difficulty = strings.ToLower(strings.TrimSpace(spec.Difficulty))
	switch spec.Difficulty {
	case "", "easy", "medium", "hard":
	default:
		return spec, errors.New("difficulty must be easy, medium, or hard")
	}
	spec.Language = strings.ToLower(strings.TrimSpace(spec.Language))
	if spec.Language == "" {
		spec.Language = "python"
	}
	if _, err := problemStrategyFor(spec.Language); err != nil {
		return spec, err
	}
	return spec, nil
}

func ValidateGeneratedProblem(raw json.RawMessage) (GeneratedProblem, error) {
	return ValidateGeneratedProblemForLanguage(raw, "python")
}

// ValidateGeneratedProblemForLanguage validates the model payload against the
// selected language's identifier, type, source, and JSON-mapping rules.
func ValidateGeneratedProblemForLanguage(raw json.RawMessage, language string) (GeneratedProblem, error) {
	strategy, err := problemStrategyFor(language)
	if err != nil {
		return GeneratedProblem{}, err
	}
	var output GeneratedProblem
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&output); err != nil {
		return output, fmt.Errorf("output is not a problem object: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return output, errors.New("output must contain exactly one problem object")
	}
	output.Title = strings.TrimSpace(output.Title)
	output.Description = strings.TrimSpace(output.Description)
	output.Category = strings.TrimSpace(output.Category)
	output.Subcategory = strings.TrimSpace(output.Subcategory)
	output.FunctionName = strings.TrimSpace(output.FunctionName)
	output.ReturnType = strings.TrimSpace(output.ReturnType)
	output.ReferenceSolution = strings.TrimSpace(output.ReferenceSolution)
	output.Checker = strings.TrimSpace(output.Checker)
	output.Entrypoint = strings.TrimSpace(output.Entrypoint)
	output.StarterCode = strings.TrimSpace(output.StarterCode)
	output.Strategy = problems.TestStrategy(strings.ToLower(strings.TrimSpace(string(output.Strategy))))
	if output.Strategy == "" {
		output.Strategy = problems.TestStrategyUnit
	}
	if output.Language == "" {
		output.Language = strategy.language
	} else {
		output.Language = strings.ToLower(strings.TrimSpace(output.Language))
		if output.Language != strategy.language {
			return output, fmt.Errorf("generated language %q does not match requested language %q", output.Language, strategy.language)
		}
	}
	if output.Title == "" || output.Description == "" || output.Category == "" {
		return output, errors.New("title, description, and category are required")
	}
	if len(output.Title) > 120 || len(output.Description) > 8000 ||
		len(output.Category) > 80 || len(output.Subcategory) > 120 {
		return output, errors.New("problem text exceeds allowed length")
	}
	if len(output.Tags) == 0 || len(output.Tags) > 8 {
		return output, errors.New("tags must contain 1..8 entries")
	}
	for _, tag := range output.Tags {
		if strings.TrimSpace(tag) == "" || len(tag) > 40 {
			return output, errors.New("tags must be non-empty and at most 40 characters")
		}
	}
	if output.Difficulty < 1 || output.Difficulty > 3 {
		return output, errors.New("difficulty must be 1..3")
	}
	if output.EstimatedMinutes < 10 || output.EstimatedMinutes > 90 {
		return output, errors.New("estimated_minutes must be 10..90")
	}
	if len(output.Hints) < 1 || len(output.Hints) > 3 {
		return output, errors.New("hints must contain 1..3 entries")
	}
	for _, hint := range output.Hints {
		if strings.TrimSpace(hint) == "" {
			return output, errors.New("hints must not be empty")
		}
	}
	if output.Strategy == problems.TestStrategyHTTP {
		if strategy.language != "go" {
			return output, errors.New("http problem generation is currently supported only for Go")
		}
		return validateGeneratedHTTPProblem(output)
	}
	if output.Strategy != problems.TestStrategyUnit {
		return output, fmt.Errorf("test strategy must be unit or http, got %q", output.Strategy)
	}
	if output.Entrypoint != "" || output.StarterCode != "" || len(output.HTTPTestCases) > 0 {
		return output, errors.New("unit problems must not include HTTP entrypoint, starter_code, or http_test_cases")
	}
	if !strategy.validIdentifier(output.FunctionName) {
		return output, fmt.Errorf("function_name is not a valid %s identifier", strategy.language)
	}
	if len(output.Parameters) < 1 || len(output.Parameters) > 5 {
		return output, errors.New("parameters must contain 1..5 entries")
	}
	seen := map[string]bool{}
	for _, parameter := range output.Parameters {
		if !strategy.validIdentifier(parameter.Name) || seen[parameter.Name] {
			return output, fmt.Errorf("parameter names must be unique %s identifiers", strategy.language)
		}
		if !strategy.validType(parameter.Type) {
			return output, fmt.Errorf("unsupported parameter type %q", parameter.Type)
		}
		seen[parameter.Name] = true
	}
	if !strategy.validType(output.ReturnType) {
		return output, fmt.Errorf("unsupported return type %q", output.ReturnType)
	}
	if len(output.TestCases) < 4 || len(output.TestCases) > 12 {
		return output, errors.New("test_cases must contain 4..12 entries")
	}
	comparator, err := problems.NormalizeComparator(output.Comparator)
	if err != nil {
		return output, err
	}
	output.Comparator = comparator
	usesChecker := comparator.Kind == problems.ComparatorChecker
	for index, test := range output.TestCases {
		if strings.TrimSpace(test.Name) == "" || len(test.Name) > 80 || len(test.Args) != len(output.Parameters) || len(test.Expected) == 0 || !json.Valid(test.Expected) {
			return output, fmt.Errorf("test case %d has invalid name, args, or expected value", index+1)
		}
		for argumentIndex, arg := range test.Args {
			if len(arg) == 0 || !json.Valid(arg) ||
				!strategy.validJSONType(arg, output.Parameters[argumentIndex].Type) {
				return output, fmt.Errorf("test case %d argument %d does not match %s", index+1, argumentIndex+1, output.Parameters[argumentIndex].Type)
			}
		}
		if !strategy.validJSONType(test.Expected, output.ReturnType) {
			return output, fmt.Errorf("test case %d expected value does not match %s", index+1, output.ReturnType)
		}
		resolved, err := problems.ResolveComparator(output.Comparator, test.Comparator)
		if err != nil {
			return output, fmt.Errorf("test case %d: %w", index+1, err)
		}
		if test.Comparator != nil {
			normalized := resolved
			output.TestCases[index].Comparator = &normalized
		}
		usesChecker = usesChecker || resolved.Kind == problems.ComparatorChecker
	}
	lowerSolution := strings.ToLower(output.ReferenceSolution)
	if !strategy.referenceDefines(output.ReferenceSolution, output.FunctionName) {
		return output, errors.New("reference_solution does not define function_name")
	}
	if len(output.ReferenceSolution) > 12000 {
		return output, errors.New("reference_solution exceeds allowed length")
	}
	for _, forbidden := range []string{"codegym_result", "subprocess", "socket", "open(", "eval(", "exec(", "__import__"} {
		if strings.Contains(lowerSolution, forbidden) {
			return output, fmt.Errorf("reference_solution contains forbidden token %q", forbidden)
		}
	}
	if usesChecker {
		if !strategy.checkerDefines(output.Checker) {
			return output, fmt.Errorf("checker comparator requires %s", strategy.checkerDescription)
		}
		if len(output.Checker) > 12000 {
			return output, errors.New("checker exceeds allowed length")
		}
		lowerChecker := strings.ToLower(output.Checker)
		for _, forbidden := range []string{"codegym_result", "subprocess", "socket", "open(", "eval(", "exec(", "__import__"} {
			if strings.Contains(lowerChecker, forbidden) {
				return output, fmt.Errorf("checker contains forbidden token %q", forbidden)
			}
		}
	} else if output.Checker != "" {
		return output, errors.New("checker source is only allowed when a checker comparator is used")
	}
	return output, nil
}

func validPythonProblemJSONType(raw json.RawMessage, expected string) bool {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return false
	}
	switch expected {
	case "int":
		number, ok := value.(float64)
		return ok && number == float64(int64(number))
	case "str":
		_, ok := value.(string)
		return ok
	case "bool":
		_, ok := value.(bool)
		return ok
	case "list[int]":
		values, ok := value.([]any)
		if !ok {
			return false
		}
		for _, item := range values {
			number, ok := item.(float64)
			if !ok || number != float64(int64(number)) {
				return false
			}
		}
		return true
	case "list[str]":
		values, ok := value.([]any)
		if !ok {
			return false
		}
		for _, item := range values {
			if _, ok := item.(string); !ok {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func GenerateProblem(ctx context.Context, orchestrator *Orchestrator, spec ProblemSpec) (problems.Definition, GeneratedProblem, GenerateResult, error) {
	spec, err := NormalizeProblemSpec(spec)
	if err != nil {
		return problems.Definition{}, GeneratedProblem{}, GenerateResult{}, err
	}
	specJSON, err := json.Marshal(spec)
	if err != nil {
		return problems.Definition{}, GeneratedProblem{}, GenerateResult{}, fmt.Errorf("encode problem spec: %w", err)
	}
	strategy, err := problemStrategyFor(spec.Language)
	if err != nil {
		return problems.Definition{}, GeneratedProblem{}, GenerateResult{}, err
	}
	instructions := strategy.systemPrompt
	var lastErr error
	var lastRaw string
	for attempt := 1; attempt <= problemMaxAttempts; attempt++ {
		result, generateErr := orchestrator.Generate(ctx, GenerateInput{
			Kind:          KindProblem,
			Spec:          specJSON,
			Schema:        Schema{Name: "generated_problem", Version: "1", JSONSchema: strategy.jsonSchema},
			ModelPolicy:   ModelPolicy{MaxTokens: problemDefaultMaxTokens},
			Instructions:  instructions,
			IntakeContext: spec.IntakeContext,
		})
		if generateErr != nil {
			return problems.Definition{}, GeneratedProblem{}, GenerateResult{}, generateErr
		}
		output, validateErr := ValidateGeneratedProblemForLanguage(result.Object, spec.Language)
		if validateErr == nil {
			definition, buildErr := BuildProblemDefinition(output)
			return definition, output, result, buildErr
		}
		lastErr, lastRaw = validateErr, string(result.Object)
		instructions = strategy.systemPrompt + "\n\nYour previous output was rejected: " + validateErr.Error() + ". Return a complete corrected object."
	}
	return problems.Definition{}, GeneratedProblem{}, GenerateResult{}, &InvalidOutputError{
		Reason:    "problem generation produced invalid output after 2 attempts",
		RawOutput: lastRaw,
		Err:       lastErr,
	}
}

// BuildProblemDefinition turns a validated model payload into the catalog
// Definition (skeleton + out-of-band verdict harness). Exported for verification repairs.
func BuildProblemDefinition(output GeneratedProblem) (problems.Definition, error) {
	strategy, err := problemStrategyFor(output.Language)
	if err != nil {
		return problems.Definition{}, err
	}
	output.Language = strategy.language
	return strategy.build(output)
}

func buildPythonProblemDefinition(output GeneratedProblem) (problems.Definition, error) {
	comparator, err := problems.NormalizeComparator(output.Comparator)
	if err != nil {
		return problems.Definition{}, err
	}
	output.Comparator = comparator
	type runnerCase struct {
		Name       string              `json:"name"`
		Args       []json.RawMessage   `json:"args"`
		Expected   json.RawMessage     `json:"expected"`
		Comparator problems.Comparator `json:"comparator"`
	}
	runnerCases := make([]runnerCase, 0, len(output.TestCases))
	for index, test := range output.TestCases {
		name := strings.TrimSpace(test.Name)
		if name == "" {
			name = fmt.Sprintf("case-%d", index+1)
		}
		comparator, err := problems.ResolveComparator(output.Comparator, test.Comparator)
		if err != nil {
			return problems.Definition{}, fmt.Errorf("case %q comparator: %w", name, err)
		}
		runnerCases = append(runnerCases, runnerCase{Name: name, Args: test.Args, Expected: test.Expected, Comparator: comparator})
	}
	casesJSON, err := json.Marshal(runnerCases)
	if err != nil {
		return problems.Definition{}, err
	}
	encodedCases := base64.StdEncoding.EncodeToString(casesJSON)
	parameters := make([]string, 0, len(output.Parameters))
	for _, parameter := range output.Parameters {
		parameters = append(parameters, parameter.Name+": "+parameter.Type)
	}
	hints := make([]problems.Hint, 0, len(output.Hints))
	for index, hint := range output.Hints {
		hints = append(hints, problems.Hint{Cost: index, Text: strings.TrimSpace(hint)})
	}
	skeleton := fmt.Sprintf("def %s(%s) -> %s:\n    \"\"\"Implement the solution described in the problem.\"\"\"\n    # TODO: implement this function.\n    raise NotImplementedError\n",
		output.FunctionName, strings.Join(parameters, ", "), output.ReturnType)
	// Named, incrementally flushed cases let the out-of-process supervisor
	// attribute timeout/OOM/crash deaths before the final verdict exists.
	// Expected/got remains in failure text so verification can adjudicate.
	runner := fmt.Sprintf(`import base64
import importlib
import json
import pathlib
import signal
import time

from codegym_comparator import compare_values

%s

CASES = json.loads(base64.b64decode(%q).decode("utf-8"))
PROTOCOL_DIR = pathlib.Path(".codegym")
CASES_PATH = PROTOCOL_DIR / "cases.jsonl"
VERDICT_PATH = PROTOCOL_DIR / "verdict.json"
CASE_TIMEOUT_SECONDS = 5

def append_event(event):
    with CASES_PATH.open("a", encoding="utf-8") as stream:
        stream.write(json.dumps(event, separators=(",", ":")) + "\n")
        stream.flush()

def write_verdict(status, cases, compile_error=None):
    VERDICT_PATH.write_text(json.dumps(
        {"schema": 1, "status": status, "compile_error": compile_error, "cases": cases},
        separators=(",", ":"),
    ), encoding="utf-8")

PROTOCOL_DIR.mkdir(exist_ok=True)
try:
    solution = importlib.import_module("solution")
    target = getattr(solution, %q)
except Exception as exc:
    write_verdict("failed", [], f"{type(exc).__name__}: {exc}")
    raise SystemExit(0)

signal.signal(signal.SIGALRM, signal.SIG_DFL)
results = []
for index, case in enumerate(CASES):
    name = case.get("name") or f"case-{index + 1}"
    append_event({"event": "case_start", "name": name})
    started = time.perf_counter()
    error = None
    status = "pass"
    signal.setitimer(signal.ITIMER_REAL, CASE_TIMEOUT_SECONDS)
    try:
        actual = target(*case["args"])
        comparator = case["comparator"]
        reason = None
        if comparator["kind"] == "checker":
            checked = check(case["args"], actual, case["expected"])
            if isinstance(checked, tuple):
                if len(checked) != 2 or not isinstance(checked[0], bool) or (checked[1] is not None and not isinstance(checked[1], str)):
                    raise TypeError("check must return bool or (bool, reason)")
                equal, reason = checked
            elif isinstance(checked, bool):
                equal = checked
            else:
                raise TypeError("check must return bool or (bool, reason)")
        else:
            equal = compare_values(comparator["kind"], case["expected"], actual, comparator.get("epsilon"))
        if not equal:
            status = "fail"
            error = reason or f"expected {json.dumps(case['expected'], separators=(',', ':'))}, got {json.dumps(actual, separators=(',', ':'))}"
    except MemoryError:
        raise
    except Exception as exc:
        status = "fail"
        error = f"{type(exc).__name__}: {exc}"
    finally:
        signal.setitimer(signal.ITIMER_REAL, 0)
    result = {"name": name, "status": status, "duration_ms": max(0, int((time.perf_counter() - started) * 1000)), "error": error}
    results.append(result)
    append_event({"event": "case_result", **result})

write_verdict("passed" if all(case["status"] == "pass" for case in results) else "failed", results)
`, output.Checker, encodedCases, output.FunctionName)
	return problems.Definition{
		Problem: problems.Problem{
			Summary: problems.Summary{
				Title: output.Title, Category: output.Category, Language: "python",
				Difficulty: output.Difficulty, Tags: output.Tags,
				EstimatedMinutes: output.EstimatedMinutes, Type: "coding",
			},
			Version: "1.0.0", Description: output.Description, Subcategory: output.Subcategory,
			Runtime:    problems.Runtime{Image: "python312", TimeoutSeconds: 30, MemoryMB: 256, NetworkMode: "block-all"},
			Files:      problems.FileManifest{Skeleton: []problems.FileRef{{Path: "solution.py", Entry: true}}},
			TestConfig: problems.TestConfig{Strategy: "unit", Comparator: output.Comparator}, Hints: hints,
		},
		SkeletonFiles: []problems.File{{Path: "solution.py", Content: skeleton}},
		HiddenTestFiles: []problems.File{
			{Path: "test_solution.py", Content: runner},
			{Path: "codegym_comparator.py", Content: execution.PythonComparatorSource},
		},
		ReferenceSolution: output.ReferenceSolution + "\n",
		Entrypoint:        "test_solution.py",
	}, nil
}
