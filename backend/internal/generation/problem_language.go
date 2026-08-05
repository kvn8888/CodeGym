package generation

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/kvn8888/codegym/backend/internal/problems"
)

const goProblemSystemPrompt = `You generate one safe Go coding problem for CodeGym.
Return one JSON object matching the supplied schema. Do not return markdown.

Rules:
- Choose a focused problem that follows the learner's prompt and memory profile. Demonstrated evidence outranks the self-reported baseline.
- difficulty is 1, 2, or 3. estimated_minutes is 10..90.
- Include 1..3 progressive hints.
- comparator is part of the problem specification. Use exact (the default), set, multiset, sorted, float with an optional positive finite epsilon (default 1e-6), or checker. A test case may override the problem comparator.

For strategy "unit" (the default when omitted):
- function_name and parameter names are valid Go identifiers and are not keywords.
- Allowed parameter and return types are int, float64, bool, string, slices of those, nested slices, and maps whose keys are string or int. Use Go syntax such as []int, [][]float64, map[string]int, or map[int][]string.
- Linked lists, trees, graphs, pointers, interfaces, structs, functions, channels, and cyclic structures are unsupported.
- Include 4..12 deterministic JSON test_cases. Each args array has exactly one value per parameter and expected matches return_type.
- Every test case includes kind (example, functional, edge, stress, or hidden), hidden, and an optional short rationale. Hidden is authoritative for pre-submit exposure. Produce a roughly even split with at least 2 public and 2 hidden cases.
- Use checker when several structurally different answers are valid. Then checker must contain a complete Go source file in package main defining func check(args []any, actual, expected any) (bool, string).
- reference_solution is a complete package main source file defining function_name with the exact typed signature.
- Never produce a test runner, hidden-test imports, shell commands, CODEGYM_RESULT, os/exec, net, filesystem access, unsafe, cgo, or network access.

For strategy "http":
- Use only the Go standard library and net/http. checker is unsupported.
- entrypoint is "main.go". starter_code and reference_solution are complete package main source files whose main reads PORT from the environment and starts an HTTP server.
- starter_code must compile and expose the described routes, leaving the exercise behavior as clear TODOs without embedding hidden expectations.
- The description must state the response schema concretely for every route, including the JSON type of every response field. Do not leave identifier, number, boolean, array, object, or null types implicit.
- The description must include at least one worked request/response example showing the request method, path, body when applicable, response status, and response body.
- Include 4..12 deterministic http_test_cases with a roughly even split of at least 2 public and 2 hidden cases. Each case has name, kind, hidden, an optional short rationale, request {method,path,headers?,body?}, expect {status,json?,headers?,body?}, and an optional comparator override.
- request.body is JSON. expect.headers is a subset. expect.json and expect.body are mutually exclusive.
- Never produce a test runner, hidden-test imports, fixed ports, shell commands, CODEGYM_RESULT, os/exec, unsafe, cgo, or external network access. CodeGym builds and owns the HTTP harness.`

