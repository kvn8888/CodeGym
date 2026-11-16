package problemquality

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fixtureManifestJSON = `{
  "schemaVersion": "environment.manifest.v1",
  "artifact": {
    "id": "fixture-artifact-2026-11",
    "technology": "python",
    "version": "0.9.0"
  },
  "environment": {
    "base": "python-3.13",
    "network": "none",
    "dependencyFiles": [],
    "lockfiles": []
  },
  "workspace": {
    "root": ".",
    "learnerEditable": ["solution.py"],
    "protected": ["protected/test_add.py"]
  },
  "commands": {
    "setup": "python3 --version",
    "build": "python3 -m py_compile solution.py protected/test_add.py",
    "test": "python3 protected/test_add.py"
  },
  "assessment": {
    "type": "unit-tests",
    "visibleTests": [],
    "hiddenTests": ["protected/test_add.py"]
  },
  "compatibility": {
    "requestedTechnology": "Python",
    "requestedObjective": "Add two integers",
    "reuseTags": []
  }
}`

const expectedOutcomesBlock = `expected_outcomes:
  generation:
    blind_agreement: null
    reconciliation_count: null
    reconciliation_direction: null
    non_convergence: null
    status: not_applicable
  assessment:
    correct_alternatives_pass: true
    wrong_solutions_rejected: true
`

type fixture struct {
	yaml  string
	files map[string]string
}

func validFixture() fixture {
	return fixture{
		yaml: `case_id: fixture-case
version: 1.0.0
technology: python
objective: Add two integers.
environment_manifest: environment/manifest.json
build_test_run:
  strategy: unit
  entrypoint: solution.py
  commands:
    test: python3 protected/test_add.py
  limits:
    timeout_seconds: 30
    memory_mb: 256
    network_mode: none
spec:
  path_or_inline: spec.json
  ambiguity_resolutions_required: true
correct_alternatives:
  - id: alt-01
    files:
      solution.py: correct/alt-01/solution.py
    reviewed_by: reviewer
    reviewed_at: "2026-11-10T00:00:00Z"
wrong_solutions:
  - id: off-by-one
    files:
      solution.py: wrong/off-by-one/solution.py
    intended_failure:
      test_id: hidden-large-values
      code: WRONG_SUM
      expected: "7"
      actual: "6"
      reason: "The implementation subtracts one from the result."
    learning_objective_rationale: "A learner may confuse inclusive and exclusive bounds."
    reviewed_by: reviewer
    reviewed_at: "2026-11-10T00:00:00Z"
    denominator_decision: meaningful
comparator_expectations:
  default:
    kind: exact
  case_overrides_allowed: true
  notes: "Exact integer equality."
expected_outcomes:
  generation:
    blind_agreement: null
    reconciliation_count: null
    reconciliation_direction: null
    non_convergence: null
    status: not_applicable
  assessment:
    correct_alternatives_pass: true
    wrong_solutions_rejected: true
review:
  reviewed_by: reviewer
  reviewed_at: "2026-11-10T00:00:00Z"
  ambiguous_spec: false
  notes: "Fixture only."
`,
		files: map[string]string{
			"environment/manifest.json":    fixtureManifestJSON,
			"spec.json":                    `{"language":"python"}`,
			"correct/alt-01/solution.py":   "def add(a, b):\n    return a + b\n",
			"wrong/off-by-one/solution.py": "def add(a, b):\n    return a + b - 1\n",
		},
	}
}

func writeFixture(t *testing.T, f fixture) string {
	t.Helper()
	return writeFixtureAt(t, t.TempDir(), f)
}

