package benchmark

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/environmentverify"
	"github.com/kvn8888/codegym/backend/internal/problemquality"
)

func TestReportVersionIsExplicitAndSchemaBound(t *testing.T) {
	report := BuildReport(Meta{
		RunID:        "pqb-version",
		StartedAt:    time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC),
		CompletedAt:  time.Date(2026, 11, 18, 12, 0, 1, 0, time.UTC),
		TestedCommit: "abc123",
		BaselineKind: "other",
		Timeout:      time.Second,
	}, nil)
	if report.ReportVersion != "2.0.0" {
		t.Fatalf("report_version = %q, want 2.0.0", report.ReportVersion)
	}
	validateReportSchema(t, report)

	data, err := MarshalReport(report)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	raw["report_version"] = "1.0.0"
	bad, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	schema := filepath.Join("..", "..", "..", "..", "benchmarks", "problem-quality", "schema", "report.schema.json")
	if err := ValidateReportJSON(bad, schema); err == nil || !strings.Contains(err.Error(), "report_version") {
		t.Fatalf("validation error = %v, want report_version rejection", err)
	}
	delete(raw, "report_version")
	missing, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateReportJSON(missing, schema); err == nil || !strings.Contains(err.Error(), "report_version") {
		t.Fatalf("validation error = %v, want missing report_version rejection", err)
	}
}

func TestQualityMetricsPreserveCountsExclusionsAndDenominators(t *testing.T) {
	eligibleA := metricCase("eligible-a", false, true, "eligible")
	eligibleA.CorrectAlternatives = []CorrectOutcome{
		metricCorrect("accepted", true, environmentverify.ExecutionCompleted),
		metricCorrect("rejected", false, environmentverify.ExecutionCompleted),
		metricCorrect("timed-out", false, environmentverify.ExecutionTimeout),
	}
	eligibleA.WrongSolutions = []WrongOutcome{
		metricWrong("matched", environmentverify.DenominatorMeaningful, environmentverify.ExecutionCompleted, true, true),
		metricWrong("survivor", environmentverify.DenominatorMeaningful, environmentverify.ExecutionCompleted, false, false),
		metricWrong("timed-out", environmentverify.DenominatorMeaningful, environmentverify.ExecutionTimeout, false, false),
		metricWrong("equivalent", environmentverify.DenominatorEquivalent, environmentverify.ExecutionExcluded, false, false),
		metricWrong("invalid", environmentverify.DenominatorInvalid, environmentverify.ExecutionExcluded, false, false),
	}
	eligibleA.AmbiguousSpecRecorded = true
	eligibleA.AmbiguousSpec = true

	eligibleB := metricCase("eligible-b", false, true, "eligible")
	eligibleB.CorrectAlternatives = []CorrectOutcome{
		metricCorrect("accepted", true, environmentverify.ExecutionCompleted),
	}
	eligibleB.WrongSolutions = []WrongOutcome{
		metricWrong("matched", environmentverify.DenominatorMeaningful, environmentverify.ExecutionCompleted, true, true),
	}
	eligibleB.AmbiguousSpecRecorded = true
	eligibleB.AmbiguousSpec = false

	fixture := metricCase("fixture", true, true, "eligible")
	fixture.CorrectAlternatives = []CorrectOutcome{metricCorrect("accepted", true, environmentverify.ExecutionCompleted)}
	fixture.WrongSolutions = []WrongOutcome{metricWrong("survivor", environmentverify.DenominatorMeaningful, environmentverify.ExecutionCompleted, false, false)}

	unresolved := metricCase("unresolved", false, false, environmentverify.BenchmarkEligibilityUnresolved)
	unresolved.CorrectAlternatives = []CorrectOutcome{metricCorrect("accepted", true, environmentverify.ExecutionCompleted)}
	unresolved.WrongSolutions = []WrongOutcome{metricWrong("matched", environmentverify.DenominatorMeaningful, environmentverify.ExecutionCompleted, true, true)}

	report := metricReport([]CaseReport{eligibleA, eligibleB, fixture, unresolved}, "other")
	metrics := report.Summary.Metrics

	if metrics.Cases.Total != 4 || metrics.Cases.Eligible != 2 ||
		metrics.Cases.DevelopmentFixturesExcluded != 1 ||
		metrics.Cases.UnresolvedEligibilityExcluded != 1 {
		t.Fatalf("case basis = %+v", metrics.Cases)
	}
	assertMetric(t, "correct alternative acceptance", metrics.CorrectAlternativeAcceptance, 2, 3, 1)
	if reason := metrics.CorrectAlternativeAcceptance.ExclusionReasons["non_completed"]; reason != 1 {
		t.Fatalf("correct exclusions = %+v", metrics.CorrectAlternativeAcceptance.ExclusionReasons)
	}
	assertMetric(t, "wrong solution rejection", metrics.WrongSolutionRejection, 2, 3, 3)
	assertMetric(t, "discrimination", metrics.DiscriminationScore, 2, 3, 3)
	for _, reason := range []string{"equivalent", "invalid", "non_completed"} {
		if metrics.WrongSolutionRejection.ExclusionReasons[reason] != 1 {
			t.Fatalf("wrong exclusion %q = %d, want 1 in %+v", reason,
				metrics.WrongSolutionRejection.ExclusionReasons[reason], metrics.WrongSolutionRejection.ExclusionReasons)
		}
	}
	assertMetric(t, "ambiguous specification", metrics.AmbiguousSpecificationRate, 1, 2, 0)
	if metrics.CorrectAlternativeAcceptance.Definition == "" ||
		metrics.WrongSolutionRejection.Definition == "" ||
		metrics.DiscriminationScore.Definition == "" ||
		metrics.AmbiguousSpecificationRate.Definition == "" {
		t.Fatal("metric definitions must be present")
	}
	if report.Summary.CorrectAlternativeAcceptance == nil || *report.Summary.CorrectAlternativeAcceptance != 2.0/3.0 ||
		report.Summary.WrongSolutionRejection == nil || *report.Summary.WrongSolutionRejection != 2.0/3.0 ||
		report.Summary.DiscriminationScore == nil || *report.Summary.DiscriminationScore != 2.0/3.0 ||
		report.Summary.AmbiguousSpecificationRate == nil || *report.Summary.AmbiguousSpecificationRate != 0.5 {
		t.Fatalf("legacy summary values = %+v", report.Summary)
	}
	validateReportSchema(t, report)
}

