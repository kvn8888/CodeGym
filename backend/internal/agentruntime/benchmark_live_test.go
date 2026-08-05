package agentruntime

import (
	"context"
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
	reportPath := strings.TrimSpace(os.Getenv("CODEGYM_AGENT_BENCHMARK_REPORT"))
	if reportPath == "" {
		reportPath = filepath.Join("internal", "agentruntime", "testdata", "benchmark-report.json")
	}
	report, err := RunBenchmark(t.Context(), BenchmarkConfig{
		PurposeBuilt: &PurposeBuiltRuntime{Client: client}, OpenCode: openCode,
		Fixtures: BenchmarkFixtures(), Repetitions: 3, TurnCeiling: DefaultBenchmarkTurnCeiling, RunDeadline: runDeadline,
		OutputCapBytes: 256 * 1024, WorkingRoot: "/private/tmp", ReportPath: reportPath,
		ModelDeployment:      provider.Name + "/" + provider.Model,
		ExecutionEnvironment: "host-temporary-workspace",
		Limitations: []string{
			"No Daytona sandboxes were created; filesystem-escape policy was observed through confined tools and output scanning, not an OS sandbox boundary.",
			"All candidates failed the result-manifest contract before black-box build and HTTP verification could award a pass.",
			"The offline Express verifier uses a controlled local Express API contract module plus proxy and source-level egress controls; a real npm dependency and OS network boundary require a pre-baked Daytona snapshot.",
		},
		RelayMaxTokens: maxTokens, RelayMaxCostUSDMicros: maxCostMicros, RelayWallClock: relayWallClock,
		RetryPolicy: "zero retries; every failure counts", MaxSandboxes: DefaultBenchmarkMaxSandboxes,
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
		SandboxMeter: func() (int, int) { return 0, 0 },
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Status == "complete" && len(report.Runs) != 12 {
		t.Fatalf("complete report has %d runs, want 12", len(report.Runs))
	}
	if report.SandboxesCreated != report.SandboxesDeleted || report.WorkspacesCreated != report.WorkspacesDeleted {
		t.Fatalf("resources leaked: sandboxes %d/%d workspaces %d/%d",
			report.SandboxesCreated, report.SandboxesDeleted, report.WorkspacesCreated, report.WorkspacesDeleted)
	}
	t.Logf("benchmark report=%s status=%s runs=%d decision=%s reason=%s", reportPath, report.Status, len(report.Runs), report.Decision.Selected, report.Decision.Reason)
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
