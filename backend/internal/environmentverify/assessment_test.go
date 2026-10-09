package environmentverify

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// stubAssessor returns canned reports per solution role without executing
// anything. Orchestration tests use it; real execution of the checked-in
// sample lives in assessment_sample_test.go.
type stubAssessor struct {
	calls   []AssessmentWork
	reports map[string]stubAssessment
}

type stubAssessment struct {
	report AssessmentReport
	err    error
}

func (s *stubAssessor) Assess(_ context.Context, work AssessmentWork) (AssessmentReport, error) {
	s.calls = append(s.calls, work)
	if outcome, ok := s.reports[work.Role]; ok {
		return outcome.report, outcome.err
	}
	return AssessmentReport{}, errors.New("stub assessor has no report for role " + work.Role)
}

func (s *stubAssessor) roles() []string {
	roles := make([]string, 0, len(s.calls))
	for _, call := range s.calls {
		roles = append(roles, call.Role)
	}
	return roles
}

func passReport(ids ...string) AssessmentReport {
	outcomes := make([]TestOutcome, 0, len(ids))
	for _, id := range ids {
		outcomes = append(outcomes, TestOutcome{TestID: id, Passed: true})
	}
	return AssessmentReport{Tests: outcomes}
}

func failReport(id, class string, others ...TestOutcome) AssessmentReport {
	outcomes := []TestOutcome{{TestID: id, Passed: false, FailureClass: class, Message: id + " failed"}}
	return AssessmentReport{Tests: append(outcomes, others...)}
}

// failAssertionReport builds a wrong-solution report whose expected test
// carries a structured assertion record.
func failAssertionReport(id, code, expected, actual string, others ...TestOutcome) AssessmentReport {
	outcomes := []TestOutcome{{
		TestID: id, Passed: false, FailureClass: ClassAssertion, Message: id + " failed",
		Assertion: &ObservedAssertion{Code: code, Expected: expected, Actual: actual},
	}}
	return AssessmentReport{Tests: append(outcomes, others...)}
}

// assessmentFixture wires a valid artifact with backend-controlled
// assessment inputs for the checked-in sample shape.
func assessmentFixture(t *testing.T, mutate func(*AssessmentSpec)) (BuilderResult, string, VerifyOptions) {
	t.Helper()
	builder, root := verifyFixture(t)
	spec := &AssessmentSpec{
		ArtifactID:      "spring-boot-di-intro-2026-09-29",
		ArtifactVersion: "0.1.0",
		Objective:       "Practice dependency injection with services",
		CorrectSolution: map[string]string{
			"src/main/java/com/codegym/exercise/GreetingService.java": "correct",
		},
		WrongSolution: map[string]string{
			"src/main/java/com/codegym/exercise/GreetingService.java": "wrong",
		},
		ExpectedFailure: ExpectedFailure{
			TestID:   "rejectsNullGreeting",
			Code:     "NULL_REJECTION",
			Expected: "rejected",
			Actual:   "accepted",
			Reason:   "null greetings must be rejected",
		},
		ReviewedBy: "review-board-case-188",
	}
	if mutate != nil {
		mutate(spec)
	}
	assessor := &stubAssessor{
		reports: map[string]stubAssessment{
			RoleCorrectSolution: {report: passReport("acceptsGreeting", "rejectsNullGreeting")},
			RoleWrongSolution: {report: failAssertionReport("rejectsNullGreeting", "NULL_REJECTION",
				"rejected", "accepted", TestOutcome{TestID: "acceptsGreeting", Passed: true})},
		},
	}
	opts := VerifyOptions{Timeouts: StageTimeouts{}, Assessment: spec, Assessor: assessor}
	return builder, root, opts
}

func assessorOf(opts VerifyOptions) *stubAssessor {
	assessor, ok := opts.Assessor.(*stubAssessor)
	if assessor == nil || !ok {
		panic("assessmentFixture always wires a *stubAssessor")
	}
	return assessor
}