func TestQualityMetricsDistinguishMeasuredZeroFromUnavailable(t *testing.T) {
	t.Run("measured zero", func(t *testing.T) {
		caseReport := metricCase("eligible", false, true, "eligible")
		caseReport.CorrectAlternatives = []CorrectOutcome{
			metricCorrect("rejected", false, environmentverify.ExecutionCompleted),
		}
		caseReport.AmbiguousSpecRecorded = true
		report := metricReport([]CaseReport{caseReport}, "other")
		if report.Summary.Metrics.CorrectAlternativeAcceptance.Status != MetricMeasured ||
			report.Summary.Metrics.CorrectAlternativeAcceptance.Value == nil ||
			*report.Summary.Metrics.CorrectAlternativeAcceptance.Value != 0 {
			t.Fatalf("correct acceptance = %+v, want measured zero", report.Summary.Metrics.CorrectAlternativeAcceptance)
		}
		if report.Summary.Metrics.AmbiguousSpecificationRate.Status != MetricMeasured ||
			report.Summary.Metrics.AmbiguousSpecificationRate.Value == nil ||
			*report.Summary.Metrics.AmbiguousSpecificationRate.Value != 0 {
			t.Fatalf("ambiguity rate = %+v, want measured zero", report.Summary.Metrics.AmbiguousSpecificationRate)
		}
		if report.Summary.Metrics.WrongSolutionRejection.Status != MetricUnavailable ||
			report.Summary.Metrics.WrongSolutionRejection.Value != nil ||
			!strings.Contains(report.Summary.Metrics.WrongSolutionRejection.UnavailableReason, "no completed") {
			t.Fatalf("wrong rejection = %+v, want unavailable", report.Summary.Metrics.WrongSolutionRejection)
		}
		validateReportSchema(t, report)
	})

	t.Run("zero eligible", func(t *testing.T) {
		report := metricReport([]CaseReport{metricCase("unresolved", false, false, environmentverify.BenchmarkEligibilityUnresolved)}, "other")
		metrics := report.Summary.Metrics
		for _, metric := range []Metric{
			metrics.CorrectAlternativeAcceptance,
			metrics.WrongSolutionRejection,
			metrics.DiscriminationScore,
			metrics.AmbiguousSpecificationRate,
		} {
			if metric.Status != MetricUnavailable || metric.Value != nil ||
				!strings.Contains(metric.UnavailableReason, "insufficient evidence") {
				t.Fatalf("metric = %+v, want insufficient-evidence unavailability", metric)
			}
		}
		if report.Summary.CorrectAlternativeAcceptance != nil || report.Summary.WrongSolutionRejection != nil {
			t.Fatalf("legacy values = %v/%v, want nil", report.Summary.CorrectAlternativeAcceptance, report.Summary.WrongSolutionRejection)
		}
		validateReportSchema(t, report)
	})
}

func TestAmbiguityRateExcludesMissingReviewedDecision(t *testing.T) {
	caseReport := metricCase("eligible", false, true, "eligible")
	report := metricReport([]CaseReport{caseReport}, "other")
	metric := report.Summary.Metrics.AmbiguousSpecificationRate
	if metric.Denominator != 0 || metric.Excluded != 1 ||
		metric.ExclusionReasons["missing_reviewed_ambiguity_decision"] != 1 ||
		metric.Status != MetricUnavailable {
		t.Fatalf("ambiguity metric = %+v, want missing decision excluded", metric)
	}
	validateReportSchema(t, report)
}

