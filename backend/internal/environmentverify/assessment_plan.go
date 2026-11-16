package environmentverify

import (
	"fmt"
	"strings"
)

// Additional solution roles assessed by the benchmark plan path. The
// single-pair path keeps RoleCorrectSolution and RoleWrongSolution.
const (
	RoleGeneratedReference = "generated-reference"
	RoleCorrectAlternative = "correct-alternative"
)

// Wrong-solution denominator decisions. Only meaningful wrong solutions are
// assessed for the intended failure; equivalent and invalid entries stay
// visible in the collection but are excluded from that denominator.
const (
	DenominatorMeaningful = "meaningful"
	DenominatorEquivalent = "equivalent"
	DenominatorInvalid    = "invalid"
)

// Assessment execution outcomes.
const (
	ExecutionCompleted     AssessmentExecutionStatus = "completed"
	ExecutionNotRun        AssessmentExecutionStatus = "not_run"
	ExecutionTimeout       AssessmentExecutionStatus = "timeout"
	ExecutionCompileError  AssessmentExecutionStatus = "compile_error"
	ExecutionPlatformError AssessmentExecutionStatus = "platform_error"
	ExecutionMalformed     AssessmentExecutionStatus = "malformed"
	ExecutionExcluded      AssessmentExecutionStatus = "excluded"
)

// Review and benchmark-eligibility states. The existing minimum review gate
// is the plan-level reviewedBy field. Per-solution review metadata is
// retained for reporting, but which per-solution review evidence is
// sufficient for benchmark eligibility remains an unresolved policy. Until
// that policy exists, every collection is explicitly unresolved and not
// benchmark-eligible, even when execution succeeds.
const (
	ReviewStatusReviewed   = "reviewed"
	ReviewStatusUnresolved = "unresolved"

	BenchmarkEligibilityUnresolved = "unresolved"
)

// Failure categories recorded for executed solutions.
const (
	FailureCategoryCompile   = "compile_failure"
	FailureCategoryTest      = "test_failure"
	FailureCategoryAssertion = "assertion_mismatch"
	FailureCategoryMalformed = "malformed_report"
	FailureCategoryPlatform  = "platform_failure"
	FailureCategoryTimeout   = "timeout"
)

// AssessmentSolution is one candidate solution in a benchmark assessment
// plan. Files are replacements keyed by declared learner-editable paths.
// Review metadata is retained for benchmark reporting; it never promotes a
// fixture or placeholder review into benchmark eligibility.
type AssessmentSolution struct {
	ID                  string            `json:"id"`
	Role                string            `json:"role,omitempty"`
	Files               map[string]string `json:"files"`
	ExpectedFailure     *ExpectedFailure  `json:"expectedFailure,omitempty"`
	DenominatorDecision string            `json:"denominatorDecision,omitempty"`
	ReviewedBy          string            `json:"reviewedBy,omitempty"`
	ReviewedAt          string            `json:"reviewedAt,omitempty"`
	ReviewedReference   string            `json:"reviewedReference,omitempty"`
}

// AssessmentPlan carries the generated reference and every correct
// alternative and wrong solution for one artifact. The reference is
// explicit: it is never inferred from the alternatives or read from the
// artifact filesystem.
type AssessmentPlan struct {
	ArtifactID          string               `json:"artifactId"`
	ArtifactVersion     string               `json:"artifactVersion"`
	Objective           string               `json:"objective"`
	Reference           *AssessmentSolution  `json:"reference,omitempty"`
	CorrectAlternatives []AssessmentSolution `json:"correctAlternatives,omitempty"`
	WrongSolutions      []AssessmentSolution `json:"wrongSolutions,omitempty"`
	ReviewedBy          string               `json:"reviewedBy,omitempty"`
	ReviewedAt          string               `json:"reviewedAt,omitempty"`
	ReviewedReference   string               `json:"reviewedReference,omitempty"`
}

// AssessmentExecutionStatus classifies whether a candidate ran and how.
type AssessmentExecutionStatus string