func TestAssessmentVerified(t *testing.T) {
	builder, root, opts := assessmentFixture(t, nil)
	result := VerifyWithOptions(context.Background(), passingRunner(), builder, root, "verify_assessed", opts)
	if result.Verdict != VerdictVerified {
		t.Fatalf("verdict = %q, want verified", result.Verdict)
	}
	if result.FailedStage != nil {
		t.Fatalf("failedStage = %q, want null", *result.FailedStage)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %+v, want empty", result.Diagnostics)
	}
	if result.Promotion.Allowed {
		t.Fatalf("promotion.allowed = true, want false pending registry integration")
	}
	stages := evidenceStages(result)
	want := []string{StageManifest, StageValidation, StageSetup, StageBuild, StageTest, StageValidation, StageValidation}
	if !equalStrings(stages, want) {
		t.Fatalf("evidence stages = %v, want %v", stages, want)
	}
	assessor := assessorOf(opts)
	if got := assessor.roles(); !equalStrings(got, []string{RoleCorrectSolution, RoleWrongSolution}) {
		t.Fatalf("assessed roles = %v, want correct then wrong", got)
	}
	// The intended behavioral failure is recorded as successful validation
	// evidence with its structured assertion values, not as a failure.
	wrongCheck := result.Evidence.Checks[len(result.Evidence.Checks)-1]
	for _, want := range []string{"rejectsNullGreeting", "NULL_REJECTION", `"rejected"`, `"accepted"`} {
		if !strings.Contains(wrongCheck.Summary, want) {
			t.Fatalf("wrong-solution evidence = %q, want it to contain %q", wrongCheck.Summary, want)
		}
	}
	// Isolation: separate workspaces, neither inside the source artifact.
	seen := map[string]bool{}
	for _, call := range assessor.calls {
		if seen[call.WorkDir] {
			t.Fatalf("duplicate assessment workspace %q", call.WorkDir)
		}
		seen[call.WorkDir] = true
		rel, err := filepath.Rel(root, call.WorkDir)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			t.Fatalf("assessment workspace %q is inside the source artifact", call.WorkDir)
		}
		if _, err := os.Stat(call.WorkDir); !os.IsNotExist(err) {
			t.Fatalf("assessment workspace %q was not cleaned up", call.WorkDir)
		}
	}
}

func TestAssessmentCorrectFails(t *testing.T) {
	builder, root, opts := assessmentFixture(t, nil)
	assessor := assessorOf(opts)
	assessor.reports[RoleCorrectSolution] = stubAssessment{
		report: failReport("acceptsGreeting", ClassAssertion, TestOutcome{TestID: "rejectsNullGreeting", Passed: true}),
	}
	result := VerifyWithOptions(context.Background(), passingRunner(), builder, root, "verify_correct_fails", opts)
	requireRejected(t, result, StageValidation, CodeCorrectSolutionFailed)
	if !result.Repairable {
		t.Fatalf("repairable = false, want true")
	}
	if got := assessor.roles(); !equalStrings(got, []string{RoleCorrectSolution}) {
		t.Fatalf("assessed roles = %v, want wrong solution never assessed after correct failure", got)
	}
}

func TestAssessmentCorrectDoesNotCompile(t *testing.T) {
	builder, root, opts := assessmentFixture(t, nil)
	compileErr := "cannot find symbol: GreetingService"
	assessor := assessorOf(opts)
	assessor.reports[RoleCorrectSolution] = stubAssessment{report: AssessmentReport{CompileError: &compileErr}}
	result := VerifyWithOptions(context.Background(), passingRunner(), builder, root, "verify_correct_compile", opts)
	requireRejected(t, result, StageValidation, CodeCorrectSolutionFailed)
}