func TestHistoricalGenerationObservationsUseIdentityAndRemainDistinct(t *testing.T) {
	expectedAgreement := true
	observedAgreement := false
	reconciliations := 2
	nonConvergent := true
	identity := &ContentIdentity{
		Algorithm:           ContentIdentityAlgorithm,
		CaseDefinition:      ContentFile{Path: "case.yaml", SHA256: strings.Repeat("c", 64)},
		Specification:       ContentFile{Path: "spec.md", SHA256: strings.Repeat("d", 64)},
		RunConfig:           ContentFile{Path: "benchmark-run.json", SHA256: strings.Repeat("e", 64)},
		Reference:           SolutionContent{ID: "reference", Files: []ContentFile{}, SHA256: strings.Repeat("b", 64)},
		CorrectAlternatives: []SolutionContent{},
		WrongSolutions:      []SolutionContent{},
		Artifact:            ArtifactContent{Files: []ContentFile{}, SHA256: strings.Repeat("a", 64)},
	}
	config := &RunConfig{GenerationProvenance: &GenerationProvenanceConfig{
		Observations: &GenerationObservationsConfig{
			Source:                  "historical generation export",
			ArtifactSHA256:          identity.Artifact.SHA256,
			InputSHA256:             identity.Reference.SHA256,
			FirstAttemptAgreement:   &observedAgreement,
			ReconciliationCount:     &reconciliations,
			ReconciliationDirection: "tests",
			NonConvergence:          &nonConvergent,
		},
	}}
	report := BuildReport(Meta{
		RunID:        "pqb-historical-generation",
		StartedAt:    time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC),
		CompletedAt:  time.Date(2026, 11, 18, 12, 0, 1, 0, time.UTC),
		TestedCommit: "abc123",
		BaselineKind: "independent",
		Timeout:      time.Second,
	}, []CaseRun{{
		Case: &problemquality.Case{
			CaseID:     "generated-case",
			Technology: "go",
			ExpectedOutcomes: &problemquality.ExpectedOutcomes{
				Generation: &problemquality.GenerationExpectations{
					BlindAgreement: &expectedAgreement,
					Status:         "measured",
				},
			},
		},
		Config:   config,
		Identity: identity,
	}})

	generation := report.Cases[0].Generation
	if generation.Status != GenerationMeasured || generation.FirstAttemptAgreement == nil ||
		*generation.FirstAttemptAgreement != observedAgreement ||
		generation.Source != "historical generation export" ||
		generation.ArtifactSHA256 != identity.Artifact.SHA256 ||
		generation.InputSHA256 != identity.Reference.SHA256 {
		t.Fatalf("generation = %+v, want supplied historical observation, not expected_outcomes", generation)
	}
	if report.Summary.BlindAgreementRate != float64(0) ||
		report.Summary.ReconciliationFrequency != float64(1) ||
		report.Summary.NonConvergenceRate != float64(1) {
		t.Fatalf("generation rates = %v/%v/%v", report.Summary.BlindAgreementRate,
			report.Summary.ReconciliationFrequency, report.Summary.NonConvergenceRate)
	}
	metrics := report.Summary.Metrics
	assertMetric(t, "first attempt", metrics.FirstAttemptAgreement, 0, 1, 0)
	assertMetric(t, "reconciliation", metrics.ReconciliationFrequency, 1, 1, 0)
	assertMetric(t, "non-convergence", metrics.NonConvergenceRate, 1, 1, 0)
	if metrics.ReconciliationDirections["tests"] != 1 || metrics.Cases.ObservedGenerationCases != 1 {
		t.Fatalf("generation detail = %+v", metrics)
	}
	for _, kind := range []string{FindingKindFirstAttemptDisagreement, FindingKindReconciliation, FindingKindNonConvergence} {
		finding, ok := findFinding(report.Findings, kind)
		if !ok || finding.CauseEstablished != true || len(finding.Evidence) == 0 ||
			finding.Evidence[0].Source != "generation-observation" {
			t.Fatalf("finding %q = %+v, present=%t", kind, finding, ok)
		}
	}
	validateReportSchema(t, report)
}

