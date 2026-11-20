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
	"time"

	"github.com/kvn8888/codegym/backend/internal/benchmarkbudget"
	"github.com/kvn8888/codegym/backend/internal/environmentverify"
	"github.com/kvn8888/codegym/backend/internal/problemquality/benchmark"
)

const (
	developmentFixtureSource = "../../internal/problemquality/testdata/development-order-tools"
	pythonSumEvenNumbersCase = "../../../benchmarks/problem-quality/cases/python-sum-even-numbers"
	goCountEvenNumbersCase   = "../../../benchmarks/problem-quality/cases/go-count-even-numbers"
	expressGreetingCase      = "../../../benchmarks/problem-quality/cases/express-greeting-endpoint"
	springBootGreetingCase   = "../../../benchmarks/problem-quality/cases/spring-boot-greeting-endpoint"
	reportSchemaPath         = "../../../benchmarks/problem-quality/schema/report.schema.json"
)

func requirePython3(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is required to execute the development fixture; not masking it with mocked success")
	}
}

func requirePythonPytest(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("python"); err != nil {
		t.Skip("skipping python-sum-even-numbers CLI test: python is not on PATH")
	}
	if output, err := exec.Command("python", "-m", "pytest", "--version").CombinedOutput(); err != nil {
		t.Skipf("skipping python-sum-even-numbers CLI test: python -m pytest is unavailable: %v (%s)", err, strings.TrimSpace(string(output)))
	}
	t.Setenv("PYTHONDONTWRITEBYTECODE", "1")
}

