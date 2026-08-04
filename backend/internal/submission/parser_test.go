package submission

import (
	"strings"
	"testing"

	"github.com/kvn8888/codegym/backend/internal/execution"
)

func TestParseTestResultUsesLastStructuredLine(t *testing.T) {
	output := strings.Join([]string{
		"user output",
		`CODEGYM_RESULT {"tests":[{"name":"stale","status":"fail","duration_ms":1,"error":"ignore"}],"compile_error":null}`,
		`CODEGYM_RESULT {"tests":[{"name":"basic","status":"pass","duration_ms":4,"error":null},{"name":"duplicates","status":"fail","duration_ms":7,"error":"expected [0, 1], got []"}],"compile_error":null}`,
	}, "\n")

	result, err := ParseTestResult(output)
	if err != nil {
		t.Fatalf("ParseTestResult: %v", err)
	}
	if result.Status != "fail" || result.Total != 2 || result.Passed != 1 || result.Failed != 1 {
		t.Fatalf("result = %#v", result)
	}
	if result.DurationMs != 11 || result.TestCases[1].Error == nil {
		t.Fatalf("result details = %#v", result)
	}
}

func TestParseTestResultRejectsMalformedLine(t *testing.T) {
	if _, err := ParseTestResult("CODEGYM_RESULT {not-json}"); err == nil {
		t.Fatal("expected malformed result line to fail")
	}
}

func TestParseTestResultRejectsEmptyResult(t *testing.T) {
	if _, err := ParseTestResult(`CODEGYM_RESULT {"tests":[],"compile_error":null}`); err == nil {
		t.Fatal("expected empty result to fail")
	}
}

func TestParseTestResultCompileError(t *testing.T) {
	result, err := ParseTestResult(
		`CODEGYM_RESULT {"tests":[],"compile_error":"SyntaxError: invalid syntax"}`,
	)
	if err != nil {
		t.Fatalf("ParseTestResult: %v", err)
	}
	if result.Status != "fail" || result.CompileError == nil ||
		*result.CompileError != "SyntaxError: invalid syntax" {
		t.Fatalf("result = %#v", result)
	}
}

func TestTestResultFromJudge(t *testing.T) {
	compileError := "SyntaxError: invalid syntax"
	tests := []struct {
		name   string
		judge  execution.JudgeResult
		status string
		total  int
	}{
		{
			name: "cases",
			judge: execution.JudgeResult{Schema: execution.JudgeSchema, Status: execution.JudgeStatusFailed, DurationMs: 9, Cases: []execution.CaseResult{
				{Name: "one", Status: "pass", DurationMs: 2},
				{Name: "two", Status: "fail", DurationMs: 3},
			}},
			status: "fail", total: 2,
		},
		{
			name: "compile error",
			judge: execution.JudgeResult{Schema: execution.JudgeSchema, Status: execution.JudgeStatusFailed,
				Cases: []execution.CaseResult{}, CompileError: &compileError},
			status: "fail",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := TestResultFromJudge(test.judge)
			if err != nil {
				t.Fatalf("TestResultFromJudge: %v", err)
			}
			if result.Status != test.status || result.Total != test.total {
				t.Fatalf("result = %#v", result)
			}
		})
	}
}