func TestGenerationObservationValidationRejectsUnsupportedRecords(t *testing.T) {
	validHash := strings.Repeat("A", 64)
	agreement := false
	zero := 0
	one := 1
	tests := []struct {
		name string
		obs  GenerationObservationsConfig
		want string
	}{
		{name: "missing source", obs: GenerationObservationsConfig{ArtifactSHA256: validHash, InputSHA256: validHash, FirstAttemptAgreement: &agreement}, want: "source is required"},
		{name: "missing observations", obs: GenerationObservationsConfig{Source: "export", ArtifactSHA256: validHash, InputSHA256: validHash}, want: "at least one"},
		{name: "bad artifact digest", obs: GenerationObservationsConfig{Source: "export", ArtifactSHA256: "abc", InputSHA256: validHash, FirstAttemptAgreement: &agreement}, want: "artifact_sha256"},
		{name: "bad input digest", obs: GenerationObservationsConfig{Source: "export", ArtifactSHA256: validHash, InputSHA256: strings.Repeat("z", 64), FirstAttemptAgreement: &agreement}, want: "input_sha256"},
		{name: "negative count", obs: GenerationObservationsConfig{Source: "export", ArtifactSHA256: validHash, InputSHA256: validHash, ReconciliationCount: intPointer(-1)}, want: "non-negative"},
		{name: "bad direction", obs: GenerationObservationsConfig{Source: "export", ArtifactSHA256: validHash, InputSHA256: validHash, ReconciliationCount: &one, ReconciliationDirection: "model"}, want: "reconciliation_direction"},
		{name: "zero direction count", obs: GenerationObservationsConfig{Source: "export", ArtifactSHA256: validHash, InputSHA256: validHash, ReconciliationCount: &zero, ReconciliationDirection: "both"}, want: "positive reconciliation_count"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateGenerationObservations(&test.obs)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}

	valid := GenerationObservationsConfig{
		Source:                  " export ",
		ArtifactSHA256:          validHash,
		InputSHA256:             validHash,
		ReconciliationCount:     &one,
		ReconciliationDirection: " both ",
	}
	if err := validateGenerationObservations(&valid); err != nil {
		t.Fatalf("valid observations rejected: %v", err)
	}
	if valid.Source != "export" || valid.ArtifactSHA256 != strings.ToLower(validHash) ||
		valid.ReconciliationDirection != "both" {
		t.Fatalf("normalized observations = %+v", valid)
	}

	fixtureConfig := []byte(`{
		"development_fixture": true,
		"generation_provenance": {
			"observations": {
				"source": "export",
				"artifact_sha256": "` + validHash + `",
				"input_sha256": "` + validHash + `",
				"first_attempt_agreement": false
			}
		}
	}`)
	if _, err := decodeRunConfig(fixtureConfig); err == nil || !strings.Contains(err.Error(), "development fixtures cannot supply") {
		t.Fatalf("fixture config error = %v", err)
	}
}

func TestGenerationObservationReconciliationDirectionRules(t *testing.T) {
	validHash := strings.Repeat("a", 64)
	zero := 0
	one := 1
	tests := []struct {
		name          string
		count         *int
		direction     string
		wantErr       string
		wantDirection string
	}{
		{name: "positive with reference", count: &one, direction: "reference", wantDirection: "reference"},
		{name: "positive with tests", count: &one, direction: "tests", wantDirection: "tests"},
		{name: "positive with both", count: &one, direction: " both ", wantDirection: "both"},
		{name: "positive missing direction", count: &one, wantErr: "reconciliation_direction is required"},
		{name: "positive whitespace direction", count: &one, direction: " \t ", wantErr: "reconciliation_direction is required"},
		{name: "direction missing count", direction: "reference", wantErr: "positive reconciliation_count"},
		{name: "direction zero count", count: &zero, direction: "tests", wantErr: "positive reconciliation_count"},
		{name: "zero without direction", count: &zero},
		{name: "zero whitespace direction", count: &zero, direction: " \t "},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			observations := GenerationObservationsConfig{
				Source:                  "export",
				ArtifactSHA256:          validHash,
				InputSHA256:             validHash,
				ReconciliationCount:     test.count,
				ReconciliationDirection: test.direction,
			}
			err := validateGenerationObservations(&observations)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("error = %v, want %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("valid observations rejected: %v", err)
			}
			if observations.ReconciliationDirection != test.wantDirection {
				t.Fatalf("direction = %q, want %q", observations.ReconciliationDirection, test.wantDirection)
			}
		})
	}
	if err := validateGenerationObservations(nil); err != nil {
		t.Fatalf("missing observations rejected: %v", err)
	}
}

