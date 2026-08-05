package execution

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	JudgeSchema        = 1
	legacyResultPrefix = "CODEGYM_RESULT "
)

// JudgeStatus is a user-code outcome written by the sandbox supervisor.
// Platform failures remain execution.StatusError and never use these values.
type JudgeStatus string

const (
	JudgeStatusPassed      JudgeStatus = "passed"
	JudgeStatusFailed      JudgeStatus = "failed"
	JudgeStatusTimeout     JudgeStatus = "timeout"
	JudgeStatusOutOfMemory JudgeStatus = "out_of_memory"
	JudgeStatusCrashed     JudgeStatus = "crashed"
)

type CaseEventType string

const (
	CaseEventStart  CaseEventType = "case_start"
	CaseEventResult CaseEventType = "case_result"
)

// CaseEvent is one append-only cases.jsonl progress record.
type CaseEvent struct {
	Event      CaseEventType `json:"event"`
	Name       string        `json:"name"`
	Status     string        `json:"status,omitempty"`
	DurationMs int64         `json:"duration_ms,omitempty"`
	Error      *string       `json:"error,omitempty"`
}

// CaseResult is the stable per-case shape shared by verdict.json and result.json.
type CaseResult struct {
	Name       string  `json:"name"`
	Status     string  `json:"status"`
	DurationMs int64   `json:"duration_ms"`
	Error      *string `json:"error"`
}

// HarnessVerdict is verdict.json, written only when the child harness finishes.
type HarnessVerdict struct {
	Schema       int          `json:"schema"`
	Status       JudgeStatus  `json:"status"`
	CompileError *string      `json:"compile_error"`
	Cases        []CaseResult `json:"cases"`
}

// JudgeResult is result.json, the single authoritative runner result written
// by the parent supervisor even when the child times out, OOMs, or crashes.
type JudgeResult struct {
	Schema          int          `json:"schema"`
	Status          JudgeStatus  `json:"status"`
	Cases           []CaseResult `json:"cases"`
	CompileError    *string      `json:"compile_error"`
	FailureDetail   *string      `json:"failure_detail"`
	ExitCode        *int         `json:"exit_code"`
	Signal          *string      `json:"signal"`
	DurationMs      int64        `json:"duration_ms"`
	Stdout          string       `json:"stdout"`
	Stderr          string       `json:"stderr"`
	OutputTruncated bool         `json:"output_truncated"`
}

// ReconstructionInput describes what remains after a child exits. It is used
// both to specify the protocol behavior in Go tests and as a backend fallback
// if a future supervisor implementation needs to reconstruct a result.
type ReconstructionInput struct {
	VerdictJSON     []byte
	CasesJSONL      []byte
	LegacyOutput    string
	DeathStatus     JudgeStatus
	FailureDetail   string
	ExitCode        *int
	Signal          *string
	DurationMs      int64
	Stdout          string
	Stderr          string
	OutputTruncated bool
}

// ReconstructJudgeResult prefers a completed verdict, then the retained
// CODEGYM_RESULT compatibility line, then partial cases.jsonl plus death mode.
// It always returns a well-formed result; malformed protocol files become a
// crashed result instead of an absent backend response.
func ReconstructJudgeResult(input ReconstructionInput) JudgeResult {
	result := JudgeResult{
		Schema:          JudgeSchema,
		Cases:           []CaseResult{},
		ExitCode:        input.ExitCode,
		Signal:          input.Signal,
		DurationMs:      max(input.DurationMs, 0),
		Stdout:          input.Stdout,
		Stderr:          input.Stderr,
		OutputTruncated: input.OutputTruncated,
	}

	var protocolErrors []string
	if len(bytes.TrimSpace(input.VerdictJSON)) > 0 {
		var verdict HarnessVerdict
		if err := json.Unmarshal(input.VerdictJSON, &verdict); err == nil {
			if err := validateHarnessVerdict(verdict); err == nil {
				result.Status = verdict.Status
				result.Cases = verdict.Cases
				result.CompileError = verdict.CompileError
				return result
			} else {
				protocolErrors = append(protocolErrors, "invalid verdict.json: "+err.Error())
			}
		} else {
			protocolErrors = append(protocolErrors, "could not parse verdict.json: "+err.Error())
		}
	}

	legacy := input.LegacyOutput
	if legacy == "" {
		legacy = input.Stdout
	}
	if verdict, ok := parseLegacyVerdict(legacy); ok {
		result.Status = verdict.Status
		result.Cases = verdict.Cases
		result.CompileError = verdict.CompileError
		return result
	}

	cases, inFlight, progressErrors := parseCaseProgress(input.CasesJSONL)
	result.Cases = cases
	protocolErrors = append(protocolErrors, progressErrors...)
	result.Status = input.DeathStatus
	if !isDeathStatus(result.Status) {
		result.Status = JudgeStatusCrashed
	}
	detail := strings.TrimSpace(input.FailureDetail)
	if detail == "" {
		detail = defaultFailureDetail(result.Status, result.DurationMs, input.Signal)
	}
	if inFlight != "" {
		detail += fmt.Sprintf(" during case '%s'", inFlight)
	}
	if len(protocolErrors) > 0 {
		detail += "; " + strings.Join(protocolErrors, "; ")
	}
	result.FailureDetail = &detail
	return result
}