// AssessmentOutcome is the per-solution result retained in the collection.
// Passed means the solution met its role criteria: correct/reference
// solutions passed every test; meaningful wrong solutions failed for the
// intended assertion. For excluded wrong solutions Passed is false and
// ExecutionStatus is excluded.
type AssessmentOutcome struct {
	ID                  string                    `json:"id"`
	Role                string                    `json:"role"`
	DenominatorDecision string                    `json:"denominatorDecision,omitempty"`
	ReviewedBy          string                    `json:"reviewedBy,omitempty"`
	ReviewedAt          string                    `json:"reviewedAt,omitempty"`
	ReviewedReference   string                    `json:"reviewedReference,omitempty"`
	ReviewStatus        string                    `json:"reviewStatus"`
	ExecutionStatus     AssessmentExecutionStatus `json:"executionStatus"`
	Passed              bool                      `json:"passed"`
	FailureCategory     string                    `json:"failureCategory,omitempty"`
	DiagnosticCode      string                    `json:"diagnosticCode,omitempty"`
	Message             string                    `json:"message,omitempty"`
	Report              AssessmentReport          `json:"report"`
}

// AssessmentCollection holds the generated-reference result plus every
// correct-alternative and wrong-solution outcome. It deliberately carries
// no aggregate metrics; callers decide how to present or score the case.
type AssessmentCollection struct {
	Reference                  *AssessmentOutcome  `json:"reference,omitempty"`
	CorrectAlternatives        []AssessmentOutcome `json:"correctAlternatives,omitempty"`
	WrongSolutions             []AssessmentOutcome `json:"wrongSolutions,omitempty"`
	RequiredTestIDs            []string            `json:"requiredTestIds"`
	Blocked                    bool                `json:"blocked"`
	BlockReason                string              `json:"blockReason,omitempty"`
	BenchmarkEligible          bool                `json:"benchmarkEligible"`
	BenchmarkEligibility       string              `json:"benchmarkEligibility"`
	BenchmarkEligibilityReason string              `json:"benchmarkEligibilityReason,omitempty"`
}

func firstDiagnosticMessage(result VerifierResult) string {
	if len(result.Diagnostics) > 0 {
		return result.Diagnostics[0].Message
	}
	return "verification did not reach assessment"
}

func newAssessmentCollection(plan AssessmentPlan) AssessmentCollection {
	collection := AssessmentCollection{RequiredTestIDs: requiredRejectionTests(plan)}
	if plan.Reference != nil {
		outcome := notRunAssessmentOutcome(*plan.Reference, RoleGeneratedReference, "not run")
		collection.Reference = &outcome
	}
	for i := range plan.CorrectAlternatives {
		collection.CorrectAlternatives = append(collection.CorrectAlternatives,
			notRunAssessmentOutcome(plan.CorrectAlternatives[i], RoleCorrectAlternative, "not run"))
	}
	for i := range plan.WrongSolutions {
		collection.WrongSolutions = append(collection.WrongSolutions, wrongAssessmentPlaceholder(plan.WrongSolutions[i]))
	}
	finalizeAssessmentEligibility(&collection)
	return collection
}

func reviewStatusFor(reviewedBy string) string {
	if strings.TrimSpace(reviewedBy) == "" {
		return ReviewStatusUnresolved
	}
	return ReviewStatusReviewed
}

func finalizeAssessmentEligibility(collection *AssessmentCollection) {
	collection.BenchmarkEligible = false
	collection.BenchmarkEligibility = BenchmarkEligibilityUnresolved
	unresolved := []string{}
	if collection.Reference != nil && collection.Reference.ReviewStatus == ReviewStatusUnresolved {
		unresolved = append(unresolved, "reference:"+collection.Reference.ID)
	}
	for i := range collection.CorrectAlternatives {
		if collection.CorrectAlternatives[i].ReviewStatus == ReviewStatusUnresolved {
			unresolved = append(unresolved, "correct-alternative:"+collection.CorrectAlternatives[i].ID)
		}
	}
	for i := range collection.WrongSolutions {
		if collection.WrongSolutions[i].ReviewStatus == ReviewStatusUnresolved {
			unresolved = append(unresolved, "wrong-solution:"+collection.WrongSolutions[i].ID)
		}
	}
	reason := "benchmark eligibility policy is unresolved; execution success alone does not establish eligibility"
	if len(unresolved) > 0 {
		reason += "; review evidence is unresolved for " + strings.Join(unresolved, ", ")
	}
	collection.BenchmarkEligibilityReason = reason
}