func writeFixtureAt(t *testing.T, dir string, f fixture) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range f.files {
		full := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, CaseFileName), []byte(f.yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func requireCaseError(t *testing.T, err error, kind ErrorKind, field string) *CaseError {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	var caseErr *CaseError
	if !errors.As(err, &caseErr) {
		t.Fatalf("error type = %T, want *CaseError: %v", err, err)
	}
	if caseErr.Kind != kind {
		t.Fatalf("error kind = %q, want %q: %v", caseErr.Kind, kind, err)
	}
	if caseErr.Field != field {
		t.Fatalf("error field = %q, want %q: %v", caseErr.Field, field, err)
	}
	if caseErr.CaseID != "fixture-case" {
		t.Fatalf("error case id = %q, want fixture-case: %v", caseErr.CaseID, err)
	}
	for _, fragment := range []string{"fixture-case", field} {
		if !strings.Contains(err.Error(), fragment) {
			t.Fatalf("error %q does not identify %q", err.Error(), fragment)
		}
	}
	return caseErr
}

func TestLoadCaseResolvesSolutions(t *testing.T) {
	dir := writeFixture(t, validFixture())
	c, err := LoadCase(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.CaseID != "fixture-case" || c.Technology != "python" {
		t.Fatalf("case identity = %#v", c)
	}
	if c.EnvironmentManifestPath != filepath.Join(dir, "environment", "manifest.json") {
		t.Fatalf("environment manifest path = %q", c.EnvironmentManifestPath)
	}
	if c.EnvironmentManifestData == nil {
		t.Fatal("environment manifest data was not loaded")
	}
	if c.ArtifactID != "fixture-artifact-2026-11" || c.ArtifactVersion != "0.9.0" {
		t.Fatalf("artifact identity = %q version %q", c.ArtifactID, c.ArtifactVersion)
	}
	if c.CaseID != "fixture-case" || c.Version != "1.0.0" {
		t.Fatalf("case identity = %q version %q", c.CaseID, c.Version)
	}
	if c.SpecInline || c.SpecPath != filepath.Join(dir, "spec.json") {
		t.Fatalf("spec path = %q inline=%v", c.SpecPath, c.SpecInline)
	}
	if c.Spec.AmbiguityResolutionsRequired == nil || !*c.Spec.AmbiguityResolutionsRequired {
		t.Fatalf("ambiguity_resolutions_required = %v, want true", c.Spec.AmbiguityResolutionsRequired)
	}
	if c.ComparatorExpectations.CaseOverridesAllowed == nil || !*c.ComparatorExpectations.CaseOverridesAllowed {
		t.Fatalf("case_overrides_allowed = %v, want true", c.ComparatorExpectations.CaseOverridesAllowed)
	}

	if len(c.CorrectAlternatives) != 1 || len(c.WrongSolutions) != 1 {
		t.Fatalf("solutions: correct=%d wrong=%d", len(c.CorrectAlternatives), len(c.WrongSolutions))
	}
	correct := c.CorrectAlternatives[0].ResolvedFiles["solution.py"]
	if correct.Target != "solution.py" || correct.Source != "correct/alt-01/solution.py" {
		t.Fatalf("correct mapping = %#v", correct)
	}
	if correct.Path != filepath.Join(dir, "correct", "alt-01", "solution.py") {
		t.Fatalf("correct path = %q", correct.Path)
	}
	if correct.Content != "def add(a, b):\n    return a + b\n" {
		t.Fatalf("correct content = %q", correct.Content)
	}

	wrongSolution := c.WrongSolutions[0]
	wrong := wrongSolution.ResolvedFiles["solution.py"]
	if wrong.Path != filepath.Join(dir, "wrong", "off-by-one", "solution.py") {
		t.Fatalf("wrong path = %q", wrong.Path)
	}
	if wrongSolution.DenominatorDecision != "meaningful" {
		t.Fatalf("denominator decision = %q, want meaningful", wrongSolution.DenominatorDecision)
	}
	if wrongSolution.IntendedFailure.Expected == nil || *wrongSolution.IntendedFailure.Expected != "7" {
		t.Fatalf("intended failure expected = %v, want 7", wrongSolution.IntendedFailure.Expected)
	}
}

func TestLoadCaseDefaultsDenominatorDecision(t *testing.T) {
	f := validFixture()
	f.yaml = strings.Replace(f.yaml, "    denominator_decision: meaningful\n", "", 1)
	c, err := LoadCase(writeFixture(t, f))
	if err != nil {
		t.Fatal(err)
	}
	if got := c.WrongSolutions[0].DenominatorDecision; got != "meaningful" {
		t.Fatalf("denominator decision = %q, want meaningful", got)
	}
}

func TestLoadCaseAcceptsEmptyExpectedOutcomes(t *testing.T) {
	f := validFixture()
	f.yaml = strings.Replace(f.yaml, expectedOutcomesBlock, "expected_outcomes: {}\n", 1)
	c, err := LoadCase(writeFixture(t, f))
	if err != nil {
		t.Fatal(err)
	}
	if c.ExpectedOutcomes == nil {
		t.Fatal("expected_outcomes = nil, want a present empty object")
	}
	if c.ExpectedOutcomes.Generation != nil || c.ExpectedOutcomes.Assessment != nil {
		t.Fatalf("expected_outcomes = %#v, want empty object", c.ExpectedOutcomes)
	}
}

func TestLoadCaseAcceptsInlineSpec(t *testing.T) {
	f := validFixture()
	f.yaml = strings.Replace(f.yaml, "  path_or_inline: spec.json", `  path_or_inline: '{"language":"python"}'`, 1)
	delete(f.files, "spec.json")
	c, err := LoadCase(writeFixture(t, f))
	if err != nil {
		t.Fatal(err)
	}
	if !c.SpecInline || c.SpecPath != "" {
		t.Fatalf("spec inline=%v path=%q", c.SpecInline, c.SpecPath)
	}
}

func TestLoadCaseRejectsMissingRequiredFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*fixture)
		field  string
	}{
		{
			name: "objective",
			mutate: func(f *fixture) {
				f.yaml = strings.Replace(f.yaml, "objective: Add two integers.\n", "", 1)
			},
			field: "objective",
		},
		{
			name: "test command",
			mutate: func(f *fixture) {
				f.yaml = strings.Replace(f.yaml, "    test: python3 protected/test_add.py\n", "", 1)
			},
			field: "build_test_run.commands.test",
		},
		{
			name: "intended failure code",
			mutate: func(f *fixture) {
				f.yaml = strings.Replace(f.yaml, "      code: WRONG_SUM\n", "", 1)
			},
			field: "wrong_solutions[0].intended_failure.code",
		},
		{
			name: "expected outcomes omitted",
			mutate: func(f *fixture) {
				f.yaml = strings.Replace(f.yaml, expectedOutcomesBlock, "", 1)
			},
			field: "expected_outcomes",
		},
		{
			name: "expected outcomes null",
			mutate: func(f *fixture) {
				f.yaml = strings.Replace(f.yaml, expectedOutcomesBlock, "expected_outcomes: null\n", 1)
			},
			field: "expected_outcomes",
		},
		{
			name: "unknown field",
			mutate: func(f *fixture) {
				f.yaml = strings.Replace(f.yaml, "technology: python\n", "technology: python\nunexpected: value\n", 1)
			},
			field: "case.yaml",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := validFixture()
			test.mutate(&f)
			_, err := LoadCase(writeFixture(t, f))
			if test.name == "unknown field" {
				var caseErr *CaseError
				if !errors.As(err, &caseErr) || caseErr.Kind != InvalidCase {
					t.Fatalf("error = %v, want invalid case error", err)
				}
				return
			}
			requireCaseError(t, err, InvalidCase, test.field)
		})
	}
}