func TestAssessmentCorrectMissingExpectedTest(t *testing.T) {
	// The correct solution passes everything it ran, but never ran the
	// expected test — so the wrong solution's later failure there proves
	// nothing and the wrong solution must never be assessed.
	builder, root, opts := assessmentFixture(t, nil)
	assessor := assessorOf(opts)
	assessor.reports[RoleCorrectSolution] = stubAssessment{report: passReport("acceptsGreeting")}
	result := VerifyWithOptions(context.Background(), passingRunner(), builder, root, "verify_correct_missing_expected", opts)
	requireRejected(t, result, StageValidation, CodeAssessmentMalformed)
	details, ok := result.Diagnostics[0].Details.(map[string]any)
	if !ok || details["assessment"] != RoleCorrectSolution || details["expectedTest"] != "rejectsNullGreeting" {
		t.Fatalf("diagnostic details = %+v, want correct-solution expected test", result.Diagnostics[0].Details)
	}
	if got := assessor.roles(); !equalStrings(got, []string{RoleCorrectSolution}) {
		t.Fatalf("assessed roles = %v, want wrong solution never assessed", got)
	}
}

func TestAssessmentWrongPasses(t *testing.T) {
	builder, root, opts := assessmentFixture(t, nil)
	assessor := assessorOf(opts)
	assessor.reports[RoleWrongSolution] = stubAssessment{report: passReport("acceptsGreeting", "rejectsNullGreeting")}
	result := VerifyWithOptions(context.Background(), passingRunner(), builder, root, "verify_wrong_passes", opts)
	requireRejected(t, result, StageValidation, CodeWrongSolutionPassed)
	if !result.Repairable {
		t.Fatalf("repairable = false, want true")
	}
}

func TestAssessmentWrongUnrelatedFailure(t *testing.T) {
	// The expected assertion passes while another test fails: the wrong
	// solution did not fail for the intended reason.
	builder, root, opts := assessmentFixture(t, nil)
	assessor := assessorOf(opts)
	assessor.reports[RoleWrongSolution] = stubAssessment{report: AssessmentReport{Tests: []TestOutcome{
		{TestID: "rejectsNullGreeting", Passed: true},
		{TestID: "acceptsGreeting", Passed: false, FailureClass: ClassAssertion},
	}}}
	result := VerifyWithOptions(context.Background(), passingRunner(), builder, root, "verify_wrong_unrelated", opts)
	requireRejected(t, result, StageValidation, CodeWrongSolutionUnexpected)
}

func TestAssessmentWrongCannotCompile(t *testing.T) {
	compileErr := "cannot find symbol: GreetingService"
	builder, root, opts := assessmentFixture(t, nil)
	assessor := assessorOf(opts)
	assessor.reports[RoleWrongSolution] = stubAssessment{report: AssessmentReport{CompileError: &compileErr}}
	result := VerifyWithOptions(context.Background(), passingRunner(), builder, root, "verify_wrong_compile", opts)
	requireRejected(t, result, StageValidation, CodeWrongSolutionUnexpected)
}

func TestAssessmentWrongErrorClass(t *testing.T) {
	// The expected test fails with a runtime error rather than an
	// assertion: unrelated reason, not the intended rejection.
	builder, root, opts := assessmentFixture(t, nil)
	assessor := assessorOf(opts)
	assessor.reports[RoleWrongSolution] = stubAssessment{
		report: failReport("rejectsNullGreeting", ClassError),
	}
	result := VerifyWithOptions(context.Background(), passingRunner(), builder, root, "verify_wrong_error", opts)
	requireRejected(t, result, StageValidation, CodeWrongSolutionUnexpected)
}

