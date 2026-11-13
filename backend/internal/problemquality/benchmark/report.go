package benchmark

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kvn8888/codegym/backend/internal/environmentverify"
	"github.com/kvn8888/codegym/backend/internal/problemquality"
)

const (
	CaseStatusPassed = "passed"
	CaseStatusFailed = "failed"
	CaseStatusError  = "error"
	CaseStatusNotRun = "not_run"
)

type Meta struct {
	RunID              string
	StartedAt          time.Time
	CompletedAt        time.Time
	TestedCommit       string
	Branch             string
	Dirty              bool
	BaselineKind       string
	ModelRevision      string
	PromptRevision     string
	CorpusRoot         string
	Selection          string
	SelectedCaseIDs    []string
	Timeout            time.Duration
	DevelopmentFixture bool
}

// CaseRun is one case's collected verification and assessment evidence.
// Err covers case loading, run-config, and execution failures that prevented
// assessment; NotRunReason covers work skipped after cancellation.
type CaseRun struct {
	Dir          string
	Case         *problemquality.Case
	Config       *RunConfig
	Identity     *ContentIdentity
	Collection   environmentverify.AssessmentCollection
	Verifier     environmentverify.VerifierResult
	Err          error
	NotRunReason string
	Cancelled    bool
	ElapsedMS    int64
}

type Report struct {
	RunID              string          `json:"run_id"`
	StartedAt          string          `json:"started_at"`
	CompletedAt        string          `json:"completed_at"`
	TestedCommit       string          `json:"tested_commit"`
	Branch             string          `json:"branch,omitempty"`
	TestedDirty        bool            `json:"tested_dirty"`
	PromptRevision     string          `json:"prompt_revision,omitempty"`
	ModelRevision      string          `json:"model_revision,omitempty"`
	BaselineKind       string          `json:"baseline_kind"`
	RuntimeConfig      map[string]any  `json:"runtime_config"`
	GenerationConfig   map[string]any  `json:"generation_config"`
	ElapsedMS          int64           `json:"elapsed_ms"`
	Preliminary        bool            `json:"preliminary"`
	DevelopmentFixture bool            `json:"development_fixture"`
	Corpus             *CorpusReport   `json:"corpus,omitempty"`
	SelectedCaseIDs    []string        `json:"selected_case_ids,omitempty"`
	Execution          ExecutionReport `json:"execution"`
	Summary            Summary         `json:"summary"`
	Cases              []CaseReport    `json:"cases"`
	Recommendations    []string        `json:"recommendations"`
}

type CorpusReport struct {
	Root      string `json:"root"`
	Selection string `json:"selection"`
	CaseCount int    `json:"case_count,omitempty"`
}

type ExecutionReport struct {
	Mode            string `json:"mode"`
	Local           bool   `json:"local"`
	Sandbox         bool   `json:"sandbox"`
	ModelOperations bool   `json:"model_operations"`
	PaidOperations  bool   `json:"paid_operations"`
	TimeoutSeconds  int    `json:"timeout_seconds"`
}

type Summary struct {
	BlindAgreementRate           any      `json:"blind_agreement_rate"`
	ReconciliationFrequency      any      `json:"reconciliation_frequency"`
	NonConvergenceRate           any      `json:"non_convergence_rate"`
	CorrectAlternativeAcceptance *float64 `json:"correct_alternative_acceptance_rate,omitempty"`
	WrongSolutionRejection       *float64 `json:"wrong_solution_rejection_rate,omitempty"`
	AmbiguousSpecificationRate   *float64 `json:"ambiguous_specification_rate,omitempty"`
	CasesTotal                   int      `json:"cases_total"`
	CasesPassed                  int      `json:"cases_passed"`
	CasesFailed                  int      `json:"cases_failed"`
	CasesError                   int      `json:"cases_error"`
	CasesNotRun                  int      `json:"cases_not_run"`
	DevelopmentFixtureCases      int      `json:"development_fixture_cases"`
	EligibleCases                int      `json:"eligible_cases"`
	UnresolvedEligibilityCases   int      `json:"unresolved_eligibility_cases"`
}

