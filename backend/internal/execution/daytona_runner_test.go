package execution

// Integration tests for DaytonaRunner against the real Daytona API. These
// are the finish line for implementing daytona_runner.go — they currently
// fail with errNotImplemented. Run them with:
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

const solutionTests = `import solution

cases = [
    (([2, 7, 11, 15], 9), [0, 1]),
    (([3, 2, 4], 6), [1, 2]),
    (([3, 3], 6), [0, 1]),
]
for args, want in cases:
    got = solution.two_sum(*args)
    assert got == want, f"two_sum{args}: got {got}, want {want}"
print(f"PASS {len(cases)} cases")
`

// networkProbe must FAIL inside the sandbox: submission runs are created
// with NetworkBlockAll and may not reach the internet.
const networkProbe = `import urllib.request

try:
    urllib.request.urlopen("https://registry.npmjs.org/", timeout=8)
    print("REACHED")
    raise SystemExit(2)
except SystemExit:
    raise
except Exception as exc:
    print(f"BLOCKED: {type(exc).__name__}")
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
	if !strings.Contains(outcome.Output, "PASS 3 cases") {
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
	if outcome.ExitCode == 0 {
		t.Fatalf("expected nonzero exit code, got 0 (output: %s)", outcome.Output)
	}
	if !strings.Contains(outcome.Output, "AssertionError") {
		t.Fatalf("expected AssertionError in output, got: %s", outcome.Output)
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
}