func TestAssessmentAssertionMismatch(t *testing.T) {
	// The expected test fails with class assertion, but the structured
	// record contradicts backend expectations: a different assertion ran,
	// the values differ, or no record was emitted at all.
	accepts := TestOutcome{TestID: "acceptsGreeting", Passed: true}
	cases := []struct {
		name   string
		report AssessmentReport
		want   string
	}{
		{"different code", failAssertionReport("rejectsNullGreeting", "OTHER_CODE", "rejected", "accepted", accepts), "assertion code"},
		{"different expected", failAssertionReport("rejectsNullGreeting", "NULL_REJECTION", "thrown", "accepted", accepts), "expected value"},
		{"different actual", failAssertionReport("rejectsNullGreeting", "NULL_REJECTION", "rejected", "thrown", accepts), "actual value"},
		{"missing record", failReport("rejectsNullGreeting", ClassAssertion, accepts), "no assertion record"},
	}
	for _, tc := range cases {
		builder, root, opts := assessmentFixture(t, nil)
		assessor := assessorOf(opts)
		assessor.reports[RoleWrongSolution] = stubAssessment{report: tc.report}
		result := VerifyWithOptions(context.Background(), passingRunner(), builder, root, "verify_mismatch", opts)
		requireRejected(t, result, StageValidation, CodeAssertionMismatch)
		if !result.Repairable {
			t.Fatalf("%s: repairable = false, want true", tc.name)
		}
		details, ok := result.Diagnostics[0].Details.(map[string]any)
		if !ok {
			t.Fatalf("%s: details = %+v, want structured mismatch", tc.name, result.Diagnostics[0].Details)
		}
		mismatch, _ := details["mismatch"].(string)
		if !strings.Contains(mismatch, tc.want) {
			t.Fatalf("%s: mismatch = %q, want %q", tc.name, mismatch, tc.want)
		}
		if _, ok := details["want"]; !ok {
			t.Fatalf("%s: details lacks backend expectations: %+v", tc.name, details)
		}
		if _, ok := details["observed"]; !ok {
			t.Fatalf("%s: details lacks observed record: %+v", tc.name, details)
		}
	}
}

func TestAssessmentMalformedReports(t *testing.T) {
	// Zero tests from the correct assessment.
	builder, root, opts := assessmentFixture(t, nil)
	assessor := assessorOf(opts)
	assessor.reports[RoleCorrectSolution] = stubAssessment{report: AssessmentReport{}}
	result := VerifyWithOptions(context.Background(), passingRunner(), builder, root, "verify_malformed", opts)
	requireRejected(t, result, StageValidation, CodeAssessmentMalformed)

	// Zero tests from the wrong assessment after a passing correct one.
	builder, root, opts = assessmentFixture(t, nil)
	assessor = assessorOf(opts)
	assessor.reports[RoleWrongSolution] = stubAssessment{report: AssessmentReport{}}
	result = VerifyWithOptions(context.Background(), passingRunner(), builder, root, "verify_malformed_wrong", opts)
	requireRejected(t, result, StageValidation, CodeAssessmentMalformed)

	// Expected test absent from an otherwise passing report.
	builder, root, opts = assessmentFixture(t, nil)
	assessor = assessorOf(opts)
	assessor.reports[RoleWrongSolution] = stubAssessment{report: passReport("someOtherTest")}
	result = VerifyWithOptions(context.Background(), passingRunner(), builder, root, "verify_missing_expected", opts)
	requireRejected(t, result, StageValidation, CodeAssessmentMalformed)
}

