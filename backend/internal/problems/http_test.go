package problems

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizeHTTPCasesResolvesComparatorsAndHeaders(t *testing.T) {
	epsilon := 0.01
	body := "created"
	config, cases, err := NormalizeHTTPCases(TestConfig{
		Strategy:   TestStrategyHTTP,
		Comparator: Comparator{Kind: ComparatorSorted},
	}, []HTTPCase{
		{
			CaseMetadata: CaseMetadata{Kind: CaseKindExample},
			Name:         " lists-items ",
			Request:      HTTPRequest{Method: "get", Path: "/items?limit=2", Headers: map[string]string{"content-type": "application/json"}},
			Expect:       HTTPExpectation{Status: 200, JSON: json.RawMessage(`[{"id":2},{"id":1}]`), Headers: map[string]string{"x-request-id": "request-1"}},
		},
		{
			CaseMetadata: CaseMetadata{Kind: CaseKindFunctional},
			Name:         "creates-item",
			Request:      HTTPRequest{Method: "post", Path: "/items", Body: json.RawMessage(`{"name":"book"}`)},
			Expect:       HTTPExpectation{Status: 201, Body: &body},
			Comparator:   &Comparator{Kind: ComparatorFloat, Epsilon: &epsilon},
		},
	})
	if err != nil {
		t.Fatalf("NormalizeHTTPCases: %v", err)
	}
	if config.ReadinessTimeoutSeconds != DefaultHTTPReadinessTimeoutSeconds {
		t.Fatalf("readiness timeout = %d", config.ReadinessTimeoutSeconds)
	}
	if cases[0].Name != "lists-items" || cases[0].Request.Method != "GET" {
		t.Fatalf("normalized first case = %#v", cases[0])
	}
	if cases[0].Request.Headers["Content-Type"] != "application/json" || cases[0].Expect.Headers["X-Request-Id"] != "request-1" {
		t.Fatalf("normalized headers = %#v / %#v", cases[0].Request.Headers, cases[0].Expect.Headers)
	}
	if cases[0].Comparator == nil || cases[0].Comparator.Kind != ComparatorSorted {
		t.Fatalf("inherited comparator = %#v", cases[0].Comparator)
	}
	if cases[1].Comparator == nil || cases[1].Comparator.Kind != ComparatorFloat || cases[1].Comparator.Epsilon == nil || *cases[1].Comparator.Epsilon != epsilon {
		t.Fatalf("overridden comparator = %#v", cases[1].Comparator)
	}
}

func TestNormalizeHTTPCasesRejectsInvalidCases(t *testing.T) {
	valid := HTTPCase{
		CaseMetadata: CaseMetadata{Kind: CaseKindFunctional},
		Name:         "valid",
		Request:      HTTPRequest{Method: "GET", Path: "/items"},
		Expect:       HTTPExpectation{Status: 200, JSON: json.RawMessage(`[]`)},
	}
	tests := []struct {
		name   string
		mutate func(*HTTPCase)
		want   string
	}{
		{"relative path", func(testCase *HTTPCase) { testCase.Request.Path = "items" }, "absolute-path"},
		{"invalid status", func(testCase *HTTPCase) { testCase.Expect.Status = 0 }, "expect.status"},
		{"both response bodies", func(testCase *HTTPCase) { body := "[]"; testCase.Expect.Body = &body }, "mutually exclusive"},
		{"checker comparator", func(testCase *HTTPCase) { testCase.Comparator = &Comparator{Kind: ComparatorChecker} }, "do not support checker"},
		{"header injection", func(testCase *HTTPCase) { testCase.Request.Headers = map[string]string{"X-Test": "ok\r\nbad"} }, "line break"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testCase := valid
			test.mutate(&testCase)
			_, _, err := NormalizeHTTPCases(TestConfig{Strategy: TestStrategyHTTP}, []HTTPCase{testCase})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestNormalizeTestConfigKeepsUnitDefaults(t *testing.T) {
	config, err := NormalizeTestConfig(TestConfig{})
	if err != nil {
		t.Fatalf("NormalizeTestConfig: %v", err)
	}
	if config != (TestConfig{Strategy: TestStrategyUnit, Comparator: Comparator{Kind: ComparatorExact}}) {
		t.Fatalf("unit config = %#v", config)
	}
}
