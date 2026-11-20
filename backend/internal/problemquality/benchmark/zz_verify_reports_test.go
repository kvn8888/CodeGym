package benchmark

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kvn8888/codegym/backend/internal/environmentverify"
)

func TestZZVerifySavedReports(t *testing.T) {
	fourPath := "/tmp/pqb-verify-four.json"
	fixturePath := "/tmp/pqb-verify-development-order-tools.json"
	if _, err := os.Stat(fourPath); err != nil {
		t.Skip("saved four-case verification report is not present")
	}
	if _, err := os.Stat(fixturePath); err != nil {
		t.Skip("saved fixture verification report is not present")
	}
	schema := filepath.Join("..", "..", "..", "..", "benchmarks", "problem-quality", "schema", "report.schema.json")

	four := readSavedReport(t, fourPath, schema)
	if four.ReportVersion != "2.0.0" || !four.Preliminary || !four.DevelopmentFixture {
		t.Fatalf("four-case labels = %q preliminary=%t fixture=%t", four.ReportVersion, four.Preliminary, four.DevelopmentFixture)
	}
	if len(four.Cases) != 4 {
		t.Fatalf("four-case count = %d", len(four.Cases))
	}
	wrongTotal := 0
	for _, caseReport := range four.Cases {
		if caseReport.Status != CaseStatusPassed {
			t.Fatalf("%s status = %q", caseReport.CaseID, caseReport.Status)
		}
		if caseReport.ReferenceOutcome == nil || !caseReport.ReferenceOutcome.Accepted || !caseReport.ReferenceOutcome.Passed {
			t.Fatalf("%s reference = %+v", caseReport.CaseID, caseReport.ReferenceOutcome)
		}
		if len(caseReport.CorrectAlternatives) == 0 {
			t.Fatalf("%s has no correct alternatives", caseReport.CaseID)
		}
		for _, correct := range caseReport.CorrectAlternatives {
			if !correct.Accepted || !correct.Passed {
				t.Fatalf("%s correct alternative %q = %+v", caseReport.CaseID, correct.ID, correct)
			}
		}
		if !caseReport.DevelopmentFixture || !caseReport.ExcludedFromReviewedTotals || caseReport.BenchmarkEligible {
			t.Fatalf("%s fixture/exclusion/eligibility = %t/%t/%t", caseReport.CaseID,
				caseReport.DevelopmentFixture, caseReport.ExcludedFromReviewedTotals, caseReport.BenchmarkEligible)
		}
		for _, wrong := range caseReport.WrongSolutions {
			wrongTotal++
			if !wrong.Rejected || wrong.Surviving || !wrong.IntendedFailureMatched || wrong.IntendedFailure == nil {
				t.Fatalf("%s wrong %q = %+v", caseReport.CaseID, wrong.ID, wrong)
			}
			matched := false
			for _, testEvidence := range wrong.Tests {
				if testEvidence.TestID == wrong.IntendedFailure.TestID &&
					testEvidence.Assertion != nil &&
					testEvidence.Assertion.Code == wrong.IntendedFailure.Code &&
					testEvidence.Assertion.Expected == wrong.IntendedFailure.Expected &&
					testEvidence.Assertion.Actual == wrong.IntendedFailure.Actual {
					matched = true
				}
			}
			if !matched {
				t.Fatalf("%s wrong %q lacks matching assertion evidence: %+v", caseReport.CaseID, wrong.ID, wrong.Tests)
			}
		}
	}
	if wrongTotal != 8 {
		t.Fatalf("wrong solutions = %d, want 8", wrongTotal)
	}
	if four.Summary.DevelopmentFixtureCases != 4 || four.Summary.EligibleCases != 0 {
		t.Fatalf("summary fixture/eligible = %d/%d", four.Summary.DevelopmentFixtureCases, four.Summary.EligibleCases)
	}
	for name, metric := range map[string]Metric{
		"correct alternative acceptance": four.Summary.Metrics.CorrectAlternativeAcceptance,
		"wrong solution rejection":       four.Summary.Metrics.WrongSolutionRejection,
		"discrimination":                 four.Summary.Metrics.DiscriminationScore,
		"ambiguous specification":        four.Summary.Metrics.AmbiguousSpecificationRate,
	} {
		if metric.Status != MetricUnavailable || metric.Value != nil || metric.UnavailableReason == "" {
			t.Fatalf("%s metric = %+v, want unavailable with no value", name, metric)
		}
	}
	if four.Execution.ModelOperations || four.Execution.PaidOperations || four.Execution.Sandbox {
		t.Fatalf("execution claims = %+v", four.Execution)
	}
	if four.Summary.Metrics.Cases.ObservedGenerationCases != 0 {
		t.Fatalf("observed generation cases = %d", four.Summary.Metrics.Cases.ObservedGenerationCases)
	}
	for _, caseReport := range four.Cases {
		if caseReport.Generation.Status != GenerationNotApplicable {
			t.Fatalf("%s generation status = %q", caseReport.CaseID, caseReport.Generation.Status)
		}
		if caseReport.Reference == nil ||
			caseReport.Reference.ReviewedBy != "development-fixture" ||
			!strings.Contains(caseReport.Reference.ReviewedReference, "not human approval") {
			t.Fatalf("%s reference approval metadata = %+v", caseReport.CaseID, caseReport.Reference)
		}
	}

	fixture := readSavedReport(t, fixturePath, schema)
	if len(fixture.Cases) != 1 || fixture.Cases[0].Status != CaseStatusFailed {
		t.Fatalf("fixture report cases = %+v", fixture.Cases)
	}
	fixtureCase := fixture.Cases[0]
	finding, ok := findFinding(fixtureCase.Findings, FindingKindSurvivingWrongSolution)
	if !ok {
		t.Fatalf("fixture findings = %+v", fixtureCase.Findings)
	}
	if finding.ArtifactID != "development-order-tools-artifact" ||
		finding.ArtifactVersion != "0.1.0" ||
		finding.ArtifactIdentityStatus != ArtifactIdentityAvailable ||
		finding.TestID == "" || finding.AssertionCode == "" || finding.ComparatorContext == "" ||
		len(finding.FollowUps) == 0 ||
		finding.QualityMetricTreatment != QualityMetricTreatmentExcluded ||
		!strings.Contains(finding.QualityMetricTreatmentReason, "Development fixture") {
		t.Fatalf("fixture survivor finding = %+v", finding)
	}
	if !fixtureCase.DevelopmentFixture || !fixtureCase.ExcludedFromReviewedTotals || fixtureCase.BenchmarkEligible {
		t.Fatalf("fixture exclusion/eligibility = %t/%t/%t", fixtureCase.DevelopmentFixture,
			fixtureCase.ExcludedFromReviewedTotals, fixtureCase.BenchmarkEligible)
	}
	if !fixtureCase.DevelopmentFixture || fixtureCase.BenchmarkEligibility != environmentverify.BenchmarkEligibilityUnresolved {
		t.Fatalf("fixture eligibility = %q", fixtureCase.BenchmarkEligibility)
	}
}

func readSavedReport(t *testing.T, path, schema string) Report {
	t.Helper()
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
	return report
}