func TestAssessmentInputsMissing(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*AssessmentSpec)
		want   string
	}{
		{"nil solutions", func(s *AssessmentSpec) {
			s.CorrectSolution = nil
			s.WrongSolution = nil
		}, "correctSolution"},
		{"artifact mismatch", func(s *AssessmentSpec) {
			s.ArtifactID = "other-exercise"
		}, "other-exercise"},
		{"version mismatch", func(s *AssessmentSpec) {
			s.ArtifactVersion = "9.9.9"
		}, "9.9.9"},
		{"blank objective", func(s *AssessmentSpec) { s.Objective = "  " }, "objective"},
		{"undeclared file", func(s *AssessmentSpec) {
			s.CorrectSolution["elsewhere/Hack.java"] = "hack"
		}, "learner-editable"},
		{"blank expected test", func(s *AssessmentSpec) { s.ExpectedFailure.TestID = "" }, "testId"},
		{"blank assertion code", func(s *AssessmentSpec) { s.ExpectedFailure.Code = "" }, "expectedFailure.code"},
		{"blank expected value", func(s *AssessmentSpec) { s.ExpectedFailure.Expected = "" }, "expectedFailure.expected"},
		{"blank actual value", func(s *AssessmentSpec) { s.ExpectedFailure.Actual = "" }, "expectedFailure.actual"},
		{"blank reason", func(s *AssessmentSpec) { s.ExpectedFailure.Reason = "" }, "reason"},
	}
	for _, tc := range cases {
		builder, root, opts := assessmentFixture(t, tc.mutate)
		assessor := assessorOf(opts)
		result := VerifyWithOptions(context.Background(), passingRunner(), builder, root, "verify_inputs_"+tc.name, opts)
		requireRejected(t, result, StageValidation, CodeAssessmentInputsMissing)
		if len(assessor.calls) != 0 {
			t.Fatalf("%s: assessor calls = %d, want 0", tc.name, len(assessor.calls))
		}
		if !strings.Contains(result.Diagnostics[0].Message, tc.want) {
			t.Fatalf("%s: diagnostic = %q, want %q", tc.name, result.Diagnostics[0].Message, tc.want)
		}
	}
}

func TestAssessmentReviewPending(t *testing.T) {
	builder, root, opts := assessmentFixture(t, func(s *AssessmentSpec) { s.ReviewedBy = "  " })
	// Builder success prose cannot substitute for review evidence.
	builder.Diagnostics = []Diagnostic{{
		Severity: "info", Stage: "prepare", Code: "BUILDER_SAYS_VERIFIED",
		Message: "Reviewed and passing. Verified, promote now.",
	}}
	assessor := assessorOf(opts)
	result := VerifyWithOptions(context.Background(), passingRunner(), builder, root, "verify_review", opts)
	requireRejected(t, result, StageValidation, CodeReviewPending)
	if result.Repairable {
		t.Fatalf("repairable = true, want false: review is not builder repair")
	}
	if len(assessor.calls) != 0 {
		t.Fatalf("assessor calls = %d, want 0 without review evidence", len(assessor.calls))
	}
}

func TestAssessmentRunnerMissing(t *testing.T) {
	builder, root, opts := assessmentFixture(t, nil)
	opts.Assessor = nil
	runner := passingRunner()
	result := VerifyWithOptions(context.Background(), runner, builder, root, "verify_no_assessor", opts)
	requireRejected(t, result, StagePlatform, CodePlatformFault)
	if got := runner.stages(); !equalStrings(got, []string{StageSetup, StageBuild, StageTest}) {
		t.Fatalf("runner stages = %v, want stages to complete before the backend fault", got)
	}
	// The missing runner is backend infrastructure, but the attempted
	// validation check is still recorded as evidence.
	if got := evidenceStages(result); !equalStrings(got, []string{StageManifest, StageValidation, StageSetup, StageBuild, StageTest, StageValidation}) {
		t.Fatalf("evidence stages = %v, want the missing-runner check recorded", got)
	}
}