type CaseReport struct {
	CaseID                     string            `json:"case_id"`
	CorpusVersion              string            `json:"corpus_version,omitempty"`
	Technology                 string            `json:"technology"`
	Status                     string            `json:"status"`
	Generation                 Generation        `json:"generation"`
	Provenance                 *CaseProvenance   `json:"provenance,omitempty"`
	ContentIdentity            *ContentIdentity  `json:"content_identity,omitempty"`
	Artifact                   *ArtifactBinding  `json:"artifact,omitempty"`
	Reference                  *ReferenceBinding `json:"reference,omitempty"`
	ReferenceOutcome           *CorrectOutcome   `json:"reference_outcome,omitempty"`
	CorrectAlternatives        []CorrectOutcome  `json:"correct_alternatives,omitempty"`
	WrongSolutions             []WrongOutcome    `json:"wrong_solutions,omitempty"`
	RequiredTestIDs            []string          `json:"required_test_ids,omitempty"`
	ComparatorConfig           string            `json:"comparator_config,omitempty"`
	ComparatorResult           string            `json:"comparator_result,omitempty"`
	AmbiguousSpec              bool              `json:"ambiguous_spec"`
	DevelopmentFixture         bool              `json:"development_fixture"`
	ExcludedFromReviewedTotals bool              `json:"excluded_from_reviewed_totals"`
	Blocked                    bool              `json:"blocked"`
	BlockReason                string            `json:"block_reason,omitempty"`
	BenchmarkEligible          bool              `json:"benchmark_eligible"`
	BenchmarkEligibility       string            `json:"benchmark_eligibility"`
	BenchmarkEligibilityReason string            `json:"benchmark_eligibility_reason,omitempty"`
	ReviewStatus               string            `json:"review_status"`
	RunConfig                  string            `json:"run_config,omitempty"`
	FailureCategory            string            `json:"failure_category,omitempty"`
	FailureDetails             string            `json:"failure_details,omitempty"`
	ElapsedMS                  int64             `json:"elapsed_ms"`
}

type Generation struct {
	BlindAgreement          *bool   `json:"blind_agreement"`
	ReconciliationCount     *int    `json:"reconciliation_count"`
	ReconciliationDirection *string `json:"reconciliation_direction"`
	NonConvergence          *bool   `json:"non_convergence"`
	Status                  string  `json:"status"`
}

// CaseProvenance separates two facts the report must not conflate: this
// assessment run performed no generation, and the saved exercise may or may
// not have known generation provenance. Fixtures legitimately have no model
// or prompt provenance; a saved generated exercise with gaps reports
// unavailable rather than pretending the question does not apply.
type CaseProvenance struct {
	GenerationDuringAssessment bool           `json:"generation_during_assessment"`
	SavedExerciseProvenance    string         `json:"saved_exercise_provenance"`
	ModelRevision              string         `json:"model_revision"`
	ModelRevisionStatus        string         `json:"model_revision_status"`
	PromptRevision             string         `json:"prompt_revision"`
	PromptRevisionStatus       string         `json:"prompt_revision_status"`
	GenerationConfig           map[string]any `json:"generation_config,omitempty"`
}

type ArtifactBinding struct {
	ID           string `json:"id"`
	Version      string `json:"version"`
	Root         string `json:"root,omitempty"`
	ManifestPath string `json:"manifest_path,omitempty"`
}

type ReferenceBinding struct {
	ID                string   `json:"id"`
	Role              string   `json:"role,omitempty"`
	Files             []string `json:"files"`
	ReviewedBy        string   `json:"reviewed_by,omitempty"`
	ReviewedAt        string   `json:"reviewed_at,omitempty"`
	ReviewedReference string   `json:"reviewed_reference,omitempty"`
	ReviewStatus      string   `json:"review_status,omitempty"`
	ArtifactID        string   `json:"artifact_id,omitempty"`
	ArtifactVersion   string   `json:"artifact_version,omitempty"`
}

