package generation

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/kvn8888/codegym/backend/internal/harness"
)

func validTestGenerationSpec() TestGenerationSpec {
	return TestGenerationSpec{
		ProblemID:   "two-sum-generated",
		Language:    harness.LanguagePython,
		Module:      "solution",
		EntryPoint:  "two_sum",
		ParamNames:  []string{"nums", "target"},
		ProblemSpec: json.RawMessage(`{"io_contract":{"nums":"list[int]","target":"int"}}`),
		Description: "Return indices whose values sum to target.",
	}
}

func validTestSuiteJSON() json.RawMessage {
	cases := make([]TestCase, 0, testGenMinCases)
	for index := 0; index < testGenMinCases; index++ {
		cases = append(cases, TestCase{
			Name:       "case-" + string(rune('a'+index)),
			Kind:       TestCaseFunctional,
			Hidden:     index%2 == 0,
			Input:      json.RawMessage(`{"nums":[2,7,11,15],"target":9}`),
			Expected:   json.RawMessage(`[0,1]`),
			Rationale:  "covers a deterministic pair",
			Comparator: harness.ComparatorUnorderedList,
		})
	}
	payload, _ := json.Marshal(TestSuite{
		ProblemID: "two-sum-generated",
		Strategy:  TestStrategyUnit,
		Cases:     cases,
	})
	return payload
}

func TestGenerateTestSuiteHappyPath(t *testing.T) {
	generator := &scriptedGenerator{payloads: []json.RawMessage{validTestSuiteJSON()}}

	suite, rendered, result, err := GenerateTestSuite(
		scopedContext(),
		newTestOrchestrator(generator),
		validTestGenerationSpec(),
	)
	if err != nil {
		t.Fatalf("GenerateTestSuite: %v", err)
	}
	if len(suite.Cases) != testGenMinCases || result.Provider != "scripted" {
		t.Fatalf("suite=%#v result=%#v", suite, result)
	}
	if rendered.Path != "test_solution.py" {
		t.Fatalf("rendered = %#v", rendered)
	}
	if len(generator.requests) != 1 {
		t.Fatalf("requests = %d", len(generator.requests))
	}
	request := generator.requests[0]
	if request.Kind != KindTests || request.Schema.Name != "generated_test_suite" {
		t.Fatalf("request = %#v", request)
	}
	if !strings.Contains(strings.ToLower(request.Instructions), "never emit comparator code") ||
		!strings.Contains(request.Instructions, string(harness.ComparatorUnorderedList)) {
		t.Fatalf("instructions = %q", request.Instructions)
	}
}

func TestGenerateTestSuiteRetriesOnceOnInvalidOutput(t *testing.T) {
	generator := &scriptedGenerator{payloads: []json.RawMessage{
		json.RawMessage(`{"problem_id":"wrong","strategy":"unit","cases":[]}`),
		validTestSuiteJSON(),
	}}

	suite, _, _, err := GenerateTestSuite(
		scopedContext(),
		newTestOrchestrator(generator),
		validTestGenerationSpec(),
	)
	if err != nil {
		t.Fatalf("GenerateTestSuite: %v", err)
	}
	if len(suite.Cases) != testGenMinCases || len(generator.requests) != 2 {
		t.Fatalf("cases=%d requests=%d", len(suite.Cases), len(generator.requests))
	}
	if !strings.Contains(generator.requests[1].Instructions, "previous output was rejected") {
		t.Fatal("retry instructions omit validation feedback")
	}
}

func TestGenerateTestSuiteFailsAfterRetryBudget(t *testing.T) {
	generator := &scriptedGenerator{payloads: []json.RawMessage{json.RawMessage(`{}`)}}
	_, _, _, err := GenerateTestSuite(
		scopedContext(),
		newTestOrchestrator(generator),
		validTestGenerationSpec(),
	)
	if err == nil || len(generator.requests) != testGenMaxAttempts {
		t.Fatalf("err=%v requests=%d", err, len(generator.requests))
	}
	if DiagnosticClass(err) != "invalid_output" {
		t.Fatalf("DiagnosticClass = %q, err=%v", DiagnosticClass(err), err)
	}
}

