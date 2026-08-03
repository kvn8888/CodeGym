package execution_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/execution"
	"github.com/kvn8888/codegym/backend/internal/problems"
	"github.com/kvn8888/codegym/backend/internal/submission"
)

const correctSeedSolution = `def two_sum(nums, target):
    print(f"searching {len(nums)} values")
    seen = {}
    for index, value in enumerate(nums):
        complement = target - value
        if complement in seen:
            return [seen[complement], index]
        seen[value] = index
    return []
`

const wrongSeedSolution = `def two_sum(nums, target):
    return []
`

const hangingSeedSolution = `def two_sum(nums, target):
    while True:
        pass
`

const memorySeedSolution = `def two_sum(nums, target):
    chunks = []
    while True:
        chunks.append(bytearray(16 * 1024 * 1024))
`

const crashingSeedSolution = `import os

def two_sum(nums, target):
    os._exit(1)
`

// TestSupervisorDaytonaEndToEnd is the acceptance gate for the out-of-process
// judge. It uses the real seeded harness and skips without Daytona credentials.
func TestSupervisorDaytonaEndToEnd(t *testing.T) {
	apiKey := os.Getenv("DAYTONA_API_KEY")
	if apiKey == "" {
		t.Skip("set DAYTONA_API_KEY (via doppler run -p codegym -c dev) to run the supervisor acceptance test")
	}
	runner, err := execution.NewDaytonaRunner(apiKey, os.Getenv("DAYTONA_API_URL"))
	if err != nil {
		t.Fatalf("NewDaytonaRunner: %v", err)
	}

	tests := []struct {
		name       string
		solution   string
		wantStatus execution.JudgeStatus
		check      func(*testing.T, execution.RunOutcome)
	}{
		{
			name:     "correct solution",
			solution: correctSeedSolution, wantStatus: execution.JudgeStatusPassed,
			check: func(t *testing.T, outcome execution.RunOutcome) {
				if len(outcome.Result.Cases) != 4 {
					t.Fatalf("cases = %#v", outcome.Result.Cases)
				}
				if !strings.Contains(outcome.Stdout, "searching 4 values") {
					t.Fatalf("captured stdout = %q", outcome.Stdout)
				}
			},
		},
		{
			name:     "wrong solution",
			solution: wrongSeedSolution, wantStatus: execution.JudgeStatusFailed,
			check: func(t *testing.T, outcome execution.RunOutcome) {
				if len(outcome.Result.Cases) != 4 || outcome.Result.Cases[0].Status != "fail" || outcome.Result.Cases[0].Error == nil {
					t.Fatalf("expected per-case failure detail, got %#v", outcome.Result.Cases)
				}
			},
		},
		{
			name:     "infinite loop",
			solution: hangingSeedSolution, wantStatus: execution.JudgeStatusTimeout,
			check: func(t *testing.T, outcome execution.RunOutcome) {
				assertFailureNamesCase(t, outcome, "finds a pair in a sorted-looking list")
			},
		},
		{
			name:     "unbounded allocation",
			solution: memorySeedSolution, wantStatus: execution.JudgeStatusOutOfMemory,
			check: func(t *testing.T, outcome execution.RunOutcome) {
				assertFailureNamesCase(t, outcome, "finds a pair in a sorted-looking list")
			},
		},
		{
			name:     "process exit",
			solution: crashingSeedSolution, wantStatus: execution.JudgeStatusCrashed,
			check: func(t *testing.T, outcome execution.RunOutcome) {
				assertFailureNamesCase(t, outcome, "finds a pair in a sorted-looking list")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			outcome, err := runner.Run(ctx, seededRunSpec(t, test.solution))
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if outcome.Result.Status != test.wantStatus {
				t.Fatalf("status = %s, want %s; result=%#v", outcome.Result.Status, test.wantStatus, outcome.Result)
			}
			test.check(t, outcome)
		})
	}
}

func seededRunSpec(t *testing.T, solution string) execution.RunSpec {
	t.Helper()
	problemService := problems.NewService(problems.NewInMemoryStore())
	if err := problemService.EnsureSeed(context.Background()); err != nil {
		t.Fatalf("EnsureSeed: %v", err)
	}
	definition, err := problemService.GetDefinition(context.Background(), "two-sum")
	if err != nil {
		t.Fatalf("GetDefinition: %v", err)
	}
	files, err := submission.AssembleFiles(
		[]execution.File{{Path: "solution.py", Content: solution}},
		definition.HiddenTestFiles,
	)
	if err != nil {
		t.Fatalf("AssembleFiles: %v", err)
	}
	language, ok := execution.LanguageFor(definition.Language)
	if !ok {
		t.Fatalf("unsupported seed language %q", definition.Language)
	}
	return execution.RunSpec{
		Language: language, Files: files, Entrypoint: definition.Entrypoint,
		Limits: execution.Limits{
			TimeoutSeconds: definition.Runtime.TimeoutSeconds,
			MemoryMB:       definition.Runtime.MemoryMB,
			NetworkMode:    definition.Runtime.NetworkMode,
		},
	}
}

func assertFailureNamesCase(t *testing.T, outcome execution.RunOutcome, caseName string) {
	t.Helper()
	if outcome.Result.FailureDetail == nil || !strings.Contains(*outcome.Result.FailureDetail, caseName) {
		t.Fatalf("failure_detail = %v, want case %q", outcome.Result.FailureDetail, caseName)
	}
}