type CorrectOutcome struct {
	ID                string         `json:"id"`
	Role              string         `json:"role,omitempty"`
	Accepted          bool           `json:"accepted"`
	Passed            bool           `json:"passed"`
	ExecutionStatus   string         `json:"execution_status"`
	FailureCategory   string         `json:"failure_category,omitempty"`
	DiagnosticCode    string         `json:"diagnostic_code,omitempty"`
	Message           string         `json:"message,omitempty"`
	Failure           string         `json:"failure,omitempty"`
	ExitCode          *int           `json:"exit_code,omitempty"`
	ElapsedMS         *int64         `json:"elapsed_ms,omitempty"`
	ReviewStatus      string         `json:"review_status,omitempty"`
	ReviewedBy        string         `json:"reviewed_by,omitempty"`
	ReviewedAt        string         `json:"reviewed_at,omitempty"`
	ReviewedReference string         `json:"reviewed_reference,omitempty"`
	RequiredTestIDs   []string       `json:"required_test_ids,omitempty"`
	Tests             []TestEvidence `json:"tests,omitempty"`
	OutputTail        string         `json:"output_tail,omitempty"`
}

type WrongOutcome struct {
	ID                     string              `json:"id"`
	Role                   string              `json:"role,omitempty"`
	Rejected               bool                `json:"rejected"`
	Passed                 bool                `json:"passed"`
	Excluded               bool                `json:"excluded,omitempty"`
	DenominatorDecision    string              `json:"denominator_decision,omitempty"`
	Surviving              bool                `json:"surviving,omitempty"`
	IntendedFailureMatched bool                `json:"intended_failure_matched"`
	IntendedFailure        *FailureExpectation `json:"intended_failure,omitempty"`
	ExecutionStatus        string              `json:"execution_status"`
	FailureCategory        string              `json:"failure_category,omitempty"`
	DiagnosticCode         string              `json:"diagnostic_code,omitempty"`
	Message                string              `json:"message,omitempty"`
	Failure                string              `json:"failure,omitempty"`
	ExitCode               *int                `json:"exit_code,omitempty"`
	ElapsedMS              *int64              `json:"elapsed_ms,omitempty"`
	ReviewStatus           string              `json:"review_status,omitempty"`
	ReviewedBy             string              `json:"reviewed_by,omitempty"`
	ReviewedAt             string              `json:"reviewed_at,omitempty"`
	ReviewedReference      string              `json:"reviewed_reference,omitempty"`
	RequiredTestIDs        []string            `json:"required_test_ids,omitempty"`
	Tests                  []TestEvidence      `json:"tests,omitempty"`
	OutputTail             string              `json:"output_tail,omitempty"`
}

type FailureExpectation struct {
	TestID   string `json:"test_id"`
	Code     string `json:"code"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
	Reason   string `json:"reason"`
}

type TestEvidence struct {
	TestID       string             `json:"test_id"`
	Passed       bool               `json:"passed"`
	FailureClass string             `json:"failure_class,omitempty"`
	Message      string             `json:"message,omitempty"`
	Assertion    *AssertionEvidence `json:"assertion,omitempty"`
}

type AssertionEvidence struct {
	Code     string `json:"code"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
}

