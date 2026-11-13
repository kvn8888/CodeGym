package environmentverify

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type sequencedAssessor struct {
	calls   []AssessmentWork
	reports []stubAssessment
}

func (s *sequencedAssessor) Assess(_ context.Context, work AssessmentWork) (AssessmentReport, error) {
	s.calls = append(s.calls, work)
	index := len(s.calls) - 1
	if index >= len(s.reports) {
		return AssessmentReport{}, errNoSequencedReport
	}
	return s.reports[index].report, s.reports[index].err
}

const planEditablePath = "src/main/java/com/codegym/exercise/GreetingService.java"

func planFixture(t *testing.T) (BuilderResult, string, AssessmentPlan) {
	t.Helper()
	builder, root := verifyFixture(t)
	plan := AssessmentPlan{
		ArtifactID:      "spring-boot-di-intro-2026-09-29",
		ArtifactVersion: "0.1.0",
		Objective:       "Practice dependency injection with services",
		ReviewedBy:      "review-board-case-207",
		Reference: &AssessmentSolution{
			ID:                "reference",
			Role:              RoleGeneratedReference,
			Files:             map[string]string{planEditablePath: "reference"},
			ReviewedBy:        "review-board-case-207",
			ReviewedReference: "review://case-207/reference",
		},
		CorrectAlternatives: []AssessmentSolution{
			{ID: "alt-ok", Files: map[string]string{planEditablePath: "alt-ok"}},
			{ID: "alt-fail", Files: map[string]string{planEditablePath: "alt-fail"}},
			{ID: "alt-compile", Files: map[string]string{planEditablePath: "alt-compile"}},
		},
		WrongSolutions: []AssessmentSolution{
			{
				ID:                  "wrong-meaningful",
				Files:               map[string]string{planEditablePath: "wrong-meaningful"},
				DenominatorDecision: DenominatorMeaningful,
				ExpectedFailure: &ExpectedFailure{
					TestID:   "rejectsNullGreeting",
					Code:     "NULL_REJECTION",
					Expected: "rejected",
					Actual:   "accepted",
					Reason:   "null greetings must be rejected",
				},
			},
			{
				ID:                  "wrong-equivalent",
				Files:               map[string]string{planEditablePath: "wrong-equivalent"},
				DenominatorDecision: DenominatorEquivalent,
			},
			{
				ID:                  "wrong-invalid",
				Files:               map[string]string{planEditablePath: "wrong-invalid"},
				DenominatorDecision: DenominatorInvalid,
			},
		},
	}
	return builder, root, plan
}

var errNoSequencedReport = errors.New("sequenced assessor ran out of reports")

