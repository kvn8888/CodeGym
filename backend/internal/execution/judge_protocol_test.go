package execution

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReconstructJudgeResult(t *testing.T) {
	compileError := "SyntaxError: invalid syntax"
	signalName := "SIGABRT"
	exitOne := 1
	cases := []struct {
		name          string
		input         ReconstructionInput
		wantStatus    JudgeStatus
		wantCases     int
		wantInDetail  string
		wantCompile   *string
		wantCaseNames []string
	}{
		{
			name:       "clean pass",
			input:      ReconstructionInput{VerdictJSON: []byte(`{"schema":1,"status":"passed","compile_error":null,"cases":[{"name":"case-1","status":"pass","duration_ms":2,"error":null}]}`)},
			wantStatus: JudgeStatusPassed, wantCases: 1,
		},
		{
			name:       "clean fail",
			input:      ReconstructionInput{VerdictJSON: []byte(`{"schema":1,"status":"failed","compile_error":null,"cases":[{"name":"case-1","status":"fail","duration_ms":3,"error":"wrong answer"}]}`)},
			wantStatus: JudgeStatusFailed, wantCases: 1,
		},
		{
			name:       "compile error",
			input:      ReconstructionInput{VerdictJSON: []byte(`{"schema":1,"status":"failed","compile_error":"SyntaxError: invalid syntax","cases":[]}`)},
			wantStatus: JudgeStatusFailed, wantCompile: &compileError,
		},
		{
			name: "timeout mid-case",
			input: ReconstructionInput{
				CasesJSONL:  []byte("{\"event\":\"case_start\",\"name\":\"case-1\"}\n{\"event\":\"case_result\",\"name\":\"case-1\",\"status\":\"pass\",\"duration_ms\":2,\"error\":null}\n{\"event\":\"case_start\",\"name\":\"case-7\"}\n"),
				DeathStatus: JudgeStatusTimeout, DurationMs: 5000,
			},
			wantStatus: JudgeStatusTimeout, wantCases: 1, wantInDetail: "case-7", wantCaseNames: []string{"case-1"},
		},
		{
			name: "crash mid-case",
			input: ReconstructionInput{
				CasesJSONL:  []byte("{\"event\":\"case_start\",\"name\":\"crash-here\"}\n"),
				DeathStatus: JudgeStatusCrashed, Signal: &signalName,
			},
			wantStatus: JudgeStatusCrashed, wantInDetail: "crash-here",
		},
		{
			name: "out of memory",
			input: ReconstructionInput{
				CasesJSONL:  []byte("{\"event\":\"case_start\",\"name\":\"large-input\"}\n"),
				DeathStatus: JudgeStatusOutOfMemory,
			},
			wantStatus: JudgeStatusOutOfMemory, wantInDetail: "large-input",
		},
		{
			name: "malformed and absent files",
			input: ReconstructionInput{
				VerdictJSON: []byte("not-json"), CasesJSONL: []byte("also-not-json\n"),
				DeathStatus: JudgeStatusCrashed, ExitCode: &exitOne,
			},
			wantStatus: JudgeStatusCrashed, wantInDetail: "malformed cases.jsonl line 1",
		},
		{
			name: "legacy CODEGYM_RESULT fallback",
			input: ReconstructionInput{LegacyOutput: strings.Join([]string{
				"debug output",
				`CODEGYM_RESULT {"tests":[{"name":"old","status":"fail","duration_ms":1,"error":"ignore"}],"compile_error":null}`,
				`CODEGYM_RESULT {"tests":[{"name":"basic","status":"pass","duration_ms":4,"error":null}],"compile_error":null}`,
			}, "\n")},
			wantStatus: JudgeStatusPassed, wantCases: 1, wantCaseNames: []string{"basic"},
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got := ReconstructJudgeResult(test.input)
			if got.Schema != JudgeSchema || got.Status != test.wantStatus {
				t.Fatalf("result = %#v, want schema %d status %s", got, JudgeSchema, test.wantStatus)
			}
			if len(got.Cases) != test.wantCases {
				t.Fatalf("cases = %#v, want %d", got.Cases, test.wantCases)
			}
			for index, name := range test.wantCaseNames {
				if got.Cases[index].Name != name {
					t.Fatalf("case %d name = %q, want %q", index, got.Cases[index].Name, name)
				}
			}
			if test.wantInDetail != "" && (got.FailureDetail == nil || !strings.Contains(*got.FailureDetail, test.wantInDetail)) {
				t.Fatalf("failure_detail = %v, want substring %q", got.FailureDetail, test.wantInDetail)
			}
			if test.wantCompile != nil && (got.CompileError == nil || *got.CompileError != *test.wantCompile) {
				t.Fatalf("compile_error = %v, want %q", got.CompileError, *test.wantCompile)
			}
		})
	}
}

func TestParseJudgeResultRejectsMalformedResults(t *testing.T) {
	tests := []string{
		`not-json`,
		`{"schema":2,"status":"passed","cases":[],"duration_ms":0}`,
		`{"schema":1,"status":"unknown","cases":[],"duration_ms":0}`,
		`{"schema":1,"status":"passed","cases":[{"name":"case","status":"maybe","duration_ms":0,"error":null}],"duration_ms":0}`,
	}
	for _, input := range tests {
		if _, err := ParseJudgeResult([]byte(input)); err == nil {
			t.Fatalf("ParseJudgeResult(%q) unexpectedly succeeded", input)
		}
	}
}

func TestJudgeResultRoundTripPreservesNulls(t *testing.T) {
	result := ReconstructJudgeResult(ReconstructionInput{DeathStatus: JudgeStatusCrashed})
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	decoded, err := ParseJudgeResult(encoded)
	if err != nil {
		t.Fatalf("ParseJudgeResult: %v", err)
	}
	if decoded.ExitCode != nil || decoded.Signal != nil || decoded.FailureDetail == nil {
		t.Fatalf("round trip = %#v", decoded)
	}
}