func TestGenerationFailureObservationValidation(t *testing.T) {
	validHash := strings.Repeat("a", 64)
	base := func() GenerationObservationsConfig {
		return GenerationObservationsConfig{Source: " export ", ArtifactSHA256: validHash, InputSHA256: validHash}
	}
	t.Run("recorded failure", func(t *testing.T) {
		observations := base()
		failed := true
		observations.GenerationFailed = &failed
		observations.GenerationFailureDetails = "  generator returned invalid structured output  "
		if err := validateGenerationObservations(&observations); err != nil {
			t.Fatalf("recorded failure rejected: %v", err)
		}
		if observations.Source != "export" || observations.GenerationFailureDetails != "generator returned invalid structured output" {
			t.Fatalf("normalized observations = %+v", observations)
		}
	})
	t.Run("explicit non-failure", func(t *testing.T) {
		observations := base()
		failed := false
		observations.GenerationFailed = &failed
		if err := validateGenerationObservations(&observations); err != nil {
			t.Fatalf("explicit non-failure rejected: %v", err)
		}
	})
	tests := []struct {
		name string
		obs  GenerationObservationsConfig
		want string
	}{
		{
			name: "failure missing details",
			obs: func() GenerationObservationsConfig {
				observations := base()
				failed := true
				observations.GenerationFailed = &failed
				return observations
			}(),
			want: "generation_failure_details is required",
		},
		{
			name: "failure whitespace details",
			obs: func() GenerationObservationsConfig {
				observations := base()
				failed := true
				observations.GenerationFailed = &failed
				observations.GenerationFailureDetails = " \t "
				return observations
			}(),
			want: "generation_failure_details is required",
		},
		{
			name: "details without failure record",
			obs: func() GenerationObservationsConfig {
				observations := base()
				observations.GenerationFailureDetails = "generator crashed"
				return observations
			}(),
			want: "requires generation_failed",
		},
		{
			name: "non-failure with details",
			obs: func() GenerationObservationsConfig {
				observations := base()
				failed := false
				observations.GenerationFailed = &failed
				observations.GenerationFailureDetails = "should not be here"
				return observations
			}(),
			want: "must be empty",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateGenerationObservations(&test.obs)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
	if err := validateGenerationObservations(nil); err != nil {
		t.Fatalf("missing observations rejected: %v", err)
	}
}

func TestGenerationFailureObservationReportAndSchema(t *testing.T) {
	failed := true
	report := buildGenerationObservationReport(t, &GenerationObservationsConfig{
		Source:                   "historical failure export",
		ArtifactSHA256:           historicalGenerationIdentity().Artifact.SHA256,
		InputSHA256:              historicalGenerationIdentity().Reference.SHA256,
		GenerationFailed:         &failed,
		GenerationFailureDetails: "generator returned invalid structured output",
	})
	generation := report.Cases[0].Generation
	if generation.Status != GenerationMeasured || generation.GenerationFailed == nil ||
		!*generation.GenerationFailed ||
		generation.GenerationFailureDetails != "generator returned invalid structured output" {
		t.Fatalf("generation = %+v, want recorded failure", generation)
	}
	finding, ok := findFinding(report.Cases[0].Findings, FindingKindGenerationFailure)
	if !ok || !finding.CauseEstablished || len(finding.FollowUps) == 0 ||
		finding.FollowUps[0].Owner != "generation-review" ||
		!strings.Contains(finding.Summary, "generator returned invalid structured output") {
		t.Fatalf("case failure finding = %+v, present=%t", finding, ok)
	}
	if len(finding.Evidence) == 0 || finding.Evidence[0].Source != "generation-observation" ||
		!strings.Contains(finding.Evidence[0].Detail, "artifact_sha256=") ||
		!strings.Contains(finding.Evidence[0].Detail, "input_sha256=") ||
		!strings.Contains(finding.Evidence[0].Detail, "failure_details=generator returned invalid structured output") {
		t.Fatalf("failure evidence = %+v, want available identity and details", finding.Evidence)
	}
	if finding.ArtifactID != "historical-artifact" || finding.ArtifactVersion != "0.9.0" ||
		finding.ArtifactIdentityStatus != ArtifactIdentityAvailable ||
		finding.Scope != FindingScopeHistoricalGeneration ||
		finding.QualityMetricTreatment != QualityMetricTreatmentNotApplicable ||
		!strings.Contains(finding.QualityMetricTreatmentReason, "assessment-quality") {
		t.Fatalf("generation failure finding context = %+v", finding)
	}
	if _, ok := findFinding(report.Findings, FindingKindGenerationFailure); !ok {
		t.Fatalf("top-level findings = %+v, want generation failure", report.Findings)
	}
	validateReportSchema(t, report)

	notFailed := false
	report = buildGenerationObservationReport(t, &GenerationObservationsConfig{
		Source:           "historical non-failure export",
		ArtifactSHA256:   historicalGenerationIdentity().Artifact.SHA256,
		InputSHA256:      historicalGenerationIdentity().Reference.SHA256,
		GenerationFailed: &notFailed,
	})
	generation = report.Cases[0].Generation
	if generation.GenerationFailed == nil || *generation.GenerationFailed ||
		generation.GenerationFailureDetails != "" {
		t.Fatalf("generation = %+v, want explicit non-failure", generation)
	}
	if _, ok := findFinding(report.Findings, FindingKindGenerationFailure); ok {
		t.Fatalf("findings = %+v, explicit non-failure must not emit a failure finding", report.Findings)
	}
	validateReportSchema(t, report)
}

func TestGenerationFailureIsNotInferredFromOtherObservations(t *testing.T) {
	t.Run("absent observations", func(t *testing.T) {
		report := buildGenerationObservationReport(t, nil)
		generation := report.Cases[0].Generation
		if generation.Status != GenerationUnavailable || generation.GenerationFailed != nil {
			t.Fatalf("generation = %+v, want unavailable without failure", generation)
		}
		if _, ok := findFinding(report.Findings, FindingKindGenerationFailure); ok {
			t.Fatalf("findings = %+v, absent observations must not infer failure", report.Findings)
		}
		validateReportSchema(t, report)
	})
	t.Run("disagreement and non-convergence", func(t *testing.T) {
		agreement := false
		reconciliations := 1
		nonConvergence := true
		report := buildGenerationObservationReport(t, &GenerationObservationsConfig{
			Source:                  "historical generation export",
			ArtifactSHA256:          historicalGenerationIdentity().Artifact.SHA256,
			InputSHA256:             historicalGenerationIdentity().Reference.SHA256,
			FirstAttemptAgreement:   &agreement,
			ReconciliationCount:     &reconciliations,
			ReconciliationDirection: "tests",
			NonConvergence:          &nonConvergence,
		})
		generation := report.Cases[0].Generation
		if generation.GenerationFailed != nil {
			t.Fatalf("generation = %+v, failure must require an explicit observation", generation)
		}
		if _, ok := findFinding(report.Findings, FindingKindGenerationFailure); ok {
			t.Fatalf("findings = %+v, disagreement/non-convergence must not infer failure", report.Findings)
		}
		for _, kind := range []string{FindingKindFirstAttemptDisagreement, FindingKindReconciliation, FindingKindNonConvergence} {
			if _, ok := findFinding(report.Findings, kind); !ok {
				t.Fatalf("findings = %+v, want %q", report.Findings, kind)
			}
		}
		validateReportSchema(t, report)
	})
}

func TestGenerationFailureObservationIdentityMismatch(t *testing.T) {
	failed := true
	observations := &GenerationObservationsConfig{
		Source:                   "historical failure export",
		ArtifactSHA256:           strings.Repeat("f", 64),
		InputSHA256:              historicalGenerationIdentity().Reference.SHA256,
		GenerationFailed:         &failed,
		GenerationFailureDetails: "generator failed after producing a partial artifact",
	}
	identity := historicalGenerationIdentity()
	config := &RunConfig{GenerationProvenance: &GenerationProvenanceConfig{Observations: observations}}
	if err := validateGenerationObservationIdentity(config, identity, "independent"); err == nil ||
		!strings.Contains(err.Error(), "artifact_sha256") {
		t.Fatalf("identity error = %v, want artifact mismatch", err)
	}
	report := BuildReport(generationObservationMeta(), []CaseRun{{
		Case:     &problemquality.Case{CaseID: "generated-case", Technology: "go"},
		Config:   config,
		Identity: identity,
	}})
	generation := report.Cases[0].Generation
	if generation.Status != GenerationUnavailable || generation.GenerationFailed != nil {
		t.Fatalf("generation = %+v, want unavailable without copied mismatch", generation)
	}
	if _, ok := findFinding(report.Findings, FindingKindGenerationFailure); ok {
		t.Fatalf("findings = %+v, identity mismatch must not emit failure evidence", report.Findings)
	}
	validateReportSchema(t, report)
}

func TestGenerationObservationIdentityMustMatchSavedContentAndIndependentBaseline(t *testing.T) {
	hashA := strings.Repeat("a", 64)
	hashB := strings.Repeat("b", 64)
	hashC := strings.Repeat("c", 64)
	identity := &ContentIdentity{
		Artifact:  ArtifactContent{SHA256: hashA},
		Reference: SolutionContent{SHA256: hashB},
	}
	observations := func(artifact, input string) *GenerationObservationsConfig {
		agreement := true
		return &GenerationObservationsConfig{
			Source:                "export",
			ArtifactSHA256:        artifact,
			InputSHA256:           input,
			FirstAttemptAgreement: &agreement,
		}
	}
	config := &RunConfig{GenerationProvenance: &GenerationProvenanceConfig{Observations: observations(hashA, hashB)}}
	if err := validateGenerationObservationIdentity(config, identity, "independent"); err != nil {
		t.Fatalf("matching identity rejected: %v", err)
	}
	if err := validateGenerationObservationIdentity(config, identity, "current"); err == nil ||
		!strings.Contains(err.Error(), "baseline_kind independent") {
		t.Fatalf("non-independent error = %v", err)
	}
	config.GenerationProvenance.Observations = observations(hashC, hashB)
	if err := validateGenerationObservationIdentity(config, identity, "independent"); err == nil ||
		!strings.Contains(err.Error(), "artifact_sha256") {
		t.Fatalf("artifact mismatch error = %v", err)
	}
	config.GenerationProvenance.Observations = observations(hashA, hashC)
	if err := validateGenerationObservationIdentity(config, identity, "independent"); err == nil ||
		!strings.Contains(err.Error(), "input_sha256") {
		t.Fatalf("input mismatch error = %v", err)
	}
	config.GenerationProvenance.Observations = observations(hashA, hashB)
	if err := validateGenerationObservationIdentity(config, nil, "independent"); err == nil ||
		!strings.Contains(err.Error(), "saved input identity") {
		t.Fatalf("missing identity error = %v", err)
	}
}

func TestSurvivingWrongSolutionFindingCarriesReviewEvidenceAndHypothesis(t *testing.T) {
	expected := "3"
	actual := "2"
	run := testCaseRun("survivor-case", false, true, "eligible", environmentverify.AssessmentCollection{
		Reference:           completedReference(true),
		CorrectAlternatives: []environmentverify.AssessmentOutcome{completedCorrect("alternative", true)},
		WrongSolutions: []environmentverify.AssessmentOutcome{{
			ID:                  "survivor",
			Role:                environmentverify.RoleWrongSolution,
			DenominatorDecision: environmentverify.DenominatorMeaningful,
			ExecutionStatus:     environmentverify.ExecutionCompleted,
			Passed:              false,
			DiagnosticCode:      environmentverify.CodeWrongSolutionPassed,
			FailureCategory:     environmentverify.FailureCategoryTest,
			Message:             "wrong solution survived",
			Report: environmentverify.AssessmentReport{Tests: []environmentverify.TestOutcome{{
				TestID:       "hidden-sum",
				Passed:       false,
				FailureClass: "assertion",
				Assertion:    &environmentverify.ObservedAssertion{Code: "sum-mismatch", Expected: expected, Actual: actual},
			}}},
		}},
	})
	run.Case.WrongSolutions = []problemquality.WrongSolution{{
		ID: "survivor",
		IntendedFailure: problemquality.IntendedFailure{
			TestID: "hidden-sum", Code: "sum-mismatch", Expected: &expected, Actual: &actual, Reason: "sum mismatch",
		},
	}}
	run.Case.ComparatorExpectations = problemquality.ComparatorExpectations{
		Default: problemquality.ComparatorDefault{Kind: "set"},
		Notes:   "Order is irrelevant.",
	}
	run.Config.ArtifactID = "survivor-artifact"
	run.Config.ArtifactVersion = "2.1.0"
	report := BuildReport(Meta{
		RunID:        "pqb-survivor",
		StartedAt:    time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC),
		CompletedAt:  time.Date(2026, 11, 18, 12, 0, 1, 0, time.UTC),
		TestedCommit: "abc123",
		BaselineKind: "other",
		Timeout:      time.Second,
	}, []CaseRun{run})

	finding, ok := findFinding(report.Cases[0].Findings, FindingKindSurvivingWrongSolution)
	if !ok {
		t.Fatalf("case findings = %+v, want survivor finding", report.Cases[0].Findings)
	}
	if finding.SolutionID != "survivor" || finding.TestID != "hidden-sum" ||
		finding.AssertionCode != "sum-mismatch" || !strings.Contains(finding.ComparatorContext, "set") ||
		finding.Hypothesis == "" || finding.CauseEstablished ||
		finding.ReviewAction == "" || len(finding.FollowUps) == 0 || len(finding.Evidence) == 0 {
		t.Fatalf("survivor finding = %+v", finding)
	}
	if finding.ArtifactID != "survivor-artifact" || finding.ArtifactVersion != "2.1.0" ||
		finding.ArtifactIdentityStatus != ArtifactIdentityAvailable ||
		finding.Scope != FindingScopeAssessmentQuality ||
		finding.QualityMetricTreatment != QualityMetricTreatmentIncluded ||
		!strings.Contains(finding.QualityMetricTreatmentReason, "wrong-solution rejection") {
		t.Fatalf("survivor finding context = %+v", finding)
	}
	if _, ok := findFinding(report.Findings, FindingKindSurvivingWrongSolution); !ok {
		t.Fatalf("top-level findings = %+v, want survivor finding", report.Findings)
	}
	validateReportSchema(t, report)
}

func TestFixtureSurvivorFindingRecordsArtifactAndMetricExclusion(t *testing.T) {
	run := testCaseRun("fixture-survivor", true, false, environmentverify.BenchmarkEligibilityUnresolved, environmentverify.AssessmentCollection{
		Reference: completedReference(true),
		WrongSolutions: []environmentverify.AssessmentOutcome{
			completedWrong("survivor", false, environmentverify.DenominatorMeaningful),
		},
	})
	run.Config.ArtifactID = "fixture-artifact"
	run.Config.ArtifactVersion = "0.9.0"
	report := BuildReport(Meta{
		RunID:        "pqb-fixture-survivor",
		StartedAt:    time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC),
		CompletedAt:  time.Date(2026, 11, 18, 12, 0, 1, 0, time.UTC),
		TestedCommit: "abc123",
		BaselineKind: "other",
		Timeout:      time.Second,
	}, []CaseRun{run})

	finding, ok := findFinding(report.Cases[0].Findings, FindingKindSurvivingWrongSolution)
	if !ok {
		t.Fatalf("case findings = %+v, want fixture survivor finding", report.Cases[0].Findings)
	}
	if finding.ArtifactID != "fixture-artifact" || finding.ArtifactVersion != "0.9.0" ||
		finding.ArtifactIdentityStatus != ArtifactIdentityAvailable ||
		finding.Scope != FindingScopeAssessmentQuality ||
		finding.QualityMetricTreatment != QualityMetricTreatmentExcluded ||
		!strings.Contains(finding.QualityMetricTreatmentReason, "Development fixture") {
		t.Fatalf("fixture survivor finding context = %+v", finding)
	}
	validateReportSchema(t, report)
}