func TestAssessmentPlanCollectsOutcomesAndContinues(t *testing.T) {
	builder, root, plan := planFixture(t)
	compileErr := "cannot find symbol"
	assessor := &sequencedAssessor{reports: []stubAssessment{
		{report: passReport("acceptsGreeting", "rejectsNullGreeting")},
		{report: passReport("acceptsGreeting", "rejectsNullGreeting")},
		{report: failReport("acceptsGreeting", ClassAssertion)},
		{report: AssessmentReport{CompileError: &compileErr}},
		{report: failAssertionReport("rejectsNullGreeting", "NULL_REJECTION",
			"rejected", "accepted", TestOutcome{TestID: "acceptsGreeting", Passed: true})},
	}}
	opts := VerifyOptions{Assessor: assessor}

	result, collection := VerifyAssessmentPlanWithOptions(context.Background(), passingRunner(), builder, root, "verify_plan", opts, plan)

	if len(assessor.calls) != 5 {
		t.Fatalf("assessor calls = %d, want 5 (reference, 3 alternatives, 1 meaningful wrong)", len(assessor.calls))
	}
	if got := []string{
		assessor.calls[0].Role, assessor.calls[1].Role, assessor.calls[2].Role,
		assessor.calls[3].Role, assessor.calls[4].Role,
	}; !equalStrings(got, []string{
		RoleGeneratedReference, RoleCorrectAlternative, RoleCorrectAlternative,
		RoleCorrectAlternative, RoleWrongSolution,
	}) {
		t.Fatalf("assessed roles = %v", got)
	}
	if collection.Blocked {
		t.Fatalf("collection blocked = true, want false: %s", collection.BlockReason)
	}
	if collection.Reference == nil || !collection.Reference.Passed {
		t.Fatalf("reference outcome = %+v, want passed", collection.Reference)
	}
	if got := collection.CorrectAlternatives; len(got) != 3 ||
		!got[0].Passed ||
		got[1].Passed || got[1].FailureCategory != FailureCategoryTest ||
		got[2].ExecutionStatus != ExecutionCompileError || got[2].FailureCategory != FailureCategoryCompile {
		t.Fatalf("correct alternative outcomes = %+v", collection.CorrectAlternatives)
	}
	if len(collection.WrongSolutions) != 3 {
		t.Fatalf("wrong outcomes = %d, want 3", len(collection.WrongSolutions))
	}
	if !collection.WrongSolutions[0].Passed {
		t.Fatalf("meaningful wrong outcome = %+v, want intended failure matched", collection.WrongSolutions[0])
	}
	for _, index := range []int{1, 2} {
		outcome := collection.WrongSolutions[index]
		if outcome.ExecutionStatus != ExecutionExcluded || outcome.Passed {
			t.Fatalf("excluded wrong outcome = %+v", outcome)
		}
	}
	if collection.WrongSolutions[1].DenominatorDecision != DenominatorEquivalent ||
		collection.WrongSolutions[2].DenominatorDecision != DenominatorInvalid {
		t.Fatalf("denominator decisions = %+v", collection.WrongSolutions)
	}
	requireRejected(t, result, StageValidation, CodeCorrectSolutionFailed)
	if result.Repairable != true {
		t.Fatalf("repairable = false, want true for a correct-alternative failure")
	}
}

func TestAssessmentPlanReferenceFailureBlocks(t *testing.T) {
	builder, root, plan := planFixture(t)
	assessor := &sequencedAssessor{reports: []stubAssessment{
		{report: failReport("rejectsNullGreeting", ClassAssertion)},
	}}
	opts := VerifyOptions{Assessor: assessor}

	result, collection := VerifyAssessmentPlanWithOptions(context.Background(), passingRunner(), builder, root, "verify_plan_ref_fail", opts, plan)

	if len(assessor.calls) != 1 {
		t.Fatalf("assessor calls = %d, want only the reference", len(assessor.calls))
	}
	requireRejected(t, result, StageValidation, CodeAssessmentReferenceFailed)
	if !collection.Blocked {
		t.Fatalf("collection blocked = false, want true")
	}
	if collection.Reference == nil || collection.Reference.Passed {
		t.Fatalf("reference outcome = %+v, want failed", collection.Reference)
	}
	for _, outcome := range collection.CorrectAlternatives {
		if outcome.ExecutionStatus != ExecutionNotRun {
			t.Fatalf("correct alternative outcome = %+v, want not_run", outcome)
		}
	}
	for _, outcome := range collection.WrongSolutions {
		want := ExecutionNotRun
		if outcome.DenominatorDecision == DenominatorEquivalent || outcome.DenominatorDecision == DenominatorInvalid {
			want = ExecutionExcluded
		}
		if outcome.ExecutionStatus != want {
			t.Fatalf("wrong solution outcome = %+v, want %s", outcome, want)
		}
	}
}

func TestAssessmentPlanMissingReferenceBlocks(t *testing.T) {
	builder, root, plan := planFixture(t)
	plan.Reference = nil
	assessor := &sequencedAssessor{}
	opts := VerifyOptions{Assessor: assessor}

	result, collection := VerifyAssessmentPlanWithOptions(context.Background(), passingRunner(), builder, root, "verify_plan_no_ref", opts, plan)

	if len(assessor.calls) != 0 {
		t.Fatalf("assessor calls = %d, want 0 without an explicit reference", len(assessor.calls))
	}
	requireRejected(t, result, StageValidation, CodeAssessmentReferenceMissing)
	if !collection.Blocked || collection.Reference != nil {
		t.Fatalf("collection = %+v, want blocked without a reference outcome", collection)
	}
}

