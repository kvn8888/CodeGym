package benchmark

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/environmentverify"
	"github.com/kvn8888/codegym/backend/internal/problemquality"
)

const fixtureDir = "../testdata/development-order-tools"

func TestRunCaseRecordsElapsedTimeOnSetupFailure(t *testing.T) {
	base := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	calls := 0
	opts := Options{now: func() time.Time {
		calls++
		return base.Add(time.Duration(calls-1) * 1500 * time.Millisecond)
	}}
	run := runCase(context.Background(), "run", opts, problemquality.CorpusCase{
		Dir: "/tmp/broken-case",
		Err: errors.New("case.yaml is missing"),
	})
	if run.ElapsedMS != 1500 {
		t.Fatalf("elapsed_ms = %d, want 1500 from the injected clock", run.ElapsedMS)
	}
}

func TestLoadRunConfigBindsArtifactAndExplicitReference(t *testing.T) {
	c, err := problemquality.LoadCase(fixtureDir)
	if err != nil {
		t.Fatal(err)
	}
	config, err := LoadRunConfig(filepath.Join(c.Dir, RunConfigFileName), c)
	if err != nil {
		t.Fatal(err)
	}
	if config.ArtifactID != "development-order-tools-artifact" || config.ArtifactVersion != "0.1.0" {
		t.Fatalf("artifact = %q/%q", config.ArtifactID, config.ArtifactVersion)
	}
	if !config.DevelopmentFixture {
		t.Fatal("development_fixture = false, want explicit fixture label")
	}
	if got := config.ReferenceData.Files["solution.py"]; !strings.Contains(got, "def normalize_tags") {
		t.Fatalf("reference content = %q", got)
	}
	if config.Manifest.Artifact == nil || config.Manifest.Artifact.ID != config.ArtifactID {
		t.Fatalf("manifest identity = %+v", config.Manifest.Artifact)
	}
}

func TestWrongOutcomeExclusionIsNotSurvival(t *testing.T) {
	outcome := environmentverify.AssessmentOutcome{
		ID:                  "equivalent",
		Role:                environmentverify.RoleWrongSolution,
		DenominatorDecision: environmentverify.DenominatorEquivalent,
		ExecutionStatus:     environmentverify.ExecutionExcluded,
		Passed:              false,
	}
	report := wrongOutcome(outcome, nil)
	if !report.Excluded || report.Rejected || report.Surviving {
		t.Fatalf("wrongOutcome = %+v, want excluded and not surviving", report)
	}
}