func ParseJudgeResult(data []byte) (JudgeResult, error) {
	var result JudgeResult
	if err := json.Unmarshal(data, &result); err != nil {
		return JudgeResult{}, fmt.Errorf("decode result.json: %w", err)
	}
	if result.Schema != JudgeSchema {
		return JudgeResult{}, fmt.Errorf("result.json schema must be %d", JudgeSchema)
	}
	if !isJudgeStatus(result.Status) {
		return JudgeResult{}, fmt.Errorf("result.json has invalid status %q", result.Status)
	}
	if result.DurationMs < 0 {
		return JudgeResult{}, errors.New("result.json has negative duration_ms")
	}
	if result.Cases == nil {
		result.Cases = []CaseResult{}
	}
	for _, testCase := range result.Cases {
		if err := validateCase(testCase); err != nil {
			return JudgeResult{}, err
		}
	}
	return result, nil
}

func validateHarnessVerdict(verdict HarnessVerdict) error {
	if verdict.Schema != JudgeSchema {
		return fmt.Errorf("schema must be %d", JudgeSchema)
	}
	if verdict.Status != JudgeStatusPassed && verdict.Status != JudgeStatusFailed {
		return fmt.Errorf("invalid status %q", verdict.Status)
	}
	if verdict.Cases == nil {
		return errors.New("cases must be present")
	}
	for _, testCase := range verdict.Cases {
		if err := validateCase(testCase); err != nil {
			return err
		}
	}
	if len(verdict.Cases) == 0 && verdict.CompileError == nil {
		return errors.New("verdict contains neither cases nor a compile error")
	}
	return nil
}

func validateCase(testCase CaseResult) error {
	if strings.TrimSpace(testCase.Name) == "" {
		return errors.New("case name is required")
	}
	if testCase.Status != "pass" && testCase.Status != "fail" {
		return fmt.Errorf("case %q has invalid status %q", testCase.Name, testCase.Status)
	}
	if testCase.DurationMs < 0 {
		return fmt.Errorf("case %q has negative duration_ms", testCase.Name)
	}
	return nil
}

func parseCaseProgress(data []byte) ([]CaseResult, string, []string) {
	cases := []CaseResult{}
	inFlight := ""
	var protocolErrors []string
	scanner := bufio.NewScanner(bytes.NewReader(data))
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
			continue
		}
		var event CaseEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			protocolErrors = append(protocolErrors, fmt.Sprintf("malformed cases.jsonl line %d", lineNumber))
			continue
		}
		switch event.Event {
		case CaseEventStart:
			if strings.TrimSpace(event.Name) != "" {
				inFlight = event.Name
			}
		case CaseEventResult:
			testCase := CaseResult{Name: event.Name, Status: event.Status, DurationMs: event.DurationMs, Error: event.Error}
			if err := validateCase(testCase); err != nil {
				protocolErrors = append(protocolErrors, fmt.Sprintf("invalid cases.jsonl line %d: %v", lineNumber, err))
				continue
			}
			cases = append(cases, testCase)
			if inFlight == event.Name {
				inFlight = ""
			}
		}
	}
	if err := scanner.Err(); err != nil {
		protocolErrors = append(protocolErrors, "could not read cases.jsonl: "+err.Error())
	}
	return cases, inFlight, protocolErrors
}

type legacyProtocol struct {
	Tests        []CaseResult `json:"tests"`
	CompileError *string      `json:"compile_error"`
}

func parseLegacyVerdict(output string) (HarnessVerdict, bool) {
	lines := strings.Split(strings.TrimRight(output, "\r\n"), "\n")
	for index := len(lines) - 1; index >= 0; index-- {
		line := strings.TrimSuffix(lines[index], "\r")
		if !strings.HasPrefix(line, legacyResultPrefix) {
			continue
		}
		var legacy legacyProtocol
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, legacyResultPrefix)), &legacy); err != nil {
			return HarnessVerdict{}, false
		}
		if legacy.Tests == nil || len(legacy.Tests) == 0 && legacy.CompileError == nil {
			return HarnessVerdict{}, false
		}
		status := JudgeStatusPassed
		for _, testCase := range legacy.Tests {
			if validateCase(testCase) != nil {
				return HarnessVerdict{}, false
			}
			if testCase.Status == "fail" {
				status = JudgeStatusFailed
			}
		}
		if legacy.CompileError != nil {
			status = JudgeStatusFailed
		}
		return HarnessVerdict{Schema: JudgeSchema, Status: status, Cases: legacy.Tests, CompileError: legacy.CompileError}, true
	}
	return HarnessVerdict{}, false
}

func isJudgeStatus(status JudgeStatus) bool {
	return status == JudgeStatusPassed || status == JudgeStatusFailed || isDeathStatus(status)
}

func isDeathStatus(status JudgeStatus) bool {
	return status == JudgeStatusTimeout || status == JudgeStatusOutOfMemory || status == JudgeStatusCrashed
}

func defaultFailureDetail(status JudgeStatus, durationMs int64, signalName *string) string {
	switch status {
	case JudgeStatusTimeout:
		return fmt.Sprintf("timed out after %dms", durationMs)
	case JudgeStatusOutOfMemory:
		return "memory limit exceeded"
	case JudgeStatusCrashed:
		if signalName != nil && *signalName != "" {
			return "submission terminated by " + *signalName
		}
	}
	return "submission exited without a verdict"
}