func wrongAssessmentPlaceholder(solution AssessmentSolution) AssessmentOutcome {
	outcome := notRunAssessmentOutcome(solution, RoleWrongSolution, "not run")
	if decision := strings.TrimSpace(solution.DenominatorDecision); decision != "" && decision != DenominatorMeaningful {
		outcome.ExecutionStatus = ExecutionExcluded
		outcome.Message = fmt.Sprintf("excluded from the meaningful wrong-solution denominator: %s", decision)
	}
	return outcome
}

func blockedAssessmentCollection(plan AssessmentPlan, reason string) AssessmentCollection {
	collection := newAssessmentCollection(plan)
	blockAssessmentCollection(&collection, reason)
	return collection
}

func blockAssessmentCollection(collection *AssessmentCollection, reason string) {
	collection.Blocked = true
	collection.BlockReason = reason
	if collection.Reference != nil && collection.Reference.ExecutionStatus == ExecutionNotRun {
		collection.Reference.Message = "not run: " + reason
	}
	for i := range collection.CorrectAlternatives {
		if collection.CorrectAlternatives[i].ExecutionStatus == ExecutionNotRun {
			collection.CorrectAlternatives[i].Message = "not run: " + reason
		}
	}
	for i := range collection.WrongSolutions {
		if collection.WrongSolutions[i].ExecutionStatus == ExecutionNotRun {
			collection.WrongSolutions[i].Message = "not run: " + reason
		}
	}
}

func notRunAssessmentOutcome(solution AssessmentSolution, role, message string) AssessmentOutcome {
	return AssessmentOutcome{
		ID:                  solution.ID,
		Role:                role,
		DenominatorDecision: solution.DenominatorDecision,
		ReviewedBy:          solution.ReviewedBy,
		ReviewedAt:          solution.ReviewedAt,
		ReviewedReference:   solution.ReviewedReference,
		ReviewStatus:        reviewStatusFor(solution.ReviewedBy),
		ExecutionStatus:     ExecutionNotRun,
		Message:             message,
	}
}