var goProblemJSONSchema = json.RawMessage(`{
  "type":"object",
  "required":["title","description","category","subcategory","tags","difficulty","estimated_minutes","hints","reference_solution"],
  "properties":{
    "language":{"type":"string","enum":["go"]},
    "strategy":{"type":"string","enum":["unit","http"]},
    "title":{"type":"string"},
    "description":{"type":"string"},
    "category":{"type":"string"},
    "subcategory":{"type":"string"},
    "tags":{"type":"array","items":{"type":"string"}},
    "difficulty":{"type":"integer","minimum":1,"maximum":3},
    "estimated_minutes":{"type":"integer","minimum":10,"maximum":90},
    "hints":{"type":"array","minItems":1,"maxItems":3,"items":{"type":"string"}},
    "reference_solution":{"type":"string"},
    "comparator":{"type":"object","required":["kind"],"properties":{"kind":{"type":"string","enum":["exact","set","multiset","sorted","float","checker"]},"epsilon":{"type":"number","exclusiveMinimum":0}}},
    "function_name":{"type":"string"},
    "parameters":{"type":"array","minItems":1,"maxItems":5,"items":{"type":"object","required":["name","type"]}},
    "return_type":{"type":"string"},
    "checker":{"type":"string"},
    "test_cases":{"type":"array","minItems":4,"maxItems":12,"items":{"type":"object","required":["name","kind","hidden","args","expected"],"properties":{"name":{"type":"string"},"kind":{"type":"string","enum":["example","functional","edge","stress","hidden"]},"hidden":{"type":"boolean"},"rationale":{"type":"string","maxLength":300},"args":{"type":"array"},"expected":{},"comparator":{"type":"object","required":["kind"],"properties":{"kind":{"type":"string","enum":["exact","set","multiset","sorted","float","checker"]},"epsilon":{"type":"number","exclusiveMinimum":0}}}}}},
    "entrypoint":{"type":"string"},
    "starter_code":{"type":"string"},
    "http_test_cases":{"type":"array","minItems":4,"maxItems":12,"items":{"type":"object","required":["name","kind","hidden","request","expect"],"properties":{"name":{"type":"string"},"kind":{"type":"string","enum":["example","functional","edge","stress","hidden"]},"hidden":{"type":"boolean"},"rationale":{"type":"string","maxLength":300},"request":{"type":"object","required":["method","path"],"properties":{"method":{"type":"string"},"path":{"type":"string"},"headers":{"type":"object","additionalProperties":{"type":"string"}},"body":{}}},"expect":{"type":"object","required":["status"],"properties":{"status":{"type":"integer","minimum":100,"maximum":599},"json":{},"headers":{"type":"object","additionalProperties":{"type":"string"}},"body":{"type":"string"}}},"comparator":{"type":"object","required":["kind"],"properties":{"kind":{"type":"string","enum":["exact","set","multiset","sorted","float"]},"epsilon":{"type":"number","exclusiveMinimum":0}}}}}}
  },
  "oneOf":[
    {"required":["function_name","parameters","return_type","test_cases"]},
    {"required":["strategy","entrypoint","starter_code","http_test_cases"],"properties":{"strategy":{"const":"http"}}}
  ]
}`)

type problemLanguageStrategy struct {
	language           string
	systemPrompt       string
	jsonSchema         json.RawMessage
	validIdentifier    func(string) bool
	validType          func(string) bool
	validJSONType      func(json.RawMessage, string) bool
	referenceDefines   func(string, string) bool
	checkerDefines     func(string) bool
	checkerDescription string
	build              func(GeneratedProblem) (problems.Definition, error)
}

func problemStrategyFor(language string) (problemLanguageStrategy, error) {
	language = strings.ToLower(strings.TrimSpace(language))
	if language == "" {
		language = "python"
	}
	switch language {
	case "python":
		return problemLanguageStrategy{
			language: "python", systemPrompt: pythonProblemSystemPrompt, jsonSchema: problemJSONSchema,
			validIdentifier: func(value string) bool { return pythonIdentifier.MatchString(value) },
			validType:       func(value string) bool { return allowedPythonProblemTypes[value] },
			validJSONType:   validPythonProblemJSONType,
			referenceDefines: func(source, functionName string) bool {
				return strings.Contains(source, "def "+functionName+"(")
			},
			checkerDefines:     func(source string) bool { return strings.Contains(source, "def check(") },
			checkerDescription: "checker to define check(args, actual, expected)",
			build:              buildPythonProblemDefinition,
		}, nil
	case "go":
		return problemLanguageStrategy{
			language: "go", systemPrompt: goProblemSystemPrompt, jsonSchema: goProblemJSONSchema,
			validIdentifier: validGoIdentifier,
			validType: func(value string) bool {
				_, err := parseGoProblemType(value, 0)
				return err == nil
			},
			validJSONType: validGoProblemJSONType,
			referenceDefines: func(source, functionName string) bool {
				return strings.Contains(source, "package main") && strings.Contains(source, "func "+functionName+"(")
			},
			checkerDefines: func(source string) bool {
				return strings.Contains(source, "package main") && strings.Contains(source, "func check(")
			},
			checkerDescription: "a package main file defining func check(args []any, actual, expected any) (bool, string)",
			build:              buildGoProblemDefinition,
		}, nil
	default:
		return problemLanguageStrategy{}, fmt.Errorf("language must be python or go, got %q", language)
	}
}