func TestSavedExampleReportsValidateAndStayLabeled(t *testing.T) {
	schema := filepath.Join("..", "..", "..", "..", "benchmarks", "problem-quality", "schema", "report.schema.json")
	paths, err := filepath.Glob(filepath.Join("..", "..", "..", "..", "benchmark-results", "examples", "*", "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) < 2 {
		t.Fatalf("example reports = %v, want survivor and setup-failure examples", paths)
	}
	kinds := map[string]bool{}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := ValidateReportJSON(data, schema); err != nil {
			t.Fatalf("%s does not validate: %v", path, err)
		}
		var report Report
		if err := json.Unmarshal(data, &report); err != nil {
			t.Fatal(err)
		}
		if report.ReportVersion != ReportFormatVersion || !report.Preliminary || !report.DevelopmentFixture {
			t.Fatalf("%s labels/version = %q preliminary=%t fixture=%t", path,
				report.ReportVersion, report.Preliminary, report.DevelopmentFixture)
		}
		if report.Summary.EligibleCases != 0 ||
			report.Summary.Metrics.CorrectAlternativeAcceptance.Status != MetricUnavailable {
			t.Fatalf("%s quality metrics = %+v, want fixture insufficient-evidence metrics", path, report.Summary.Metrics)
		}
		for _, finding := range report.Findings {
			kinds[finding.Kind] = true
			if len(finding.FollowUps) == 0 {
				t.Fatalf("%s finding has no follow-ups: %+v", path, finding)
			}
		}
	}
	if !kinds[FindingKindSurvivingWrongSolution] || !kinds[FindingKindSetupFailure] {
		t.Fatalf("example finding kinds = %+v, want survivor and setup_failure", kinds)
	}
}