func BuildReport(meta Meta, runs []CaseRun) Report {
	if meta.StartedAt.IsZero() {
		meta.StartedAt = time.Now().UTC()
	}
	if meta.CompletedAt.IsZero() {
		meta.CompletedAt = time.Now().UTC()
	}
	rawModelRevision := strings.TrimSpace(meta.ModelRevision)
	rawPromptRevision := strings.TrimSpace(meta.PromptRevision)
	modelRevision := rawModelRevision
	if modelRevision == "" {
		modelRevision = unavailableReason(meta.DevelopmentFixture)
	}
	promptRevision := rawPromptRevision
	if promptRevision == "" {
		promptRevision = unavailableReason(meta.DevelopmentFixture)
	}
	report := Report{
		RunID:              meta.RunID,
		StartedAt:          meta.StartedAt.UTC().Format(time.RFC3339Nano),
		CompletedAt:        meta.CompletedAt.UTC().Format(time.RFC3339Nano),
		TestedCommit:       meta.TestedCommit,
		Branch:             meta.Branch,
		TestedDirty:        meta.Dirty,
		PromptRevision:     promptRevision,
		ModelRevision:      modelRevision,
		BaselineKind:       meta.BaselineKind,
		ElapsedMS:          meta.CompletedAt.Sub(meta.StartedAt).Milliseconds(),
		Preliminary:        true,
		DevelopmentFixture: meta.DevelopmentFixture,
		SelectedCaseIDs:    append([]string(nil), meta.SelectedCaseIDs...),
		RuntimeConfig: map[string]any{
			"execution":           "local-shell",
			"sandbox":             "none",
			"network_isolation":   "not_provided",
			"workspace_isolation": "verifier_temporary_directories",
			"command_source":      "artifact_manifest",
			"assessment_protocol": environmentverify.TestOutcomeProtocol,
			"timeout_seconds":     int(meta.Timeout.Seconds()),
		},
		GenerationConfig: map[string]any{
			"status":                             "not_applicable",
			"usage":                              "not_applicable",
			"model_operations":                   false,
			"paid_operations":                    false,
			"baseline_kind":                      meta.BaselineKind,
			"assessment_run_generation_occurred": false,
			"saved_exercise_provenance":          savedProvenanceFor(rawModelRevision, rawPromptRevision, meta.DevelopmentFixture),
			"note": "No generation, model calls, token usage, or paid sandboxes were invoked by this " +
				"assessment run. That describes this run only; it is not evidence that the paid-budget milestone was met.",
		},
		Execution: ExecutionReport{
			Mode:            "local-development",
			Local:           true,
			Sandbox:         false,
			ModelOperations: false,
			PaidOperations:  false,
			TimeoutSeconds:  int(meta.Timeout.Seconds()),
		},
	}
	if strings.TrimSpace(meta.CorpusRoot) != "" || strings.TrimSpace(meta.Selection) != "" {
		report.Corpus = &CorpusReport{Root: meta.CorpusRoot, Selection: meta.Selection, CaseCount: len(runs)}
	}
	report.Cases = make([]CaseReport, 0, len(runs))
	recommendations := []string{
		"Treat this report as preliminary local development evidence, not as broad trust in generated problems.",
		"Benchmark eligibility remains unresolved until the shared per-solution review policy is agreed; execution success alone is not eligibility.",
		"This local run does not establish sandbox network or memory isolation and does not satisfy the paid-budget milestone.",
	}
	for _, run := range runs {
		caseReport, recs := buildCaseReport(run, meta)
		report.Cases = append(report.Cases, caseReport)
		recommendations = append(recommendations, recs...)
	}
	report.Recommendations = append(report.Recommendations, recommendations...)
	finalizeSummary(&report)
	return report
}

