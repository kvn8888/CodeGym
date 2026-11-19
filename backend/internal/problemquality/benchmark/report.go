package benchmark

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kvn8888/codegym/backend/internal/benchmarkbudget"
	"github.com/kvn8888/codegym/backend/internal/environmentverify"
	"github.com/kvn8888/codegym/backend/internal/problemquality"
)

const (
	ReportFormatVersion = "2.0.0"

	CaseStatusPassed        = "passed"
	CaseStatusFailed        = "failed"
	CaseStatusError         = "error"
	CaseStatusNotRun        = "not_run"
	GenerationMeasured      = "measured"
	GenerationUnavailable   = "unavailable"
	GenerationNotApplicable = "not_applicable"

	MetricMeasured      = "measured"
	MetricUnavailable   = "unavailable"
	MetricNotApplicable = "not_applicable"

	FindingScopeAssessmentQuality    = "assessment_quality"
	FindingScopeHistoricalGeneration = "historical_generation"

	ArtifactIdentityAvailable   = "available"
	ArtifactIdentityUnavailable = "unavailable"

	QualityMetricTreatmentIncluded      = "included"
	QualityMetricTreatmentExcluded      = "excluded"
	QualityMetricTreatmentNotApplicable = "not_applicable"

	FindingKindSurvivingWrongSolution   = "surviving_wrong_solution"
	FindingKindSetupFailure             = "setup_failure"
	FindingKindCaseNotRun               = "case_not_run"
	FindingKindAmbiguousSpecification   = "ambiguous_specification"
	FindingKindFirstAttemptDisagreement = "first_attempt_disagreement"
	FindingKindReconciliation           = "reconciliation"
	FindingKindNonConvergence           = "non_convergence"
	FindingKindGenerationFailure        = "generation_failure"
)

type Meta struct {
	RunID              string
	StartedAt          time.Time
	CompletedAt        time.Time
	TestedCommit       string
	Branch             string
	Dirty              bool
	BaselineKind       string
	ExecutionMode      string
	GenerationUsage    []GenerationUsageRecord
	ModelRevision      string
	PromptRevision     string
	CorpusRoot         string
	Selection          string
	SelectedCaseIDs    []string
	Timeout            time.Duration
	DevelopmentFixture bool
	BudgetSnapshot     *benchmarkbudget.Snapshot
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
	ReportVersion      string                    `json:"report_version"`
	RunID              string                    `json:"run_id"`
	StartedAt          string                    `json:"started_at"`
	CompletedAt        string                    `json:"completed_at"`
	TestedCommit       string                    `json:"tested_commit"`
	Branch             string                    `json:"branch,omitempty"`
	TestedDirty        bool                      `json:"tested_dirty"`
	PromptRevision     string                    `json:"prompt_revision,omitempty"`
	ModelRevision      string                    `json:"model_revision,omitempty"`
	BaselineKind       string                    `json:"baseline_kind"`
	RuntimeConfig      map[string]any            `json:"runtime_config"`
	GenerationConfig   map[string]any            `json:"generation_config"`
	ElapsedMS          int64                     `json:"elapsed_ms"`
	Preliminary        bool                      `json:"preliminary"`
	DevelopmentFixture bool                      `json:"development_fixture"`
	Corpus             *CorpusReport             `json:"corpus,omitempty"`
	SelectedCaseIDs    []string                  `json:"selected_case_ids,omitempty"`
	Budget             *benchmarkbudget.Snapshot `json:"budget,omitempty"`
	Execution          ExecutionReport           `json:"execution"`
	Summary            Summary                   `json:"summary"`
	Cases              []CaseReport              `json:"cases"`
	Findings           []Finding                 `json:"findings,omitempty"`
	Recommendations    []string                  `json:"recommendations"`
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
	DiscriminationScore          *float64 `json:"discrimination_score,omitempty"`
	AmbiguousSpecificationRate   *float64 `json:"ambiguous_specification_rate,omitempty"`
	CasesTotal                   int      `json:"cases_total"`
	CasesPassed                  int      `json:"cases_passed"`
	CasesFailed                  int      `json:"cases_failed"`
	CasesError                   int      `json:"cases_error"`
	CasesNotRun                  int      `json:"cases_not_run"`
	DevelopmentFixtureCases      int      `json:"development_fixture_cases"`
	EligibleCases                int      `json:"eligible_cases"`
	UnresolvedEligibilityCases   int      `json:"unresolved_eligibility_cases"`
	Metrics                      Metrics  `json:"metrics"`
}

// Metrics keeps the count basis next to each rate. The legacy scalar fields
// above remain for report compatibility, but Metrics is the authoritative
// explanation of numerators, denominators, exclusions, and unavailable data.
type Metrics struct {
	CorrectAlternativeAcceptance Metric          `json:"correct_alternative_acceptance"`
	WrongSolutionRejection       Metric          `json:"wrong_solution_rejection"`
	DiscriminationScore          Metric          `json:"discrimination_score"`
	AmbiguousSpecificationRate   Metric          `json:"ambiguous_specification_rate"`
	FirstAttemptAgreement        Metric          `json:"first_attempt_agreement"`
	ReconciliationFrequency      Metric          `json:"reconciliation_frequency"`
	NonConvergenceRate           Metric          `json:"non_convergence_rate"`
	ReconciliationDirections     map[string]int  `json:"reconciliation_directions,omitempty"`
	Cases                        MetricCaseBasis `json:"cases"`
}

type MetricCaseBasis struct {
	Total                         int `json:"total"`
	Eligible                      int `json:"eligible"`
	DevelopmentFixturesExcluded   int `json:"development_fixtures_excluded"`
	UnresolvedEligibilityExcluded int `json:"unresolved_eligibility_excluded"`
	ObservedGenerationCases       int `json:"observed_generation_cases"`
}

