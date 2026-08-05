package execution

import (
	"context"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/daytona/clients/sdk-go/pkg/types"
	"github.com/kvn8888/codegym/backend/internal/environment"
)

const hedgeCreateSamples = 15

// TestHedgeDaytonaEndToEndAndInterleavedLatency is the gated production
// promotion check. It measures the registered custom Go snapshot with the
// legacy and hedged paths interleaved, while also running the same real Go
// verdict once through each configuration.
func TestHedgeDaytonaEndToEndAndInterleavedLatency(t *testing.T) {
	apiKey := os.Getenv("DAYTONA_API_KEY")
	if apiKey == "" {
		t.Skip("set DAYTONA_API_KEY via doppler run -p codegym -c dev")
	}

	runners := make(map[int]*DaytonaRunner, 2)
	for _, hedgeCount := range []int{1, 2} {
		runner, err := NewDaytonaRunner(apiKey, os.Getenv("DAYTONA_API_URL"), environment.Dev)
		if err != nil {
			t.Fatalf("NewDaytonaRunner hedge_count=%d: %v", hedgeCount, err)
		}
		runner.WithHedgeCountProvider(fixedHedgeCount(hedgeCount))
		runners[hedgeCount] = runner
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	initial := waitForNoSubmissionSandboxes(t, ctx, runners[1], 3*time.Second)
	t.Logf("hedge_initial_leaked_sandboxes count=%d", initial)
	measurements := map[int][]int64{
		1: make([]int64, 0, hedgeCreateSamples),
		2: make([]int64, 0, hedgeCreateSamples),
	}
	var verdicts = make(map[int]JudgeResult, 2)

	for sample := 1; sample <= hedgeCreateSamples; sample++ {
		order := []int{1, 2}
		if sample%2 == 0 {
			order = []int{2, 1}
		}
		for _, hedgeCount := range order {
			runner := runners[hedgeCount]
			var createMS int64
			if sample == 1 {
				outcome, err := runner.Run(ctx, realHedgeGoSpec())
				if err != nil {
					t.Fatalf("real submission hedge_count=%d: %v", hedgeCount, err)
				}
				if outcome.Result.Status != JudgeStatusPassed || len(outcome.Result.Cases) != 2 {
					t.Fatalf("real submission hedge_count=%d result=%#v", hedgeCount, outcome.Result)
				}
				verdicts[hedgeCount] = outcome.Result
				createMS = outcome.StageDurations.CreateMs
			} else {
				createMS = measureRealHedgeCreate(t, ctx, runner)
			}
			waitForRunnerIdle(t, runner, 90*time.Second)
			measurements[hedgeCount] = append(measurements[hedgeCount], createMS)
			t.Logf("hedge_create_sample hedge_count=%d sample=%02d create_ms=%d", hedgeCount, sample, createMS)
		}
	}

	if !sameHedgeVerdict(verdicts[1], verdicts[2]) {
		t.Fatalf("hedged verdict differs from legacy\nlegacy=%#v\nhedged=%#v", verdicts[1], verdicts[2])
	}
	for _, hedgeCount := range []int{1, 2} {
		median, p90, maximum := summarizeMilliseconds(measurements[hedgeCount])
		t.Logf("hedge_create_summary hedge_count=%d n=%d median_ms=%d p90_ms=%d max_ms=%d",
			hedgeCount, len(measurements[hedgeCount]), median, p90, maximum)
	}

	remaining := waitForNoSubmissionSandboxes(t, ctx, runners[1], 45*time.Second)
	t.Logf("hedge_final_leaked_sandboxes count=%d", remaining)
}

func measureRealHedgeCreate(t *testing.T, ctx context.Context, runner *DaytonaRunner) int64 {
	t.Helper()
	started := time.Now()
	sandbox, _, err := runner.createSandbox(ctx, types.SnapshotParams{
		Snapshot: GoSnapshotName,
		SandboxBaseParams: types.SandboxBaseParams{
			Labels:          map[string]string{codegymSandboxLabel: codegymSandboxLabelValue},
			NetworkBlockAll: true,
			Ephemeral:       true,
		},
	})
	createMS := time.Since(started).Milliseconds()
	if err != nil {
		t.Fatalf("create Go sandbox: %v", err)
	}
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	deleteErr := sandbox.Delete(cleanupCtx)
	cancel()
	if deleteErr != nil {
		t.Fatalf("delete winning Go sandbox: %v", deleteErr)
	}
	return createMS
}

func realHedgeGoSpec() RunSpec {
	language, _ := LanguageFor("go")
	return RunSpec{
		Language: language,
		Files: []File{
			{Path: "solution.go", Content: "package main\n\nfunc add(left, right int) int { return left + right }\n"},
			{Path: "codegym_comparator.go", Content: GoComparatorSource},
			{Path: "test_solution.go", Content: localGoTestHarness},
			{Path: ".codegym/compile_and_run.py", Content: GoCompileRunnerSource},
		},
		Entrypoint: ".codegym/compile_and_run.py",
		Strategy:   TestStrategyUnit,
		Limits: Limits{
			TimeoutSeconds: 30,
			MemoryMB:       1024,
			NetworkMode:    NetworkModeBlockAll,
		},
	}
}

func sameHedgeVerdict(left, right JudgeResult) bool {
	if left.Status != right.Status || len(left.Cases) != len(right.Cases) {
		return false
	}
	for index := range left.Cases {
		if left.Cases[index].Name != right.Cases[index].Name ||
			left.Cases[index].Status != right.Cases[index].Status ||
			pointerString(left.Cases[index].Error) != pointerString(right.Cases[index].Error) {
			return false
		}
	}
	return pointerString(left.CompileError) == pointerString(right.CompileError)
}

func waitForRunnerIdle(t *testing.T, runner *DaytonaRunner, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for runner.activeRuns.count() != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("runner still tracks %d active sandbox run(s)", runner.activeRuns.count())
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func waitForNoSubmissionSandboxes(
	t *testing.T,
	ctx context.Context,
	runner *DaytonaRunner,
	timeout time.Duration,
) int {
	t.Helper()
	client, ok := runner.client.(daytonaSweepClient)
	if !ok {
		t.Fatal("Daytona client does not support labelled sandbox listing")
	}
	deadline := time.Now().Add(timeout)
	var remaining []sweepSandbox
	for {
		var err error
		remaining, err = client.ListSubmissionSandboxes(ctx)
		if err != nil {
			t.Fatalf("list CodeGym submission sandboxes: %v", err)
		}
		if len(remaining) == 0 {
			return 0
		}
		if time.Now().After(deadline) {
			ids := make([]string, 0, len(remaining))
			for _, sandbox := range remaining {
				ids = append(ids, sandbox.id)
			}
			sort.Strings(ids)
			t.Fatalf("%d labelled sandbox(es) remained after cleanup: %s", len(ids), strings.Join(ids, ", "))
		}
		time.Sleep(time.Second)
	}
}

func pointerString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
