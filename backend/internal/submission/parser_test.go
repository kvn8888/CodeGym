package submission

import (
	"strings"
	"testing"
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