// assessPlan runs the collection after setup/build/test pass. It validates
// the plan, assesses the generated reference first, then keeps going through
// every correct alternative and meaningful wrong solution. Individual
// timeout or expectation failures are recorded and do not stop the
// collection. A missing/failing reference, parent cancellation, or shared
// platform fault blocks the collection and marks remaining work not_run.
func (v *verifier) assessPlan(workspaceDir string, manifest Manifest, plan AssessmentPlan) *VerifierResult {
	collection := newAssessmentCollection(plan)
	v.collection = &collection

	block := func(outcome *AssessmentOutcome, code, message, summary string, details map[string]any) *VerifierResult {
		if outcome != nil {
			outcome.Passed = false
			if outcome.DiagnosticCode == "" {
				outcome.DiagnosticCode = code
			}
			if outcome.Message == "" {
				outcome.Message = message
			}
		}
		blockAssessmentCollection(&collection, message)
		v.collection = &collection
		stage := StageValidation
		if outcome != nil && outcome.ExecutionStatus == ExecutionTimeout {
			stage = StageTimeout
		}
		return v.rejectWithCheck(stage, code, false, message, details, summary)
	}

	if err := validateAssessmentPlan(plan, manifest); err != nil {
		return block(nil, CodeAssessmentInputsMissing,
			fmt.Sprintf("Assessment plan is missing or invalid: %v.", err),
			"Assessment plan missing or invalid.",
			map[string]any{"error": err.Error()})
	}
	if strings.TrimSpace(plan.ReviewedBy) == "" {
		return block(nil, CodeReviewPending,
			"Assessment plan lacks human review evidence. Human review is pending; no verified verdict is issued.",
			"Human review pending.",
			map[string]any{"artifact": plan.ArtifactID, "reviewedBy": nil})
	}
	if plan.Reference == nil {
		return block(nil, CodeAssessmentReferenceMissing,
			"The assessment plan is missing the generated reference; no solution was assessed.",
			"Generated reference missing.",
			map[string]any{"field": "reference"})
	}
	requiredTests := requiredRejectionTests(plan)
	if v.assessor == nil {
		platform := v.platformFailure(nil, StageValidation,
			fmt.Errorf("assessment runner is not configured"), "No assessment runner is configured.")
		blockAssessmentCollection(&collection, "assessment runner is not configured")
		v.collection = &collection
		return &platform
	}

	reference, failure := v.assessOne(RoleGeneratedReference, manifest, workspaceDir, plan.Reference.Files)
	if failure != nil {
		outcome := assessmentFailureOutcome(*plan.Reference, RoleGeneratedReference, reference, failure)
		collection.Reference = &outcome
		return block(&outcome, outcome.DiagnosticCode,
			fmt.Sprintf("The generated reference could not be assessed: %s.", outcome.Message),
			"Generated reference did not complete.",
			map[string]any{"assessment": RoleGeneratedReference, "reference": plan.Reference.ID})
	}
	referenceOutcome := correctAssessmentOutcome(*plan.Reference, RoleGeneratedReference, reference, requiredTests)
	collection.Reference = &referenceOutcome
	if !referenceOutcome.Passed {
		return block(&referenceOutcome, CodeAssessmentReferenceFailed,
			fmt.Sprintf("The generated reference did not pass assessment: %s.", referenceOutcome.Message),
			"Generated reference failed assessment.",
			map[string]any{"assessment": RoleGeneratedReference, "reference": plan.Reference.ID,
				"failureCategory": referenceOutcome.FailureCategory})
	}
	v.appendCheck(EvidenceCheck{Stage: StageValidation,
		Summary: fmt.Sprintf("Generated reference %q passed %d assessment tests.", plan.Reference.ID, len(reference.Tests))})

	for i := range plan.CorrectAlternatives {
		solution := plan.CorrectAlternatives[i]
		if v.ctx.Err() != nil {
			return block(nil, CodePlatformFault,
				fmt.Sprintf("Assessment stopped before correct alternative %q: the parent context was cancelled.", solution.ID),
				"Assessment cancelled.", map[string]any{"assessment": RoleCorrectAlternative})
		}
		report, failure := v.assessOne(RoleCorrectAlternative, manifest, workspaceDir, solution.Files)
		if failure != nil {
			outcome := assessmentFailureOutcome(solution, RoleCorrectAlternative, report, failure)
			collection.CorrectAlternatives[i] = outcome
			if v.ctx.Err() != nil {
				return block(&outcome, CodePlatformFault,
					fmt.Sprintf("Assessment stopped at correct alternative %q: the parent context ended.", solution.ID),
					"Assessment cancelled.", map[string]any{"assessment": RoleCorrectAlternative})
			}
			if isPlatformFailure(*failure) {
				return block(&outcome, CodePlatformFault,
					fmt.Sprintf("Assessment stopped at correct alternative %q: the shared environment failed.", solution.ID),
					"Assessment platform fault.", map[string]any{"assessment": RoleCorrectAlternative})
			}
			continue
		}
		outcome := correctAssessmentOutcome(solution, RoleCorrectAlternative, report, requiredTests)
		collection.CorrectAlternatives[i] = outcome
		v.appendCheck(EvidenceCheck{Stage: StageValidation,
			Summary: fmt.Sprintf("Correct alternative %q %s.", solution.ID, assessmentSummary(outcome))})
	}

	for i := range plan.WrongSolutions {
		solution := plan.WrongSolutions[i]
		decision := strings.TrimSpace(solution.DenominatorDecision)
		if decision != DenominatorMeaningful {
			collection.WrongSolutions[i] = wrongAssessmentPlaceholder(solution)
			continue
		}
		if v.ctx.Err() != nil {
			return block(nil, CodePlatformFault,
				fmt.Sprintf("Assessment stopped before wrong solution %q: the parent context was cancelled.", solution.ID),
				"Assessment cancelled.", map[string]any{"assessment": RoleWrongSolution})
		}
		report, failure := v.assessOne(RoleWrongSolution, manifest, workspaceDir, solution.Files)
		if failure != nil {
			outcome := assessmentFailureOutcome(solution, RoleWrongSolution, report, failure)
			collection.WrongSolutions[i] = outcome
			if v.ctx.Err() != nil {
				return block(&outcome, CodePlatformFault,
					fmt.Sprintf("Assessment stopped at wrong solution %q: the parent context ended.", solution.ID),
					"Assessment cancelled.", map[string]any{"assessment": RoleWrongSolution})
			}
			if isPlatformFailure(*failure) {
				return block(&outcome, CodePlatformFault,
					fmt.Sprintf("Assessment stopped at wrong solution %q: the shared environment failed.", solution.ID),
					"Assessment platform fault.", map[string]any{"assessment": RoleWrongSolution})
			}
			continue
		}
		outcome := wrongAssessmentOutcome(solution, report)
		collection.WrongSolutions[i] = outcome
		v.appendCheck(EvidenceCheck{Stage: StageValidation,
			Summary: fmt.Sprintf("Wrong solution %q %s.", solution.ID, assessmentSummary(outcome))})
	}

	if outcome := firstFailedAssessmentOutcome(collection); outcome != nil {
		stage := StageValidation
		if outcome.ExecutionStatus == ExecutionTimeout {
			stage = StageTimeout
		}
		code := outcome.DiagnosticCode
		if code == "" {
			code = CodeAssessmentMalformed
		}
		return v.rejectWithCheck(stage, code, stage == StageValidation,
			fmt.Sprintf("%s %q did not meet its assessment criteria: %s.",
				roleTitle(outcome.Role), outcome.ID, outcome.Message),
			map[string]any{"assessment": outcome.Role, "solution": outcome.ID,
				"failureCategory": outcome.FailureCategory},
			roleTitle(outcome.Role)+" failed assessment.")
	}
	return nil
}