func TestSetupFailureFindingCarriesRepairFollowUps(t *testing.T) {
	report := BuildReport(Meta{
		RunID:        "pqb-setup-failure",
		StartedAt:    time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC),
		CompletedAt:  time.Date(2026, 11, 18, 12, 0, 1, 0, time.UTC),
		TestedCommit: "abc123",
		BaselineKind: "other",
		Timeout:      time.Second,
	}, []CaseRun{{Dir: "/tmp/missing-case", Err: errors.New("artifact manifest is missing")}})
	if report.Cases[0].Status != CaseStatusError {
		t.Fatalf("case status = %q, want error", report.Cases[0].Status)
	}
	finding, ok := findFinding(report.Cases[0].Findings, FindingKindSetupFailure)
	if !ok || !finding.CauseEstablished || len(finding.FollowUps) == 0 ||
		!strings.Contains(finding.Evidence[0].Detail, "artifact manifest is missing") {
		t.Fatalf("setup failure finding = %+v, present=%t", finding, ok)
	}
	if finding.ArtifactID != "" || finding.ArtifactVersion != "" ||
		finding.ArtifactIdentityStatus != ArtifactIdentityUnavailable ||
		!strings.Contains(finding.ArtifactIdentityUnavailableReason, "case loading failed") ||
		finding.Scope != FindingScopeAssessmentQuality ||
		finding.QualityMetricTreatment != QualityMetricTreatmentExcluded ||
		!strings.Contains(finding.QualityMetricTreatmentReason, "no completed assessment sample") {
		t.Fatalf("setup failure finding context = %+v", finding)
	}
	if report.Summary.Metrics.CorrectAlternativeAcceptance.Status != MetricUnavailable {
		t.Fatalf("quality status = %q, want unavailable", report.Summary.Metrics.CorrectAlternativeAcceptance.Status)
	}
	validateReportSchema(t, report)
}

