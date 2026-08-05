package generation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/kvn8888/codegym/backend/internal/problems"
)

const problemSpecMaxTokens = 4096

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