func firstFailedAssessmentOutcome(collection AssessmentCollection) *AssessmentOutcome {
	for i := range collection.CorrectAlternatives {
		outcome := &collection.CorrectAlternatives[i]
		if outcome.ExecutionStatus == ExecutionExcluded {
			continue
		}
		if !outcome.Passed {
			return outcome
		}
	}
	for i := range collection.WrongSolutions {
		outcome := &collection.WrongSolutions[i]
		if outcome.ExecutionStatus == ExecutionExcluded {
			continue
		}
		if !outcome.Passed {
			return outcome
		}
	}
	return nil
}

func isPlatformFailure(failure VerifierResult) bool {
	return len(failure.Diagnostics) > 0 && failure.Diagnostics[0].Code == CodePlatformFault
}

func assessmentFailureOutcome(solution AssessmentSolution, role string, report AssessmentReport, failure *VerifierResult) AssessmentOutcome {
	outcome := notRunAssessmentOutcome(solution, role, "")
	outcome.Report = report
	outcome.FailureCategory = FailureCategoryPlatform
	if failure.FailedStage != nil && *failure.FailedStage == StageTimeout {
		outcome.ExecutionStatus = ExecutionTimeout
		outcome.FailureCategory = FailureCategoryTimeout
	} else {
		outcome.ExecutionStatus = ExecutionPlatformError
	}
	if len(failure.Diagnostics) > 0 {
		outcome.DiagnosticCode = failure.Diagnostics[0].Code
		outcome.Message = failure.Diagnostics[0].Message
	}
	return outcome
}

func requiredRejectionTests(plan AssessmentPlan) []string {
	seen := map[string]struct{}{}
	required := []string{}
	for _, solution := range plan.WrongSolutions {
		if strings.TrimSpace(solution.DenominatorDecision) != DenominatorMeaningful || solution.ExpectedFailure == nil {
			continue
		}
		testID := strings.TrimSpace(solution.ExpectedFailure.TestID)
		if testID == "" {
			continue
		}
		if _, ok := seen[testID]; ok {
			continue
		}
		seen[testID] = struct{}{}
		required = append(required, testID)
	}
	return required
}

func correctAssessmentOutcome(solution AssessmentSolution, role string, report AssessmentReport, required []string) AssessmentOutcome {
	outcome := notRunAssessmentOutcome(solution, role, "")
	outcome.ExecutionStatus = ExecutionCompleted
	outcome.Report = report
	if issue := positiveReportIssue(report, required, role); issue != nil {
		outcome.ExecutionStatus = issue.Status
		outcome.FailureCategory = issue.Category
		outcome.DiagnosticCode = issue.Code
		outcome.Message = issue.Message
		return outcome
	}
	outcome.Passed = true
	return outcome
}