func TestGenerateTestSuiteSurfacesProviderErrorsWithoutRetry(t *testing.T) {
	providerErr := &ProviderError{StatusCode: http.StatusBadRequest, Message: "bad model"}
	generator := &scriptedGenerator{
		payloads: []json.RawMessage{json.RawMessage(`{}`)},
		errs:     []error{providerErr},
	}
	_, _, _, err := GenerateTestSuite(
		scopedContext(),
		newTestOrchestrator(generator),
		validTestGenerationSpec(),
	)
	if !errors.Is(err, providerErr) || len(generator.requests) != 1 {
		t.Fatalf("err=%v requests=%d", err, len(generator.requests))
	}
}

func TestGenerateTestSuiteRejectsOutOfRangeCaseCount(t *testing.T) {
	var suite TestSuite
	if err := json.Unmarshal(validTestSuiteJSON(), &suite); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	suite.Cases = suite.Cases[:testGenMinCases-1]
	raw, _ := json.Marshal(suite)
	if _, err := ValidateTestSuite(raw, "two-sum-generated", []string{"nums", "target"}); err == nil {
		t.Fatal("expected out-of-range suite to fail")
	}
}

func TestValidateTestSuite(t *testing.T) {
	valid := string(validTestSuiteJSON())
	cases := []struct {
		name string
		raw  string
	}{
		{"duplicate names", strings.Replace(valid, `"case-b"`, `"case-a"`, 1)},
		{"unknown kind", strings.Replace(valid, `"functional"`, `"mystery"`, 1)},
		{"stdin stdout unimplemented", strings.Replace(valid, `"strategy":"unit"`, `"strategy":"stdin_stdout"`, 1)},
		{"unknown comparator", strings.Replace(valid, `"unordered_list"`, `"execute_code"`, 1)},
		{"wrong problem id", strings.Replace(valid, `"two-sum-generated"`, `"other-problem"`, 1)},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ValidateTestSuite(
				json.RawMessage(test.raw),
				"two-sum-generated",
				[]string{"nums", "target"},
			); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestGenerateTestSuiteRejectsModelSuppliedComparatorCode(t *testing.T) {
	injected := strings.Replace(
		string(validTestSuiteJSON()),
		`"unordered_list"`,
		`"lambda a,e: __import__('os').system('id')"`,
		1,
	)
	generator := &scriptedGenerator{payloads: []json.RawMessage{
		json.RawMessage(injected),
		json.RawMessage(injected),
	}}
	_, _, _, err := GenerateTestSuite(
		scopedContext(),
		newTestOrchestrator(generator),
		validTestGenerationSpec(),
	)
	if err == nil || DiagnosticClass(err) != "invalid_output" || len(generator.requests) != 2 {
		t.Fatalf("err=%v class=%q requests=%d", err, DiagnosticClass(err), len(generator.requests))
	}
}

func TestGenerateTestSuiteReturnsRunnableHarness(t *testing.T) {
	generator := &scriptedGenerator{payloads: []json.RawMessage{validTestSuiteJSON()}}
	suite, rendered, _, err := GenerateTestSuite(
		scopedContext(),
		newTestOrchestrator(generator),
		validTestGenerationSpec(),
	)
	if err != nil {
		t.Fatalf("GenerateTestSuite: %v", err)
	}
	if len(suite.Cases) != testGenMinCases {
		t.Fatalf("cases = %d", len(suite.Cases))
	}
	if rendered.Path != "test_solution.py" ||
		!strings.Contains(rendered.Content, `PREFIX = "CODEGYM_RESULT "`) {
		t.Fatalf("rendered harness = %#v", rendered)
	}
}