func TestAssessmentPlanWrongFailureIsNotOverallSuccess(t *testing.T) {
	builder, root, plan := planFixture(t)
	plan.Reference = &AssessmentSolution{ID: "reference", Files: map[string]string{planEditablePath: "reference"}}
	plan.CorrectAlternatives = []AssessmentSolution{{ID: "alt-ok", Files: map[string]string{planEditablePath: "alt-ok"}}}
	plan.WrongSolutions = []AssessmentSolution{
		{
			ID:                  "wrong-passes",
			Files:               map[string]string{planEditablePath: "wrong-passes"},
			DenominatorDecision: DenominatorMeaningful,
			ExpectedFailure: &ExpectedFailure{
				TestID: "rejectsNullGreeting", Code: "NULL_REJECTION",
				Expected: "rejected", Actual: "accepted", Reason: "null greetings must be rejected",
			},
		},
		{
			ID:                  "wrong-invalid",
			Files:               map[string]string{planEditablePath: "wrong-invalid"},
			DenominatorDecision: DenominatorInvalid,
		},
	}
	assessor := &sequencedAssessor{reports: []stubAssessment{
		{report: passReport("acceptsGreeting", "rejectsNullGreeting")},
		{report: passReport("acceptsGreeting", "rejectsNullGreeting")},
		{report: passReport("acceptsGreeting", "rejectsNullGreeting")},
	}}
	opts := VerifyOptions{Assessor: assessor}

	result, collection := VerifyAssessmentPlanWithOptions(context.Background(), passingRunner(), builder, root, "verify_plan_wrong", opts, plan)

	requireRejected(t, result, StageValidation, CodeWrongSolutionPassed)
	if collection.Blocked {
		t.Fatalf("collection blocked = true, want false")
	}
	if len(collection.WrongSolutions) != 2 || collection.WrongSolutions[0].Passed ||
		collection.WrongSolutions[1].ExecutionStatus != ExecutionExcluded {
		t.Fatalf("wrong outcomes = %+v", collection.WrongSolutions)
	}
	if len(assessor.calls) != 3 {
		t.Fatalf("assessor calls = %d, want reference, correct alternative, and meaningful wrong", len(assessor.calls))
	}
}

func TestAssessmentPlanMissingReviewEvidenceBlocks(t *testing.T) {
	builder, root, plan := planFixture(t)
	plan.ReviewedBy = ""
	assessor := &sequencedAssessor{}
	opts := VerifyOptions{Assessor: assessor}

	result, collection := VerifyAssessmentPlanWithOptions(context.Background(), passingRunner(), builder, root, "verify_plan_no_review", opts, plan)

	if len(assessor.calls) != 0 {
		t.Fatalf("assessor calls = %d, want 0 without plan-level review evidence", len(assessor.calls))
	}
	requireRejected(t, result, StageValidation, CodeReviewPending)
	if !collection.Blocked {
		t.Fatalf("collection blocked = false, want true")
	}
	if collection.BenchmarkEligible || collection.BenchmarkEligibility != BenchmarkEligibilityUnresolved {
		t.Fatalf("benchmark eligibility = %v/%q, want false/unresolved", collection.BenchmarkEligible, collection.BenchmarkEligibility)
	}
}

