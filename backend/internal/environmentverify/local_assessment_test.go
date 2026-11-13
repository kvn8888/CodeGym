package environmentverify

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestAssessmentReportDistinguishesMissingFromMeasuredZero(t *testing.T) {
	measured, err := json.Marshal(AssessmentReport{
		Tests:     []TestOutcome{},
		ExitCode:  intPointer(0),
		ElapsedMS: int64Pointer(0),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(measured), `"exitCode":0`) ||
		!strings.Contains(string(measured), `"elapsedMs":0`) {
		t.Fatalf("measured zero evidence = %s, want both fields present", measured)
	}

	missing, err := json.Marshal(AssessmentReport{Tests: []TestOutcome{}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(missing), `"exitCode"`) ||
		strings.Contains(string(missing), `"elapsedMs"`) {
		t.Fatalf("missing evidence = %s, want both fields omitted", missing)
	}
}

func TestLocalProtocolAssessorRunsRealCommandAndParsesEvidence(t *testing.T) {
	assessor := &LocalProtocolAssessor{}
	report, err := assessor.Assess(context.Background(), AssessmentWork{
		Role:        RoleCorrectAlternative,
		WorkDir:     t.TempDir(),
		TestCommand: `printf 'CODEGYM_TEST exact PASS\nCODEGYM_TEST order FAIL code=ORDER expected=1,2 actual=2,1\n'; exit 1`,
	})
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	if report.ExitCode == nil || *report.ExitCode != 1 {
		t.Fatalf("exit code = %v, want 1", report.ExitCode)
	}
	if len(report.Tests) != 2 {
		t.Fatalf("tests = %+v, want two parsed outcomes", report.Tests)
	}
	if !report.Tests[0].Passed || report.Tests[1].Passed {
		t.Fatalf("tests = %+v", report.Tests)
	}
	assertion := report.Tests[1].Assertion
	if assertion == nil || assertion.Code != "ORDER" || assertion.Expected != "1,2" || assertion.Actual != "2,1" {
		t.Fatalf("assertion = %+v", assertion)
	}
}

func TestNonzeroExitWithOnlyPassingEvidenceCannotAcceptCorrectSolution(t *testing.T) {
	report := AssessmentReport{
		Tests:    []TestOutcome{{TestID: "only", Passed: true}},
		ExitCode: intPointer(2),
	}
	issue := positiveReportIssue(report, []string{"only"}, RoleCorrectAlternative)
	if issue == nil {
		t.Fatal("positiveReportIssue = nil, want a command-exit failure")
	}
	if issue.Category != FailureCategoryTest || issue.Status != ExecutionCompleted {
		t.Fatalf("issue = %+v", issue)
	}
	if !strings.Contains(issue.Message, "exited with code 2") {
		t.Fatalf("message = %q", issue.Message)
	}
}

func TestZeroExitDoesNotEraseStructuredFailure(t *testing.T) {
	report := AssessmentReport{
		Tests: []TestOutcome{{
			TestID: "only", Passed: false, FailureClass: ClassAssertion,
			Assertion: &ObservedAssertion{Code: "BAD", Expected: "1", Actual: "2"},
		}},
		ExitCode: intPointer(0),
	}
	if issue := positiveReportIssue(report, []string{"only"}, RoleCorrectAlternative); issue == nil {
		t.Fatal("positiveReportIssue = nil, want the structured failure to win")
	}
}

func TestLocalProtocolAssessorReportsMalformedAsNoEvidence(t *testing.T) {
	assessor := &LocalProtocolAssessor{}
	report, err := assessor.Assess(context.Background(), AssessmentWork{
		WorkDir:     t.TempDir(),
		TestCommand: `printf 'not a protocol line\n'; exit 3`,
	})
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	if len(report.Tests) != 0 || report.ExitCode == nil || *report.ExitCode != 3 {
		t.Fatalf("report = %+v", report)
	}
	if issue := positiveReportIssue(report, nil, RoleCorrectAlternative); issue == nil ||
		issue.Category != FailureCategoryMalformed {
		t.Fatalf("issue = %+v, want malformed", issue)
	}
}

func TestRunLocalAssessmentCommandBoundsOutputAndTerminatesProcessGroup(t *testing.T) {
	started := time.Now()
	_, output, timedOut, exhausted, err := runLocalAssessmentCommand(
		context.Background(), "yes overflow", t.TempDir(), 128)
	if err != nil {
		t.Fatalf("runLocalAssessmentCommand: %v", err)
	}
	if !exhausted {
		t.Fatal("exhausted = false, want the output ceiling to terminate the command")
	}
	if timedOut {
		t.Fatal("timedOut = true, want output exhaustion to be distinct from a deadline")
	}
	if len(output) > 128 {
		t.Fatalf("captured %d bytes, want at most the 128-byte ceiling", len(output))
	}
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Fatalf("command took %s; the process group was not terminated promptly", elapsed)
	}
}

func TestLocalProtocolAssessorOutputExhaustionRejectsPartialPassEvidence(t *testing.T) {
	assessor := &LocalProtocolAssessor{MaxOutputBytes: 64}
	report, err := assessor.Assess(context.Background(), AssessmentWork{
		WorkDir:     t.TempDir(),
		TestCommand: `printf 'CODEGYM_TEST early PASS\n'; yes overflow`,
	})
	if !errors.Is(err, ErrAssessmentOutputExceeded) {
		t.Fatalf("error = %v, want ErrAssessmentOutputExceeded", err)
	}
	if len(report.Tests) != 0 {
		t.Fatalf("tests = %+v, want no partial PASS records after output exhaustion", report.Tests)
	}
	if report.OutputTail == "" {
		t.Fatal("output tail is empty, want bounded diagnostics preserved")
	}
	if len(report.OutputTail) > 1000 {
		t.Fatalf("output tail = %d bytes, want a bounded diagnostic", len(report.OutputTail))
	}
	if report.ExitCode == nil || report.ElapsedMS == nil {
		t.Fatalf("report = %+v, want execution evidence retained", report)
	}
}

func TestLocalProtocolAssessorHonorsDeadline(t *testing.T) {
	assessor := &LocalProtocolAssessor{}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := assessor.Assess(ctx, AssessmentWork{
		WorkDir:     t.TempDir(),
		TestCommand: `sleep 5`,
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want deadline exceeded", err)
	}
}