func buildCaseReport(run CaseRun, meta Meta) (CaseReport, []string) {
	config := run.Config
	fixture := config != nil && config.DevelopmentFixture
	caseReport := CaseReport{
		CaseID:                     caseIDForRun(run),
		Technology:                 "unknown",
		Status:                     CaseStatusError,
		Generation:                 Generation{Status: "not_applicable"},
		Provenance:                 caseProvenance(fixture, config, meta.ModelRevision, meta.PromptRevision),
		ContentIdentity:            run.Identity,
		DevelopmentFixture:         fixture,
		ExcludedFromReviewedTotals: fixture,
		BenchmarkEligibility:       environmentverify.BenchmarkEligibilityUnresolved,
		ReviewStatus:               environmentverify.ReviewStatusUnresolved,
		ElapsedMS:                  run.ElapsedMS,
	}
	recommendations := []string{}
	if run.Case != nil {
		caseReport.Technology = run.Case.Technology
		caseReport.CorpusVersion = run.Case.Version
		caseReport.ComparatorConfig = comparatorConfig(*run.Case)
		caseReport.ComparatorResult = "not_measured_by_harness"
		if run.Case.Review.AmbiguousSpec != nil {
			caseReport.AmbiguousSpec = *run.Case.Review.AmbiguousSpec
		}
		if strings.TrimSpace(run.Case.Review.ReviewedBy) != "" {
			caseReport.ReviewStatus = environmentverify.ReviewStatusReviewed
		}
	}
	if config != nil {
		caseReport.RunConfig = config.Path
		caseReport.Artifact = &ArtifactBinding{
			ID:           config.ArtifactID,
			Version:      config.ArtifactVersion,
			Root:         config.ArtifactRoot,
			ManifestPath: config.ArtifactManifestPath(),
		}
		caseReport.Reference = &ReferenceBinding{
			ID:                config.ReferenceData.ID,
			Role:              environmentverify.RoleGeneratedReference,
			Files:             config.ReferenceTargets(),
			ReviewedBy:        config.ReferenceData.ReviewedBy,
			ReviewedAt:        config.ReferenceData.ReviewedAt,
			ReviewedReference: config.ReferenceData.ReviewedReference,
			ReviewStatus:      reviewStatus(config.ReferenceData.ReviewedBy),
			ArtifactID:        config.ArtifactID,
			ArtifactVersion:   config.ArtifactVersion,
		}
	}
	if run.NotRunReason != "" {
		caseReport.Status = CaseStatusNotRun
		caseReport.Blocked = true
		caseReport.BlockReason = run.NotRunReason
		caseReport.FailureDetails = run.NotRunReason
		return caseReport, nil
	}
	if run.Err != nil {
		caseReport.Status = CaseStatusError
		caseReport.FailureCategory = "setup_failure"
		caseReport.FailureDetails = run.Err.Error()
		caseReport.Blocked = true
		caseReport.BlockReason = run.Err.Error()
		return caseReport, nil
	}
	collection := run.Collection
	caseReport.Blocked = collection.Blocked
	caseReport.BlockReason = collection.BlockReason
	caseReport.RequiredTestIDs = append([]string(nil), collection.RequiredTestIDs...)
	caseReport.BenchmarkEligible = collection.BenchmarkEligible
	if collection.BenchmarkEligibility != "" {
		caseReport.BenchmarkEligibility = collection.BenchmarkEligibility
	}
	caseReport.BenchmarkEligibilityReason = collection.BenchmarkEligibilityReason
	caseReport.ReferenceOutcome = correctOutcome(collection.Reference)
	caseReport.CorrectAlternatives = make([]CorrectOutcome, 0, len(collection.CorrectAlternatives))
	for i := range collection.CorrectAlternatives {
		caseReport.CorrectAlternatives = append(caseReport.CorrectAlternatives, *correctOutcome(&collection.CorrectAlternatives[i]))
	}
	expected := expectedFailures(run.Case)
	caseReport.WrongSolutions = make([]WrongOutcome, 0, len(collection.WrongSolutions))
	for i := range collection.WrongSolutions {
		item := wrongOutcome(collection.WrongSolutions[i], expected[collection.WrongSolutions[i].ID])
		caseReport.WrongSolutions = append(caseReport.WrongSolutions, item)
		if item.Surviving {
			recommendations = append(recommendations,
				fmt.Sprintf("Strengthen the fixture tests for case %q: wrong solution %q survived the current assertions.", caseReport.CaseID, item.ID))
		}
	}
	status, category, details := assessCaseStatus(run)
	if run.Cancelled {
		status = CaseStatusNotRun
		if details == "" {
			details = "benchmark cancelled; partial assessment evidence is preserved"
		}
	}
	caseReport.Status = status
	caseReport.FailureCategory = category
	caseReport.FailureDetails = details
	if status == CaseStatusError {
		caseReport.Blocked = true
		if caseReport.BlockReason == "" {
			caseReport.BlockReason = details
		}
	}
	if status == CaseStatusFailed && category == environmentverify.FailureCategoryAssertion {
		recommendations = append(recommendations,
			fmt.Sprintf("Review assertion evidence for case %q; at least one wrong solution did not fail for its intended assertion.", caseReport.CaseID))
	}
	return caseReport, recommendations
}