func TestAssessmentPlanPerSolutionReviewRemainsUnresolvedEligibility(t *testing.T) {
	// The enforced minimum is the plan-level reviewedBy field, matching the
	// legacy pair path. Per-solution review evidence is retained but its
	// sufficiency for benchmark eligibility is still an unresolved policy,
	// so execution success must never mark the collection eligible.
	builder, root, plan := planFixture(t)
	plan.Reference.ReviewedBy = ""
	plan.CorrectAlternatives = plan.CorrectAlternatives[:1]
	plan.CorrectAlternatives[0].ReviewedBy = ""
	plan.WrongSolutions = plan.WrongSolutions[:1]
	plan.WrongSolutions[0].ReviewedBy = ""
	assessor := &sequencedAssessor{reports: []stubAssessment{
		{report: passReport("acceptsGreeting", "rejectsNullGreeting")},
		{report: passReport("acceptsGreeting", "rejectsNullGreeting")},
		{report: failAssertionReport("rejectsNullGreeting", "NULL_REJECTION",
			"rejected", "accepted", TestOutcome{TestID: "acceptsGreeting", Passed: true})},
	}}
	opts := VerifyOptions{Assessor: assessor}

	result, collection := VerifyAssessmentPlanWithOptions(context.Background(), passingRunner(), builder, root, "verify_plan_unresolved_review", opts, plan)

	if result.Verdict != VerdictVerified {
		t.Fatalf("verdict = %q, diagnostics = %+v", result.Verdict, result.Diagnostics)
	}
	if collection.BenchmarkEligible || collection.BenchmarkEligibility != BenchmarkEligibilityUnresolved {
		t.Fatalf("benchmark eligibility = %v/%q, want false/unresolved", collection.BenchmarkEligible, collection.BenchmarkEligibility)
	}
	for _, fragment := range []string{"reference:reference", "correct-alternative:alt-ok", "wrong-solution:wrong-meaningful"} {
		if !strings.Contains(collection.BenchmarkEligibilityReason, fragment) {
			t.Fatalf("eligibility reason = %q, want %q", collection.BenchmarkEligibilityReason, fragment)
		}
	}
	if collection.Reference.ReviewStatus != ReviewStatusUnresolved ||
		collection.CorrectAlternatives[0].ReviewStatus != ReviewStatusUnresolved ||
		collection.WrongSolutions[0].ReviewStatus != ReviewStatusUnresolved {
		t.Fatalf("review statuses = reference:%q correct:%q wrong:%q",
			collection.Reference.ReviewStatus,
			collection.CorrectAlternatives[0].ReviewStatus,
			collection.WrongSolutions[0].ReviewStatus)
	}
}

func TestAssessmentPlanPreservesReportEvidenceOnTimeout(t *testing.T) {
	builder, root, plan := planFixture(t)
	exitCode := 124
	assessor := &sequencedAssessor{reports: []stubAssessment{{
		report: AssessmentReport{
			Tests:      []TestOutcome{{TestID: "acceptsGreeting", Passed: true}},
			ExitCode:   &exitCode,
			ElapsedMS:  int64Pointer(250),
			OutputTail: "timed out after partial output",
		},
		err: context.DeadlineExceeded,
	}}}
	opts := VerifyOptions{Assessor: assessor}

	result, collection := VerifyAssessmentPlanWithOptions(context.Background(), passingRunner(), builder, root, "verify_plan_timeout", opts, plan)

	requireRejected(t, result, StageTimeout, CodeStageTimeout)
	reference := collection.Reference
	if reference == nil || reference.ExecutionStatus != ExecutionTimeout {
		t.Fatalf("reference outcome = %+v, want timeout", reference)
	}
	if reference.Report.ExitCode == nil || *reference.Report.ExitCode != exitCode ||
		reference.Report.ElapsedMS == nil || *reference.Report.ElapsedMS != 250 ||
		reference.Report.OutputTail != "timed out after partial output" {
		t.Fatalf("timeout report evidence = %+v, want preserved command evidence", reference.Report)
	}
	if len(reference.Report.Tests) != 1 || !reference.Report.Tests[0].Passed {
		t.Fatalf("timeout test evidence = %+v, want the partial test record", reference.Report.Tests)
	}
}

func TestAssessmentPlanReferenceMustCoverRequiredRejectionTests(t *testing.T) {
	builder, root, plan := planFixture(t)
	plan.CorrectAlternatives = nil
	plan.WrongSolutions = plan.WrongSolutions[:1]
	assessor := &sequencedAssessor{reports: []stubAssessment{
		{report: passReport("unrelated-case")},
	}}
	opts := VerifyOptions{Assessor: assessor}

	result, collection := VerifyAssessmentPlanWithOptions(context.Background(), passingRunner(), builder, root, "verify_plan_ref_coverage", opts, plan)

	if len(assessor.calls) != 1 {
		t.Fatalf("assessor calls = %d, want only the reference", len(assessor.calls))
	}
	requireRejected(t, result, StageValidation, CodeAssessmentReferenceFailed)
	if !collection.Blocked {
		t.Fatalf("collection blocked = false, want true")
	}
	reference := collection.Reference
	if reference == nil || reference.Passed || reference.ExecutionStatus != ExecutionMalformed ||
		reference.FailureCategory != FailureCategoryMalformed ||
		!strings.Contains(reference.Message, "rejectsNullGreeting") {
		t.Fatalf("reference outcome = %+v, want missing required-test evidence", reference)
	}
}