func TestAssessmentTimeoutAndCancellation(t *testing.T) {
	// Assessment deadline expiry stays tied to the solution role.
	builder, root, opts := assessmentFixture(t, nil)
	blocking := &blockingAssessor{}
	opts.Assessor = blocking
	opts.Timeouts = StageTimeouts{Assessment: 30 * time.Millisecond}
	result := VerifyWithOptions(context.Background(), passingRunner(), builder, root, "verify_assess_timeout", opts)
	requireRejected(t, result, StageTimeout, CodeStageTimeout)
	details, ok := result.Diagnostics[0].Details.(map[string]any)
	if !ok || details["assessment"] != RoleCorrectSolution {
		t.Fatalf("diagnostic details = %+v, want correct-solution role", result.Diagnostics[0].Details)
	}
	if len(blocking.calls) != 1 {
		t.Fatalf("assessor calls = %d, want 1", len(blocking.calls))
	}

	// Parent cancellation during the wrong assessment is a platform fault.
	builder, root, opts = assessmentFixture(t, nil)
	roles := &roleAssessor{
		reports: map[string]stubAssessment{
			RoleCorrectSolution: {report: passReport("acceptsGreeting", "rejectsNullGreeting")},
		},
		cancelAfter: map[string]bool{RoleWrongSolution: true},
	}
	opts.Assessor = roles
	ctx, cancel := context.WithCancel(context.Background())
	roles.cancel = cancel
	result = VerifyWithOptions(ctx, passingRunner(), builder, root, "verify_assess_cancel", opts)
	requireRejected(t, result, StagePlatform, CodePlatformFault)
	if got := roles.roles(); !equalStrings(got, []string{RoleCorrectSolution, RoleWrongSolution}) {
		t.Fatalf("assessed roles = %v", got)
	}
}

// cancelThenSucceedAssessor cancels the backend context and then reports
// success, simulating an assessor that outlives cancellation.
type cancelThenSucceedAssessor struct {
	cancel context.CancelFunc
	report AssessmentReport
	calls  int
}

func (a *cancelThenSucceedAssessor) Assess(_ context.Context, work AssessmentWork) (AssessmentReport, error) {
	a.calls++
	a.cancel()
	return a.report, nil
}

