package execution

// Integration tests for DaytonaRunner against the real Daytona API. Run them
// with:
//
//	doppler run -p codegym -c dev -- go test ./internal/execution -run TestDaytonaRunner -count=1
//
// Without DAYTONA_API_KEY set they skip, so `go test ./...` stays green.

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

const passingSolution = `def two_sum(nums, target):
    seen = {}
    for i, n in enumerate(nums):
        if target - n in seen:
            return [seen[target - n], i]
        seen[n] = i
    return []
`

const brokenSolution = `def two_sum(nums, target):
    return []
`

const solutionTests = `import json
import solution
import time

cases = [
    ("basic", ([2, 7, 11, 15], 9), [0, 1]),
    ("unordered", ([3, 2, 4], 6), [1, 2]),
    ("duplicates", ([3, 3], 6), [0, 1]),
]
results = []
for name, args, want in cases:
    started = time.perf_counter()
    got = solution.two_sum(*args)
    passed = got == want
    results.append({"name": name, "status": "pass" if passed else "fail", "duration_ms": int((time.perf_counter() - started) * 1000), "error": None if passed else f"got {got}, want {want}"})
print("CODEGYM_RESULT " + json.dumps({"tests": results, "compile_error": None}, separators=(",", ":")))
print(f"finished {len(cases)} cases")
`

// networkProbe must FAIL inside the sandbox: submission runs are created
// with NetworkBlockAll and may not reach the internet.
const networkProbe = `import json
import urllib.request

try:
    urllib.request.urlopen("https://registry.npmjs.org/", timeout=8)
    print("REACHED")
    raise SystemExit(2)
except SystemExit:
    raise
except Exception as exc:
    print(f"BLOCKED: {type(exc).__name__}")
    print("CODEGYM_RESULT " + json.dumps({"tests":[{"name":"network","status":"pass","duration_ms":0,"error":None}],"compile_error":None}, separators=(",", ":")))
`

func daytonaRunner(t *testing.T) *DaytonaRunner {
	t.Helper()
	apiKey := os.Getenv("DAYTONA_API_KEY")
	if apiKey == "" {
		t.Skip("set DAYTONA_API_KEY (via doppler run -p codegym -c dev) to run Daytona integration tests")
	}
	runner, err := NewDaytonaRunner(apiKey, os.Getenv("DAYTONA_API_URL"))
	if err != nil {
		t.Fatalf("NewDaytonaRunner returned error: %v", err)
	}
	return runner
}

func runnerContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	return ctx
}

func pythonSpec(solution, tests string) RunSpec {
	lang, _ := LanguageFor("python")
	return RunSpec{
		Language:   lang,
		Entrypoint: "test_solution.py",
		Files: []File{
			{Path: "solution.py", Content: solution},
			{Path: "test_solution.py", Content: tests},
		},
	}
}

func TestDaytonaRunnerPassingSubmission(t *testing.T) {
	runner := daytonaRunner(t)

	outcome, err := runner.Run(runnerContext(t), pythonSpec(passingSolution, solutionTests))
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if outcome.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d (output: %s)", outcome.ExitCode, outcome.Output)
	}
	if outcome.Result.Status != JudgeStatusPassed {
		t.Fatalf("expected judge status passed, got %s", outcome.Result.Status)
	}
	if !strings.Contains(outcome.Output, "finished 3 cases") {
		t.Fatalf("expected passing output, got: %s", outcome.Output)
	}
	if outcome.Duration <= 0 {
		t.Fatal("expected a positive duration")
	}
}

func TestDaytonaRunnerFailingSubmission(t *testing.T) {
	runner := daytonaRunner(t)

	outcome, err := runner.Run(runnerContext(t), pythonSpec(brokenSolution, solutionTests))
	if err != nil {
		t.Fatalf("Run must not error on a failing submission (nonzero exit is a result): %v", err)
	}
	if outcome.Result.Status != JudgeStatusFailed {
		t.Fatalf("expected failed judge status, got %s (output: %s)", outcome.Result.Status, outcome.Output)
	}
	if len(outcome.Result.Cases) != 3 || outcome.Result.Cases[0].Status != "fail" {
		t.Fatalf("expected structured failing cases, got: %#v", outcome.Result.Cases)
	}
}

func TestDaytonaRunnerNetworkBlocked(t *testing.T) {
	runner := daytonaRunner(t)

	spec := pythonSpec(passingSolution, networkProbe)
	outcome, err := runner.Run(runnerContext(t), spec)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if strings.Contains(outcome.Output, "REACHED") {
		t.Fatalf("sandbox reached the internet — NetworkBlockAll is not set: %s", outcome.Output)
	}
	if !strings.Contains(outcome.Output, "BLOCKED") {
		t.Fatalf("expected blocked-egress output, got (exit %d): %s", outcome.ExitCode, outcome.Output)
	}
	if outcome.Result.Status != JudgeStatusPassed {
		t.Fatalf("expected passed network probe result, got %s", outcome.Result.Status)
	}
}