func TestLoadCaseReportsMissingReferencesAsIncomplete(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*fixture)
		field  string
	}{
		{
			name: "environment manifest",
			mutate: func(f *fixture) {
				delete(f.files, "environment/manifest.json")
			},
			field: "environment_manifest",
		},
		{
			name: "spec",
			mutate: func(f *fixture) {
				delete(f.files, "spec.json")
			},
			field: "spec.path_or_inline",
		},
		{
			name: "correct solution",
			mutate: func(f *fixture) {
				delete(f.files, "correct/alt-01/solution.py")
			},
			field: `correct_alternatives[0].files["solution.py"]`,
		},
		{
			name: "wrong solution",
			mutate: func(f *fixture) {
				delete(f.files, "wrong/off-by-one/solution.py")
			},
			field: `wrong_solutions[0].files["solution.py"]`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := validFixture()
			test.mutate(&f)
			_, err := LoadCase(writeFixture(t, f))
			requireCaseError(t, err, IncompleteCase, test.field)
			if !IsIncomplete(err) {
				t.Fatalf("IsIncomplete(%v) = false, want true", err)
			}
			if !strings.Contains(err.Error(), "does not exist") {
				t.Fatalf("error %q does not explain the missing reference", err.Error())
			}
		})
	}
}

func TestLoadCaseRejectsDuplicateSolutionIDs(t *testing.T) {
	t.Run("within wrong solutions", func(t *testing.T) {
		f := validFixture()
		duplicate := `  - id: off-by-one
    files:
      solution.py: wrong/off-by-one/solution.py
    intended_failure:
      test_id: hidden-large-values
      code: WRONG_SUM
      expected: "7"
      actual: "6"
      reason: "Duplicate."
    learning_objective_rationale: "Duplicate."
    reviewed_by: reviewer
    reviewed_at: "2026-11-10T00:00:00Z"
    denominator_decision: meaningful
`
		f.yaml = strings.Replace(f.yaml, "comparator_expectations:\n", duplicate+"comparator_expectations:\n", 1)
		_, err := LoadCase(writeFixture(t, f))
		requireCaseError(t, err, InvalidCase, "wrong_solutions[1].id")
		if !strings.Contains(err.Error(), "duplicates solution id") {
			t.Fatalf("error = %v, want duplicate id explanation", err)
		}
	})

	t.Run("across correct and wrong", func(t *testing.T) {
		f := validFixture()
		f.yaml = strings.Replace(f.yaml, "  - id: off-by-one\n", "  - id: alt-01\n", 1)
		_, err := LoadCase(writeFixture(t, f))
		requireCaseError(t, err, InvalidCase, "wrong_solutions[0].id")
		if !strings.Contains(err.Error(), "duplicates solution id") {
			t.Fatalf("error = %v, want duplicate id explanation", err)
		}
	})
}