func TestAssessmentCancelledSuccessNeverVerifies(t *testing.T) {
	// A passing report delivered after the backend context died must not
	// verify: cancellation supersedes it, the wrong solution never runs,
	// and no verified verdict is issued.
	builder, root, opts := assessmentFixture(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	opts.Assessor = &cancelThenSucceedAssessor{
		cancel: cancel,
		report: passReport("acceptsGreeting", "rejectsNullGreeting"),
	}
	result := VerifyWithOptions(ctx, passingRunner(), builder, root, "verify_assess_cancel_success", opts)
	requireRejected(t, result, StagePlatform, CodePlatformFault)
	if result.Verdict == VerdictVerified {
		t.Fatalf("verdict = verified after cancellation, want rejected")
	}
	assessor := opts.Assessor.(*cancelThenSucceedAssessor)
	if assessor.calls != 1 {
		t.Fatalf("assessor calls = %d, want correct-solution only", assessor.calls)
	}
}

// blockingAssessor waits for cancellation like a well-behaved runner must.
// It proves deadline propagation only, not subprocess termination.
type blockingAssessor struct {
	calls []AssessmentWork
}

func (b *blockingAssessor) Assess(ctx context.Context, work AssessmentWork) (AssessmentReport, error) {
	b.calls = append(b.calls, work)
	<-ctx.Done()
	return AssessmentReport{}, ctx.Err()
}

// roleAssessor serves canned reports and cancels the parent context when a
// flagged role starts, simulating mid-run cancellation.
type roleAssessor struct {
	reports     map[string]stubAssessment
	cancelAfter map[string]bool
	cancel      context.CancelFunc
	calls       []AssessmentWork
}

func (r *roleAssessor) Assess(_ context.Context, work AssessmentWork) (AssessmentReport, error) {
	r.calls = append(r.calls, work)
	if r.cancelAfter[work.Role] && r.cancel != nil {
		r.cancel()
		return AssessmentReport{}, context.Canceled
	}
	if outcome, ok := r.reports[work.Role]; ok {
		return outcome.report, outcome.err
	}
	return AssessmentReport{}, errors.New("no report for role " + work.Role)
}

func (r *roleAssessor) roles() []string {
	roles := make([]string, 0, len(r.calls))
	for _, call := range r.calls {
		roles = append(roles, call.Role)
	}
	return roles
}

func TestAssessmentWorkspaceIsolation(t *testing.T) {
	// The assessor observes the prepared workspaces: protected content
	// intact, solution overlaid, source artifact untouched afterward.
	builder, root, opts := assessmentFixture(t, nil)
	before := snapshotTree(t, root)
	inspector := &inspectingAssessor{reports: map[string]stubAssessment{
		RoleCorrectSolution: {report: passReport("acceptsGreeting", "rejectsNullGreeting")},
		RoleWrongSolution: {report: failAssertionReport("rejectsNullGreeting", "NULL_REJECTION",
			"rejected", "accepted")},
	}}
	opts.Assessor = inspector
	result := VerifyWithOptions(context.Background(), passingRunner(), builder, root, "verify_isolation", opts)
	if result.Verdict != VerdictVerified {
		t.Fatalf("verdict = %q, want verified", result.Verdict)
	}
	after := snapshotTree(t, root)
	if len(before) != len(after) {
		t.Fatalf("source artifact changed: %d files before, %d after", len(before), len(after))
	}
	for path, content := range before {
		if after[path] != content {
			t.Fatalf("source file %q modified by assessment", path)
		}
	}
	if len(inspector.workspaces) != 2 {
		t.Fatalf("workspaces = %d, want 2", len(inspector.workspaces))
	}
	// Only declared learner-editable files are replaced; protected content
	// is intact in both copies.
	const editable = "src/main/java/com/codegym/exercise/GreetingService.java"
	if inspector.workspaces[RoleCorrectSolution][editable] != "correct" {
		t.Fatalf("correct workspace solution = %q, want overlaid content", inspector.workspaces[RoleCorrectSolution][editable])
	}
	if inspector.workspaces[RoleWrongSolution][editable] != "wrong" {
		t.Fatalf("wrong workspace solution = %q, want overlaid content", inspector.workspaces[RoleWrongSolution][editable])
	}
	for role, files := range inspector.workspaces {
		for path, content := range files {
			if path == editable {
				continue
			}
			if before[path] != content {
				t.Fatalf("workspace %s file %q differs from the source artifact", role, path)
			}
		}
	}
}

// inspectingAssessor records what each solution workspace contains.
type inspectingAssessor struct {
	reports    map[string]stubAssessment
	workspaces map[string]map[string]string
}

func (s *inspectingAssessor) Assess(_ context.Context, work AssessmentWork) (AssessmentReport, error) {
	files := map[string]string{}
	_ = filepath.Walk(work.WorkDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, err := filepath.Rel(work.WorkDir, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if s.workspaces == nil {
		s.workspaces = map[string]map[string]string{}
	}
	s.workspaces[work.Role] = files
	if outcome, ok := s.reports[work.Role]; ok {
		return outcome.report, outcome.err
	}
	return AssessmentReport{}, errors.New("no report for role " + work.Role)
}

func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	return files
}

func TestAssessmentVerifiedContractShape(t *testing.T) {
	builder, root, opts := assessmentFixture(t, nil)
	result := VerifyWithOptions(context.Background(), passingRunner(), builder, root, "verify_shape", opts)
	if result.Verdict != VerdictVerified {
		t.Fatalf("verdict = %q, want verified", result.Verdict)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"schemaVersion", "verifierRunId", "builderRunId", "verdict", "failedStage", "repairable", "diagnostics", "evidence", "promotion"} {
		value, ok := object[key]
		if !ok {
			t.Fatalf("verified result missing key %q", key)
		}
		if key == "failedStage" && value != nil {
			t.Fatalf("failedStage = %v, want null", value)
		}
	}
	if object["verdict"] != VerdictVerified {
		t.Fatalf("verdict = %v", object["verdict"])
	}
	promotion, ok := object["promotion"].(map[string]any)
	if !ok || promotion["allowed"] != false {
		t.Fatalf("promotion = %v, want allowed=false", object["promotion"])
	}
}

func writeCopyFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func dstHasContent(t *testing.T, dst, needle string) bool {
	t.Helper()
	found := false
	_ = filepath.Walk(dst, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if strings.Contains(string(data), needle) {
			found = true
		}
		return nil
	})
	return found
}

func TestCopyWorkspaceTreeRejectsOutsideFileSymlink(t *testing.T) {
	src := t.TempDir()
	writeCopyFixture(t, filepath.Join(src, "solution.py"), "ok")
	outside := t.TempDir()
	writeCopyFixture(t, filepath.Join(outside, "secret.txt"), "outside-secret")
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(src, "evil.py")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	dst := filepath.Join(t.TempDir(), "dst")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatalf("mkdir dst: %v", err)
	}
	if err := copyWorkspaceTree(src, dst); err == nil {
		t.Fatalf("copyWorkspaceTree = nil, want boundary error")
	} else if !strings.Contains(err.Error(), "outside the approved source boundary") {
		t.Fatalf("copyWorkspaceTree = %q, want boundary error", err.Error())
	}
	if dstHasContent(t, dst, "outside-secret") {
		t.Fatalf("outside content leaked into the assessment workspace")
	}
}