func TestAssessmentPlanCorrectAlternativeCoverageFailureContinues(t *testing.T) {
	builder, root, plan := planFixture(t)
	plan.CorrectAlternatives = []AssessmentSolution{
		{ID: "alt-missing", Files: map[string]string{planEditablePath: "alt-missing"}},
		{ID: "alt-complete", Files: map[string]string{planEditablePath: "alt-complete"}},
	}
	plan.WrongSolutions = []AssessmentSolution{
		meaningfulWrongSolution("wrong-first", "first-required-test"),
		meaningfulWrongSolution("wrong-second", "second-required-test"),
	}
	assessor := &sequencedAssessor{reports: []stubAssessment{
		{report: passReport("first-required-test", "second-required-test")},
		{report: passReport("first-required-test")},
		{report: passReport("first-required-test", "second-required-test")},
		{report: failAssertionReport("first-required-test", "FIRST-REQUIRED-TEST",
			"expected-first-required-test", "actual-first-required-test",
			TestOutcome{TestID: "second-required-test", Passed: true})},
		{report: failAssertionReport("second-required-test", "SECOND-REQUIRED-TEST",
			"expected-second-required-test", "actual-second-required-test",
			TestOutcome{TestID: "first-required-test", Passed: true})},
	}}
	opts := VerifyOptions{Assessor: assessor}

	result, collection := VerifyAssessmentPlanWithOptions(context.Background(), passingRunner(), builder, root, "verify_plan_alt_coverage", opts, plan)

	if len(assessor.calls) != 5 {
		t.Fatalf("assessor calls = %d, want reference, both alternatives, and both wrong solutions", len(assessor.calls))
	}
	requireRejected(t, result, StageValidation, CodeAssessmentMalformed)
	if collection.Blocked {
		t.Fatalf("collection blocked = true, want an individual malformed alternative to continue")
	}
	missing := collection.CorrectAlternatives[0]
	if missing.Passed || missing.ExecutionStatus != ExecutionMalformed ||
		missing.FailureCategory != FailureCategoryMalformed ||
		!strings.Contains(missing.Message, "second-required-test") {
		t.Fatalf("missing-coverage alternative = %+v", missing)
	}
	if !collection.CorrectAlternatives[1].Passed {
		t.Fatalf("complete alternative = %+v, want passed", collection.CorrectAlternatives[1])
	}
	if !collection.WrongSolutions[0].Passed || !collection.WrongSolutions[1].Passed {
		t.Fatalf("wrong outcomes = %+v, want both intended failures matched", collection.WrongSolutions)
	}
}