// Metric is a rate or an explicit reason it could not be measured. A nil
// Value with measured status is invalid; zero is represented by a non-nil
// pointer to 0. Excluded counts observations or cases kept out of the
// denominator, and ExclusionReasons explains why.
type Metric struct {
	Status            string         `json:"status"`
	Definition        string         `json:"definition"`
	Value             *float64       `json:"value,omitempty"`
	Numerator         int            `json:"numerator"`
	Denominator       int            `json:"denominator"`
	Excluded          int            `json:"excluded"`
	ExclusionReasons  map[string]int `json:"exclusion_reasons,omitempty"`
	UnavailableReason string         `json:"unavailable_reason,omitempty"`
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
	AmbiguousSpecRecorded      bool              `json:"ambiguous_spec_recorded,omitempty"`
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
	Findings                   []Finding         `json:"findings,omitempty"`
	ElapsedMS                  int64             `json:"elapsed_ms"`
}

// Generation records historical observations about how a saved exercise was
// produced. It never derives observed values from expected_outcomes. The
// artifact and input digests bind the record to the saved content.
type Generation struct {
	FirstAttemptAgreement    *bool   `json:"first_attempt_agreement"`
	ReconciliationCount      *int    `json:"reconciliation_count"`
	ReconciliationDirection  *string `json:"reconciliation_direction"`
	NonConvergence           *bool   `json:"non_convergence"`
	GenerationFailed         *bool   `json:"generation_failed"`
	GenerationFailureDetails string  `json:"generation_failure_details,omitempty"`
	Status                   string  `json:"status"`
	Source                   string  `json:"source,omitempty"`
	ArtifactSHA256           string  `json:"artifact_sha256,omitempty"`
	InputSHA256              string  `json:"input_sha256,omitempty"`
	UnavailableReason        string  `json:"unavailable_reason,omitempty"`
}

// Finding is a structured, case-specific observation with an explicit review
// or repair follow-up. Suspected causes stay hypotheses until evidence
// establishes them.
type Finding struct {
	ID                                string            `json:"id"`
	CaseID                            string            `json:"case_id"`
	Kind                              string            `json:"kind"`
	Scope                             string            `json:"scope"`
	ArtifactID                        string            `json:"artifact_id,omitempty"`
	ArtifactVersion                   string            `json:"artifact_version,omitempty"`
	ArtifactIdentityStatus            string            `json:"artifact_identity_status"`
	ArtifactIdentityUnavailableReason string            `json:"artifact_identity_unavailable_reason,omitempty"`
	QualityMetricTreatment            string            `json:"quality_metric_treatment"`
	QualityMetricTreatmentReason      string            `json:"quality_metric_treatment_reason"`
	Summary                           string            `json:"summary"`
	SolutionID                        string            `json:"solution_id,omitempty"`
	TestID                            string            `json:"test_id,omitempty"`
	AssertionCode                     string            `json:"assertion_code,omitempty"`
	ComparatorContext                 string            `json:"comparator_context,omitempty"`
	Hypothesis                        string            `json:"hypothesis,omitempty"`
	CauseEstablished                  bool              `json:"cause_established"`
	Evidence                          []FindingEvidence `json:"evidence,omitempty"`
	ReviewAction                      string            `json:"review_action,omitempty"`
	FollowUps                         []FollowUp        `json:"follow_ups"`
}

type FindingEvidence struct {
	Source string `json:"source"`
	Detail string `json:"detail"`
}

