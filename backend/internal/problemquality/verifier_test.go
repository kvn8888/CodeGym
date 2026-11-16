package problemquality

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kvn8888/codegym/backend/internal/environmentverify"
)

type fixtureCommandRunner struct {
	stages []string
}

func (r *fixtureCommandRunner) Run(_ context.Context, spec environmentverify.CommandSpec) (environmentverify.CommandResult, error) {
	r.stages = append(r.stages, spec.Stage)
	return environmentverify.CommandResult{ExitCode: 0}, nil
}

type fixtureAssessment struct {
	report environmentverify.AssessmentReport
	err    error
}

type fixtureAssessor struct {
	roles   []string
	reports map[string]fixtureAssessment
}

func (a *fixtureAssessor) Assess(_ context.Context, work environmentverify.AssessmentWork) (environmentverify.AssessmentReport, error) {
	a.roles = append(a.roles, work.Role)
	if assessment, ok := a.reports[work.Role]; ok {
		return assessment.report, assessment.err
	}
	return environmentverify.AssessmentReport{}, errors.New("fixture assessor has no report for role " + work.Role)
}

func fixturePassReport(ids ...string) environmentverify.AssessmentReport {
	tests := make([]environmentverify.TestOutcome, 0, len(ids))
	for _, id := range ids {
		tests = append(tests, environmentverify.TestOutcome{TestID: id, Passed: true})
	}
	return environmentverify.AssessmentReport{Tests: tests}
}

func fixtureFailureReport(id, class string) environmentverify.AssessmentReport {
	return environmentverify.AssessmentReport{Tests: []environmentverify.TestOutcome{{
		TestID: id, Passed: false, FailureClass: class,
	}}}
}

func fixtureAssertionFailure(id, code, expected, actual string) environmentverify.AssessmentReport {
	return environmentverify.AssessmentReport{Tests: []environmentverify.TestOutcome{{
		TestID: id, Passed: false, FailureClass: environmentverify.ClassAssertion,
		Assertion: &environmentverify.ObservedAssertion{Code: code, Expected: expected, Actual: actual},
	}}}
}

