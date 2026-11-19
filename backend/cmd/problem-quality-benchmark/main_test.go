package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/kvn8888/codegym/backend/internal/environmentverify"
	"github.com/kvn8888/codegym/backend/internal/problemquality/benchmark"
)

const (
	developmentFixtureSource = "../../internal/problemquality/testdata/development-order-tools"
	reportSchemaPath         = "../../../benchmarks/problem-quality/schema/report.schema.json"
)

func requirePython3(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is required to execute the development fixture; not masking it with mocked success")
	}
}

func copyDevelopmentFixture(t *testing.T) string {
	t.Helper()
	destination := filepath.Join(t.TempDir(), "development-order-tools")
	if err := os.CopyFS(destination, os.DirFS(developmentFixtureSource)); err != nil {
		t.Fatalf("copy development fixture: %v", err)
	}
	for _, cache := range []string{
		filepath.Join(destination, "artifact", "__pycache__"),
		filepath.Join(destination, "artifact", "protected", "__pycache__"),
	} {
		if err := os.RemoveAll(cache); err != nil {
			t.Fatalf("remove stale bytecode cache: %v", err)
		}
	}
	return destination
}

// strengthenNormalizationInput changes the temporary fixture copy's test input
// from lowercase to uppercase so the deliberate case-sensitive survivor from
// the checked-in fixture fails exactly its declared intended assertion. The
// checked-in fixture itself is never modified.
func strengthenNormalizationInput(t *testing.T, fixture string) {
	t.Helper()
	path := filepath.Join(fixture, "artifact", "protected", "test_order_tools.py")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	const original = `    # Intentionally weak: only lowercase input is exercised. A case-sensitive
    # implementation survives, exposing the missing uppercase coverage.
    try:
        got = normalize(["alpha"])`
	const replacement = `    # Temporary success-case input: uppercase coverage rejects the deliberate
    # case-sensitive survivor from the checked-in development fixture.
    try:
        got = normalize(["Alpha"])`
	normalized := strings.ReplaceAll(string(data), "\r\n", "\n")
	updated := strings.Replace(normalized, original, replacement, 1)
	if updated == string(data) {
		t.Fatalf("temporary fixture test does not contain the expected weak normalization block")
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
}

func breakReferenceSolution(t *testing.T, fixture string) {
	t.Helper()
	path := filepath.Join(fixture, "reference", "solution.py")
	if err := os.WriteFile(path, []byte("def normalize_tags(tags)\n    return []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func markFixtureSetupCommand(t *testing.T, fixture, marker string) {
	t.Helper()
	path := filepath.Join(fixture, "artifact", "manifest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	commands, ok := document["commands"].(map[string]any)
	if !ok {
		t.Fatalf("manifest commands = %T, want object", document["commands"])
	}
	commands["setup"] = fmt.Sprintf("python3 -c 'open(%s, \"w\").write(\"ran\")'", strconv.Quote(marker))
	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runCommand(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func loadValidatedReport(t *testing.T, path string) benchmark.Report {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	if err := benchmark.ValidateReportJSON(data, reportSchemaPath); err != nil {
		t.Fatalf("report does not validate: %v", err)
	}
	var report benchmark.Report
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	return report
}

func TestRunSuccessfulAssessmentExitsZero(t *testing.T) {
	requirePython3(t)
	fixture := copyDevelopmentFixture(t)
	strengthenNormalizationInput(t, fixture)
	output := filepath.Join(t.TempDir(), "report.json")

	code, stdout, stderr := runCommand(t,
		"-execution", "local",
		"-case", fixture,
		"-development-fixture",
		"-output", output,
		"-schema", reportSchemaPath,
		"-budget-state", "",
		"-timeout", "30s",
	)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%s stdout=%s", code, stderr, stdout)
	}
	report := loadValidatedReport(t, output)
	if !report.Preliminary || !report.DevelopmentFixture {
		t.Fatalf("report labels preliminary=%v development_fixture=%v, want both true for a temporary fixture copy",
			report.Preliminary, report.DevelopmentFixture)
	}
	if report.Summary.CasesTotal != 1 || report.Summary.CasesPassed != 1 ||
		report.Summary.CasesFailed != 0 || report.Summary.CasesError != 0 || report.Summary.CasesNotRun != 0 {
		t.Fatalf("summary = %+v, want one passed case", report.Summary)
	}
	caseReport := report.Cases[0]
	if caseReport.Status != benchmark.CaseStatusPassed {
		t.Fatalf("case status = %q, want passed", caseReport.Status)
	}
	if !caseReport.DevelopmentFixture || !caseReport.ExcludedFromReviewedTotals {
		t.Fatalf("case fixture labels = %+v, want development fixture excluded from reviewed totals", caseReport)
	}
	if caseReport.ReferenceOutcome == nil || !caseReport.ReferenceOutcome.Accepted {
		t.Fatalf("reference outcome = %+v, want accepted", caseReport.ReferenceOutcome)
	}
	for _, alternative := range caseReport.CorrectAlternatives {
		if !alternative.Accepted {
			t.Fatalf("correct alternative %q was not accepted: %+v", alternative.ID, alternative)
		}
	}
	for _, wrong := range caseReport.WrongSolutions {
		if wrong.Surviving {
			t.Fatalf("wrong solution %q survived in the temporary success case", wrong.ID)
		}
		if !wrong.Rejected {
			t.Fatalf("wrong solution %q was not rejected as intended: %+v", wrong.ID, wrong)
		}
	}
}

func TestRunCheckedInFixtureExitsOneForDeliberateSurvivor(t *testing.T) {
	requirePython3(t)
	output := filepath.Join(t.TempDir(), "report.json")

	code, stdout, stderr := runCommand(t,
		"-execution", "local",
		"-case", developmentFixtureSource,
		"-development-fixture",
		"-output", output,
		"-schema", reportSchemaPath,
		"-budget-state", "",
		"-timeout", "30s",
	)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1; stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stderr, "quality expectations were not met") {
		t.Fatalf("stderr = %q, want quality failure explanation", stderr)
	}
	report := loadValidatedReport(t, output)
	if !report.Preliminary {
		t.Fatal("preliminary = false, want a preliminary report on quality failure")
	}
	if report.Summary.CasesFailed != 1 || report.Summary.CasesError != 0 {
		t.Fatalf("summary = %+v, want one quality failure and no execution errors", report.Summary)
	}
	caseReport := report.Cases[0]
	if caseReport.Status != benchmark.CaseStatusFailed {
		t.Fatalf("case status = %q, want failed", caseReport.Status)
	}
	surviving := 0
	for _, wrong := range caseReport.WrongSolutions {
		if !wrong.Surviving {
			continue
		}
		surviving++
		if wrong.DiagnosticCode != environmentverify.CodeWrongSolutionPassed {
			t.Fatalf("surviving wrong solution %q diagnostic = %q", wrong.ID, wrong.DiagnosticCode)
		}
	}
	if surviving != 1 {
		t.Fatalf("surviving wrong solutions = %d, want the one deliberate survivor", surviving)
	}
}

func TestRunPerCaseExecutionFailureWritesPreliminaryReportAndExitsTwo(t *testing.T) {
	requirePython3(t)
	fixture := copyDevelopmentFixture(t)
	breakReferenceSolution(t, fixture)
	output := filepath.Join(t.TempDir(), "report.json")

	code, stdout, stderr := runCommand(t,
		"-execution", "local",
		"-case", fixture,
		"-development-fixture",
		"-output", output,
		"-schema", reportSchemaPath,
		"-budget-state", "",
		"-timeout", "30s",
	)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2; stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stderr, "execution/configuration failure") {
		t.Fatalf("stderr = %q, want execution failure explanation", stderr)
	}
	report := loadValidatedReport(t, output)
	if !report.Preliminary {
		t.Fatal("preliminary = false, want a preliminary report on per-case execution failure")
	}
	if report.Summary.CasesError != 1 || report.Summary.CasesPassed != 0 || report.Summary.CasesFailed != 0 {
		t.Fatalf("summary = %+v, want one per-case execution error", report.Summary)
	}
	caseReport := report.Cases[0]
	if caseReport.Status != benchmark.CaseStatusError {
		t.Fatalf("case status = %q, want error", caseReport.Status)
	}
	if !caseReport.Blocked || strings.TrimSpace(caseReport.FailureDetails) == "" {
		t.Fatalf("case failure evidence = %+v, want a blocked case with details", caseReport)
	}
}

func TestRunRejectsUnsupportedExecutionModesBeforeAnyWork(t *testing.T) {
	requirePython3(t)
	for _, mode := range []string{"paid", "model"} {
		t.Run(mode, func(t *testing.T) {
			workDir := t.TempDir()
			fixture := copyDevelopmentFixture(t)
			marker := filepath.Join(workDir, "command-ran")
			markFixtureSetupCommand(t, fixture, marker)
			output := filepath.Join(workDir, "report.json")
			budget := filepath.Join(workDir, "budget.json")

			code, stdout, stderr := runCommand(t,
				"-execution", mode,
				"-case", fixture,
				"-development-fixture",
				"-output", output,
				"-schema", reportSchemaPath,
				"-budget-state", budget,
				"-timeout", "30s",
			)
			if code != 2 {
				t.Fatalf("exit code = %d, want 2; stderr=%s stdout=%s", code, stderr, stdout)
			}
			if !strings.Contains(stderr, "unsupported") {
				t.Fatalf("stderr = %q, want unsupported execution mode explanation", stderr)
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("marker %q exists, want no artifact commands to run before mode rejection", marker)
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatalf("report %q exists, want no report before mode rejection", output)
			}
			if _, err := os.Stat(budget); !os.IsNotExist(err) {
				t.Fatalf("budget %q exists, want no budget file before mode rejection", budget)
			}
		})
	}
}

func TestRunInvalidConfigurationExitsTwoWithoutReport(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "missing execution mode",
			args: []string{"-case", developmentFixtureSource, "-development-fixture"},
		},
		{
			name: "missing case selection",
			args: []string{"-execution", "local", "-development-fixture"},
		},
		{
			name: "invalid baseline kind",
			args: []string{"-execution", "local", "-case", developmentFixtureSource, "-development-fixture", "-baseline-kind", "unknown"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "report.json")
			args := append([]string{"-output", output, "-schema", reportSchemaPath, "-budget-state", ""}, test.args...)
			code, stdout, stderr := runCommand(t, args...)
			if code != 2 {
				t.Fatalf("exit code = %d, want 2; stderr=%s stdout=%s", code, stderr, stdout)
			}
			if strings.TrimSpace(stderr) == "" {
				t.Fatal("stderr is empty, want a configuration error")
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatalf("report %q exists, want no report for invalid configuration", output)
			}
		})
	}
}
