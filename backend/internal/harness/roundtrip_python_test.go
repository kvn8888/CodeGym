package harness_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/harness"
	"github.com/kvn8888/codegym/backend/internal/submission"
)

const twoSumReference = `def two_sum(nums, target):
    seen = {}
    for index, value in enumerate(nums):
        complement = target - value
        if complement in seen:
            return [seen[complement], index]
        seen[value] = index
    return []
`

func twoSumSpec(comparator harness.Comparator) harness.Spec {
	return harness.Spec{
		Language:   harness.LanguagePython,
		Module:     "solution",
		EntryPoint: "two_sum",
		ParamNames: []string{"nums", "target"},
		Cases: []harness.Case{
			{
				Name:       "sorted-looking",
				Args:       []json.RawMessage{json.RawMessage(`[2,7,11,15]`), json.RawMessage(`9`)},
				Expected:   json.RawMessage(`[0,1]`),
				Comparator: comparator,
			},
			{
				Name:       "unordered",
				Args:       []json.RawMessage{json.RawMessage(`[3,2,4]`), json.RawMessage(`6`)},
				Expected:   json.RawMessage(`[1,2]`),
				Comparator: comparator,
			},
			{
				Name:       "duplicates",
				Args:       []json.RawMessage{json.RawMessage(`[3,3]`), json.RawMessage(`6`)},
				Expected:   json.RawMessage(`[0,1]`),
				Comparator: comparator,
			},
			{
				Name:       "negative",
				Args:       []json.RawMessage{json.RawMessage(`[-3,4,3,90]`), json.RawMessage(`0`)},
				Expected:   json.RawMessage(`[0,2]`),
				Comparator: comparator,
			},
		},
	}
}

// These tests prove the server-owned harness and submission parser agree on
// the execution protocol. They do not verify that model-generated expected
// values are correct; reference-solution verification owns that boundary.
func TestPythonHarnessRoundTripReferenceSolution(t *testing.T) {
	result := runPythonHarness(t, twoSumSpec(harness.ComparatorUnorderedList), twoSumReference)

	if result.Status != "pass" || result.Total != 4 || result.Passed != 4 || result.Failed != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestPythonHarnessRoundTripReportsFailuresAndIsolatesExceptions(t *testing.T) {
	t.Run("wrong solution", func(t *testing.T) {
		result := runPythonHarness(t, twoSumSpec(harness.ComparatorUnorderedList), `def two_sum(nums, target):
    return []
`)
		if result.Total != 4 || result.Passed != 0 || result.Failed != 4 {
			t.Fatalf("result = %#v", result)
		}
	})

	t.Run("per-case exception", func(t *testing.T) {
		result := runPythonHarness(t, twoSumSpec(harness.ComparatorUnorderedList), `def two_sum(nums, target):
    if target == 6:
        raise ValueError("case exploded")
    seen = {}
    for index, value in enumerate(nums):
        if target - value in seen:
            return [seen[target - value], index]
        seen[value] = index
    return []
`)
		if result.Total != 4 || result.Passed != 2 || result.Failed != 2 {
			t.Fatalf("result = %#v", result)
		}
		if result.TestCases[1].Error == nil || !strings.Contains(*result.TestCases[1].Error, "ValueError") {
			t.Fatalf("exception result = %#v", result.TestCases[1])
		}
	})
}

func TestPythonHarnessRoundTripReportsCompileAndEntryPointErrors(t *testing.T) {
	t.Run("syntax error exits cleanly", func(t *testing.T) {
		result := runPythonHarness(t, twoSumSpec(harness.ComparatorUnorderedList), "def two_sum(:\n")
		if result.CompileError == nil || !strings.Contains(*result.CompileError, "SyntaxError") {
			t.Fatalf("result = %#v", result)
		}
	})

	t.Run("missing entry point", func(t *testing.T) {
		result := runPythonHarness(t, twoSumSpec(harness.ComparatorUnorderedList), `def another_function():
    return []
`)
		if result.CompileError == nil || !strings.Contains(*result.CompileError, "AttributeError") {
			t.Fatalf("result = %#v", result)
		}
	})
}

func TestPythonHarnessRoundTripUsesLastResultAndComparatorSlot(t *testing.T) {
	t.Run("user lookalike output is ignored", func(t *testing.T) {
		solution := `print('CODEGYM_RESULT {"tests":[],"compile_error":"spoof"}')
` + twoSumReference
		result := runPythonHarness(t, twoSumSpec(harness.ComparatorUnorderedList), solution)
		if result.Status != "pass" || result.Passed != 4 || result.CompileError != nil {
			t.Fatalf("result = %#v", result)
		}
	})

	t.Run("comparator changes behavior", func(t *testing.T) {
		solution := `def two_sum(nums, target):
    return [1, 0]
`
		unorderedSpec := twoSumSpec(harness.ComparatorUnorderedList)
		unorderedSpec.Cases = unorderedSpec.Cases[:1]
		unordered := runPythonHarness(t, unorderedSpec, solution)
		if unordered.Passed != 1 {
			t.Fatalf("unordered result = %#v", unordered)
		}

		equalSpec := unorderedSpec
		equalSpec.Cases = append([]harness.Case(nil), unorderedSpec.Cases...)
		equalSpec.Cases[0].Comparator = harness.ComparatorEqual
		equal := runPythonHarness(t, equalSpec, solution)
		if equal.Failed != 1 {
			t.Fatalf("equal result = %#v", equal)
		}
	})
}

func runPythonHarness(t *testing.T, spec harness.Spec, solution string) submission.TestResult {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatal("python3 is required in CI")
		}
		t.Skip("python3 is not installed")
	}
	rendered, err := harness.Render(spec)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "solution.py"), []byte(solution), 0o600); err != nil {
		t.Fatalf("write solution: %v", err)
	}
	if err := os.WriteFile(filepath.Join(directory, rendered.Path), []byte(rendered.Content), 0o600); err != nil {
		t.Fatalf("write harness: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, python, "-B", rendered.Path)
	command.Dir = directory
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("python harness exited nonzero: %v\nstderr:\n%s", err, stderr.String())
	}
	if ctx.Err() != nil {
		t.Fatalf("python harness timed out: %v", ctx.Err())
	}
	result, err := submission.ParseTestResult(stdout.String())
	if err != nil {
		t.Fatalf("ParseTestResult: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	return result
}
