package problems

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const (
	DefaultHTTPReadinessTimeoutSeconds = 10
	MaxHTTPReadinessTimeoutSeconds     = 60
	MaxHTTPCaseCount                   = 12
)

// HTTPCase is a server-owned request and expectation. It is rendered into a
// hidden harness artifact and must never be included in the public Problem.
type HTTPCase struct {
	Name       string          `json:"name"`
	Request    HTTPRequest     `json:"request"`
	Expect     HTTPExpectation `json:"expect"`
	Comparator *Comparator     `json:"comparator,omitempty"`
}

type HTTPRequest struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    json.RawMessage   `json:"body,omitempty"`
}

type HTTPExpectation struct {
	Status  int               `json:"status"`
	JSON    json.RawMessage   `json:"json,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    *string           `json:"body,omitempty"`
}

// NormalizeTestConfig validates the public strategy configuration and fills
// only strategy-specific defaults. Unit's existing zero-value behavior stays
// unchanged apart from making its implicit exact comparator explicit.
func NormalizeTestConfig(input TestConfig) (TestConfig, error) {
	input.Strategy = TestStrategy(strings.ToLower(strings.TrimSpace(string(input.Strategy))))
	if input.Strategy == "" {
		input.Strategy = TestStrategyUnit
	}
	comparator, err := NormalizeComparator(input.Comparator)
	if err != nil {
		return TestConfig{}, err
	}
	input.Comparator = comparator

	switch input.Strategy {
	case TestStrategyUnit:
		if input.ReadinessTimeoutSeconds != 0 {
			return TestConfig{}, errors.New("unit strategy does not accept readiness_timeout_seconds")
		}
	case TestStrategyHTTP:
		if input.ReadinessTimeoutSeconds == 0 {
			input.ReadinessTimeoutSeconds = DefaultHTTPReadinessTimeoutSeconds
		}
		if input.ReadinessTimeoutSeconds < 1 || input.ReadinessTimeoutSeconds > MaxHTTPReadinessTimeoutSeconds {
			return TestConfig{}, fmt.Errorf("http readiness_timeout_seconds must be 1..%d", MaxHTTPReadinessTimeoutSeconds)
		}
	default:
		return TestConfig{}, fmt.Errorf("test strategy must be unit or http, got %q", input.Strategy)
	}
	return input, nil
}

// NormalizeHTTPCases validates hidden HTTP cases, canonicalizes request data,
// and resolves each case comparator against the problem-level default.
func NormalizeHTTPCases(config TestConfig, cases []HTTPCase) (TestConfig, []HTTPCase, error) {
	config, err := NormalizeTestConfig(config)
	if err != nil {
		return TestConfig{}, nil, err
	}
	if config.Strategy != TestStrategyHTTP {
		return TestConfig{}, nil, fmt.Errorf("http cases require strategy %q", TestStrategyHTTP)
	}
	if len(cases) < 1 || len(cases) > MaxHTTPCaseCount {
		return TestConfig{}, nil, fmt.Errorf("http cases must contain 1..%d entries", MaxHTTPCaseCount)
	}

	normalized := make([]HTTPCase, len(cases))
	seenNames := make(map[string]struct{}, len(cases))
	for index, testCase := range cases {
		resolved, err := normalizeHTTPCase(config.Comparator, testCase)
		if err != nil {
			return TestConfig{}, nil, fmt.Errorf("http case %d: %w", index+1, err)
		}
		key := strings.ToLower(resolved.Name)
		if _, duplicate := seenNames[key]; duplicate {
			return TestConfig{}, nil, fmt.Errorf("http case %d: duplicate name %q", index+1, resolved.Name)
		}
		seenNames[key] = struct{}{}
		normalized[index] = resolved
	}
	return config, normalized, nil
}

func normalizeHTTPCase(problemComparator Comparator, input HTTPCase) (HTTPCase, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 80 {
		return HTTPCase{}, errors.New("name must be non-empty and at most 80 characters")
	}

	input.Request.Method = strings.ToUpper(strings.TrimSpace(input.Request.Method))
	if input.Request.Method == "" {
		return HTTPCase{}, errors.New("request.method is required")
	}
	for _, character := range input.Request.Method {
		if !isHTTPTokenCharacter(character) {
			return HTTPCase{}, fmt.Errorf("request.method %q is invalid", input.Request.Method)
		}
	}
	input.Request.Path = strings.TrimSpace(input.Request.Path)
	parsedPath, err := url.ParseRequestURI(input.Request.Path)
	if err != nil || parsedPath.IsAbs() || parsedPath.Host != "" || !strings.HasPrefix(input.Request.Path, "/") {
		return HTTPCase{}, fmt.Errorf("request.path %q must be an absolute-path reference", input.Request.Path)
	}
	input.Request.Headers, err = normalizeHTTPHeaders(input.Request.Headers)
	if err != nil {
		return HTTPCase{}, fmt.Errorf("request.headers: %w", err)
	}
	if len(input.Request.Body) > 0 && !json.Valid(input.Request.Body) {
		return HTTPCase{}, errors.New("request.body must be valid JSON")
	}

	if input.Expect.Status < 100 || input.Expect.Status > 599 {
		return HTTPCase{}, errors.New("expect.status must be 100..599")
	}
	input.Expect.Headers, err = normalizeHTTPHeaders(input.Expect.Headers)
	if err != nil {
		return HTTPCase{}, fmt.Errorf("expect.headers: %w", err)
	}
	if len(input.Expect.JSON) > 0 && !json.Valid(input.Expect.JSON) {
		return HTTPCase{}, errors.New("expect.json must be valid JSON")
	}
	if len(input.Expect.JSON) > 0 && input.Expect.Body != nil {
		return HTTPCase{}, errors.New("expect.json and expect.body are mutually exclusive")
	}

	comparator, err := ResolveComparator(problemComparator, input.Comparator)
	if err != nil {
		return HTTPCase{}, err
	}
	if comparator.Kind == ComparatorChecker {
		return HTTPCase{}, errors.New("http cases do not support checker comparators")
	}
	input.Comparator = &comparator
	return input, nil
}

func normalizeHTTPHeaders(input map[string]string) (map[string]string, error) {
	if len(input) == 0 {
		return nil, nil
	}
	normalized := make(map[string]string, len(input))
	for name, value := range input {
		name = http.CanonicalHeaderKey(strings.TrimSpace(name))
		if name == "" {
			return nil, errors.New("header name is required")
		}
		for _, character := range name {
			if !isHTTPTokenCharacter(character) {
				return nil, fmt.Errorf("header name %q is invalid", name)
			}
		}
		if strings.ContainsAny(value, "\r\n") {
			return nil, fmt.Errorf("header %q contains a line break", name)
		}
		if _, duplicate := normalized[name]; duplicate {
			return nil, fmt.Errorf("header %q is duplicated with different casing", name)
		}
		normalized[name] = value
	}
	return normalized, nil
}

func isHTTPTokenCharacter(character rune) bool {
	if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' {
		return true
	}
	return strings.ContainsRune("!#$%&'*+-.^_`|~", character)
}