func TestAssessmentPlanCompleteCoveragePasses(t *testing.T) {
	builder, root, plan := planFixture(t)
	plan.CorrectAlternatives = []AssessmentSolution{
		{ID: "alt-complete", Files: map[string]string{planEditablePath: "alt-complete"}},
	}
	plan.WrongSolutions = []AssessmentSolution{
		meaningfulWrongSolution("wrong-first", "first-required-test"),
		meaningfulWrongSolution("wrong-second", "second-required-test"),
		{
			ID:                  "wrong-equivalent",
			Files:               map[string]string{planEditablePath: "wrong-equivalent"},
			DenominatorDecision: DenominatorEquivalent,
			ExpectedFailure:     &ExpectedFailure{TestID: "must-not-be-required"},
		},
	}
	plan.Reference.ReviewedBy = "review-board-case-207"
	plan.CorrectAlternatives[0].ReviewedBy = "review-board-case-207"
	for i := range plan.WrongSolutions {
		plan.WrongSolutions[i].ReviewedBy = "review-board-case-207"
	}
	assessor := &sequencedAssessor{reports: []stubAssessment{
		{report: passReport("first-required-test", "second-required-test")},
		{report: passReport("first-required-test", "second-required-test")},
		{report: failAssertionReport("first-required-test", "FIRST-REQUIRED-TEST",
			"expected-first-required-test", "actual-first-required-test",
			TestOutcome{TestID: "second-required-test", Passed: true})},
		{report: failAssertionReport("second-required-test", "SECOND-REQUIRED-TEST",
			"expected-second-required-test", "actual-second-required-test",
			TestOutcome{TestID: "first-required-test", Passed: true})},
	}}
	opts := VerifyOptions{Assessor: assessor}

	result, collection := VerifyAssessmentPlanWithOptions(context.Background(), passingRunner(), builder, root, "verify_plan_complete_coverage", opts, plan)

	if result.Verdict != VerdictVerified {
		t.Fatalf("verdict = %q, diagnostics = %+v", result.Verdict, result.Diagnostics)
	}
	if len(assessor.calls) != 4 {
		t.Fatalf("assessor calls = %d, want excluded equivalent wrong solution not assessed", len(assessor.calls))
	}
	if !collection.Reference.Passed || !collection.CorrectAlternatives[0].Passed ||
		!collection.WrongSolutions[0].Passed || !collection.WrongSolutions[1].Passed {
		t.Fatalf("collection = %+v, want every eligible solution to pass", collection)
	}
	if collection.WrongSolutions[2].ExecutionStatus != ExecutionExcluded {
		t.Fatalf("equivalent wrong outcome = %+v, want excluded", collection.WrongSolutions[2])
	}
	if got := collection.RequiredTestIDs; !equalStrings(got, []string{"first-required-test", "second-required-test"}) {
		t.Fatalf("required test ids = %v, want only meaningful wrong-solution tests", got)
	}
	// Execution success and complete review metadata still do not establish
	// benchmark eligibility: that policy remains explicitly unresolved.
	if collection.Reference.ReviewStatus != ReviewStatusReviewed ||
		collection.CorrectAlternatives[0].ReviewStatus != ReviewStatusReviewed {
		t.Fatalf("review statuses = %+v, want reviewed metadata retained", collection)
	}
	if collection.BenchmarkEligible || collection.BenchmarkEligibility != BenchmarkEligibilityUnresolved ||
		!strings.Contains(collection.BenchmarkEligibilityReason, "policy is unresolved") {
		t.Fatalf("benchmark eligibility = %v/%q (%q), want false/unresolved",
			collection.BenchmarkEligible, collection.BenchmarkEligibility, collection.BenchmarkEligibilityReason)
	}
}

func meaningfulWrongSolution(id, testID string) AssessmentSolution {
	return AssessmentSolution{
		ID:                  id,
		Files:               map[string]string{planEditablePath: id},
		DenominatorDecision: DenominatorMeaningful,
		ExpectedFailure: &ExpectedFailure{
			TestID: testID, Code: strings.ToUpper(testID),
			Expected: "expected-" + testID, Actual: "actual-" + testID,
			Reason: "intended rejection for " + testID,
		},
	}
}

func TestAssessmentPlanRejectsNonzeroExitWithOnlyPassingEvidence(t *testing.T) {
	builder, root, plan := planFixture(t)
	plan.CorrectAlternatives = nil
	plan.WrongSolutions = nil
	assessor := &sequencedAssessor{reports: []stubAssessment{{
		report: AssessmentReport{
			Tests:    []TestOutcome{{TestID: "acceptsGreeting", Passed: true}},
			ExitCode: intPointer(1),
		},
	}}}

	result, collection := VerifyAssessmentPlanWithOptions(context.Background(), passingRunner(), builder, root, "verify_nonzero_pass", VerifyOptions{Assessor: assessor}, plan)

	if result.Verdict != VerdictRejected {
		t.Fatalf("verdict = %q, want rejected", result.Verdict)
	}
	if collection.Reference == nil || collection.Reference.Passed ||
		collection.Reference.FailureCategory != FailureCategoryTest {
		t.Fatalf("reference outcome = %+v, want a test failure from the command exit", collection.Reference)
	}
	if !collection.Blocked {
		t.Fatalf("collection blocked = false, want blocked after reference failure")
	}
}