type FollowUp struct {
	Action string `json:"action"`
	Owner  string `json:"owner"`
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
	SavedExerciseUsage         UsageFact      `json:"saved_exercise_usage"`
	AssessmentRunUsage         UsageFact      `json:"assessment_run_usage"`
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
	executionMode := strings.TrimSpace(meta.ExecutionMode)
	if executionMode == "" {
		executionMode = "local"
	}
	sandboxMode := executionMode == "sandbox"
	liveGeneration := len(meta.GenerationUsage) > 0
	modeLabel := "local-development"
	if sandboxMode {
		modeLabel = "sandbox"
	}
	report := Report{
		ReportVersion:      ReportFormatVersion,
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
		Budget:             meta.BudgetSnapshot,
		RuntimeConfig: map[string]any{
			"execution":           "local-shell",
			"sandbox":             "none",
			"network_isolation":   "not_provided",
			"workspace_isolation": "verifier_temporary_directories",
			"command_source":      "artifact_manifest",
			"assessment_protocol": environmentverify.TestOutcomeProtocol,
			"timeout_seconds":     int(meta.Timeout.Seconds()),
		},
		GenerationConfig: func() map[string]any {
			config := map[string]any{
				"status":                             generationMetricStatus(meta.BaselineKind, meta.DevelopmentFixture),
				"assessment_run_usage":               UsageStatusNotApplicable,
				"paid_operations":                    false,
				"baseline_kind":                      meta.BaselineKind,
				"saved_exercise_provenance":          savedProvenanceFor(rawModelRevision, rawPromptRevision, meta.DevelopmentFixture),
				"assessment_run_generation_occurred": liveGeneration,
				"model_operations":                   liveGeneration,
			}
			if liveGeneration {
				config["usage"] = "measured"
				config["live_generation_usage"] = meta.GenerationUsage
				config["note"] = "This run invoked the selected generation baseline to produce a fresh " +
					"reference for each selected case before assessment. Token usage is recorded from the provider; " +
					"paid-budget settlement is not yet wired to these calls."
			} else {
				config["usage"] = "not_applicable"
				config["note"] = "No generation, model calls, token usage, or paid sandboxes were invoked by this " +
					"assessment run. Saved-exercise usage is reported separately per case, is not charged to the " +
					"budget ledger, and is not evidence that paid enforcement occurred."
			}
			return config
		}(),
		Execution: ExecutionReport{
			Mode:            modeLabel,
			Local:           executionMode == "local",
			Sandbox:         sandboxMode,
			ModelOperations: liveGeneration,
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
		"Benchmark eligibility requires explicit human review evidence and a configured reviewer authority; execution success alone is not eligibility.",
		"This local run does not establish sandbox network or memory isolation and does not satisfy the paid-budget milestone.",
	}
	if meta.BudgetSnapshot != nil {
		recommendations = append(recommendations,
			"The budget snapshot shows persisted accounting only: no paid adapter is connected to reserve, settle, or reconcile paid operations, so this run does not demonstrate paid-budget enforcement.")
	}
	for _, run := range runs {
		caseReport, recs := buildCaseReport(run, meta)
		report.Cases = append(report.Cases, caseReport)
		report.Findings = append(report.Findings, caseReport.Findings...)
		recommendations = append(recommendations, recs...)
	}
	if hasKnownSavedExerciseUsage(report.Cases) {
		recommendations = append(recommendations,
			"Saved exercise generation usage is historical evidence only; it is not charged to the benchmark budget and does not demonstrate paid-operation enforcement.")
	}
	report.Recommendations = append(report.Recommendations, recommendations...)
	finalizeSummary(&report)
	return report
}

func buildCaseReport(run CaseRun, meta Meta) (caseReport CaseReport, recommendations []string) {
	artifactContext := findingArtifactContextForRun(run)
	defer func() {
		for i := range caseReport.Findings {
			applyFindingContext(&caseReport.Findings[i], caseReport, artifactContext)
		}
	}()
	config := run.Config
	fixture := config != nil && config.DevelopmentFixture
	caseReport = CaseReport{
		CaseID:                     caseIDForRun(run),
		Technology:                 "unknown",
		Status:                     CaseStatusError,
		Generation:                 generationForCase(fixture, meta.BaselineKind, config, run.Identity),
		Provenance:                 caseProvenance(fixture, config, meta.ModelRevision, meta.PromptRevision),
		ContentIdentity:            run.Identity,
		DevelopmentFixture:         fixture,
		ExcludedFromReviewedTotals: fixture,
		BenchmarkEligibility:       environmentverify.BenchmarkEligibilityUnresolved,
		ReviewStatus:               environmentverify.ReviewStatusUnresolved,
		ElapsedMS:                  run.ElapsedMS,
	}
	caseReport.Findings = append(caseReport.Findings, generationFindings(caseReport)...)
	recommendations = []string{}
	if run.Case != nil {
		caseReport.Technology = run.Case.Technology
		caseReport.CorpusVersion = run.Case.Version
		caseReport.ComparatorConfig = comparatorConfig(*run.Case)
		caseReport.ComparatorResult = "not_measured_by_harness"
		if run.Case.Review.AmbiguousSpec != nil {
			caseReport.AmbiguousSpec = *run.Case.Review.AmbiguousSpec
			caseReport.AmbiguousSpecRecorded = true
			if caseReport.AmbiguousSpec {
				caseReport.Findings = append(caseReport.Findings,
					ambiguousSpecificationFinding(caseReport.CaseID, run.Case.Review.Notes))
			}
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
		caseReport.Findings = append(caseReport.Findings,
			caseNotRunFinding(caseReport.CaseID, run.NotRunReason))
		return caseReport, nil
	}
	if run.Err != nil {
		caseReport.Status = CaseStatusError
		caseReport.FailureCategory = "setup_failure"
		caseReport.FailureDetails = run.Err.Error()
		caseReport.Blocked = true
		caseReport.BlockReason = run.Err.Error()
		caseReport.Findings = append(caseReport.Findings,
			setupFailureFinding(caseReport.CaseID, "setup_failure", run.Err.Error()))
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
			caseReport.Findings = append(caseReport.Findings,
				survivingWrongSolutionFinding(caseReport, item, fixture))
			if fixture {
				recommendations = append(recommendations,
					fmt.Sprintf("Preserve the intentional development-fixture survivor for case %q and wrong solution %q; review it as harness regression evidence rather than strengthening checked-in expectations.", caseReport.CaseID, item.ID))
			} else {
				recommendations = append(recommendations,
					fmt.Sprintf("Review the intended assertion and comparator context for case %q: wrong solution %q survived the current assessment.", caseReport.CaseID, item.ID))
			}
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
		caseReport.Findings = append(caseReport.Findings,
			setupFailureFinding(caseReport.CaseID, category, details))
	}
	if status == CaseStatusFailed && category == environmentverify.FailureCategoryAssertion {
		recommendations = append(recommendations,
			fmt.Sprintf("Review assertion evidence for case %q; at least one wrong solution did not fail for its intended assertion.", caseReport.CaseID))
	}
	return caseReport, recommendations
}

type findingArtifactContext struct {
	ID                string
	Version           string
	UnavailableReason string
}

func findingArtifactContextForRun(run CaseRun) findingArtifactContext {
	context := findingArtifactContext{}
	if run.Config != nil {
		context.ID = strings.TrimSpace(run.Config.ArtifactID)
		context.Version = strings.TrimSpace(run.Config.ArtifactVersion)
	}
	if run.Case != nil {
		if context.ID == "" {
			context.ID = strings.TrimSpace(run.Case.ArtifactID)
		}
		if context.Version == "" {
			context.Version = strings.TrimSpace(run.Case.ArtifactVersion)
		}
	}
	if context.ID == "" || context.Version == "" {
		switch {
		case run.Case == nil:
			context.UnavailableReason = "artifact identity unavailable because case loading failed before artifact ID/version was established"
		case context.ID != "" || context.Version != "":
			context.UnavailableReason = "artifact identity unavailable because artifact ID/version was not fully established"
		default:
			context.UnavailableReason = "artifact identity unavailable because the case did not declare an artifact ID/version"
		}
	}
	return context
}

func applyFindingContext(finding *Finding, caseReport CaseReport, artifact findingArtifactContext) {
	finding.Scope = findingScope(finding.Kind)
	if artifact.ID != "" && artifact.Version != "" {
		finding.ArtifactID = artifact.ID
		finding.ArtifactVersion = artifact.Version
		finding.ArtifactIdentityStatus = ArtifactIdentityAvailable
	} else {
		finding.ArtifactID = artifact.ID
		finding.ArtifactVersion = artifact.Version
		finding.ArtifactIdentityStatus = ArtifactIdentityUnavailable
		finding.ArtifactIdentityUnavailableReason = artifact.UnavailableReason
	}
	finding.QualityMetricTreatment, finding.QualityMetricTreatmentReason = qualityMetricTreatment(caseReport, *finding)
}

func findingScope(kind string) string {
	switch kind {
	case FindingKindFirstAttemptDisagreement, FindingKindReconciliation,
		FindingKindNonConvergence, FindingKindGenerationFailure:
		return FindingScopeHistoricalGeneration
	default:
		return FindingScopeAssessmentQuality
	}
}

func qualityMetricTreatment(caseReport CaseReport, finding Finding) (string, string) {
	switch finding.Kind {
	case FindingKindFirstAttemptDisagreement, FindingKindReconciliation,
		FindingKindNonConvergence, FindingKindGenerationFailure:
		return QualityMetricTreatmentNotApplicable, "Historical generation evidence does not contribute to assessment-quality metric denominators."
	case FindingKindSetupFailure:
		return QualityMetricTreatmentExcluded, "Setup failure produced no completed assessment sample, so the case is excluded from quality denominators."
	case FindingKindCaseNotRun:
		return QualityMetricTreatmentExcluded, "Incomplete execution is not counted as a completed quality sample."
	case FindingKindAmbiguousSpecification:
		if treatment, reason, excluded := caseQualityExclusion(caseReport); excluded {
			return treatment, reason
		}
		return QualityMetricTreatmentIncluded, "Eligible case is included in the ambiguous-specification rate; correctness denominators are unaffected."
	case FindingKindSurvivingWrongSolution:
		if treatment, reason, excluded := caseQualityExclusion(caseReport); excluded {
			return treatment, reason
		}
		item, ok := wrongSolutionByID(caseReport.WrongSolutions, finding.SolutionID)
		if !ok {
			return QualityMetricTreatmentNotApplicable, "Surviving wrong-solution evidence is not linked to a recorded wrong-solution outcome."
		}
		switch item.DenominatorDecision {
		case environmentverify.DenominatorEquivalent:
			return QualityMetricTreatmentExcluded, "Wrong solution is equivalent and excluded from meaningful-wrong denominators."
		case environmentverify.DenominatorInvalid:
			return QualityMetricTreatmentExcluded, "Wrong solution is invalid and excluded from meaningful-wrong denominators."
		}
		if item.ExecutionStatus != string(environmentverify.ExecutionCompleted) {
			return QualityMetricTreatmentExcluded, "Wrong solution did not complete, so it is excluded from quality denominators."
		}
		if item.DenominatorDecision != environmentverify.DenominatorMeaningful {
			return QualityMetricTreatmentExcluded, "Wrong solution is not classified as a meaningful wrong solution for quality denominators."
		}
		return QualityMetricTreatmentIncluded, "Meaningful completed wrong solution is included in wrong-solution rejection and discrimination denominators."
	default:
		return QualityMetricTreatmentNotApplicable, "No quality-metric treatment is defined for this finding kind."
	}
}

func caseQualityExclusion(caseReport CaseReport) (string, string, bool) {
	if caseReport.DevelopmentFixture {
		return QualityMetricTreatmentExcluded, "Development fixture is excluded from reviewed quality totals.", true
	}
	if !caseReport.BenchmarkEligible {
		reason := "Benchmark eligibility is not established, so the case is excluded from quality denominators."
		if trimmed := strings.TrimSpace(caseReport.BenchmarkEligibilityReason); trimmed != "" {
			reason += " " + trimmed
		}
		return QualityMetricTreatmentExcluded, reason, true
	}
	return "", "", false
}

func wrongSolutionByID(items []WrongOutcome, id string) (WrongOutcome, bool) {
	for _, item := range items {
		if item.ID == id {
			return item, true
		}
	}
	return WrongOutcome{}, false
}

func generationForCase(fixture bool, baselineKind string, config *RunConfig, identity *ContentIdentity) Generation {
	generation := Generation{Status: generationMetricStatus(baselineKind, fixture)}
	if generation.Status == GenerationNotApplicable {
		return generation
	}
	if config == nil || config.GenerationProvenance == nil || config.GenerationProvenance.Observations == nil {
		generation.UnavailableReason = "missing applicable historical generation observation"
		return generation
	}
	observations := config.GenerationProvenance.Observations
	if identity == nil || observations.ArtifactSHA256 != identity.Artifact.SHA256 ||
		observations.InputSHA256 != identity.Reference.SHA256 {
		generation.UnavailableReason = "historical generation observation does not match the saved artifact/input identity"
		return generation
	}
	generation.Status = GenerationMeasured
	generation.FirstAttemptAgreement = observations.FirstAttemptAgreement
	generation.ReconciliationCount = observations.ReconciliationCount
	if strings.TrimSpace(observations.ReconciliationDirection) != "" {
		direction := observations.ReconciliationDirection
		generation.ReconciliationDirection = &direction
	}
	generation.NonConvergence = observations.NonConvergence
	generation.GenerationFailed = observations.GenerationFailed
	generation.GenerationFailureDetails = observations.GenerationFailureDetails
	generation.Source = observations.Source
	generation.ArtifactSHA256 = observations.ArtifactSHA256
	generation.InputSHA256 = observations.InputSHA256
	return generation
}

func survivingWrongSolutionFinding(caseReport CaseReport, item WrongOutcome, fixture bool) Finding {
	testID, assertionCode := "", ""
	if item.IntendedFailure != nil {
		testID = item.IntendedFailure.TestID
		assertionCode = item.IntendedFailure.Code
	}
	scope := "case"
	if fixture {
		scope = "development fixture"
	}
	return Finding{
		ID:                findingID(caseReport.CaseID, FindingKindSurvivingWrongSolution, item.ID),
		CaseID:            caseReport.CaseID,
		Kind:              FindingKindSurvivingWrongSolution,
		Summary:           fmt.Sprintf("Wrong solution %q survived its intended rejection test %q in %s %q.", item.ID, testID, scope, caseReport.CaseID),
		SolutionID:        item.ID,
		TestID:            testID,
		AssertionCode:     assertionCode,
		ComparatorContext: caseReport.ComparatorConfig,
		Hypothesis:        "The survivor may indicate a missing or weak assertion, a comparator mismatch, or a specification gap. The assessment establishes survival, not which cause applies.",
		CauseEstablished:  false,
		Evidence: []FindingEvidence{{
			Source: "assessment",
			Detail: fmt.Sprintf("solution=%s intended_test=%s assertion=%s excluded=%t denominator_decision=%s",
				item.ID, testID, assertionCode, item.Excluded, item.DenominatorDecision),
		}},
		ReviewAction: fmt.Sprintf("Review solution %q, intended test %q, assertion %q, comparator %q, and the case specification before deciding whether the survivor is intentional, a test/comparator gap, or a denominator reclassification.", item.ID, testID, assertionCode, caseReport.ComparatorConfig),
		FollowUps: []FollowUp{
			{Action: "Record the review decision for the surviving wrong solution and preserve its assessment evidence.", Owner: "case-review"},
			{Action: "Keep the case excluded from reviewed quality totals until valid approval evidence and a configured reviewer authority exist.", Owner: "harness"},
		},
	}
}

func setupFailureFinding(caseID, category, details string) Finding {
	if strings.TrimSpace(category) == "" {
		category = "setup_failure"
	}
	return Finding{
		ID:               findingID(caseID, FindingKindSetupFailure, category),
		CaseID:           caseID,
		Kind:             FindingKindSetupFailure,
		Summary:          fmt.Sprintf("Case %q did not complete assessment because of %s.", caseID, category),
		CauseEstablished: true,
		Evidence:         []FindingEvidence{{Source: "execution", Detail: details}},
		ReviewAction:     "Repair the declared setup/build/test contract or local prerequisite before treating this case as quality evidence.",
		FollowUps: []FollowUp{
			{Action: "Inspect the failing stage, command, and environment prerequisite; rerun only after the cause is repaired.", Owner: "harness"},
			{Action: "Do not count the blocked case as a passed or failed quality sample.", Owner: "reporting"},
		},
	}
}

func caseNotRunFinding(caseID, details string) Finding {
	return Finding{
		ID:               findingID(caseID, FindingKindCaseNotRun, "cancelled"),
		CaseID:           caseID,
		Kind:             FindingKindCaseNotRun,
		Summary:          fmt.Sprintf("Case %q was not run to completion.", caseID),
		CauseEstablished: true,
		Evidence:         []FindingEvidence{{Source: "execution", Detail: details}},
		FollowUps: []FollowUp{{
			Action: "Rerun the case when execution can complete; do not count partial evidence as a quality sample.",
			Owner:  "harness",
		}},
	}
}

func ambiguousSpecificationFinding(caseID, notes string) Finding {
	detail := "case review records ambiguous_spec=true"
	if strings.TrimSpace(notes) != "" {
		detail += ": " + strings.TrimSpace(notes)
	}
	return Finding{
		ID:               findingID(caseID, FindingKindAmbiguousSpecification, "review"),
		CaseID:           caseID,
		Kind:             FindingKindAmbiguousSpecification,
		Summary:          fmt.Sprintf("Case %q is marked as having an ambiguous specification.", caseID),
		CauseEstablished: true,
		Evidence:         []FindingEvidence{{Source: "case-review", Detail: detail}},
		ReviewAction:     "Review the specification and ambiguity_resolutions before relying on this case for quality metrics.",
		FollowUps: []FollowUp{{
			Action: "Clarify the specification or record an explicit reviewed exclusion; keep eligibility unresolved until valid approval evidence exists.",
			Owner:  "case-review",
		}},
	}
}

func generationFindings(caseReport CaseReport) []Finding {
	generation := caseReport.Generation
	if generation.Status != GenerationMeasured {
		return nil
	}
	identity := fmt.Sprintf("artifact_sha256=%s input_sha256=%s source=%s",
		generation.ArtifactSHA256, generation.InputSHA256, generation.Source)
	findings := []Finding{}
	if generation.FirstAttemptAgreement != nil && !*generation.FirstAttemptAgreement {
		findings = append(findings, Finding{
			ID:               findingID(caseReport.CaseID, FindingKindFirstAttemptDisagreement, "historical"),
			CaseID:           caseReport.CaseID,
			Kind:             FindingKindFirstAttemptDisagreement,
			Summary:          fmt.Sprintf("Historical generation record for case %q reports first-attempt disagreement.", caseReport.CaseID),
			CauseEstablished: true,
			Evidence:         []FindingEvidence{{Source: "generation-observation", Detail: identity}},
			ReviewAction:     "Review the saved generation record and its reconciliation evidence; do not change generation behavior from one historical observation.",
			FollowUps:        []FollowUp{{Action: "Inspect the recorded disagreement and reconciliation evidence.", Owner: "generation-review"}},
		})
	}
	if generation.ReconciliationCount != nil && *generation.ReconciliationCount > 0 {
		direction := "unspecified"
		if generation.ReconciliationDirection != nil {
			direction = *generation.ReconciliationDirection
		}
		findings = append(findings, Finding{
			ID:               findingID(caseReport.CaseID, FindingKindReconciliation, "historical"),
			CaseID:           caseReport.CaseID,
			Kind:             FindingKindReconciliation,
			Summary:          fmt.Sprintf("Historical generation record for case %q reports %d reconciliation(s) toward %s.", caseReport.CaseID, *generation.ReconciliationCount, direction),
			CauseEstablished: true,
			Evidence:         []FindingEvidence{{Source: "generation-observation", Detail: identity}},
			ReviewAction:     "Review the recorded reconciliation direction and evidence; do not infer a generation change from a single case.",
			FollowUps:        []FollowUp{{Action: "Inspect the saved reconciliation record and its evidence source.", Owner: "generation-review"}},
		})
	}
	if generation.NonConvergence != nil && *generation.NonConvergence {
		findings = append(findings, Finding{
			ID:               findingID(caseReport.CaseID, FindingKindNonConvergence, "historical"),
			CaseID:           caseReport.CaseID,
			Kind:             FindingKindNonConvergence,
			Summary:          fmt.Sprintf("Historical generation record for case %q reports non-convergence.", caseReport.CaseID),
			CauseEstablished: true,
			Evidence:         []FindingEvidence{{Source: "generation-observation", Detail: identity}},
			ReviewAction:     "Review the saved non-convergence record and its evidence source; do not propose generation changes without broader measured support.",
			FollowUps:        []FollowUp{{Action: "Inspect the saved non-convergence evidence and record the review outcome.", Owner: "generation-review"}},
		})
	}
	if generation.GenerationFailed != nil && *generation.GenerationFailed {
		findings = append(findings, Finding{
			ID:               findingID(caseReport.CaseID, FindingKindGenerationFailure, "historical"),
			CaseID:           caseReport.CaseID,
			Kind:             FindingKindGenerationFailure,
			Summary:          fmt.Sprintf("Historical generation record for case %q reports a generation failure: %s", caseReport.CaseID, generation.GenerationFailureDetails),
			CauseEstablished: true,
			Evidence: []FindingEvidence{{
				Source: "generation-observation",
				Detail: fmt.Sprintf("%s failure_details=%s", identity, generation.GenerationFailureDetails),
			}},
			ReviewAction: "Review the recorded generation failure details and evidence source; do not infer success or a different failure cause without supporting evidence.",
			FollowUps: []FollowUp{{
				Action: "Inspect the saved failure details and record the generation-review outcome.",
				Owner:  "generation-review",
			}},
		})
	}
	return findings
}

func findingID(caseID, kind, qualifier string) string {
	parts := []string{strings.TrimSpace(caseID), strings.TrimSpace(kind)}
	if trimmed := strings.TrimSpace(qualifier); trimmed != "" {
		parts = append(parts, trimmed)
	}
	return strings.Join(parts, ":")
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

func generationMetricStatus(baselineKind string, fixture bool) string {
	if fixture || strings.TrimSpace(baselineKind) != "independent" {
		return GenerationNotApplicable
	}
	return GenerationUnavailable
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
		SavedExerciseUsage:         savedExerciseUsageFact(config, fixture),
		AssessmentRunUsage:         assessmentRunUsageFact(),
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
	var executionError, qualityFailure *statusFinding
	consider := func(outcome *environmentverify.AssessmentOutcome) {
		if outcome == nil || outcome.ExecutionStatus == environmentverify.ExecutionExcluded {
			return
		}
		if outcome.ExecutionStatus != environmentverify.ExecutionCompleted {
			if executionError == nil {
				executionError = executionStatusFinding(outcome)
			}
			return
		}
		if !outcome.Passed && qualityFailure == nil {
			qualityFailure = &statusFinding{
				category: firstCategory(outcome.FailureCategory),
				details:  outcome.Message,
			}
		}
	}
	consider(collection.Reference)
	for i := range collection.CorrectAlternatives {
		consider(&collection.CorrectAlternatives[i])
	}
	for i := range collection.WrongSolutions {
		consider(&collection.WrongSolutions[i])
	}
	if executionError != nil {
		return CaseStatusError, executionError.category, executionError.details
	}
	if qualityFailure != nil {
		return CaseStatusFailed, qualityFailure.category, qualityFailure.details
	}
	if collection.Blocked || (collection.Reference == nil && len(collection.CorrectAlternatives) == 0 && len(collection.WrongSolutions) == 0) {
		category, details := verifierFailure(run.Verifier)
		return CaseStatusError, category, details
	}
	return CaseStatusPassed, "", ""
}

type statusFinding struct {
	category string
	details  string
}

func executionStatusFinding(outcome *environmentverify.AssessmentOutcome) *statusFinding {
	category := strings.TrimSpace(outcome.FailureCategory)
	if category == "" {
		switch outcome.ExecutionStatus {
		case environmentverify.ExecutionTimeout:
			category = environmentverify.FailureCategoryTimeout
		case environmentverify.ExecutionPlatformError:
			category = environmentverify.FailureCategoryPlatform
		case environmentverify.ExecutionCompileError:
			category = environmentverify.FailureCategoryCompile
		case environmentverify.ExecutionMalformed:
			category = environmentverify.FailureCategoryMalformed
		default:
			category = "setup_failure"
		}
	}
	details := strings.TrimSpace(outcome.Message)
	if details == "" {
		details = fmt.Sprintf("assessment %q did not run to completion (%s)", outcome.ID, outcome.ExecutionStatus)
	}
	return &statusFinding{category: category, details: details}
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

const (
	definitionCorrectAlternativeAcceptance = "Completed correct alternatives that passed every protected test divided by completed correct alternatives in benchmark-eligible, non-development-fixture cases."
	definitionWrongSolutionRejection       = "Completed meaningful wrong solutions rejected by any assessment assertion divided by completed meaningful wrong solutions in benchmark-eligible, non-development-fixture cases."
	definitionDiscriminationScore          = "Mutation/discrimination score: completed meaningful wrong solutions that failed the declared intended test, assertion code, expected value, and actual value divided by the same completed meaningful wrong-solution denominator."
	definitionAmbiguousSpecificationRate   = "Benchmark-eligible cases marked ambiguous_spec=true divided by benchmark-eligible cases with a recorded reviewed ambiguity decision."
	definitionFirstAttemptAgreement        = "Historical saved exercises whose first generation attempt agreed with the accepted reference divided by saved exercises that supplied first-attempt agreement evidence."
	definitionReconciliationFrequency      = "Historical saved exercises with one or more reconciliation records divided by saved exercises that supplied a reconciliation count."
	definitionNonConvergenceRate           = "Historical saved exercises marked non-convergent divided by saved exercises that supplied a non-convergence observation."
)

type metricAccumulator struct {
	definition  string
	numerator   int
	denominator int
	exclusions  map[string]int
}

func newMetricAccumulator(definition string) metricAccumulator {
	return metricAccumulator{definition: definition, exclusions: map[string]int{}}
}

func (a *metricAccumulator) observe(value bool) {
	a.denominator++
	if value {
		a.numerator++
	}
}

func (a *metricAccumulator) exclude(reason string) {
	a.exclusions[reason]++
}

func (a *metricAccumulator) excludedTotal() int {
	total := 0
	for _, count := range a.exclusions {
		total += count
	}
	return total
}

func (a *metricAccumulator) metric(unavailableReason string) Metric {
	result := Metric{
		Definition:       a.definition,
		Numerator:        a.numerator,
		Denominator:      a.denominator,
		Excluded:         a.excludedTotal(),
		ExclusionReasons: a.exclusions,
	}
	if len(result.ExclusionReasons) == 0 {
		result.ExclusionReasons = nil
	}
	if a.denominator == 0 {
		result.Status = MetricUnavailable
		result.UnavailableReason = unavailableReason
		return result
	}
	value := float64(a.numerator) / float64(a.denominator)
	result.Status = MetricMeasured
	result.Value = &value
	return result
}

func notApplicableMetric(definition, reason string) Metric {
	return Metric{
		Status:            MetricNotApplicable,
		Definition:        definition,
		ExclusionReasons:  nil,
		UnavailableReason: reason,
	}
}

func unavailableMetric(definition, reason string) Metric {
	return Metric{
		Status:            MetricUnavailable,
		Definition:        definition,
		UnavailableReason: reason,
	}
}

func metricSummaryValue(metric Metric) any {
	if metric.Status == MetricMeasured && metric.Value != nil {
		return *metric.Value
	}
	return metric.Status
}

func finalizeSummary(report *Report) {
	total := len(report.Cases)
	report.Summary.CasesTotal = total
	correct := newMetricAccumulator(definitionCorrectAlternativeAcceptance)
	wrong := newMetricAccumulator(definitionWrongSolutionRejection)
	discrimination := newMetricAccumulator(definitionDiscriminationScore)
	ambiguous := newMetricAccumulator(definitionAmbiguousSpecificationRate)
	firstAttempt := newMetricAccumulator(definitionFirstAttemptAgreement)
	reconciliation := newMetricAccumulator(definitionReconciliationFrequency)
	nonConvergence := newMetricAccumulator(definitionNonConvergenceRate)
	reconciliationDirections := map[string]int{}
	eligibleCases := 0
	applicableGenerationCases := 0
	observedGenerationCases := 0
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

		if report.BaselineKind == "independent" && !caseReport.DevelopmentFixture {
			applicableGenerationCases++
			generation := caseReport.Generation
			if generation.Status != GenerationMeasured {
				for _, accumulator := range []*metricAccumulator{&firstAttempt, &reconciliation, &nonConvergence} {
					accumulator.exclude("missing_observation")
				}
			} else {
				observedGenerationCases++
				if generation.FirstAttemptAgreement == nil {
					firstAttempt.exclude("missing_observation_field")
				} else {
					firstAttempt.observe(*generation.FirstAttemptAgreement)
				}
				if generation.ReconciliationCount == nil {
					reconciliation.exclude("missing_observation_field")
				} else {
					reconciliation.observe(*generation.ReconciliationCount > 0)
				}
				if generation.ReconciliationDirection != nil {
					reconciliationDirections[*generation.ReconciliationDirection]++
				}
				if generation.NonConvergence == nil {
					nonConvergence.exclude("missing_observation_field")
				} else {
					nonConvergence.observe(*generation.NonConvergence)
				}
			}
		}

		if caseReport.DevelopmentFixture || !caseReport.BenchmarkEligible {
			continue
		}
		eligibleCases++
		for _, alternative := range caseReport.CorrectAlternatives {
			if alternative.ExecutionStatus != string(environmentverify.ExecutionCompleted) {
				correct.exclude("non_completed")
				continue
			}
			correct.observe(alternative.Accepted)
		}
		for _, wrongSolution := range caseReport.WrongSolutions {
			decision := strings.TrimSpace(wrongSolution.DenominatorDecision)
			switch {
			case decision == environmentverify.DenominatorEquivalent:
				wrong.exclude("equivalent")
				discrimination.exclude("equivalent")
				continue
			case decision == environmentverify.DenominatorInvalid:
				wrong.exclude("invalid")
				discrimination.exclude("invalid")
				continue
			case decision != environmentverify.DenominatorMeaningful:
				wrong.exclude("unknown_denominator_decision")
				discrimination.exclude("unknown_denominator_decision")
				continue
			case wrongSolution.ExecutionStatus != string(environmentverify.ExecutionCompleted):
				wrong.exclude("non_completed")
				discrimination.exclude("non_completed")
				continue
			}
			wrong.observe(wrongSolution.Rejected)
			discrimination.observe(wrongSolution.IntendedFailureMatched)
		}
		if caseReport.AmbiguousSpecRecorded {
			ambiguous.observe(caseReport.AmbiguousSpec)
		} else {
			ambiguous.exclude("missing_reviewed_ambiguity_decision")
		}
	}

	insufficient := "insufficient evidence: zero benchmark-eligible cases; development fixtures and cases with unresolved benchmark eligibility are excluded from quality denominators"
	metrics := Metrics{
		Cases: MetricCaseBasis{
			Total:                         total,
			Eligible:                      eligibleCases,
			DevelopmentFixturesExcluded:   report.Summary.DevelopmentFixtureCases,
			UnresolvedEligibilityExcluded: report.Summary.UnresolvedEligibilityCases,
			ObservedGenerationCases:       observedGenerationCases,
		},
	}
	if eligibleCases == 0 {
		metrics.CorrectAlternativeAcceptance = unavailableMetric(definitionCorrectAlternativeAcceptance, insufficient)
		metrics.WrongSolutionRejection = unavailableMetric(definitionWrongSolutionRejection, insufficient)
		metrics.DiscriminationScore = unavailableMetric(definitionDiscriminationScore, insufficient)
		metrics.AmbiguousSpecificationRate = unavailableMetric(definitionAmbiguousSpecificationRate, insufficient)
	} else {
		metrics.CorrectAlternativeAcceptance = correct.metric("no completed correct-alternative observations in benchmark-eligible cases")
		metrics.WrongSolutionRejection = wrong.metric("no completed meaningful wrong-solution observations in benchmark-eligible cases")
		metrics.DiscriminationScore = discrimination.metric("no completed meaningful wrong-solution observations in benchmark-eligible cases")
		metrics.AmbiguousSpecificationRate = ambiguous.metric("no benchmark-eligible case recorded a reviewed ambiguity decision")
	}
	if report.BaselineKind != "independent" {
		reason := "baseline_kind does not use independent generation"
		metrics.FirstAttemptAgreement = notApplicableMetric(definitionFirstAttemptAgreement, reason)
		metrics.ReconciliationFrequency = notApplicableMetric(definitionReconciliationFrequency, reason)
		metrics.NonConvergenceRate = notApplicableMetric(definitionNonConvergenceRate, reason)
	} else if applicableGenerationCases == 0 {
		reason := "no non-development-fixture cases were selected"
		metrics.FirstAttemptAgreement = notApplicableMetric(definitionFirstAttemptAgreement, reason)
		metrics.ReconciliationFrequency = notApplicableMetric(definitionReconciliationFrequency, reason)
		metrics.NonConvergenceRate = notApplicableMetric(definitionNonConvergenceRate, reason)
	} else {
		metrics.FirstAttemptAgreement = firstAttempt.metric("missing applicable observations: no case supplied first-attempt agreement evidence")
		metrics.ReconciliationFrequency = reconciliation.metric("missing applicable observations: no case supplied a reconciliation count")
		metrics.NonConvergenceRate = nonConvergence.metric("missing applicable observations: no case supplied a non-convergence observation")
		if len(reconciliationDirections) > 0 {
			metrics.ReconciliationDirections = reconciliationDirections
		}
	}
	report.Summary.Metrics = metrics
	report.Summary.BlindAgreementRate = metricSummaryValue(metrics.FirstAttemptAgreement)
	report.Summary.ReconciliationFrequency = metricSummaryValue(metrics.ReconciliationFrequency)
	report.Summary.NonConvergenceRate = metricSummaryValue(metrics.NonConvergenceRate)
	report.Summary.CorrectAlternativeAcceptance = metrics.CorrectAlternativeAcceptance.Value
	report.Summary.WrongSolutionRejection = metrics.WrongSolutionRejection.Value
	report.Summary.DiscriminationScore = metrics.DiscriminationScore.Value
	report.Summary.AmbiguousSpecificationRate = metrics.AmbiguousSpecificationRate.Value

	if eligibleCases == 0 {
		report.Recommendations = append(report.Recommendations,
			"Quality metrics are unavailable because there are no benchmark-eligible cases; this is insufficient evidence. "+
				"Development fixtures and cases with unresolved benchmark eligibility are excluded from quality denominators.")
	} else if metrics.CorrectAlternativeAcceptance.Denominator == 0 && metrics.WrongSolutionRejection.Denominator == 0 {
		report.Recommendations = append(report.Recommendations,
			"Quality metrics have no completed eligible samples at their current denominators; do not treat the unavailable rates as zero.")
	}
	if report.BaselineKind == "independent" && applicableGenerationCases > 0 &&
		metrics.FirstAttemptAgreement.Denominator == 0 && metrics.ReconciliationFrequency.Denominator == 0 &&
		metrics.NonConvergenceRate.Denominator == 0 {
		report.Recommendations = append(report.Recommendations,
			"Generation metrics are unavailable because no applicable historical observations were supplied; missing observations are not zero and were not copied from expected_outcomes.")
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