func TestRunPythonSumEvenNumbersCaseExitsZero(t *testing.T) {
	requirePythonPytest(t)
	output := filepath.Join(t.TempDir(), "report.json")

	code, stdout, stderr := runCommand(t,
		"-execution", "local",
		"-case", pythonSumEvenNumbersCase,
		"-output", output,
		"-schema", reportSchemaPath,
		"-budget-state", "",
		"-timeout", "30s",
	)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%s stdout=%s", code, stderr, stdout)
	}
	assertNoPythonCaches(t, filepath.Join(pythonSumEvenNumbersCase, "artifact"))
	if _, err := os.Stat(filepath.Join(goCountEvenNumbersCase, "artifact", "go-count-even-numbers")); !os.IsNotExist(err) {
		t.Fatalf("Go build left a binary in the artifact: %v", err)
	}
	report := loadValidatedReport(t, output)
	if !report.Preliminary || report.DevelopmentFixture {
		t.Fatalf("report labels preliminary=%v development_fixture=%v, want preliminary true and development_fixture false",
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
	if caseReport.ReferenceOutcome == nil || !caseReport.ReferenceOutcome.Accepted {
		t.Fatalf("reference outcome = %+v, want accepted", caseReport.ReferenceOutcome)
	}
	if len(caseReport.CorrectAlternatives) != 1 || !caseReport.CorrectAlternatives[0].Accepted {
		t.Fatalf("correct alternatives = %+v, want one accepted alternative", caseReport.CorrectAlternatives)
	}
	if len(caseReport.WrongSolutions) != 2 {
		t.Fatalf("wrong solutions = %d, want 2", len(caseReport.WrongSolutions))
	}
	expectations := map[string]struct {
		testID   string
		expected string
		actual   string
	}{
		"sum-odd-numbers":       {testID: "hidden-mixed-values", expected: "6", actual: "4"},
		"ignore-negative-evens": {testID: "hidden-negative-values", expected: "0", actual: "2"},
	}
	for _, wrong := range caseReport.WrongSolutions {
		want, ok := expectations[wrong.ID]
		if !ok {
			t.Fatalf("unexpected wrong solution %q", wrong.ID)
		}
		if wrong.Surviving || !wrong.Rejected || !wrong.IntendedFailureMatched {
			t.Fatalf("wrong solution %q = %+v, want rejected for its intended failure", wrong.ID, wrong)
		}
		if wrong.IntendedFailure == nil ||
			wrong.IntendedFailure.TestID != want.testID ||
			wrong.IntendedFailure.Code != "WRONG_SUM" ||
			wrong.IntendedFailure.Expected != want.expected ||
			wrong.IntendedFailure.Actual != want.actual {
			t.Fatalf("wrong solution %q intended failure = %+v, want %+v", wrong.ID, wrong.IntendedFailure, want)
		}
	}
	if caseReport.BenchmarkEligible || caseReport.BenchmarkEligibility != environmentverify.BenchmarkEligibilityUnresolved {
		t.Fatalf("eligibility = %v/%q, want false/unresolved", caseReport.BenchmarkEligible, caseReport.BenchmarkEligibility)
	}
	if caseReport.DevelopmentFixture || caseReport.ExcludedFromReviewedTotals {
		t.Fatalf("fixture labels = %+v, want reviewed non-fixture case", caseReport)
	}
	if report.Summary.CorrectAlternativeAcceptance != nil || report.Summary.WrongSolutionRejection != nil {
		t.Fatalf("quality rates = %v/%v, want unset for a development fixture",
			report.Summary.CorrectAlternativeAcceptance, report.Summary.WrongSolutionRejection)
	}
}

func TestRunPythonAndGoCasesTogetherExitZero(t *testing.T) {
	requirePythonPytest(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("skipping python+go CLI test: go is not on PATH")
	}
	output := filepath.Join(t.TempDir(), "report.json")

	code, stdout, stderr := runCommand(t,
		"-execution", "local",
		"-case", pythonSumEvenNumbersCase,
		"-case", goCountEvenNumbersCase,
		"-output", output,
		"-schema", reportSchemaPath,
		"-budget-state", "",
		"-timeout", "60s",
	)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%s stdout=%s", code, stderr, stdout)
	}
	report := loadValidatedReport(t, output)
	if !report.Preliminary || report.DevelopmentFixture {
		t.Fatalf("report labels preliminary=%v development_fixture=%v, want preliminary true and development_fixture false",
			report.Preliminary, report.DevelopmentFixture)
	}
	if report.Summary.CasesTotal != 2 || report.Summary.CasesPassed != 2 ||
		report.Summary.CasesFailed != 0 || report.Summary.CasesError != 0 || report.Summary.CasesNotRun != 0 {
		t.Fatalf("summary = %+v, want two passed cases", report.Summary)
	}
	cases := make(map[string]benchmark.CaseReport, len(report.Cases))
	for _, caseReport := range report.Cases {
		cases[caseReport.CaseID] = caseReport
	}
	for _, caseID := range []string{"python-sum-even-numbers", "go-count-even-numbers"} {
		caseReport, ok := cases[caseID]
		if !ok {
			t.Fatalf("report is missing case %q", caseID)
		}
		if caseReport.Status != benchmark.CaseStatusPassed {
			t.Fatalf("case %q status = %q, want passed", caseID, caseReport.Status)
		}
		if caseReport.ReferenceOutcome == nil || !caseReport.ReferenceOutcome.Accepted {
			t.Fatalf("case %q reference outcome = %+v, want accepted", caseID, caseReport.ReferenceOutcome)
		}
		if len(caseReport.CorrectAlternatives) != 1 || !caseReport.CorrectAlternatives[0].Accepted {
			t.Fatalf("case %q correct alternatives = %+v, want one accepted alternative", caseID, caseReport.CorrectAlternatives)
		}
		if len(caseReport.WrongSolutions) != 2 {
			t.Fatalf("case %q wrong solutions = %d, want 2", caseID, len(caseReport.WrongSolutions))
		}
		if caseReport.BenchmarkEligible || caseReport.BenchmarkEligibility != environmentverify.BenchmarkEligibilityUnresolved {
			t.Fatalf("case %q eligibility = %v/%q, want false/unresolved", caseID, caseReport.BenchmarkEligible, caseReport.BenchmarkEligibility)
		}
		if caseReport.DevelopmentFixture || caseReport.ExcludedFromReviewedTotals {
			t.Fatalf("case %q fixture labels = %+v, want reviewed non-fixture case", caseID, caseReport)
		}
	}

	expected := map[string]map[string]struct {
		testID   string
		expected string
		actual   string
	}{
		"python-sum-even-numbers": {
			"sum-odd-numbers":       {testID: "hidden-mixed-values", expected: "6", actual: "4"},
			"ignore-negative-evens": {testID: "hidden-negative-values", expected: "0", actual: "2"},
		},
		"go-count-even-numbers": {
			"count-odd-numbers":         {testID: "hidden-mixed-values", expected: "2", actual: "3"},
			"count-positive-evens-only": {testID: "hidden-negative-values", expected: "2", actual: "1"},
		},
	}
	for caseID, wrongExpectations := range expected {
		for _, wrong := range cases[caseID].WrongSolutions {
			want, ok := wrongExpectations[wrong.ID]
			if !ok {
				t.Fatalf("case %q unexpected wrong solution %q", caseID, wrong.ID)
			}
			if wrong.Surviving || !wrong.Rejected || !wrong.IntendedFailureMatched {
				t.Fatalf("case %q wrong solution %q = %+v, want rejected for its intended failure", caseID, wrong.ID, wrong)
			}
			if wrong.IntendedFailure == nil ||
				wrong.IntendedFailure.TestID != want.testID ||
				wrong.IntendedFailure.Expected != want.expected ||
				wrong.IntendedFailure.Actual != want.actual {
				t.Fatalf("case %q wrong solution %q intended failure = %+v, want test=%s expected=%s actual=%s",
					caseID, wrong.ID, wrong.IntendedFailure, want.testID, want.expected, want.actual)
			}
		}
	}
}