var (
	goIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	goKeywords   = map[string]struct{}{
		"break": {}, "default": {}, "func": {}, "interface": {}, "select": {},
		"case": {}, "defer": {}, "go": {}, "map": {}, "struct": {}, "chan": {},
		"else": {}, "goto": {}, "package": {}, "switch": {}, "const": {}, "fallthrough": {},
		"if": {}, "range": {}, "type": {}, "continue": {}, "for": {}, "import": {}, "return": {}, "var": {},
	}
)

func validGoIdentifier(value string) bool {
	if !goIdentifier.MatchString(value) {
		return false
	}
	_, keyword := goKeywords[value]
	return !keyword
}

type goProblemType struct {
	kind string
	key  string
	elem *goProblemType
}

func parseGoProblemType(source string, depth int) (goProblemType, error) {
	if depth > 8 {
		return goProblemType{}, errors.New("type nesting exceeds 8 levels")
	}
	source = strings.TrimSpace(source)
	switch source {
	case "int", "float64", "bool", "string":
		return goProblemType{kind: source}, nil
	}
	if strings.HasPrefix(source, "[]") {
		element, err := parseGoProblemType(source[2:], depth+1)
		if err != nil {
			return goProblemType{}, err
		}
		return goProblemType{kind: "slice", elem: &element}, nil
	}
	if strings.HasPrefix(source, "map[") {
		end := strings.IndexByte(source, ']')
		if end < 0 {
			return goProblemType{}, errors.New("map type is missing closing bracket")
		}
		key := source[len("map["):end]
		if key != "string" && key != "int" {
			return goProblemType{}, errors.New("map key must be string or int")
		}
		element, err := parseGoProblemType(source[end+1:], depth+1)
		if err != nil {
			return goProblemType{}, err
		}
		return goProblemType{kind: "map", key: key, elem: &element}, nil
	}
	return goProblemType{}, fmt.Errorf("unsupported Go type %q", source)
}

func validGoProblemJSONType(raw json.RawMessage, expected string) bool {
	parsedType, err := parseGoProblemType(expected, 0)
	if err != nil {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return false
	}
	return validGoJSONValue(value, parsedType)
}

func validGoJSONValue(value any, expected goProblemType) bool {
	switch expected.kind {
	case "int":
		number, ok := value.(json.Number)
		if !ok {
			return false
		}
		_, err := strconv.ParseInt(number.String(), 10, 64)
		return err == nil
	case "float64":
		number, ok := value.(json.Number)
		if !ok {
			return false
		}
		_, err := strconv.ParseFloat(number.String(), 64)
		return err == nil
	case "bool":
		_, ok := value.(bool)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "slice":
		values, ok := value.([]any)
		if !ok {
			return false
		}
		for _, item := range values {
			if !validGoJSONValue(item, *expected.elem) {
				return false
			}
		}
		return true
	case "map":
		values, ok := value.(map[string]any)
		if !ok {
			return false
		}
		for key, item := range values {
			if expected.key == "int" {
				if _, err := strconv.ParseInt(key, 10, 64); err != nil {
					return false
				}
			}
			if !validGoJSONValue(item, *expected.elem) {
				return false
			}
		}
		return true
	default:
		return false
	}
}