func TestExecuteDevelopmentFixtureWritesDeliberateQualityFailure(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is required to execute the development fixture; not masking it with mocked success")
	}
	output := filepath.Join(t.TempDir(), "report.json")
	schema := filepath.Join("..", "..", "..", "..", "benchmarks", "problem-quality", "schema", "report.schema.json")
	result, err := Execute(context.Background(), Options{
		CaseDirs:           []string{fixtureDir},
		OutputPath:         output,
		SchemaPath:         schema,
		Timeout:            30 * time.Second,
		BaselineKind:       "other",
		DevelopmentFixture: true,
		ExecuteLocal:       true,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.QualityFailure {
		t.Fatalf("quality failure = false, want the surviving wrong solution to fail the benchmark")
	}
	if result.ExecutionFailure {
		t.Fatalf("execution failure = true, report=%+v", result.Report)
	}
	if len(result.Report.Cases) != 1 {
		t.Fatalf("cases = %d", len(result.Report.Cases))
	}
	caseReport := result.Report.Cases[0]
	if caseReport.Status != CaseStatusFailed {
		t.Fatalf("case status = %q, want failed", caseReport.Status)
	}
	if caseReport.ReferenceOutcome == nil || !caseReport.ReferenceOutcome.Accepted {
		t.Fatalf("reference outcome = %+v", caseReport.ReferenceOutcome)
	}
	if len(caseReport.CorrectAlternatives) != 2 {
		t.Fatalf("correct alternatives = %d", len(caseReport.CorrectAlternatives))
	}
	for _, alternative := range caseReport.CorrectAlternatives {
		if !alternative.Accepted {
			t.Fatalf("correct alternative %q was not accepted: %+v", alternative.ID, alternative)
		}
	}
	surviving := 0
	for _, wrong := range caseReport.WrongSolutions {
		if wrong.Surviving {
			surviving++
			if wrong.DiagnosticCode != environmentverify.CodeWrongSolutionPassed {
				t.Fatalf("surviving wrong diagnostic = %q", wrong.DiagnosticCode)
			}
		}
	}
	if surviving != 1 {
		t.Fatalf("surviving wrong solutions = %d, want 1", surviving)
	}
	if caseReport.BenchmarkEligibility != environmentverify.BenchmarkEligibilityUnresolved || caseReport.BenchmarkEligible {
		t.Fatalf("eligibility = %q eligible=%v, want unresolved and ineligible", caseReport.BenchmarkEligibility, caseReport.BenchmarkEligible)
	}
	if !strings.HasPrefix(caseReport.ComparatorConfig, "set:") {
		t.Fatalf("comparator config = %q, want the declared set comparator", caseReport.ComparatorConfig)
	}
	if caseReport.ComparatorResult != "not_measured_by_harness" {
		t.Fatalf("comparator result = %q, want an explicit unmeasured marker", caseReport.ComparatorResult)
	}
	if result.Report.GenerationConfig["usage"] != "not_applicable" {
		t.Fatalf("generation usage = %v, want not_applicable", result.Report.GenerationConfig["usage"])
	}
	if !result.Report.Preliminary || !caseReport.DevelopmentFixture || !caseReport.ExcludedFromReviewedTotals {
		t.Fatalf("fixture/preliminary labels missing: report=%+v case=%+v", result.Report, caseReport)
	}
	if caseReport.ContentIdentity == nil || caseReport.ContentIdentity.Artifact.SHA256 == "" ||
		caseReport.ContentIdentity.Specification.SHA256 == "" ||
		caseReport.ContentIdentity.Reference.SHA256 == "" {
		t.Fatalf("content identity missing from report: %+v", caseReport.ContentIdentity)
	}
	if caseReport.Provenance == nil ||
		caseReport.Provenance.SavedExerciseProvenance != "not_applicable" ||
		caseReport.Provenance.GenerationDuringAssessment {
		t.Fatalf("fixture provenance = %+v, want not_applicable with no generation", caseReport.Provenance)
	}
	if result.Report.ModelRevision != "not_applicable" || result.Report.PromptRevision != "not_applicable" {
		t.Fatalf("run revisions = %q/%q, want not_applicable", result.Report.ModelRevision, result.Report.PromptRevision)
	}
	if result.Report.Execution.ModelOperations || result.Report.Execution.PaidOperations || result.Report.Execution.Sandbox {
		t.Fatalf("execution claims operations that did not happen: %+v", result.Report.Execution)
	}
	if result.Report.Summary.WrongSolutionRejection == nil || *result.Report.Summary.WrongSolutionRejection >= 1 {
		t.Fatalf("wrong-solution rejection rate = %v, want an observed survival below 1", result.Report.Summary.WrongSolutionRejection)
	}
	if result.Report.Summary.BlindAgreementRate != "not_applicable" {
		t.Fatalf("blind agreement = %v, want not_applicable", result.Report.Summary.BlindAgreementRate)
	}
	if err := ValidateReportJSON(mustRead(t, output), schema); err != nil {
		t.Fatalf("written report does not validate: %v", err)
	}
}

func TestBuildReportFixtureProvenanceIsNotApplicable(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	report := BuildReport(Meta{
		RunID:              "pqb-fixture",
		StartedAt:          now,
		CompletedAt:        now.Add(time.Second),
		TestedCommit:       "abc123",
		BaselineKind:       "other",
		Timeout:            time.Second,
		DevelopmentFixture: true,
	}, []CaseRun{{Config: &RunConfig{DevelopmentFixture: true}}})
	if report.ModelRevision != "not_applicable" || report.PromptRevision != "not_applicable" {
		t.Fatalf("run revisions = %q/%q, want not_applicable", report.ModelRevision, report.PromptRevision)
	}
	if report.GenerationConfig["assessment_run_generation_occurred"] != false {
		t.Fatalf("assessment run generation = %v", report.GenerationConfig["assessment_run_generation_occurred"])
	}
	if report.GenerationConfig["saved_exercise_provenance"] != "not_applicable" {
		t.Fatalf("saved provenance = %v, want not_applicable", report.GenerationConfig["saved_exercise_provenance"])
	}
	provenance := report.Cases[0].Provenance
	if provenance == nil || provenance.SavedExerciseProvenance != "not_applicable" ||
		provenance.GenerationDuringAssessment ||
		provenance.ModelRevision != "not_applicable" ||
		provenance.PromptRevision != "not_applicable" ||
		provenance.ModelRevisionStatus != "not_applicable" ||
		provenance.PromptRevisionStatus != "not_applicable" {
		t.Fatalf("case provenance = %+v", provenance)
	}
}

func TestBuildReportSavedExerciseMissingProvenanceIsUnavailable(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	report := BuildReport(Meta{
		RunID:        "pqb-generated",
		StartedAt:    now,
		CompletedAt:  now.Add(time.Second),
		TestedCommit: "abc123",
		BaselineKind: "current",
		Timeout:      time.Second,
	}, []CaseRun{{Config: &RunConfig{}}})
	if report.ModelRevision != "unavailable" || report.PromptRevision != "unavailable" {
		t.Fatalf("run revisions = %q/%q, want unavailable", report.ModelRevision, report.PromptRevision)
	}
	if report.GenerationConfig["saved_exercise_provenance"] != "unavailable" {
		t.Fatalf("saved provenance = %v, want unavailable", report.GenerationConfig["saved_exercise_provenance"])
	}
	provenance := report.Cases[0].Provenance
	if provenance == nil || provenance.SavedExerciseProvenance != "unavailable" ||
		provenance.ModelRevision != "unavailable" ||
		provenance.PromptRevision != "unavailable" ||
		provenance.ModelRevisionStatus != "unavailable" ||
		provenance.PromptRevisionStatus != "unavailable" {
		t.Fatalf("case provenance = %+v", provenance)
	}
}

func TestBuildReportPreservesKnownSavedProvenance(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	report := BuildReport(Meta{
		RunID:        "pqb-known",
		StartedAt:    now,
		CompletedAt:  now.Add(time.Second),
		TestedCommit: "abc123",
		BaselineKind: "independent",
		Timeout:      time.Second,
	}, []CaseRun{{Config: &RunConfig{
		GenerationProvenance: &GenerationProvenanceConfig{
			ModelRevision:  "model-x",
			PromptRevision: "prompt-y",
			Config:         map[string]any{"temperature": 0.2},
		},
	}}})
	provenance := report.Cases[0].Provenance
	if provenance == nil || provenance.SavedExerciseProvenance != "known" ||
		provenance.ModelRevision != "model-x" || provenance.PromptRevision != "prompt-y" ||
		provenance.ModelRevisionStatus != "known" || provenance.PromptRevisionStatus != "known" {
		t.Fatalf("case provenance = %+v", provenance)
	}
	if provenance.GenerationConfig["temperature"] != 0.2 {
		t.Fatalf("generation config = %+v, want supplied config preserved", provenance.GenerationConfig)
	}
}

func TestExecuteRejectsPaidModeBeforeWork(t *testing.T) {
	result, err := Execute(context.Background(), Options{
		CaseDirs:     []string{fixtureDir},
		BaselineKind: "other",
		ExecuteLocal: false,
	})
	if err == nil {
		t.Fatalf("Execute = %+v, want explicit local-execution rejection", result)
	}
	if !strings.Contains(err.Error(), "local execution must be explicitly selected") {
		t.Fatalf("error = %v", err)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestBuildReportDoesNotCopyExpectedOutcomesIntoObservedFields(t *testing.T) {
	blindAgreement := true
	wrongRejected := false
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	report := BuildReport(Meta{
		RunID:        "pqb-expected",
		StartedAt:    now,
		CompletedAt:  now.Add(time.Second),
		TestedCommit: "abc123",
		BaselineKind: "other",
		Timeout:      time.Second,
	}, []CaseRun{{
		Case: &problemquality.Case{
			CaseID:     "observed-case",
			Technology: "python",
			ExpectedOutcomes: &problemquality.ExpectedOutcomes{
				Generation: &problemquality.GenerationExpectations{
					BlindAgreement: &blindAgreement,
					Status:         "measured",
				},
				Assessment: &problemquality.AssessmentExpectations{
					WrongSolutionsRejected: &wrongRejected,
				},
			},
		},
		Collection: environmentverify.AssessmentCollection{
			Reference: &environmentverify.AssessmentOutcome{
				ID:              "reference",
				Role:            environmentverify.RoleGeneratedReference,
				Passed:          true,
				ExecutionStatus: environmentverify.ExecutionCompleted,
			},
		},
	}})
	caseReport := report.Cases[0]
	if caseReport.Status != CaseStatusPassed {
		t.Fatalf("status = %q, want passed from observed evidence", caseReport.Status)
	}
	if caseReport.Generation.Status != "not_applicable" ||
		caseReport.Generation.BlindAgreement != nil ||
		caseReport.Generation.ReconciliationCount != nil ||
		caseReport.Generation.NonConvergence != nil {
		t.Fatalf("generation = %+v, want unobserved not_applicable fields", caseReport.Generation)
	}
	if report.Summary.WrongSolutionRejection != nil {
		t.Fatalf("wrong rejection = %v, want nil without observed wrong solutions", report.Summary.WrongSolutionRejection)
	}
}

func TestBuildReportRecordsBlockedCaseErrorWithoutAborting(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	report := BuildReport(Meta{
		RunID:        "pqb-test",
		StartedAt:    now,
		CompletedAt:  now.Add(time.Second),
		TestedCommit: "abc123",
		BaselineKind: "other",
		Timeout:      time.Second,
		Selection:    "corpus",
		CorpusRoot:   "/tmp/corpus",
	}, []CaseRun{{
		Dir: "/tmp/corpus/incomplete-case",
		Err: errors.New("case.yaml is missing"),
	}})
	if len(report.Cases) != 1 {
		t.Fatalf("cases = %d", len(report.Cases))
	}
	caseReport := report.Cases[0]
	if caseReport.Status != CaseStatusError || !caseReport.Blocked ||
		!strings.Contains(caseReport.BlockReason, "case.yaml is missing") {
		t.Fatalf("case report = %+v, want explicit blocked error", caseReport)
	}
	if report.Summary.CasesError != 1 {
		t.Fatalf("summary = %+v", report.Summary)
	}
	schema := filepath.Join("..", "..", "..", "..", "benchmarks", "problem-quality", "schema", "report.schema.json")
	data, err := MarshalReport(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateReportJSON(data, schema); err != nil {
		t.Fatalf("blocked-case report does not validate: %v", err)
	}
}