func historicalGenerationIdentity() *ContentIdentity {
	return &ContentIdentity{
		Algorithm:           ContentIdentityAlgorithm,
		CaseDefinition:      ContentFile{Path: "case.yaml", SHA256: strings.Repeat("a", 64)},
		Specification:       ContentFile{Path: "spec.md", SHA256: strings.Repeat("b", 64)},
		RunConfig:           ContentFile{Path: "benchmark-run.json", SHA256: strings.Repeat("c", 64)},
		Reference:           SolutionContent{ID: "reference", Files: []ContentFile{}, SHA256: strings.Repeat("d", 64)},
		CorrectAlternatives: []SolutionContent{},
		WrongSolutions:      []SolutionContent{},
		Artifact:            ArtifactContent{Files: []ContentFile{}, SHA256: strings.Repeat("e", 64)},
	}
}

func generationObservationMeta() Meta {
	return Meta{
		RunID:        "pqb-generation-observation",
		StartedAt:    time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC),
		CompletedAt:  time.Date(2026, 11, 18, 12, 0, 1, 0, time.UTC),
		TestedCommit: "abc123",
		BaselineKind: "independent",
		Timeout:      time.Second,
	}
}

func buildGenerationObservationReport(t *testing.T, observations *GenerationObservationsConfig) Report {
	t.Helper()
	config := &RunConfig{
		ArtifactID:           "historical-artifact",
		ArtifactVersion:      "0.9.0",
		GenerationProvenance: &GenerationProvenanceConfig{Observations: observations},
	}
	return BuildReport(generationObservationMeta(), []CaseRun{{
		Case:     &problemquality.Case{CaseID: "generated-case", Technology: "go"},
		Config:   config,
		Identity: historicalGenerationIdentity(),
	}})
}

func metricReport(cases []CaseReport, baselineKind string) Report {
	report := BuildReport(Meta{
		RunID:        "pqb-metrics",
		StartedAt:    time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC),
		CompletedAt:  time.Date(2026, 11, 18, 12, 0, 1, 0, time.UTC),
		TestedCommit: "abc123",
		BaselineKind: baselineKind,
		Timeout:      time.Second,
	}, nil)
	report.Cases = cases
	report.BaselineKind = baselineKind
	report.Summary = Summary{}
	report.Recommendations = []string{}
	finalizeSummary(&report)
	return report
}

func metricCase(id string, fixture, eligible bool, eligibility string) CaseReport {
	return CaseReport{
		CaseID:               id,
		Technology:           "python",
		Status:               CaseStatusPassed,
		Generation:           Generation{Status: GenerationNotApplicable},
		DevelopmentFixture:   fixture,
		BenchmarkEligible:    eligible,
		BenchmarkEligibility: eligibility,
	}
}

func metricCorrect(id string, accepted bool, status environmentverify.AssessmentExecutionStatus) CorrectOutcome {
	return CorrectOutcome{ID: id, Accepted: accepted, Passed: accepted, ExecutionStatus: string(status)}
}

func metricWrong(id, decision string, status environmentverify.AssessmentExecutionStatus, rejected, matched bool) WrongOutcome {
	return WrongOutcome{
		ID:                     id,
		DenominatorDecision:    decision,
		ExecutionStatus:        string(status),
		Rejected:               rejected,
		Passed:                 rejected,
		IntendedFailureMatched: matched,
	}
}

func assertMetric(t *testing.T, name string, metric Metric, numerator, denominator, excluded int) {
	t.Helper()
	if metric.Status != MetricMeasured || metric.Value == nil ||
		metric.Numerator != numerator || metric.Denominator != denominator ||
		metric.Excluded != excluded {
		t.Fatalf("%s metric = %+v, want %d/%d excluded %d measured", name, metric, numerator, denominator, excluded)
	}
	if expected := float64(numerator) / float64(denominator); *metric.Value != expected {
		t.Fatalf("%s value = %v, want %v", name, *metric.Value, expected)
	}
}

func findFinding(findings []Finding, kind string) (Finding, bool) {
	for _, finding := range findings {
		if finding.Kind == kind {
			return finding, true
		}
	}
	return Finding{}, false
}

func intPointer(value int) *int {
	return &value
}
