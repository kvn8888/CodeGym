package environmentverify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// This file is test-only support for milestone 4. It runs the checked-in
// sample fixture (testdata/greet) locally with bounded output and real
// cancellation. It must never become a production host-shell runner for
// arbitrary generated commands: it only executes python3 with arguments
// from the fixture's own declared test command, inside verifier-prepared
// workspaces. Real subprocess termination here comes from os/exec context
// handling in test support; production runners (sandboxed) must implement
// equivalent termination themselves.

const sampleMaxOutput = 64 * 1024

var (
	unittestResultLine  = regexp.MustCompile(`^(\S+) \((\S+)\) \.\.\. (ok|FAIL|ERROR|skipped|expected failure|unexpected success)$`)
	unittestDetailStart = regexp.MustCompile(`^(FAIL|ERROR): (\S+)`)
	sampleAssertLine    = regexp.MustCompile(`^SAMPLE_ASSERT (\{.*\})$`)
)

// maxAssertionField bounds one expected/actual value kept from an assertion
// record so a pathological solution cannot bloat evidence.
const maxAssertionField = 1000

// sampleAssertionRecord is the fixture's structured assertion emission.
type sampleAssertionRecord struct {
	Test     string `json:"test"`
	Code     string `json:"code"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
}

// localSampleExecutor assesses the known sample fixture by running its
// declared test command with python3 in the prepared workspace.
type localSampleExecutor struct {
	workdirs []string
}

func (e *localSampleExecutor) Assess(ctx context.Context, work AssessmentWork) (AssessmentReport, error) {
	e.workdirs = append(e.workdirs, work.WorkDir)
	fields := strings.Fields(work.TestCommand)
	if len(fields) == 0 || fields[0] != "python3" {
		return AssessmentReport{}, fmt.Errorf("sample executor refuses command %q: fixture scope is python3 only", work.TestCommand)
	}
	if err := ctx.Err(); err != nil {
		return AssessmentReport{}, err
	}
	cmd := exec.CommandContext(ctx, "python3", fields[1:]...)
	cmd.Dir = work.WorkDir
	output, exitCode, err := boundedCombinedOutput(cmd, sampleMaxOutput)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return AssessmentReport{}, ctxErr
		}
		return AssessmentReport{}, err
	}
	return parseUnittestReport(output, exitCode), nil
}

// localCommandRunner runs fixture setup/build/test commands locally with
// bounded output. Test-only: same scope limits as localSampleExecutor.
type localCommandRunner struct {
	calls []CommandSpec
}

func (r *localCommandRunner) Run(ctx context.Context, spec CommandSpec) (CommandResult, error) {
	r.calls = append(r.calls, spec)
	fields := strings.Fields(spec.Command)
	if len(fields) == 0 || fields[0] != "python3" {
		return CommandResult{}, fmt.Errorf("sample runner refuses command %q: fixture scope is python3 only", spec.Command)
	}
	if err := ctx.Err(); err != nil {
		return CommandResult{}, err
	}
	cmd := exec.CommandContext(ctx, "python3", fields[1:]...)
	cmd.Dir = spec.WorkDir
	output, exitCode, err := boundedCombinedOutput(cmd, sampleMaxOutput)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			if errors.Is(ctxErr, context.DeadlineExceeded) {
				return CommandResult{ExitCode: exitCode, Output: output, TimedOut: true}, nil
			}
			return CommandResult{}, ctxErr
		}
		return CommandResult{}, err
	}
	return CommandResult{ExitCode: exitCode, Output: output}, nil
}

// cappedWriter bounds captured output.
type cappedWriter struct {
	buf     bytes.Buffer
	limit   int
	clipped bool
}

func (w *cappedWriter) Write(p []byte) (int, error) {
	if w.buf.Len() >= w.limit {
		w.clipped = true
		return len(p), nil
	}
	room := w.limit - w.buf.Len()
	if len(p) > room {
		w.clipped = true
		p = p[:room]
	}
	return w.buf.Write(p)
}

func boundedCombinedOutput(cmd *exec.Cmd, limit int) (string, int, error) {
	writer := &cappedWriter{limit: limit}
	cmd.Stdout = writer
	cmd.Stderr = writer
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
			err = nil
		}
	}
	output := writer.buf.String()
	if writer.clipped {
		output += "…[truncated]"
	}
	return output, exitCode, err
}

// parseUnittestReport converts verbose unittest output into structured
// outcomes. Test failures and errors are data; only execution problems
// (zero tests with a nonzero exit, i.e. nothing could run) become a
// compile error, and silent success with zero tests stays an empty report
// for the verifier to reject as malformed.
func parseUnittestReport(output string, exitCode int) AssessmentReport {
	type detail struct {
		class   string
		message []string
	}
	details := map[string]*detail{}
	assertions := map[string]*ObservedAssertion{}
	var current string
	flush := func() { current = "" }
	lines := strings.Split(output, "\n")
	var outcomes []TestOutcome
	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r")
		if match := sampleAssertLine.FindStringSubmatch(strings.TrimSpace(trimmed)); match != nil {
			var record sampleAssertionRecord
			if err := json.Unmarshal([]byte(match[1]), &record); err == nil &&
				record.Test != "" && record.Code != "" {
				assertions[record.Test] = &ObservedAssertion{
					Code:     record.Code,
					Expected: clipField(record.Expected),
					Actual:   clipField(record.Actual),
				}
			}
			continue
		}
		if match := unittestDetailStart.FindStringSubmatch(trimmed); match != nil {
			class := ClassAssertion
			if match[1] == "ERROR" {
				class = ClassError
			}
			current = match[2]
			details[current] = &detail{class: class}
			continue
		}
		if current != "" {
			// Separator runs are decoration, not content.
			if strings.HasPrefix(trimmed, "----") || strings.HasPrefix(trimmed, "====") {
				continue
			}
			if trimmed == "" ||
				trimmed == "OK" || strings.HasPrefix(trimmed, "OK ") ||
				strings.HasPrefix(trimmed, "FAILED") || strings.HasPrefix(trimmed, "Ran ") {
				flush()
			} else {
				d := details[current]
				if len(d.message) < 20 {
					d.message = append(d.message, strings.TrimSpace(trimmed))
				}
			}
			continue
		}
		if match := unittestResultLine.FindStringSubmatch(trimmed); match != nil {
			id, status := match[1], match[3]
			outcome := TestOutcome{TestID: id, Passed: status == "ok"}
			if status == "FAIL" {
				outcome.FailureClass = ClassAssertion
			} else if !outcome.Passed {
				outcome.FailureClass = ClassError
			}
			outcomes = append(outcomes, outcome)
		}
	}
	for i, outcome := range outcomes {
		if d, ok := details[outcome.TestID]; ok && !outcome.Passed {
			outcomes[i].FailureClass = d.class
			// Keep both ends of long tracebacks: the head names the failing
			// test, the tail carries the error line (e.g. AssertionError).
			joined := strings.Join(d.message, " ")
			joined = strings.Join(strings.Fields(joined), " ")
			if len(joined) > 500 {
				joined = joined[:200] + " … " + joined[len(joined)-280:]
			}
			outcomes[i].Message = joined
		}
		if assertion, ok := assertions[outcome.TestID]; ok && !outcome.Passed {
			outcomes[i].Assertion = assertion
		}
	}
	if len(outcomes) > 0 {
		return AssessmentReport{Tests: outcomes}
	}
	if exitCode != 0 {
		excerpt := output
		if len(excerpt) > 1000 {
			excerpt = excerpt[len(excerpt)-1000:]
		}
		compileErr := strings.TrimSpace(excerpt)
		return AssessmentReport{CompileError: &compileErr}
	}
	return AssessmentReport{}
}

func clipField(value string) string {
	if len(value) > maxAssertionField {
		return value[:maxAssertionField] + "…[truncated]"
	}
	return value
}

// stageSampleArtifact copies the checked-in fixture into a temp artifact
// root with the given solution content in place of the learner stub.
func stageSampleArtifact(t *testing.T, solution string) (string, string, string) {
	t.Helper()
	root := t.TempDir()
	manifest, err := os.ReadFile(filepath.Join("testdata", "greet", "manifest.json"))
	if err != nil {
		t.Fatalf("read fixture manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), manifest, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	test, err := os.ReadFile(filepath.Join("testdata", "greet", "protected", "test_greet.py"))
	if err != nil {
		t.Fatalf("read fixture tests: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "protected"), 0o755); err != nil {
		t.Fatalf("mkdir protected: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "protected", "test_greet.py"), test, 0o644); err != nil {
		t.Fatalf("write tests: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "solution.py"), []byte(solution), 0o644); err != nil {
		t.Fatalf("write solution: %v", err)
	}
	correct, err := os.ReadFile(filepath.Join("testdata", "greet", "solutions", "correct.py"))
	if err != nil {
		t.Fatalf("read correct solution: %v", err)
	}
	wrong, err := os.ReadFile(filepath.Join("testdata", "greet", "solutions", "wrong.py"))
	if err != nil {
		t.Fatalf("read wrong solution: %v", err)
	}
	return root, string(correct), string(wrong)
}

// syntheticTestReviewer labels review data invented by unit tests. It
// exercises the verified path without claiming human approval: any verdict
// built on it is synthetic evidence only. Production review evidence must
// name a real reviewer; see testdata/greet/README.md.
const syntheticTestReviewer = "synthetic-test-review: unit-test placeholder (not a human review; human review pending)"

func sampleAssessmentSpec(correct, wrong string) *AssessmentSpec {
	return &AssessmentSpec{
		ArtifactID:      "greet-basics-2026-10-01",
		ArtifactVersion: "0.1.0",
		Objective:       "Practice string handling with functions",
		CorrectSolution: map[string]string{"solution.py": correct},
		WrongSolution:   map[string]string{"solution.py": wrong},
		ExpectedFailure: ExpectedFailure{
			TestID:   "test_strips_whitespace",
			Code:     "STRIP_WHITESPACE",
			Expected: "Hello, Bob!",
			Actual:   "Hello,   Bob  !",
			Reason:   "leading/trailing whitespace is not stripped",
		},
		ReviewedBy: syntheticTestReviewer,
	}
}

// TestSampleAssessmentEndToEnd executes the checked-in sample for real:
// stages run the fixture commands locally, then the correct solution must
// pass and the wrong one must fail the expected assertion.
func TestSampleAssessmentEndToEnd(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is required for sample assessment execution")
	}
	root, correct, wrong := stageSampleArtifact(t, "")
	// The staged artifact carries the correct solution so the manifest
	// setup/build/test stages pass against it.
	if err := os.WriteFile(filepath.Join(root, "solution.py"), []byte(correct), 0o644); err != nil {
		t.Fatalf("write correct solution: %v", err)
	}
	before := snapshotTree(t, root)
	executor := &localSampleExecutor{}
	commands := &localCommandRunner{}
	spec := sampleAssessmentSpec(correct, wrong)
	opts := VerifyOptions{
		Timeouts:   StageTimeouts{Setup: time.Minute, Build: time.Minute, Test: time.Minute, Assessment: 2 * time.Minute},
		Assessment: spec,
		Assessor:   executor,
	}
	builder := BuilderResult{
		SchemaVersion:        BuilderResultSchemaVersion,
		BuilderRunID:         "buildrun_greet_sample",
		Runtime:              &RuntimeInfo{Name: "test-loop", Version: "0.0.1"},
		ProposedManifestPath: strptr("manifest.json"),
		Diagnostics:          []Diagnostic{},
		Telemetry:            &BuilderTelemetry{},
	}
	result := VerifyWithOptions(context.Background(), commands, builder, root, "verify_greet_sample", opts)
	if result.Verdict != VerdictVerified {
		t.Fatalf("verdict = %q (%+v), want verified", result.Verdict, result.Diagnostics)
	}
	if result.FailedStage != nil {
		t.Fatalf("failedStage = %q, want null", *result.FailedStage)
	}
	if result.Promotion.Allowed {
		t.Fatalf("promotion.allowed = true, want false pending registry integration")
	}
	if len(commands.calls) != 3 {
		t.Fatalf("stage commands = %d, want setup/build/test", len(commands.calls))
	}
	if len(executor.workdirs) != 2 {
		t.Fatalf("assessment workspaces = %d, want correct then wrong", len(executor.workdirs))
	}
	for _, dir := range executor.workdirs {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("assessment workspace %q was not cleaned up", dir)
		}
	}
	// The source artifact is untouched by assessment execution.
	after := snapshotTree(t, root)
	for path, content := range before {
		if after[path] != content {
			t.Fatalf("source file %q modified by assessment", path)
		}
	}
	// Evidence names the intended wrong-solution rejection with its
	// structured assertion values.
	found := false
	for _, check := range result.Evidence.Checks {
		if strings.Contains(check.Summary, "test_strips_whitespace") &&
			strings.Contains(check.Summary, "STRIP_WHITESPACE") &&
			strings.Contains(check.Summary, `"Hello, Bob!"`) &&
			strings.Contains(check.Summary, `"Hello,   Bob  !"`) {
			found = true
		}
	}
	if !found {
		t.Fatalf("evidence %+v does not record the structured rejection", result.Evidence.Checks)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if object["verdict"] != VerdictVerified || object["failedStage"] != nil {
		t.Fatalf("verified JSON shape = %s", raw)
	}
}

// TestSampleWrongSolutionRealFailure pins the fixture behavior the verifier
// depends on: the wrong solution fails the expected assertion with an
// assertion failure (not an error, not a compile failure).
func TestSampleWrongSolutionRealFailure(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is required for sample assessment execution")
	}
	_, _, wrong := stageSampleArtifact(t, "")
	dir := t.TempDir()
	materializeSampleWorkdir(t, dir, wrong)
	executor := &localSampleExecutor{}
	report, err := executor.Assess(context.Background(), AssessmentWork{
		Role:        RoleWrongSolution,
		WorkDir:     dir,
		TestCommand: "python3 protected/test_greet.py",
	})
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	if report.CompileError != nil {
		t.Fatalf("compile error = %q, want runnable wrong solution", *report.CompileError)
	}
	if len(report.Tests) != 4 {
		t.Fatalf("tests = %+v, want 4 outcomes", report.Tests)
	}
	expected, found := findTest(report.Tests, "test_strips_whitespace")
	if !found || expected.Passed || expected.FailureClass != ClassAssertion {
		t.Fatalf("expected outcome = %+v, want failed assertion", expected)
	}
	if !strings.Contains(expected.Message, "AssertionError") {
		t.Fatalf("message = %q, want AssertionError detail", expected.Message)
	}
	if expected.Assertion == nil {
		t.Fatalf("expected outcome has no assertion record")
	}
	if expected.Assertion.Code != "STRIP_WHITESPACE" ||
		expected.Assertion.Expected != "Hello, Bob!" ||
		expected.Assertion.Actual != "Hello,   Bob  !" {
		t.Fatalf("assertion record = %+v, want documented behavior", expected.Assertion)
	}
	// The empty-string input is blank too: the wrong solution must fail it
	// with the stranger-fallback record as an additional (tolerated) failure.
	empty, found := findTest(report.Tests, "test_empty_string_is_stranger")
	if !found || empty.Passed || empty.FailureClass != ClassAssertion {
		t.Fatalf("empty-string outcome = %+v, want failed assertion", empty)
	}
	if empty.Assertion == nil || empty.Assertion.Code != "STRANGER_FALLBACK" ||
		empty.Assertion.Expected != "Hello, stranger!" ||
		empty.Assertion.Actual != "Hello, !" {
		t.Fatalf("empty-string assertion record = %+v, want stranger fallback", empty.Assertion)
	}
	os.RemoveAll(dir)
}

// TestSampleCorrectSolutionRealPass pins the other half of the fixture
// behavior: the correct solution passes all four protected tests for real.
func TestSampleCorrectSolutionRealPass(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is required for sample assessment execution")
	}
	_, correct, _ := stageSampleArtifact(t, "")
	dir := t.TempDir()
	materializeSampleWorkdir(t, dir, correct)
	executor := &localSampleExecutor{}
	report, err := executor.Assess(context.Background(), AssessmentWork{
		Role:        RoleCorrectSolution,
		WorkDir:     dir,
		TestCommand: "python3 protected/test_greet.py",
	})
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	if len(report.Tests) != 4 {
		t.Fatalf("tests = %+v, want 4 outcomes", report.Tests)
	}
	for _, outcome := range report.Tests {
		if !outcome.Passed {
			t.Fatalf("outcome = %+v, want all passing", outcome)
		}
	}
	os.RemoveAll(dir)
}

// TestSyntheticReviewLabel keeps unit-test review data clearly separated
// from real acceptance evidence: synthetic reviewers carry an explicit
// prefix disclaiming human approval, and the checked-in sample still
// reports review pending.
func TestSyntheticReviewLabel(t *testing.T) {
	if !strings.HasPrefix(syntheticTestReviewer, "synthetic-test-review:") {
		t.Fatalf("synthetic reviewer = %q, want explicit synthetic-test-review prefix", syntheticTestReviewer)
	}
	if !strings.Contains(syntheticTestReviewer, "not a human review") {
		t.Fatalf("synthetic reviewer = %q, want human-review disclaimer", syntheticTestReviewer)
	}
	readme, err := os.ReadFile(filepath.Join("testdata", "greet", "README.md"))
	if err != nil {
		t.Fatalf("read sample README: %v", err)
	}
	if !strings.Contains(string(readme), "REVIEW STATUS: PENDING") {
		t.Fatalf("sample README must still report review pending; no approval may be invented")
	}
}

// TestSamplePendingReviewStaysRejected runs the real sample end to end with
// genuinely pending review: correct passes and wrong fails for real, yet
// overall readiness stays rejected and no assessment even runs.
func TestSamplePendingReviewStaysRejected(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is required for sample assessment execution")
	}
	root, correct, wrong := stageSampleArtifact(t, "")
	if err := os.WriteFile(filepath.Join(root, "solution.py"), []byte(correct), 0o644); err != nil {
		t.Fatalf("write correct solution: %v", err)
	}
	executor := &localSampleExecutor{}
	commands := &localCommandRunner{}
	spec := sampleAssessmentSpec(correct, wrong)
	spec.ReviewedBy = ""
	opts := VerifyOptions{
		Timeouts:   StageTimeouts{Setup: time.Minute, Build: time.Minute, Test: time.Minute, Assessment: 2 * time.Minute},
		Assessment: spec,
		Assessor:   executor,
	}
	builder := BuilderResult{
		SchemaVersion:        BuilderResultSchemaVersion,
		BuilderRunID:         "buildrun_greet_pending",
		Runtime:              &RuntimeInfo{Name: "test-loop", Version: "0.0.1"},
		ProposedManifestPath: strptr("manifest.json"),
		Diagnostics:          []Diagnostic{},
		Telemetry:            &BuilderTelemetry{},
	}
	result := VerifyWithOptions(context.Background(), commands, builder, root, "verify_greet_pending", opts)
	requireRejected(t, result, StageValidation, CodeReviewPending)
	if result.Repairable {
		t.Fatalf("repairable = true, want false: review is not builder repair")
	}
	if result.Promotion.Allowed {
		t.Fatalf("promotion.allowed = true, want false")
	}
	if len(executor.workdirs) != 0 {
		t.Fatalf("assessment workspaces = %d, want 0 without review evidence", len(executor.workdirs))
	}
}

// materializeSampleWorkdir stages the fixture test files with one solution
// for direct executor checks.
func materializeSampleWorkdir(t *testing.T, dir, solution string) {
	t.Helper()
	test, err := os.ReadFile(filepath.Join("testdata", "greet", "protected", "test_greet.py"))
	if err != nil {
		t.Fatalf("read fixture tests: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "protected"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "protected", "test_greet.py"), test, 0o644); err != nil {
		t.Fatalf("write tests: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "solution.py"), []byte(solution), 0o644); err != nil {
		t.Fatalf("write solution: %v", err)
	}
}
