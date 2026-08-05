package agentruntime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBenchmarkDecisionRule(t *testing.T) {
	tests := []struct {
		name     string
		purpose  RuntimeBenchmarkSummary
		opencode RuntimeBenchmarkSummary
		selected string
		outcome  string
	}{
		{
			name:     "completion rate primary",
			purpose:  RuntimeBenchmarkSummary{Runtime: "purpose-built", Runs: 6, Passes: 4, CompletionRate: 4.0 / 6},
			opencode: RuntimeBenchmarkSummary{Runtime: "opencode", Runs: 6, Passes: 5, CompletionRate: 5.0 / 6},
			selected: "opencode", outcome: "completion-rate",
		},
		{
			name:     "policy disqualification",
			purpose:  RuntimeBenchmarkSummary{Runtime: "purpose-built", CompletionRate: 1},
			opencode: RuntimeBenchmarkSummary{Runtime: "opencode", CompletionRate: 1, Disqualified: true},
			selected: "purpose-built", outcome: "policy-disqualification",
		},
		{
			name:     "material no-regression tie break",
			purpose:  RuntimeBenchmarkSummary{Runtime: "purpose-built", CompletionRate: 1, MeanRepairIterations: 2, MedianWallTimeMS: 1000, Tokens: TokenUsage{Total: 100}, CostUSDMicros: 100},
			opencode: RuntimeBenchmarkSummary{Runtime: "opencode", CompletionRate: 1, MeanRepairIterations: 1, MedianWallTimeMS: 900, Tokens: TokenUsage{Total: 100}, CostUSDMicros: 100},
			selected: "opencode", outcome: "material-tie-break",
		},
		{
			name:     "inconclusive defaults purpose built",
			purpose:  RuntimeBenchmarkSummary{Runtime: "purpose-built", CompletionRate: 0, MedianWallTimeMS: 1000, Tokens: TokenUsage{Total: 100}, CostUSDMicros: 100},
			opencode: RuntimeBenchmarkSummary{Runtime: "opencode", CompletionRate: 0, MedianWallTimeMS: 900, Tokens: TokenUsage{Total: 120}, CostUSDMicros: 100},
			selected: "purpose-built", outcome: "effective-tie-or-inconclusive",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision := decideBenchmark([]RuntimeBenchmarkSummary{test.purpose, test.opencode})
			if decision.Selected != test.selected || decision.Outcome != test.outcome {
				t.Fatalf("decision = %#v", decision)
			}
		})
	}
}

func TestBenchmarkInterleavesAndWritesReport(t *testing.T) {
	fixtures := BenchmarkFixtures()
	runtimes := []*scriptedBenchmarkRuntime{
		{name: "purpose-built"}, {name: "opencode"},
	}
	reportPath := filepath.Join(t.TempDir(), "report.json")
	report, err := RunBenchmark(t.Context(), BenchmarkConfig{
		PurposeBuilt: runtimes[0], OpenCode: runtimes[1], Fixtures: fixtures,
		Repetitions: 3, TurnCeiling: 3, RunDeadline: 10 * time.Second, OutputCapBytes: DefaultOutputCap,
		WorkingRoot: t.TempDir(), ReportPath: reportPath, ModelDeployment: "fake/model",
		RelayMaxTokens: 100_000, RelayMaxCostUSDMicros: 1_000_000, RelayWallClock: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Runs) != 12 || report.Status != "complete" || report.WorkspacesCreated != 12 || report.WorkspacesDeleted != 12 {
		t.Fatalf("report = %#v", report)
	}
	for index := 0; index < len(report.Runs); index += 2 {
		if report.Runs[index].Runtime == report.Runs[index+1].Runtime {
			t.Fatalf("pair %d did not contain both runtimes", index/2+1)
		}
		if index >= 2 && report.Runs[index].Runtime == report.Runs[index-2].Runtime {
			t.Fatalf("pair %d did not alternate first runtime", index/2+1)
		}
	}
	if _, err := os.Stat(reportPath); err != nil {
		t.Fatal(err)
	}
}

type scriptedBenchmarkRuntime struct {
	name string
}

func (r *scriptedBenchmarkRuntime) Name() string { return r.name }

func (r *scriptedBenchmarkRuntime) Run(_ context.Context, task TaskSpec) (RunResult, error) {
	fixture := BenchmarkFixtures()[0]
	if stringsContains(task.Goal, "Express") {
		fixture = BenchmarkFixtures()[1]
	}
	if err := fixture.MaterializeGolden(task.WorkingDirectory); err != nil {
		return RunResult{}, err
	}
	manifest := filepath.Join(task.WorkingDirectory, ManifestRelativePath)
	if err := os.MkdirAll(filepath.Dir(manifest), 0o700); err != nil {
		return RunResult{}, err
	}
	if err := os.WriteFile(manifest, []byte(`{"version":1,"completed":true}`), 0o600); err != nil {
		return RunResult{}, err
	}
	return RunResult{
		Telemetry: Telemetry{Runtime: r.name, Termination: TerminationCompleted, WallTime: time.Millisecond},
		Manifest:  LoadManifest(task.WorkingDirectory),
	}, nil
}

func stringsContains(value, substring string) bool {
	for index := 0; index+len(substring) <= len(value); index++ {
		if value[index:index+len(substring)] == substring {
			return true
		}
	}
	return false
}