func caseIDForRun(run CaseRun) string {
	if run.Case != nil && strings.TrimSpace(run.Case.CaseID) != "" {
		return run.Case.CaseID
	}
	if strings.TrimSpace(run.Dir) != "" {
		return filepath.Base(run.Dir)
	}
	return "unknown-case"
}

func comparatorConfig(c problemquality.Case) string {
	kind := strings.TrimSpace(c.ComparatorExpectations.Default.Kind)
	notes := strings.TrimSpace(c.ComparatorExpectations.Notes)
	if notes == "" {
		return kind
	}
	return kind + ": " + notes
}

func reviewStatus(reviewedBy string) string {
	if strings.TrimSpace(reviewedBy) == "" {
		return environmentverify.ReviewStatusUnresolved
	}
	return environmentverify.ReviewStatusReviewed
}

func unavailableReason(fixture bool) string {
	if fixture {
		return "not_applicable"
	}
	return "unavailable"
}

func savedProvenanceFor(model, prompt string, fixture bool) string {
	known := strings.TrimSpace(model) != "" && strings.TrimSpace(prompt) != ""
	switch {
	case known:
		return "known"
	case fixture:
		return "not_applicable"
	default:
		return "unavailable"
	}
}

func caseProvenance(fixture bool, config *RunConfig, runModel, runPrompt string) *CaseProvenance {
	model := strings.TrimSpace(runModel)
	prompt := strings.TrimSpace(runPrompt)
	var generationConfig map[string]any
	if config != nil && config.GenerationProvenance != nil {
		if value := strings.TrimSpace(config.GenerationProvenance.ModelRevision); value != "" {
			model = value
		}
		if value := strings.TrimSpace(config.GenerationProvenance.PromptRevision); value != "" {
			prompt = value
		}
		generationConfig = config.GenerationProvenance.Config
	}
	modelStatus := "unavailable"
	promptStatus := "unavailable"
	if model != "" {
		modelStatus = "known"
	} else if fixture {
		modelStatus = "not_applicable"
	}
	if prompt != "" {
		promptStatus = "known"
	} else if fixture {
		promptStatus = "not_applicable"
	}
	modelValue := model
	if modelValue == "" {
		modelValue = modelStatus
	}
	promptValue := prompt
	if promptValue == "" {
		promptValue = promptStatus
	}
	return &CaseProvenance{
		GenerationDuringAssessment: false,
		SavedExerciseProvenance:    savedProvenanceFor(model, prompt, fixture),
		ModelRevision:              modelValue,
		ModelRevisionStatus:        modelStatus,
		PromptRevision:             promptValue,
		PromptRevisionStatus:       promptStatus,
		GenerationConfig:           generationConfig,
	}
}

func correctOutcome(outcome *environmentverify.AssessmentOutcome) *CorrectOutcome {
	if outcome == nil {
		return nil
	}
	result := &CorrectOutcome{
		ID:                outcome.ID,
		Role:              outcome.Role,
		Accepted:          outcome.Passed,
		Passed:            outcome.Passed,
		ExecutionStatus:   string(outcome.ExecutionStatus),
		FailureCategory:   outcome.FailureCategory,
		DiagnosticCode:    outcome.DiagnosticCode,
		Message:           outcome.Message,
		Failure:           outcome.Message,
		ExitCode:          outcome.Report.ExitCode,
		ElapsedMS:         outcome.Report.ElapsedMS,
		ReviewStatus:      outcome.ReviewStatus,
		ReviewedBy:        outcome.ReviewedBy,
		ReviewedAt:        outcome.ReviewedAt,
		ReviewedReference: outcome.ReviewedReference,
		Tests:             testEvidence(outcome.Report.Tests),
		OutputTail:        outcome.Report.OutputTail,
	}
	if len(result.Tests) == 0 {
		result.Tests = nil
	}
	return result
}

