package agentruntime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/agentrelay"
	"github.com/kvn8888/codegym/backend/internal/config"
	"github.com/kvn8888/codegym/backend/internal/generation/openaicompat"
	"github.com/kvn8888/codegym/backend/internal/usage"
)

func TestInterleavedRuntimeBenchmark(t *testing.T) {
	if os.Getenv("CODEGYM_RUN_AGENT_RUNTIME_BENCHMARK") != "1" {
		t.Skip("set CODEGYM_RUN_AGENT_RUNTIME_BENCHMARK=1 under Doppler to run the bounded model benchmark")
	}
	cfg := config.Load()
	if len(cfg.GenAIProviders) == 0 {
		t.Fatal("no GenAI provider is configured")
	}
	provider := cfg.GenAIProviders[0]
	upstream, err := openaicompat.New(openaicompat.Config{
		Name: provider.Name, BaseURL: provider.BaseURL, APIKey: provider.APIKey, Model: provider.Model,
		AuthStyle: provider.AuthStyle, APIVersion: provider.APIVersion, DefaultMaxTokens: provider.DefaultMaxTokens,
	})
	if err != nil {
		t.Fatal(err)
	}
	maxTokens := benchmarkInt64Env("CODEGYM_AGENT_BENCHMARK_MAX_TOKENS", 300_000)
	maxCostMicros := benchmarkInt64Env("CODEGYM_AGENT_BENCHMARK_MAX_COST_USD_MICROS", 2_000_000)
	startOrder := int(benchmarkInt64Env("CODEGYM_AGENT_BENCHMARK_START_ORDER", 1))
	relayWallClock := benchmarkDurationEnv("CODEGYM_AGENT_BENCHMARK_RELAY_WALL_CLOCK", 20*time.Minute)
	runDeadline := benchmarkDurationEnv("CODEGYM_AGENT_BENCHMARK_RUN_DEADLINE", DefaultBenchmarkRunDeadline)

	checker := benchmarkOperationChecker{}
	relayStore := agentrelay.NewInMemoryStore()
	tokenService, err := agentrelay.NewService(relayStore, agentrelay.ServiceConfig{
		TokenSecret: strings.Repeat("benchmark-secret-", 3), TokenTTL: relayWallClock + 5*time.Minute,
		DefaultMaxTotalTokens: maxTokens, DefaultMaxCostUSDMicros: maxCostMicros,
		DefaultMaxWallClock: relayWallClock, OperationChecker: checker,
	})
	if err != nil {
		t.Fatal(err)
	}
	issued, err := tokenService.Issue(t.Context(), agentrelay.IssueInput{
		OperationID: "agent-runtime-benchmark", WorkspaceID: "benchmark-workspace", UserID: "benchmark-user",
	})
	if err != nil {
		t.Fatal(err)
	}
	usageStore := usage.NewInMemoryStore()
	relayHandler, err := agentrelay.NewHTTPHandler(tokenService, upstream, usage.NewService(usageStore, time.Now), cfg.Relay.PublicModel)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", relayHandler.Models)
	mux.HandleFunc("POST /v1/chat/completions", relayHandler.ChatCompletions)
	relayServer := httptest.NewServer(mux)
	defer relayServer.Close()

	client, err := NewOpenAICompatibleClient(OpenAICompatibleClientConfig{
		BaseURL: relayServer.URL + "/v1", APIKey: issued.Token, Model: cfg.Relay.PublicModel,
		MaxTokens: provider.DefaultMaxTokens, MaxResponseBytes: 2 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	binary, template := pinnedOpenCodePaths(t)
	openCode, err := NewOpenCodeRuntime(OpenCodeConfig{
		BinaryPath: binary, RuntimeTemplate: template, RelayBaseURL: relayServer.URL + "/v1",
		RelayToken: issued.Token, Model: cfg.Relay.PublicModel,
	})
	if err != nil {
		t.Fatal(err)
	}
	var daytonaSuite *daytonaBenchmarkSuite
	executionEnvironment := "host-temporary-workspace"
	baseSnapshot := ""
	limitations := []string{
		"No Daytona sandboxes were created; filesystem-escape policy was observed through confined tools and output scanning, not an OS sandbox boundary.",
		"The offline Express verifier uses a controlled local Express API contract module plus proxy and source-level egress controls; a real npm dependency and OS network boundary require a pre-baked Daytona snapshot.",
	}
	sandboxMeter := SandboxMeter(func() (int, int) { return 0, 0 })
	var sandboxSecondsMeter SandboxSecondsMeter
	var verifierFactory BenchmarkVerifierFactory
	if os.Getenv("CODEGYM_RUN_AGENT_RUNTIME_DAYTONA_BENCHMARK") == "1" {
		daytonaSuite = newDaytonaBenchmarkSuite(t)
		executionEnvironment = "host-agent-runtime/daytona-real-dependency-verifier"
		baseSnapshot = daytonaBenchmarkSnapshot
		limitations = []string{
			"The coding-agent adapters ran in isolated host workspaces because the operation-scoped local relay is not reachable from Daytona; candidate dependency resolution and black-box verification ran in one ephemeral Daytona sandbox per contract-valid run.",
			"The shared Daytona base snapshot was codegym-go-1-25-4-v3. Dependency resolution used Daytona's organization-restricted network; the account tier rejected sandbox-level block-all overrides, so the loopback HTTP probe ran under the organization policy after source-level egress checks.",
		}
		sandboxMeter = daytonaSuite.Counts
		sandboxSecondsMeter = daytonaSuite.Seconds
		verifierFactory = daytonaSuite.VerifierFactory
	}
	reportPath := strings.TrimSpace(os.Getenv("CODEGYM_AGENT_BENCHMARK_REPORT"))
	if reportPath == "" {
		name := "benchmark-report.json"
		if daytonaSuite != nil {
			name = "benchmark-daytona-report.json"
		}
		reportPath = filepath.Join("internal", "agentruntime", "testdata", name)
	}
	report, err := RunBenchmark(t.Context(), BenchmarkConfig{
		PurposeBuilt: &PurposeBuiltRuntime{Client: client}, OpenCode: openCode,
		Fixtures: BenchmarkFixtures(), Repetitions: 3, StartOrder: startOrder,
		TurnCeiling: DefaultBenchmarkTurnCeiling, RunDeadline: runDeadline,
		OutputCapBytes: 256 * 1024, WorkingRoot: "/private/tmp", ReportPath: reportPath,
		ModelDeployment:      provider.Name + "/" + provider.Model,
		ExecutionEnvironment: executionEnvironment, BaseSnapshot: baseSnapshot, Limitations: limitations,
		RelayMaxTokens: maxTokens, RelayMaxCostUSDMicros: maxCostMicros, RelayWallClock: relayWallClock,
		RetryPolicy: "zero retries; every failure counts", MaxSandboxes: DefaultBenchmarkMaxSandboxes,
		VerifierFactory: verifierFactory,
		UsageMeter: func(ctx context.Context) (UsageSnapshot, error) {
			budget, err := relayStore.Get(ctx, "benchmark-workspace", "benchmark-user", "agent-runtime-benchmark")
			if err != nil {
				return UsageSnapshot{}, err
			}
			return UsageSnapshot{
				Tokens: TokenUsage{
					Total: budget.UsedTotalTokens, Input: budget.InputTokens, Output: budget.OutputTokens,
					Reasoning: budget.ReasoningTokens, CacheRead: budget.CacheReadTokens, CacheWrite: budget.CacheWriteTokens,
				},
				CostUSDMicros: budget.UsedCostUSDMicros,
			}, nil
		},
		SandboxMeter: sandboxMeter, SandboxSecondsMeter: sandboxSecondsMeter,
	})
	if err != nil {
		t.Fatal(err)
	}
	expectedRuns := report.ScheduleTotalRuns - report.ScheduleStartOrder + 1
	if report.Status == "complete" && len(report.Runs) != expectedRuns {
		t.Fatalf("complete report has %d runs, want %d for scheduled orders %d..%d",
			len(report.Runs), expectedRuns, report.ScheduleStartOrder, report.ScheduleTotalRuns)
	}
	if report.SandboxesCreated != report.SandboxesDeleted || report.WorkspacesCreated != report.WorkspacesDeleted {
		t.Fatalf("resources leaked: sandboxes %d/%d workspaces %d/%d",
			report.SandboxesCreated, report.SandboxesDeleted, report.WorkspacesCreated, report.WorkspacesDeleted)
	}
	if daytonaSuite != nil {
		daytonaSuite.AssertNoLabeledSandboxes(t)
	}
	t.Logf("benchmark report=%s status=%s runs=%d decision=%s reason=%s", reportPath, report.Status, len(report.Runs), report.Decision.Selected, report.Decision.Reason)
}

func TestMergeAgentRuntimeBenchmarkSegments(t *testing.T) {
	if os.Getenv("CODEGYM_MERGE_AGENT_RUNTIME_BENCHMARK") != "1" {
		t.Skip("set CODEGYM_MERGE_AGENT_RUNTIME_BENCHMARK=1 with segment and output paths")
	}
	paths := []string{
		strings.TrimSpace(os.Getenv("CODEGYM_AGENT_BENCHMARK_SEGMENT_ONE")),
		strings.TrimSpace(os.Getenv("CODEGYM_AGENT_BENCHMARK_SEGMENT_TWO")),
	}
	segments := make([]BenchmarkReport, 0, len(paths))
	for _, path := range paths {
		payload, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var report BenchmarkReport
		if err := json.Unmarshal(payload, &report); err != nil {
			t.Fatal(err)
		}
		segments = append(segments, report)
	}
	merged, err := mergeBenchmarkReportSegments(12, segments...)
	if err != nil {
		t.Fatal(err)
	}
	merged.Limitations = append(merged.Limitations,
		"The corrected 12-run result combines non-overlapping scheduled orders 1-11 and 12. A suite-wide relay cost guard stopped the first segment before order 12; the continuation did not replay any completed order and used the same conditions.")
	output := strings.TrimSpace(os.Getenv("CODEGYM_AGENT_BENCHMARK_MERGED_REPORT"))
	if err := writeBenchmarkReport(output, merged); err != nil {
		t.Fatal(err)
	}
	t.Logf("merged report=%s runs=%d decision=%s reason=%s", output, len(merged.Runs), merged.Decision.Selected, merged.Decision.Reason)
}

type benchmarkOperationChecker struct{}

func (benchmarkOperationChecker) OperationActive(context.Context, string, string, string) (bool, error) {
	return true, nil
}

func benchmarkInt64Env(name string, fallback int64) int64 {
	value, err := strconv.ParseInt(strings.TrimSpace(os.Getenv(name)), 10, 64)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func benchmarkDurationEnv(name string, fallback time.Duration) time.Duration {
	value, err := time.ParseDuration(strings.TrimSpace(os.Getenv(name)))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