func TestLoadCaseRejectsEscapingPaths(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*fixture)
		field  string
		want   string
	}{
		{
			name: "environment manifest escape",
			mutate: func(f *fixture) {
				f.yaml = strings.Replace(f.yaml, "environment_manifest: environment/manifest.json",
					"environment_manifest: ../manifest.json", 1)
			},
			field: "environment_manifest",
			want:  "escapes the case directory",
		},
		{
			name: "absolute environment manifest",
			mutate: func(f *fixture) {
				f.yaml = strings.Replace(f.yaml, "environment_manifest: environment/manifest.json",
					"environment_manifest: /tmp/manifest.json", 1)
			},
			field: "environment_manifest",
			want:  "must be relative to the case directory",
		},
		{
			name: "solution source escape",
			mutate: func(f *fixture) {
				f.yaml = strings.Replace(f.yaml, "solution.py: correct/alt-01/solution.py",
					"solution.py: ../../../../secret.py", 1)
			},
			field: `correct_alternatives[0].files["solution.py"]`,
			want:  "escapes the case directory",
		},
		{
			name: "workspace target escape",
			mutate: func(f *fixture) {
				f.yaml = strings.Replace(f.yaml, "      solution.py: correct/alt-01/solution.py",
					"      ../solution.py: correct/alt-01/solution.py", 1)
			},
			field: `correct_alternatives[0].files["../solution.py"]`,
			want:  "escapes the workspace",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := validFixture()
			test.mutate(&f)
			_, err := LoadCase(writeFixture(t, f))
			requireCaseError(t, err, InvalidCase, test.field)
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestLoadCaseRejectsSymlinkEscape(t *testing.T) {
	f := validFixture()
	delete(f.files, "correct/alt-01/solution.py")
	dir := writeFixture(t, f)

	outsideDir := t.TempDir()
	outside := filepath.Join(outsideDir, "secret.py")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "correct", "alt-01", "solution.py")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}

	_, err := LoadCase(dir)
	requireCaseError(t, err, InvalidCase, `correct_alternatives[0].files["solution.py"]`)
	if !strings.Contains(err.Error(), "resolves outside the case directory") {
		t.Fatalf("error = %v, want symlink escape explanation", err)
	}
}

func TestLoadCorpusReportsEveryCase(t *testing.T) {
	root := t.TempDir()
	writeFixtureAt(t, filepath.Join(root, "valid"), validFixture())

	incomplete := validFixture()
	delete(incomplete.files, "environment/manifest.json")
	writeFixtureAt(t, filepath.Join(root, "incomplete"), incomplete)

	invalid := validFixture()
	invalid.yaml = strings.Replace(invalid.yaml, "objective: Add two integers.\n", "", 1)
	writeFixtureAt(t, filepath.Join(root, "invalid"), invalid)

	results, err := LoadCorpus(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("loaded %d cases, want 3", len(results))
	}
	byName := make(map[string]CorpusCase, len(results))
	for _, result := range results {
		byName[filepath.Base(result.Dir)] = result
	}
	if byName["valid"].Err != nil {
		t.Fatalf("valid case error = %v", byName["valid"].Err)
	}
	if byName["valid"].Case == nil {
		t.Fatal("valid case was silently skipped")
	}
	if !IsIncomplete(byName["incomplete"].Err) {
		t.Fatalf("incomplete error = %v, want incomplete", byName["incomplete"].Err)
	}
	var invalidErr *CaseError
	if !errors.As(byName["invalid"].Err, &invalidErr) || invalidErr.Kind != InvalidCase {
		t.Fatalf("invalid error = %v, want invalid", byName["invalid"].Err)
	}
}