func assertNoPythonCaches(t *testing.T, root string) {
	t.Helper()
	var found []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && (info.Name() == "__pycache__" || info.Name() == ".pytest_cache") {
			found = append(found, path)
			return filepath.SkipDir
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".pyc") {
			found = append(found, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk Python artifact: %v", err)
	}
	if len(found) > 0 {
		t.Fatalf("Python artifact contains caches: %v", found)
	}
}

func requireNodeNPM(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("skipping express-greeting-endpoint CLI test: node is not on PATH")
	}
	if _, err := exec.LookPath("npm"); err != nil {
		t.Skip("skipping express-greeting-endpoint CLI test: npm is not on PATH")
	}
	t.Setenv("npm_config_audit", "false")
	t.Setenv("npm_config_fund", "false")
	t.Setenv("npm_config_update_notifier", "false")

	artifactDir := filepath.Join(expressGreetingCase, "artifact")
	if _, err := os.Stat(filepath.Join(artifactDir, "node_modules", "express")); err == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	install := exec.CommandContext(ctx, "npm", "ci", "--ignore-scripts")
	install.Dir = artifactDir
	if output, err := install.CombinedOutput(); err != nil {
		t.Skipf("skipping express-greeting-endpoint CLI test: npm ci failed: %v (%s)", err, strings.TrimSpace(string(output)))
	}
}

func requireJava21Maven(t *testing.T) {
	t.Helper()
	javaPath, err := exec.LookPath("java")
	if err != nil {
		t.Skip("skipping spring-boot-greeting-endpoint CLI test: java is not on PATH")
	}
	output, err := exec.Command(javaPath, "-version").CombinedOutput()
	if err != nil {
		t.Skipf("skipping spring-boot-greeting-endpoint CLI test: java -version failed: %v (%s)", err, strings.TrimSpace(string(output)))
	}
	major, ok := javaMajorVersion(string(output))
	if !ok {
		t.Skipf("skipping spring-boot-greeting-endpoint CLI test: could not determine Java version from %q", strings.TrimSpace(string(output)))
	}
	if major != 21 {
		t.Skipf("skipping spring-boot-greeting-endpoint CLI test: Java 21 is required, found %d", major)
	}
	if _, err := exec.LookPath("mvn"); err != nil {
		t.Skip("skipping spring-boot-greeting-endpoint CLI test: mvn is not on PATH")
	}
}

func javaMajorVersion(output string) (int, bool) {
	const marker = `version "`
	index := strings.Index(output, marker)
	if index < 0 {
		return 0, false
	}
	version := output[index+len(marker):]
	if end := strings.Index(version, `"`); end >= 0 {
		version = version[:end]
	}
	if separator := strings.IndexAny(version, ".[+-]"); separator >= 0 {
		version = version[:separator]
	}
	major, err := strconv.Atoi(version)
	if err != nil {
		return 0, false
	}
	return major, true
}

