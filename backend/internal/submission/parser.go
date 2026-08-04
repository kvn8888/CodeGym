package submission

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/kvn8888/codegym/backend/internal/execution"
)

const resultPrefix = "CODEGYM_RESULT "

type resultProtocol struct {
	Tests        []TestCaseResult `json:"tests"`
	CompileError *string          `json:"compile_error"`
}

// ParseTestResult reads the last structured result line. User code may write
// arbitrary stdout before it, so earlier lookalike lines are ignored.
func ParseTestResult(output string) (TestResult, error) {
	lines := strings.Split(strings.TrimRight(output, "\r\n"), "\n")
	var payload string
	for index := len(lines) - 1; index >= 0; index-- {
		line := strings.TrimSuffix(lines[index], "\r")
		if strings.HasPrefix(line, resultPrefix) {
			payload = strings.TrimPrefix(line, resultPrefix)
			break
		}
	}
	if payload == "" {
		return TestResult{}, errors.New("execution output is missing a CODEGYM_RESULT line")
	}

	var protocol resultProtocol
	if err := json.Unmarshal([]byte(payload), &protocol); err != nil {
		return TestResult{}, fmt.Errorf("decode CODEGYM_RESULT: %w", err)
	}
	if protocol.Tests == nil {
		protocol.Tests = []TestCaseResult{}
	}
	if len(protocol.Tests) == 0 && protocol.CompileError == nil {
		return TestResult{}, errors.New("CODEGYM_RESULT contains neither tests nor a compile error")
	}

	result := TestResult{
		Status:       "pass",
		Total:        len(protocol.Tests),
		TestCases:    protocol.Tests,
		CompileError: protocol.CompileError,
	}
	for _, testCase := range protocol.Tests {
		switch testCase.Status {
		case "pass":
			result.Passed++
		case "fail":
			result.Failed++
			result.Status = "fail"
		default:
			return TestResult{}, fmt.Errorf("test case %q has invalid status %q", testCase.Name, testCase.Status)
		}
		if testCase.DurationMs < 0 {
			return TestResult{}, fmt.Errorf("test case %q has negative duration", testCase.Name)
		}
		result.DurationMs += testCase.DurationMs
	}
	if protocol.CompileError != nil {
		result.Status = "fail"
	}
	return result, nil
}

// TestResultFromJudge maps a completed out-of-band harness verdict into the
// submission API's existing per-test shape. Death statuses are handled by the
// submission status itself and intentionally do not masquerade as test output.
func TestResultFromJudge(judge execution.JudgeResult) (TestResult, error) {
	if judge.Schema != execution.JudgeSchema {
		return TestResult{}, fmt.Errorf("unsupported judge schema %d", judge.Schema)
	}
	if judge.Status != execution.JudgeStatusPassed && judge.Status != execution.JudgeStatusFailed {
		return TestResult{}, fmt.Errorf("judge status %q is not a completed verdict", judge.Status)
	}
	result := TestResult{
		Status:       "pass",
		Total:        len(judge.Cases),
		DurationMs:   judge.DurationMs,
		TestCases:    make([]TestCaseResult, 0, len(judge.Cases)),
		CompileError: judge.CompileError,
	}
	for _, testCase := range judge.Cases {
		mapped := TestCaseResult{
			Name: testCase.Name, Status: testCase.Status,
			DurationMs: testCase.DurationMs, Error: testCase.Error,
		}
		switch mapped.Status {
		case "pass":
			result.Passed++
		case "fail":
			result.Failed++
			result.Status = "fail"
		default:
			return TestResult{}, fmt.Errorf("test case %q has invalid status %q", mapped.Name, mapped.Status)
		}
		if mapped.DurationMs < 0 {
			return TestResult{}, fmt.Errorf("test case %q has negative duration", mapped.Name)
		}
		result.TestCases = append(result.TestCases, mapped)
	}
	if judge.Status == execution.JudgeStatusFailed || judge.CompileError != nil {
		result.Status = "fail"
	}
	if len(result.TestCases) == 0 && result.CompileError == nil {
		return TestResult{}, errors.New("judge verdict contains neither tests nor a compile error")
	}
	return result, nil
}