func wrongAssessmentOutcome(solution AssessmentSolution, report AssessmentReport) AssessmentOutcome {
	outcome := notRunAssessmentOutcome(solution, RoleWrongSolution, "")
	outcome.ExecutionStatus = ExecutionCompleted
	outcome.Report = report
	if solution.ExpectedFailure == nil {
		outcome.ExecutionStatus = ExecutionMalformed
		outcome.FailureCategory = FailureCategoryMalformed
		outcome.DiagnosticCode = CodeAssessmentInputsMissing
		outcome.Message = "meaningful wrong solution has no expected failure"
		return outcome
	}
	if issue := wrongReportIssue(report, *solution.ExpectedFailure, RoleWrongSolution); issue != nil {
		outcome.ExecutionStatus = issue.Status
		outcome.FailureCategory = issue.Category
		outcome.DiagnosticCode = issue.Code
		outcome.Message = issue.Message
		return outcome
	}
	outcome.Passed = true
	return outcome
}

func assessmentSummary(outcome AssessmentOutcome) string {
	if outcome.Passed {
		return "met its assessment criteria"
	}
	if outcome.Message == "" {
		return "did not meet its assessment criteria"
	}
	return "did not meet its assessment criteria: " + outcome.Message
}

// validateAssessmentPlan binds the plan to the manifest artifact and checks
// that every solution only replaces declared learner-editable files.
// Meaningful wrong solutions must carry a fully specified expected failure;
// equivalent and invalid entries are retained but not assessed.
func validateAssessmentPlan(plan AssessmentPlan, manifest Manifest) error {
	if strings.TrimSpace(plan.ArtifactID) != strings.TrimSpace(manifest.Artifact.ID) ||
		strings.TrimSpace(plan.ArtifactVersion) != strings.TrimSpace(manifest.Artifact.Version) {
		return fmt.Errorf("assessment plan targets artifact %q version %q, want manifest artifact %q version %q",
			strings.TrimSpace(plan.ArtifactID), strings.TrimSpace(plan.ArtifactVersion),
			strings.TrimSpace(manifest.Artifact.ID), strings.TrimSpace(manifest.Artifact.Version))
	}
	if strings.TrimSpace(plan.Objective) == "" {
		return fmt.Errorf("assessment plan: objective is required")
	}
	editable := make(map[string]struct{}, len(manifest.Workspace.LearnerEditable))
	for _, path := range manifest.Workspace.LearnerEditable {
		editable[strings.TrimSpace(path)] = struct{}{}
	}
	seen := map[string]struct{}{}
	if plan.Reference != nil {
		if err := validatePlanSolution("generated-reference", plan.Reference, editable, seen, false); err != nil {
			return err
		}
	}
	for i := range plan.CorrectAlternatives {
		if err := validatePlanSolution("correct-alternative", &plan.CorrectAlternatives[i], editable, seen, false); err != nil {
			return err
		}
	}
	for i := range plan.WrongSolutions {
		if err := validatePlanSolution("wrong-solution", &plan.WrongSolutions[i], editable, seen, true); err != nil {
			return err
		}
	}
	return nil
}

func validatePlanSolution(kind string, solution *AssessmentSolution, editable, seen map[string]struct{}, wrong bool) error {
	id := strings.TrimSpace(solution.ID)
	if id == "" {
		return fmt.Errorf("assessment plan: %s id is required", kind)
	}
	if _, ok := seen[id]; ok {
		return fmt.Errorf("assessment plan: solution id %q is duplicated", id)
	}
	seen[id] = struct{}{}
	if len(solution.Files) == 0 {
		return fmt.Errorf("assessment plan: %s %q has no files", kind, id)
	}
	for path := range solution.Files {
		if strings.TrimSpace(path) == "" {
			return fmt.Errorf("assessment plan: %s %q has a blank file path", kind, id)
		}
		if _, ok := editable[strings.TrimSpace(path)]; !ok {
			return fmt.Errorf("assessment plan: %s %q file %q is not a declared learner-editable file", kind, id, path)
		}
	}
	if !wrong {
		return nil
	}
	decision := strings.TrimSpace(solution.DenominatorDecision)
	switch decision {
	case DenominatorMeaningful, DenominatorEquivalent, DenominatorInvalid:
	default:
		return fmt.Errorf("assessment plan: wrong solution %q denominatorDecision must be meaningful, equivalent, or invalid, got %q",
			id, solution.DenominatorDecision)
	}
	if decision != DenominatorMeaningful {
		return nil
	}
	if solution.ExpectedFailure == nil {
		return fmt.Errorf("assessment plan: meaningful wrong solution %q expectedFailure is required", id)
	}
	return validateExpectedFailure(*solution.ExpectedFailure, "assessment plan: wrong solution "+id)
}
