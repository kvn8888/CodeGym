package environmentverify

import (
	"bufio"
	"strings"
)

// TestOutcomeProtocol is the single harness protocol generated tests must
// speak for backend assessment. One line per assertion, printed to stdout:
//
//	CODEGYM_TEST <id> PASS
//	CODEGYM_TEST <id> FAIL code=<code> expected=<expected> actual=<actual>
//
// IDs and values are single whitespace-free tokens. Lines that do not parse
// are ignored, and output with no lines carries no evidence. Print
// statements work in every language, which keeps assessment generic instead
// of per-framework. The verification assessor and browser practice Submit
// both evaluate this protocol; hidden-test source itself is never shown to
// learners, only the parsed verdicts.
const TestOutcomeProtocol = "CODEGYM_TEST"

// ParseTestOutcomes parses protocol lines into structured test outcomes.
// Malformed lines are skipped; an empty input yields no outcomes, never a
// fabricated pass.
func ParseTestOutcomes(output string) []TestOutcome {
	outcomes := []TestOutcome{}
	scanner := bufio.NewScanner(strings.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		fields := strings.Fields(line)
		if len(fields) < 3 || fields[0] != TestOutcomeProtocol {
			continue
		}
		outcome := TestOutcome{TestID: fields[1]}
		switch fields[2] {
		case "PASS":
			outcome.Passed = true
		case "FAIL":
			outcome.Passed = false
			outcome.FailureClass = ClassAssertion
			assertion := &ObservedAssertion{}
			for _, field := range fields[3:] {
				name, value, ok := strings.Cut(field, "=")
				if !ok {
					continue
				}
				switch name {
				case "code":
					assertion.Code = value
				case "expected":
					assertion.Expected = value
				case "actual":
					assertion.Actual = value
				}
			}
			outcome.Assertion = assertion
		default:
			continue
		}
		outcomes = append(outcomes, outcome)
	}
	return outcomes
}

// FailedOutcomes filters parsed outcomes to the failing ones, preserving
// manifest order for deterministic feedback.
func FailedOutcomes(outcomes []TestOutcome) []TestOutcome {
	failed := []TestOutcome{}
	for _, outcome := range outcomes {
		if !outcome.Passed {
			failed = append(failed, outcome)
		}
	}
	return failed
}