func TestCopyWorkspaceTreeRejectsOutsideDirSymlink(t *testing.T) {
	src := t.TempDir()
	writeCopyFixture(t, filepath.Join(src, "solution.py"), "ok")
	outside := t.TempDir()
	writeCopyFixture(t, filepath.Join(outside, "sub", "secret.txt"), "outside-secret")
	if err := os.Symlink(filepath.Join(outside, "sub"), filepath.Join(src, "linked")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	dst := filepath.Join(t.TempDir(), "dst")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatalf("mkdir dst: %v", err)
	}
	if err := copyWorkspaceTree(src, dst); err == nil {
		t.Fatalf("copyWorkspaceTree = nil, want boundary error")
	} else if !strings.Contains(err.Error(), "outside the approved source boundary") {
		t.Fatalf("copyWorkspaceTree = %q, want boundary error", err.Error())
	}
	if dstHasContent(t, dst, "outside-secret") {
		t.Fatalf("outside content leaked into the assessment workspace")
	}
}

func TestCopyWorkspaceTreeRejectsDirectoryCycle(t *testing.T) {
	src := t.TempDir()
	writeCopyFixture(t, filepath.Join(src, "solution.py"), "ok")
	writeCopyFixture(t, filepath.Join(src, "sub", "nested.txt"), "nested")
	// A directory symlink pointing back at an ancestor must fail closed
	// instead of recursing forever.
	if err := os.Symlink(src, filepath.Join(src, "sub", "loop")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	dst := filepath.Join(t.TempDir(), "dst")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatalf("mkdir dst: %v", err)
	}
	if err := copyWorkspaceTree(src, dst); err == nil {
		t.Fatalf("copyWorkspaceTree = nil, want cycle error")
	} else if !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("copyWorkspaceTree = %q, want cycle error", err.Error())
	}
}

func TestCopyWorkspaceTreeMaterializesInternalSymlink(t *testing.T) {
	src := t.TempDir()
	writeCopyFixture(t, filepath.Join(src, "solution.py"), "print('ok')")
	if err := os.Symlink(filepath.Join(src, "solution.py"), filepath.Join(src, "alias.py")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	dst := filepath.Join(t.TempDir(), "dst")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatalf("mkdir dst: %v", err)
	}
	if err := copyWorkspaceTree(src, dst); err != nil {
		t.Fatalf("copyWorkspaceTree: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dst, "alias.py"))
	if err != nil {
		t.Fatalf("read alias: %v", err)
	}
	if string(data) != "print('ok')" {
		t.Fatalf("alias content = %q, want materialized target content", data)
	}
	if info, err := os.Lstat(filepath.Join(dst, "alias.py")); err != nil {
		t.Fatalf("lstat alias: %v", err)
	} else if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("alias preserved as symlink, want materialized regular file")
	}
}