func stageFixtureArtifact(t *testing.T) (environmentverify.BuilderResult, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte(fixtureManifestJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "solution.py"), []byte("def add(a, b):\n    return a + b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "protected"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "protected", "test_add.py"), []byte("def test_add():\n    pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifestPath := "manifest.json"
	return environmentverify.BuilderResult{
		SchemaVersion:        environmentverify.BuilderResultSchemaVersion,
		BuilderRunID:         "fixture-build-run",
		Runtime:              &environmentverify.RuntimeInfo{Name: "fixture-runner", Version: "0.0.0"},
		ProposedManifestPath: &manifestPath,
		Diagnostics:          []environmentverify.Diagnostic{},
	}, root
}

func loadFixtureCase(t *testing.T) *Case {
	t.Helper()
	c, err := LoadCase(writeFixture(t, validFixture()))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func runFixtureCase(t *testing.T, assessor environmentverify.AssessmentRunner, mutate func(*Case)) environmentverify.VerifierResult {
	t.Helper()
	c := loadFixtureCase(t)
	if mutate != nil {
		mutate(c)
	}
	spec, err := c.AssessmentSpec("alt-01", "off-by-one")
	if err != nil {
		t.Fatalf("AssessmentSpec: %v", err)
	}
	builder, root := stageFixtureArtifact(t)
	opts := environmentverify.DefaultVerifyOptions()
	opts.Assessment = spec
	opts.Assessor = assessor
	return environmentverify.VerifyWithOptions(context.Background(), &fixtureCommandRunner{}, builder, root, "fixture-verify-run", opts)
}

func requireFixtureRejected(t *testing.T, result environmentverify.VerifierResult, code string) {
	t.Helper()
	if result.Verdict != environmentverify.VerdictRejected {
		t.Fatalf("verdict = %q, want rejected", result.Verdict)
	}
	if len(result.Diagnostics) == 0 || result.Diagnostics[0].Code != code {
		t.Fatalf("diagnostics = %+v, want code %q", result.Diagnostics, code)
	}
	if result.Promotion.Allowed {
		t.Fatalf("promotion.allowed = true, want false")
	}
}

func TestAssessmentSpecUsesArtifactIdentityAndSelectedSolutions(t *testing.T) {
	c := loadFixtureCase(t)
	spec, err := c.AssessmentSpec("alt-01", "off-by-one")
	if err != nil {
		t.Fatal(err)
	}
	if spec.ArtifactID != "fixture-artifact-2026-11" || spec.ArtifactVersion != "0.9.0" {
		t.Fatalf("artifact = %q version %q", spec.ArtifactID, spec.ArtifactVersion)
	}
	if spec.ArtifactID == c.CaseID || spec.ArtifactVersion == c.Version {
		t.Fatalf("case identity %q/%q leaked into artifact identity %q/%q", c.CaseID, c.Version, spec.ArtifactID, spec.ArtifactVersion)
	}
	if spec.CorrectSolution["solution.py"] != "def add(a, b):\n    return a + b\n" {
		t.Fatalf("correct solution = %q", spec.CorrectSolution["solution.py"])
	}
	if spec.WrongSolution["solution.py"] != "def add(a, b):\n    return a + b - 1\n" {
		t.Fatalf("wrong solution = %q", spec.WrongSolution["solution.py"])
	}
	if spec.ExpectedFailure.TestID != "hidden-large-values" || spec.ExpectedFailure.Code != "WRONG_SUM" ||
		spec.ExpectedFailure.Expected != "7" || spec.ExpectedFailure.Actual != "6" {
		t.Fatalf("expected failure = %#v", spec.ExpectedFailure)
	}
	if spec.ReviewedBy != "reviewer" {
		t.Fatalf("reviewedBy = %q", spec.ReviewedBy)
	}
}

func TestAssessmentSpecUsesExplicitCorrectAlternative(t *testing.T) {
	f := validFixture()
	second := `  - id: alt-02
    files:
      solution.py: correct/alt-02/solution.py
    reviewed_by: reviewer
    reviewed_at: "2026-11-10T00:00:00Z"
`
	f.yaml = strings.Replace(f.yaml, "wrong_solutions:\n", second+"wrong_solutions:\n", 1)
	f.files["correct/alt-02/solution.py"] = "def add(a, b):\n    return b + a\n"
	c, err := LoadCase(writeFixture(t, f))
	if err != nil {
		t.Fatal(err)
	}

	spec, err := c.AssessmentSpec("alt-02", "off-by-one")
	if err != nil {
		t.Fatal(err)
	}
	if spec.CorrectSolution["solution.py"] != "def add(a, b):\n    return b + a\n" {
		t.Fatalf("correct solution = %q, want the explicitly selected alt-02", spec.CorrectSolution["solution.py"])
	}

	_, err = c.AssessmentSpec("missing-alt", "off-by-one")
	var compatibility *CompatibilityError
	if !errors.As(err, &compatibility) {
		t.Fatalf("error = %v, want *CompatibilityError", err)
	}
	if !strings.Contains(err.Error(), "missing-alt") {
		t.Fatalf("error %q does not identify the selection", err)
	}
}

func TestAssessmentSpecRejectsNonMeaningfulWrongSolution(t *testing.T) {
	f := validFixture()
	f.yaml = strings.Replace(f.yaml, "denominator_decision: meaningful", "denominator_decision: equivalent", 1)
	c, err := LoadCase(writeFixture(t, f))
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.AssessmentSpec("alt-01", "off-by-one")
	var compatibility *CompatibilityError
	if !errors.As(err, &compatibility) {
		t.Fatalf("error = %v, want *CompatibilityError", err)
	}
	if !strings.Contains(err.Error(), "meaningful wrong solution") {
		t.Fatalf("error = %v, want a denominator compatibility error", err)
	}
}

func TestAssessmentSpecRejectsUnrepresentableAssertionEvidence(t *testing.T) {
	f := validFixture()
	f.yaml = strings.Replace(f.yaml, `      expected: "7"`, `      expected: ""`, 1)
	c, err := LoadCase(writeFixture(t, f))
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.AssessmentSpec("alt-01", "off-by-one")
	var compatibility *CompatibilityError
	if !errors.As(err, &compatibility) {
		t.Fatalf("error = %v, want *CompatibilityError", err)
	}
	if !strings.Contains(err.Error(), "intended_failure.expected") {
		t.Fatalf("error = %v, want expected-field compatibility error", err)
	}
}

func TestVerifyFixtureCaseVerified(t *testing.T) {
	assessor := &fixtureAssessor{reports: map[string]fixtureAssessment{
		environmentverify.RoleCorrectSolution: {report: fixturePassReport("hidden-large-values", "other-case")},
		environmentverify.RoleWrongSolution:   {report: fixtureAssertionFailure("hidden-large-values", "WRONG_SUM", "7", "6")},
	}}
	result := runFixtureCase(t, assessor, nil)
	if result.Verdict != environmentverify.VerdictVerified {
		t.Fatalf("verdict = %q, diagnostics = %+v", result.Verdict, result.Diagnostics)
	}
	if result.FailedStage != nil {
		t.Fatalf("failedStage = %v, want nil", result.FailedStage)
	}
	if len(assessor.roles) != 2 || assessor.roles[0] != environmentverify.RoleCorrectSolution ||
		assessor.roles[1] != environmentverify.RoleWrongSolution {
		t.Fatalf("assessed roles = %v", assessor.roles)
	}
}

func TestVerifyFixtureRejectsTargetOutsideLearnerEditable(t *testing.T) {
	f := validFixture()
	f.yaml = strings.Replace(f.yaml, "solution.py: correct/alt-01/solution.py",
		"other.py: correct/alt-01/solution.py", 1)
	c, err := LoadCase(writeFixture(t, f))
	if err != nil {
		t.Fatal(err)
	}
	spec, err := c.AssessmentSpec("alt-01", "off-by-one")
	if err != nil {
		t.Fatal(err)
	}
	builder, root := stageFixtureArtifact(t)
	assessor := &fixtureAssessor{reports: map[string]fixtureAssessment{}}
	opts := environmentverify.DefaultVerifyOptions()
	opts.Assessment = spec
	opts.Assessor = assessor
	result := environmentverify.VerifyWithOptions(context.Background(), &fixtureCommandRunner{}, builder, root, "fixture-target-run", opts)
	requireFixtureRejected(t, result, environmentverify.CodeAssessmentInputsMissing)
	if len(assessor.roles) != 0 {
		t.Fatalf("assessed roles = %v, want none for an invalid target", assessor.roles)
	}
}

func TestVerifyFixtureWrongSolutionPasses(t *testing.T) {
	assessor := &fixtureAssessor{reports: map[string]fixtureAssessment{
		environmentverify.RoleCorrectSolution: {report: fixturePassReport("hidden-large-values")},
		environmentverify.RoleWrongSolution:   {report: fixturePassReport("hidden-large-values")},
	}}
	result := runFixtureCase(t, assessor, nil)
	requireFixtureRejected(t, result, environmentverify.CodeWrongSolutionPassed)
}

func TestVerifyFixtureWrongSolutionAssertionMismatch(t *testing.T) {
	assessor := &fixtureAssessor{reports: map[string]fixtureAssessment{
		environmentverify.RoleCorrectSolution: {report: fixturePassReport("hidden-large-values")},
		environmentverify.RoleWrongSolution:   {report: fixtureAssertionFailure("hidden-large-values", "WRONG_SUM", "7", "8")},
	}}
	result := runFixtureCase(t, assessor, nil)
	requireFixtureRejected(t, result, environmentverify.CodeAssertionMismatch)
}

func TestVerifyFixtureCorrectAlternativeFails(t *testing.T) {
	assessor := &fixtureAssessor{reports: map[string]fixtureAssessment{
		environmentverify.RoleCorrectSolution: {report: fixtureFailureReport("hidden-large-values", environmentverify.ClassAssertion)},
		environmentverify.RoleWrongSolution:   {report: fixtureAssertionFailure("hidden-large-values", "WRONG_SUM", "7", "6")},
	}}
	result := runFixtureCase(t, assessor, nil)
	requireFixtureRejected(t, result, environmentverify.CodeCorrectSolutionFailed)
	if len(assessor.roles) != 1 || assessor.roles[0] != environmentverify.RoleCorrectSolution {
		t.Fatalf("assessed roles = %v, want only the correct alternative", assessor.roles)
	}
}

func TestVerifyFixtureMissingReviewEvidence(t *testing.T) {
	assessor := &fixtureAssessor{reports: map[string]fixtureAssessment{}}
	result := runFixtureCase(t, assessor, func(c *Case) {
		c.Review.ReviewedBy = ""
	})
	requireFixtureRejected(t, result, environmentverify.CodeReviewPending)
	if len(assessor.roles) != 0 {
		t.Fatalf("assessed roles = %v, want none before review", assessor.roles)
	}
}

func TestAssessmentPlanIncludesAllSolutionsAndExclusions(t *testing.T) {
	f := validFixture()
	secondCorrect := `  - id: alt-02
    files:
      solution.py: correct/alt-02/solution.py
    reviewed_by: reviewer
    reviewed_at: "2026-11-10T00:00:00Z"
`
	f.yaml = strings.Replace(f.yaml, "wrong_solutions:\n", secondCorrect+"wrong_solutions:\n", 1)
	equivalentWrong := `  - id: equivalent
    files:
      solution.py: wrong/equivalent/solution.py
    intended_failure:
      test_id: hidden-large-values
      code: WRONG_SUM
      expected: "7"
      actual: "6"
      reason: "Equivalent but differently written."
    learning_objective_rationale: "Same behavior as the reference."
    reviewed_by: reviewer
    reviewed_at: "2026-11-10T00:00:00Z"
    denominator_decision: equivalent
`
	f.yaml = strings.Replace(f.yaml, "comparator_expectations:\n", equivalentWrong+"comparator_expectations:\n", 1)
	f.files["correct/alt-02/solution.py"] = "def add(a, b):\n    return b + a\n"
	f.files["wrong/equivalent/solution.py"] = "def add(a, b):\n    return b + a\n"
	c, err := LoadCase(writeFixture(t, f))
	if err != nil {
		t.Fatal(err)
	}

	plan, err := c.AssessmentPlan(&ReferenceInput{
		ID:                "reference",
		Files:             map[string]string{"solution.py": "def add(a, b):\n    return a + b\n"},
		ReviewedBy:        "reference-reviewer",
		ReviewedAt:        "2026-11-10T00:00:00Z",
		ReviewedReference: "review://reference",
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.ArtifactID != "fixture-artifact-2026-11" || plan.ArtifactVersion != "0.9.0" {
		t.Fatalf("artifact = %q version %q", plan.ArtifactID, plan.ArtifactVersion)
	}
	if plan.ReviewedBy != "reviewer" || plan.ReviewedReference != "case:fixture-case" {
		t.Fatalf("plan review = %+v", plan)
	}
	if plan.Reference == nil || plan.Reference.ID != "reference" ||
		plan.Reference.Role != environmentverify.RoleGeneratedReference ||
		plan.Reference.Files["solution.py"] != "def add(a, b):\n    return a + b\n" {
		t.Fatalf("reference = %+v", plan.Reference)
	}
	if len(plan.CorrectAlternatives) != 2 ||
		plan.CorrectAlternatives[0].Role != environmentverify.RoleCorrectAlternative ||
		plan.CorrectAlternatives[0].Files["solution.py"] != "def add(a, b):\n    return a + b\n" ||
		plan.CorrectAlternatives[1].Files["solution.py"] != "def add(a, b):\n    return b + a\n" {
		t.Fatalf("correct alternatives = %+v", plan.CorrectAlternatives)
	}
	if len(plan.WrongSolutions) != 2 {
		t.Fatalf("wrong solutions = %+v", plan.WrongSolutions)
	}
	meaningful := plan.WrongSolutions[0]
	if meaningful.Role != environmentverify.RoleWrongSolution ||
		meaningful.DenominatorDecision != environmentverify.DenominatorMeaningful ||
		meaningful.ExpectedFailure == nil || meaningful.ExpectedFailure.TestID != "hidden-large-values" ||
		meaningful.ExpectedFailure.Expected != "7" || meaningful.ExpectedFailure.Actual != "6" {
		t.Fatalf("meaningful wrong = %+v", meaningful)
	}
	excluded := plan.WrongSolutions[1]
	if excluded.ID != "equivalent" || excluded.DenominatorDecision != environmentverify.DenominatorEquivalent ||
		excluded.ExpectedFailure != nil {
		t.Fatalf("excluded wrong = %+v", excluded)
	}
}

func TestAssessmentPlanMissingReferenceIsRepresented(t *testing.T) {
	c := loadFixtureCase(t)
	plan, err := c.AssessmentPlan(nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Reference != nil {
		t.Fatalf("reference = %+v, want nil for a missing explicit reference", plan.Reference)
	}
}

func TestAssessmentPlanRejectsCommandMismatch(t *testing.T) {
	f := validFixture()
	f.yaml = strings.Replace(f.yaml, "test: python3 protected/test_add.py",
		"test: python -m pytest -q", 1)
	c, err := LoadCase(writeFixture(t, f))
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.AssessmentPlan(&ReferenceInput{ID: "reference", Files: map[string]string{"solution.py": "x"}})
	var compatibility *CompatibilityError
	if !errors.As(err, &compatibility) {
		t.Fatalf("error = %v, want *CompatibilityError", err)
	}
	if compatibility.Field != "build_test_run.commands.test" {
		t.Fatalf("field = %q, want build_test_run.commands.test", compatibility.Field)
	}
	if !strings.Contains(err.Error(), "authoritative") {
		t.Fatalf("error = %v, want it to state manifest precedence", err)
	}
}

func TestVerifyFixturePlanCollectsOutcomes(t *testing.T) {
	c := loadFixtureCase(t)
	plan, err := c.AssessmentPlan(&ReferenceInput{
		ID:    "reference",
		Files: map[string]string{"solution.py": "def add(a, b):\n    return a + b\n"},
	})
	if err != nil {
		t.Fatal(err)
	}
	assessor := &fixtureAssessor{reports: map[string]fixtureAssessment{
		environmentverify.RoleGeneratedReference: {report: fixturePassReport("hidden-large-values")},
		environmentverify.RoleCorrectAlternative: {report: fixturePassReport("hidden-large-values")},
		environmentverify.RoleWrongSolution:      {report: fixtureAssertionFailure("hidden-large-values", "WRONG_SUM", "7", "6")},
	}}
	builder, root := stageFixtureArtifact(t)
	opts := environmentverify.DefaultVerifyOptions()
	opts.Assessor = assessor
	result, collection := environmentverify.VerifyAssessmentPlanWithOptions(
		context.Background(), &fixtureCommandRunner{}, builder, root, "fixture-plan-run", opts, *plan)
	if result.Verdict != environmentverify.VerdictVerified {
		t.Fatalf("verdict = %q, diagnostics = %+v", result.Verdict, result.Diagnostics)
	}
	if collection.Blocked || collection.Reference == nil || !collection.Reference.Passed ||
		len(collection.CorrectAlternatives) != 1 || !collection.CorrectAlternatives[0].Passed ||
		len(collection.WrongSolutions) != 1 || !collection.WrongSolutions[0].Passed {
		t.Fatalf("collection = %+v", collection)
	}
	if got := assessor.roles; !equalRoleStrings(got, []string{
		environmentverify.RoleGeneratedReference,
		environmentverify.RoleCorrectAlternative,
		environmentverify.RoleWrongSolution,
	}) {
		t.Fatalf("assessed roles = %v", got)
	}
}

func equalRoleStrings(actual, want []string) bool {
	if len(actual) != len(want) {
		return false
	}
	for i := range actual {
		if actual[i] != want[i] {
			return false
		}
	}
	return true
}