func wrongOutcome(outcome environmentverify.AssessmentOutcome, expected *FailureExpectation) WrongOutcome {
	excluded := outcome.ExecutionStatus == environmentverify.ExecutionExcluded
	result := WrongOutcome{
		ID:                  outcome.ID,
		Role:                outcome.Role,
		Rejected:            outcome.Passed && !excluded,
		Passed:              outcome.Passed,
		Excluded:            excluded,
		DenominatorDecision: outcome.DenominatorDecision,
		Surviving: !excluded && strings.TrimSpace(outcome.DenominatorDecision) == environmentverify.DenominatorMeaningful &&
			outcome.ExecutionStatus == environmentverify.ExecutionCompleted && !outcome.Passed &&
			outcome.DiagnosticCode == environmentverify.CodeWrongSolutionPassed,
		IntendedFailure:   expected,
		ExecutionStatus:   string(outcome.ExecutionStatus),
		FailureCategory:   outcome.FailureCategory,
		DiagnosticCode:    outcome.DiagnosticCode,
		Message:           outcome.Message,
		Failure:           outcome.Message,
		ExitCode:          outcome.Report.ExitCode,
		ElapsedMS:         outcome.Report.ElapsedMS,
		ReviewStatus:      outcome.ReviewStatus,
		ReviewedBy:        outcome.ReviewedBy,
		ReviewedAt:        outcome.ReviewedAt,
		ReviewedReference: outcome.ReviewedReference,
		Tests:             testEvidence(outcome.Report.Tests),
		OutputTail:        outcome.Report.OutputTail,
	}
	if expected != nil {
		result.IntendedFailureMatched = intendedFailureMatched(outcome.Report.Tests, *expected)
	}
	if len(result.Tests) == 0 {
		result.Tests = nil
	}
	return result
}

func expectedFailures(c *problemquality.Case) map[string]*FailureExpectation {
	expected := map[string]*FailureExpectation{}
	if c == nil {
		return expected
	}
	for _, wrong := range c.WrongSolutions {
		if wrong.IntendedFailure.Expected == nil || wrong.IntendedFailure.Actual == nil {
			continue
		}
		expected[wrong.ID] = &FailureExpectation{
			TestID:   wrong.IntendedFailure.TestID,
			Code:     wrong.IntendedFailure.Code,
			Expected: *wrong.IntendedFailure.Expected,
			Actual:   *wrong.IntendedFailure.Actual,
			Reason:   wrong.IntendedFailure.Reason,
		}
	}
	return expected
}

func intendedFailureMatched(tests []environmentverify.TestOutcome, expected FailureExpectation) bool {
	for _, test := range tests {
		if test.TestID != expected.TestID || test.Passed {
			continue
		}
		assertion := test.Assertion
		if assertion == nil {
			return false
		}
		return assertion.Code == expected.Code &&
			assertion.Expected == expected.Expected &&
			assertion.Actual == expected.Actual
	}
	return false
}

func testEvidence(tests []environmentverify.TestOutcome) []TestEvidence {
	evidence := make([]TestEvidence, 0, len(tests))
	for _, test := range tests {
		item := TestEvidence{
			TestID:       test.TestID,
			Passed:       test.Passed,
			FailureClass: test.FailureClass,
			Message:      test.Message,
		}
		if test.Assertion != nil {
			item.Assertion = &AssertionEvidence{
				Code:     test.Assertion.Code,
				Expected: test.Assertion.Expected,
				Actual:   test.Assertion.Actual,
			}
		}
		evidence = append(evidence, item)
	}
	return evidence
}

func assessCaseStatus(run CaseRun) (string, string, string) {
	collection := run.Collection
	if collection.Reference != nil {
		if !collection.Reference.Passed {
			return CaseStatusFailed, firstCategory(collection.Reference.FailureCategory), collection.Reference.Message
		}
	}
	for i := range collection.CorrectAlternatives {
		outcome := &collection.CorrectAlternatives[i]
		if !outcome.Passed {
			return CaseStatusFailed, firstCategory(outcome.FailureCategory), outcome.Message
		}
	}
	for i := range collection.WrongSolutions {
		outcome := &collection.WrongSolutions[i]
		if outcome.ExecutionStatus == environmentverify.ExecutionExcluded {
			continue
		}
		if !outcome.Passed {
			return CaseStatusFailed, firstCategory(outcome.FailureCategory), outcome.Message
		}
	}
	if collection.Blocked || (collection.Reference == nil && len(collection.CorrectAlternatives) == 0 && len(collection.WrongSolutions) == 0) {
		category, details := verifierFailure(run.Verifier)
		return CaseStatusError, category, details
	}
	return CaseStatusPassed, "", ""
}