func TestRunSpringBootGreetingEndpointCaseExitsZero(t *testing.T) {
	requireJava21Maven(t)
	output := filepath.Join(t.TempDir(), "report.json")

	code, stdout, stderr := runCommand(t,
		"-execution", "local",
		"-case", springBootGreetingCase,
		"-output", output,
		"-schema", reportSchemaPath,
		"-budget-state", "",
		"-timeout", "300s",
	)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%s stdout=%s", code, stderr, stdout)
	}
	report := loadValidatedReport(t, output)
	if !report.Preliminary || report.DevelopmentFixture {
		t.Fatalf("report labels preliminary=%v development_fixture=%v, want preliminary true and development_fixture false",
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
	if caseReport.ReferenceOutcome == nil || !caseReport.ReferenceOutcome.Accepted {
		t.Fatalf("reference outcome = %+v, want accepted", caseReport.ReferenceOutcome)
	}
	if len(caseReport.CorrectAlternatives) != 1 || !caseReport.CorrectAlternatives[0].Accepted {
		t.Fatalf("correct alternatives = %+v, want one accepted alternative", caseReport.CorrectAlternatives)
	}
	if len(caseReport.WrongSolutions) != 2 {
		t.Fatalf("wrong solutions = %d, want 2", len(caseReport.WrongSolutions))
	}
	expectations := map[string]struct {
		testID   string
		code     string
		expected string
		actual   string
	}{
		"missing-name-validation": {testID: "hidden-missing-name", code: "HTTP_STATUS", expected: "400", actual: "200"},
		"wrong-response-field":    {testID: "hidden-response-shape", code: "JSON_FIELD", expected: "message", actual: "greeting"},
	}
	for _, wrong := range caseReport.WrongSolutions {
		want, ok := expectations[wrong.ID]
		if !ok {
			t.Fatalf("unexpected wrong solution %q", wrong.ID)
		}
		if wrong.Surviving || !wrong.Rejected || !wrong.IntendedFailureMatched {
			t.Fatalf("wrong solution %q = %+v, want rejected for its intended failure", wrong.ID, wrong)
		}
		if wrong.IntendedFailure == nil ||
			wrong.IntendedFailure.TestID != want.testID ||
			wrong.IntendedFailure.Code != want.code ||
			wrong.IntendedFailure.Expected != want.expected ||
			wrong.IntendedFailure.Actual != want.actual {
			t.Fatalf("wrong solution %q intended failure = %+v, want %+v", wrong.ID, wrong.IntendedFailure, want)
		}
	}
	if caseReport.BenchmarkEligible || caseReport.BenchmarkEligibility != environmentverify.BenchmarkEligibilityUnresolved {
		t.Fatalf("eligibility = %v/%q, want false/unresolved", caseReport.BenchmarkEligible, caseReport.BenchmarkEligibility)
	}
	if caseReport.DevelopmentFixture || caseReport.ExcludedFromReviewedTotals {
		t.Fatalf("fixture labels = %+v, want reviewed non-fixture case", caseReport)
	}
}

func TestRunExpressGreetingEndpointCaseExitsZero(t *testing.T) {
	requireNodeNPM(t)
	output := filepath.Join(t.TempDir(), "report.json")

	code, stdout, stderr := runCommand(t,
		"-execution", "local",
		"-case", expressGreetingCase,
		"-output", output,
		"-schema", reportSchemaPath,
		"-budget-state", "",
		"-timeout", "120s",
	)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%s stdout=%s", code, stderr, stdout)
	}
	report := loadValidatedReport(t, output)
	if !report.Preliminary || report.DevelopmentFixture {
		t.Fatalf("report labels preliminary=%v development_fixture=%v, want preliminary true and development_fixture false",
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
	if caseReport.ReferenceOutcome == nil || !caseReport.ReferenceOutcome.Accepted {
		t.Fatalf("reference outcome = %+v, want accepted", caseReport.ReferenceOutcome)
	}
	if len(caseReport.CorrectAlternatives) != 1 || !caseReport.CorrectAlternatives[0].Accepted {
		t.Fatalf("correct alternatives = %+v, want one accepted alternative", caseReport.CorrectAlternatives)
	}
	if len(caseReport.WrongSolutions) != 2 {
		t.Fatalf("wrong solutions = %d, want 2", len(caseReport.WrongSolutions))
	}
	expectations := map[string]struct {
		testID   string
		code     string
		expected string
		actual   string
	}{
		"missing-name-validation": {testID: "hidden-missing-name", code: "HTTP_STATUS", expected: "400", actual: "200"},
		"wrong-response-field":    {testID: "hidden-response-shape", code: "JSON_FIELD", expected: "message", actual: "greeting"},
	}
	for _, wrong := range caseReport.WrongSolutions {
		want, ok := expectations[wrong.ID]
		if !ok {
			t.Fatalf("unexpected wrong solution %q", wrong.ID)
		}
		if wrong.Surviving || !wrong.Rejected || !wrong.IntendedFailureMatched {
			t.Fatalf("wrong solution %q = %+v, want rejected for its intended failure", wrong.ID, wrong)
		}
		if wrong.IntendedFailure == nil ||
			wrong.IntendedFailure.TestID != want.testID ||
			wrong.IntendedFailure.Code != want.code ||
			wrong.IntendedFailure.Expected != want.expected ||
			wrong.IntendedFailure.Actual != want.actual {
			t.Fatalf("wrong solution %q intended failure = %+v, want %+v", wrong.ID, wrong.IntendedFailure, want)
		}
	}
	if caseReport.BenchmarkEligible || caseReport.BenchmarkEligibility != environmentverify.BenchmarkEligibilityUnresolved {
		t.Fatalf("eligibility = %v/%q, want false/unresolved", caseReport.BenchmarkEligible, caseReport.BenchmarkEligibility)
	}
	if caseReport.DevelopmentFixture || caseReport.ExcludedFromReviewedTotals {
		t.Fatalf("fixture labels = %+v, want reviewed non-fixture case", caseReport)
	}
}

func TestRunPythonGoAndExpressCasesTogetherExitZero(t *testing.T) {
	requirePythonPytest(t)
	requireNodeNPM(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("skipping python+go+express CLI test: go is not on PATH")
	}
	output := filepath.Join(t.TempDir(), "report.json")

	code, stdout, stderr := runCommand(t,
		"-execution", "local",
		"-case", pythonSumEvenNumbersCase,
		"-case", goCountEvenNumbersCase,
		"-case", expressGreetingCase,
		"-output", output,
		"-schema", reportSchemaPath,
		"-budget-state", "",
		"-timeout", "120s",
	)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%s stdout=%s", code, stderr, stdout)
	}
	assertNoPythonCaches(t, filepath.Join(pythonSumEvenNumbersCase, "artifact"))
	if _, err := os.Stat(filepath.Join(goCountEvenNumbersCase, "artifact", "go-count-even-numbers")); !os.IsNotExist(err) {
		t.Fatalf("Go build left a binary in the artifact: %v", err)
	}
	report := loadValidatedReport(t, output)
	if !report.Preliminary || report.DevelopmentFixture {
		t.Fatalf("report labels preliminary=%v development_fixture=%v, want preliminary true and development_fixture false",
			report.Preliminary, report.DevelopmentFixture)
	}
	if report.Summary.CasesTotal != 3 || report.Summary.CasesPassed != 3 ||
		report.Summary.CasesFailed != 0 || report.Summary.CasesError != 0 || report.Summary.CasesNotRun != 0 {
		t.Fatalf("summary = %+v, want three passed cases", report.Summary)
	}
	expected := map[string]map[string]struct {
		testID   string
		code     string
		expected string
		actual   string
	}{
		"python-sum-even-numbers": {
			"sum-odd-numbers":       {testID: "hidden-mixed-values", code: "WRONG_SUM", expected: "6", actual: "4"},
			"ignore-negative-evens": {testID: "hidden-negative-values", code: "WRONG_SUM", expected: "0", actual: "2"},
		},
		"go-count-even-numbers": {
			"count-odd-numbers":         {testID: "hidden-mixed-values", code: "WRONG_COUNT", expected: "2", actual: "3"},
			"count-positive-evens-only": {testID: "hidden-negative-values", code: "WRONG_COUNT", expected: "2", actual: "1"},
		},
		"express-greeting-endpoint": {
			"missing-name-validation": {testID: "hidden-missing-name", code: "HTTP_STATUS", expected: "400", actual: "200"},
			"wrong-response-field":    {testID: "hidden-response-shape", code: "JSON_FIELD", expected: "message", actual: "greeting"},
		},
	}
	cases := make(map[string]benchmark.CaseReport, len(report.Cases))
	for _, caseReport := range report.Cases {
		cases[caseReport.CaseID] = caseReport
	}
	for caseID, wrongExpectations := range expected {
		caseReport, ok := cases[caseID]
		if !ok {
			t.Fatalf("report is missing case %q", caseID)
		}
		if caseReport.Status != benchmark.CaseStatusPassed {
			t.Fatalf("case %q status = %q, want passed", caseID, caseReport.Status)
		}
		if caseReport.ReferenceOutcome == nil || !caseReport.ReferenceOutcome.Accepted {
			t.Fatalf("case %q reference outcome = %+v, want accepted", caseID, caseReport.ReferenceOutcome)
		}
		if len(caseReport.CorrectAlternatives) != 1 || !caseReport.CorrectAlternatives[0].Accepted {
			t.Fatalf("case %q correct alternatives = %+v, want one accepted alternative", caseID, caseReport.CorrectAlternatives)
		}
		if len(caseReport.WrongSolutions) != 2 {
			t.Fatalf("case %q wrong solutions = %d, want 2", caseID, len(caseReport.WrongSolutions))
		}
		if caseReport.BenchmarkEligible || caseReport.BenchmarkEligibility != environmentverify.BenchmarkEligibilityUnresolved {
			t.Fatalf("case %q eligibility = %v/%q, want false/unresolved", caseID, caseReport.BenchmarkEligible, caseReport.BenchmarkEligibility)
		}
		if caseReport.DevelopmentFixture || caseReport.ExcludedFromReviewedTotals {
			t.Fatalf("case %q fixture labels = %+v, want reviewed non-fixture case", caseID, caseReport)
		}
		for _, wrong := range caseReport.WrongSolutions {
			want, ok := wrongExpectations[wrong.ID]
			if !ok {
				t.Fatalf("case %q unexpected wrong solution %q", caseID, wrong.ID)
			}
			if wrong.Surviving || !wrong.Rejected || !wrong.IntendedFailureMatched {
				t.Fatalf("case %q wrong solution %q = %+v, want rejected for its intended failure", caseID, wrong.ID, wrong)
			}
			if wrong.IntendedFailure == nil ||
				wrong.IntendedFailure.TestID != want.testID ||
				wrong.IntendedFailure.Code != want.code ||
				wrong.IntendedFailure.Expected != want.expected ||
				wrong.IntendedFailure.Actual != want.actual {
				t.Fatalf("case %q wrong solution %q intended failure = %+v, want %+v", caseID, wrong.ID, wrong.IntendedFailure, want)
			}
		}
	}
}

func TestRunAllFourCorpusCasesTogetherExitZero(t *testing.T) {
	requirePythonPytest(t)
	requireNodeNPM(t)
	requireJava21Maven(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("skipping all-four CLI test: go is not on PATH")
	}
	output := filepath.Join(t.TempDir(), "report.json")

	code, stdout, stderr := runCommand(t,
		"-execution", "local",
		"-case", pythonSumEvenNumbersCase,
		"-case", goCountEvenNumbersCase,
		"-case", expressGreetingCase,
		"-case", springBootGreetingCase,
		"-output", output,
		"-schema", reportSchemaPath,
		"-budget-state", "",
		"-timeout", "300s",
	)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%s stdout=%s", code, stderr, stdout)
	}
	assertNoPythonCaches(t, filepath.Join(pythonSumEvenNumbersCase, "artifact"))
	if _, err := os.Stat(filepath.Join(goCountEvenNumbersCase, "artifact", "go-count-even-numbers")); !os.IsNotExist(err) {
		t.Fatalf("Go build left a binary in the artifact: %v", err)
	}
	report := loadValidatedReport(t, output)
	if !report.Preliminary || report.DevelopmentFixture {
		t.Fatalf("report labels preliminary=%v development_fixture=%v, want preliminary true and development_fixture false",
			report.Preliminary, report.DevelopmentFixture)
	}
	if report.Summary.CasesTotal != 4 || report.Summary.CasesPassed != 4 ||
		report.Summary.CasesFailed != 0 || report.Summary.CasesError != 0 || report.Summary.CasesNotRun != 0 {
		t.Fatalf("summary = %+v, want four passed cases", report.Summary)
	}
	expected := map[string]map[string]struct {
		testID   string
		code     string
		expected string
		actual   string
	}{
		"python-sum-even-numbers": {
			"sum-odd-numbers":       {testID: "hidden-mixed-values", code: "WRONG_SUM", expected: "6", actual: "4"},
			"ignore-negative-evens": {testID: "hidden-negative-values", code: "WRONG_SUM", expected: "0", actual: "2"},
		},
		"go-count-even-numbers": {
			"count-odd-numbers":         {testID: "hidden-mixed-values", code: "WRONG_COUNT", expected: "2", actual: "3"},
			"count-positive-evens-only": {testID: "hidden-negative-values", code: "WRONG_COUNT", expected: "2", actual: "1"},
		},
		"express-greeting-endpoint": {
			"missing-name-validation": {testID: "hidden-missing-name", code: "HTTP_STATUS", expected: "400", actual: "200"},
			"wrong-response-field":    {testID: "hidden-response-shape", code: "JSON_FIELD", expected: "message", actual: "greeting"},
		},
		"spring-boot-greeting-endpoint": {
			"missing-name-validation": {testID: "hidden-missing-name", code: "HTTP_STATUS", expected: "400", actual: "200"},
			"wrong-response-field":    {testID: "hidden-response-shape", code: "JSON_FIELD", expected: "message", actual: "greeting"},
		},
	}
	cases := make(map[string]benchmark.CaseReport, len(report.Cases))
	for _, caseReport := range report.Cases {
		cases[caseReport.CaseID] = caseReport
	}
	if len(cases) != len(expected) {
		t.Fatalf("report cases = %d, want %d", len(cases), len(expected))
	}
	for caseID, wrongExpectations := range expected {
		caseReport, ok := cases[caseID]
		if !ok {
			t.Fatalf("report is missing case %q", caseID)
		}
		if caseReport.Status != benchmark.CaseStatusPassed {
			t.Fatalf("case %q status = %q, want passed", caseID, caseReport.Status)
		}
		if caseReport.ReferenceOutcome == nil || !caseReport.ReferenceOutcome.Accepted {
			t.Fatalf("case %q reference outcome = %+v, want accepted", caseID, caseReport.ReferenceOutcome)
		}
		if len(caseReport.CorrectAlternatives) != 1 || !caseReport.CorrectAlternatives[0].Accepted {
			t.Fatalf("case %q correct alternatives = %+v, want one accepted alternative", caseID, caseReport.CorrectAlternatives)
		}
		if len(caseReport.WrongSolutions) != 2 {
			t.Fatalf("case %q wrong solutions = %d, want 2", caseID, len(caseReport.WrongSolutions))
		}
		if caseReport.BenchmarkEligible || caseReport.BenchmarkEligibility != environmentverify.BenchmarkEligibilityUnresolved {
			t.Fatalf("case %q eligibility = %v/%q, want false/unresolved", caseID, caseReport.BenchmarkEligible, caseReport.BenchmarkEligibility)
		}
		if caseReport.DevelopmentFixture || caseReport.ExcludedFromReviewedTotals {
			t.Fatalf("case %q fixture labels = %+v, want reviewed non-fixture case", caseID, caseReport)
		}
		for _, wrong := range caseReport.WrongSolutions {
			want, ok := wrongExpectations[wrong.ID]
			if !ok {
				t.Fatalf("case %q unexpected wrong solution %q", caseID, wrong.ID)
			}
			if wrong.Surviving || !wrong.Rejected || !wrong.IntendedFailureMatched {
				t.Fatalf("case %q wrong solution %q = %+v, want rejected for its intended failure", caseID, wrong.ID, wrong)
			}
			if wrong.IntendedFailure == nil ||
				wrong.IntendedFailure.TestID != want.testID ||
				wrong.IntendedFailure.Code != want.code ||
				wrong.IntendedFailure.Expected != want.expected ||
				wrong.IntendedFailure.Actual != want.actual {
				t.Fatalf("case %q wrong solution %q intended failure = %+v, want %+v", caseID, wrong.ID, wrong.IntendedFailure, want)
			}
		}
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

// addSyntheticGenerationProvenance writes a temporary saved-case provenance
// record. The fixture copy and the values are synthetic evidence for checking
// propagation only; they do not represent a real generation run or promotion
// to reviewed eligibility.
func addSyntheticGenerationProvenance(t *testing.T, fixture string) {
	t.Helper()
	path := filepath.Join(fixture, "benchmark-run.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	document["generation_provenance"] = map[string]any{
		"model_revision":  "synthetic-model-2026-11",
		"prompt_revision": "synthetic-prompt-v3",
		"config": map[string]any{
			"temperature":       0.3,
			"max_output_tokens": 2048,
		},
		"usage": map[string]any{
			"input_tokens":  1234,
			"output_tokens": 567,
			"cost_micros":   8900,
			"currency":      "USD",
			"cost_status":   "provider_reported",
			"source":        "synthetic saved-case export",
		},
	}
	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
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

func TestRunCLIPropagatesSyntheticSavedCaseProvenanceWithoutBudgetMutation(t *testing.T) {
	requirePython3(t)
	fixture := copyDevelopmentFixture(t)
	strengthenNormalizationInput(t, fixture)
	addSyntheticGenerationProvenance(t, fixture)
	output := filepath.Join(t.TempDir(), "report.json")
	budget := filepath.Join(t.TempDir(), "budget.json")

	code, stdout, stderr := runCommand(t,
		"-execution", "local",
		"-case", fixture,
		"-development-fixture",
		"-output", output,
		"-schema", reportSchemaPath,
		"-budget-state", budget,
		"-prior-spend-micros", "12345",
		"-timeout", "30s",
	)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%s stdout=%s", code, stderr, stdout)
	}
	report := loadValidatedReport(t, output)
	if report.ReportVersion != "2.0.0" || !report.Preliminary {
		t.Fatalf("report version/preliminary = %q/%v, want 2.0.0/true", report.ReportVersion, report.Preliminary)
	}
	if len(report.Cases) != 1 {
		t.Fatalf("cases = %d, want 1", len(report.Cases))
	}
	caseReport := report.Cases[0]
	if !caseReport.DevelopmentFixture || !caseReport.ExcludedFromReviewedTotals ||
		caseReport.BenchmarkEligible ||
		caseReport.BenchmarkEligibility != environmentverify.BenchmarkEligibilityUnresolved {
		t.Fatalf("eligibility/fixture labels = fixture:%t excluded:%t eligible:%t eligibility:%q",
			caseReport.DevelopmentFixture, caseReport.ExcludedFromReviewedTotals,
			caseReport.BenchmarkEligible, caseReport.BenchmarkEligibility)
	}
	if report.Summary.EligibleCases != 0 || report.Summary.UnresolvedEligibilityCases != 1 {
		t.Fatalf("summary eligibility = eligible:%d unresolved:%d, want 0/1",
			report.Summary.EligibleCases, report.Summary.UnresolvedEligibilityCases)
	}

	provenance := caseReport.Provenance
	if provenance == nil {
		t.Fatal("per-case provenance is missing")
	}
	if provenance.SavedExerciseProvenance != "known" ||
		provenance.ModelRevision != "synthetic-model-2026-11" ||
		provenance.ModelRevisionStatus != "known" ||
		provenance.PromptRevision != "synthetic-prompt-v3" ||
		provenance.PromptRevisionStatus != "known" {
		t.Fatalf("saved provenance = %+v", provenance)
	}
	if provenance.GenerationConfig["temperature"] != 0.3 ||
		provenance.GenerationConfig["max_output_tokens"] != float64(2048) {
		t.Fatalf("generation config = %+v, want synthetic settings unchanged", provenance.GenerationConfig)
	}
	saved := provenance.SavedExerciseUsage
	if saved.Status != "known" ||
		saved.InputTokens == nil || *saved.InputTokens != 1234 ||
		saved.OutputTokens == nil || *saved.OutputTokens != 567 ||
		saved.CostMicros == nil || *saved.CostMicros != 8900 ||
		saved.Currency != "USD" || saved.CostStatus != "provider_reported" ||
		saved.Source != "synthetic saved-case export" {
		t.Fatalf("saved usage = %+v, want synthetic token/cost values unchanged", saved)
	}
	assessment := provenance.AssessmentRunUsage
	if assessment.Status != "not_applicable" || assessment.InputTokens != nil ||
		assessment.OutputTokens != nil || assessment.CostMicros != nil {
		t.Fatalf("assessment usage = %+v, want separate not_applicable fact", assessment)
	}
	if report.GenerationConfig["assessment_run_usage"] != "not_applicable" ||
		report.GenerationConfig["model_operations"] != false ||
		report.GenerationConfig["paid_operations"] != false {
		t.Fatalf("generation config = %+v", report.GenerationConfig)
	}

	if report.Budget == nil {
		t.Fatal("budget snapshot is missing")
	}
	if report.Budget.PriorSpendMicros != 12345 || report.Budget.ReservedMicros != 0 ||
		report.Budget.SettledMicros != 0 || report.Budget.CommittedMicros != 12345 ||
		report.Budget.RemainingMicros != benchmarkbudget.MilestoneLimitMicros-12345 ||
		len(report.Budget.Reservations) != 0 {
		t.Fatalf("budget = %+v, want imported historical usage excluded from the ledger", report.Budget)
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

func TestRunReadinessReportsIncompleteProvenanceWithoutExecuting(t *testing.T) {
	code, stdout, stderr := runCommand(t,
		"-readiness",
		"-case", developmentFixtureSource,
		"-baseline-kind", "current",
	)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%s stdout=%s", code, stderr, stdout)
	}
	for _, want := range []string{
		"case: development-order-tools",
		"runnable: true",
		"development_fixture: true",
		"provenance_complete: false",
		"missing_provenance: generation_provenance",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout = %q, want %q", stdout, want)
		}
	}
}

func TestRunReadinessRequiresExplicitCase(t *testing.T) {
	code, stdout, stderr := runCommand(t, "-readiness")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2; stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stderr, "requires at least one explicit -case") {
		t.Fatalf("stderr = %q, want explicit case requirement", stderr)
	}
}