func firstCategory(category string) string {
	if strings.TrimSpace(category) != "" {
		return category
	}
	return environmentverify.FailureCategoryTest
}

func verifierFailure(result environmentverify.VerifierResult) (string, string) {
	if len(result.Diagnostics) == 0 {
		return "setup_failure", "verification produced no assessment outcomes"
	}
	diagnostic := result.Diagnostics[0]
	category := "setup_failure"
	stage := ""
	if result.FailedStage != nil {
		stage = *result.FailedStage
	}
	switch stage {
	case environmentverify.StageTimeout:
		category = environmentverify.FailureCategoryTimeout
	case environmentverify.StagePlatform:
		category = environmentverify.FailureCategoryPlatform
	case environmentverify.StageBuild:
		category = environmentverify.FailureCategoryCompile
	case environmentverify.StageTest:
		category = environmentverify.FailureCategoryTest
	case environmentverify.StageSetup, environmentverify.StageManifest, environmentverify.StageValidation:
		category = "setup_failure"
	}
	return category, diagnostic.Message
}

func finalizeSummary(report *Report) {
	total := len(report.Cases)
	report.Summary.CasesTotal = total
	correctTotal, correctAccepted := 0, 0
	wrongTotal, wrongRejected := 0, 0
	for i := range report.Cases {
		caseReport := &report.Cases[i]
		switch caseReport.Status {
		case CaseStatusPassed:
			report.Summary.CasesPassed++
		case CaseStatusFailed:
			report.Summary.CasesFailed++
		case CaseStatusNotRun:
			report.Summary.CasesNotRun++
		default:
			report.Summary.CasesError++
		}
		if caseReport.DevelopmentFixture {
			report.Summary.DevelopmentFixtureCases++
		}
		if caseReport.BenchmarkEligible {
			report.Summary.EligibleCases++
		}
		if caseReport.BenchmarkEligibility == environmentverify.BenchmarkEligibilityUnresolved {
			report.Summary.UnresolvedEligibilityCases++
		}
		for _, alternative := range caseReport.CorrectAlternatives {
			correctTotal++
			if alternative.Accepted {
				correctAccepted++
			}
		}
		for _, wrong := range caseReport.WrongSolutions {
			if wrong.Excluded || strings.TrimSpace(wrong.DenominatorDecision) != environmentverify.DenominatorMeaningful {
				continue
			}
			wrongTotal++
			if wrong.Rejected {
				wrongRejected++
			}
		}
	}
	report.Summary.BlindAgreementRate = "not_applicable"
	report.Summary.ReconciliationFrequency = "not_applicable"
	report.Summary.NonConvergenceRate = "not_applicable"
	if correctTotal > 0 {
		value := float64(correctAccepted) / float64(correctTotal)
		report.Summary.CorrectAlternativeAcceptance = &value
	}
	if wrongTotal > 0 {
		value := float64(wrongRejected) / float64(wrongTotal)
		report.Summary.WrongSolutionRejection = &value
	}
}

func MarshalReport(report Report) ([]byte, error) {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal benchmark report: %w", err)
	}
	return append(data, '\n'), nil
}

func WriteReport(path string, report Report, schemaPath string) error {
	data, err := MarshalReport(report)
	if err != nil {
		return err
	}
	if err := ValidateReportJSON(data, schemaPath); err != nil {
		return fmt.Errorf("benchmark report does not match %s: %w", schemaPath, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create report directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write benchmark report: %w", err)
	}
	return nil
}
