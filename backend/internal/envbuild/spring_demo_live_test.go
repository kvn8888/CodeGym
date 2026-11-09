package envbuild

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/agentrelay"
	"github.com/kvn8888/codegym/backend/internal/agentruntime"
	"github.com/kvn8888/codegym/backend/internal/config"
	"github.com/kvn8888/codegym/backend/internal/generation/openaicompat"
	"github.com/kvn8888/codegym/backend/internal/usage"
)

// TestSpringBootColdBuildDemo runs real cold preparation for a Spring Boot
// dependency-injection exercise through the primary adapter. It gates on
// CODEGYM_RUN_SPRING_DEMO=1, requires the pinned opencode binary with its
// Node tool runtime, a JDK plus Maven in the execution environment, and
// CODEGYM_ALLOW_UNCACHED_SETUP=1 acknowledging live download time and cost.
// It records a build record (manifest, telemetry, file list); the verifier
// verdict and promotion follow human review of the generated exercise with
// reviewed assessment inputs, so this run asserts completion and drafting,
// never readiness.
func TestSpringBootColdBuildDemo(t *testing.T) {
	if os.Getenv("CODEGYM_RUN_SPRING_DEMO") != "1" {
		t.Skip("set CODEGYM_RUN_SPRING_DEMO=1 under Doppler to run the live Spring Boot cold build")
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
	relayStore := agentrelay.NewInMemoryStore()
	tokenService, err := agentrelay.NewService(relayStore, agentrelay.ServiceConfig{
		TokenSecret: strings.Repeat("spring-demo-secret-", 3), TokenTTL: 60 * time.Minute,
		DefaultMaxTotalTokens: 1_000_000, DefaultMaxCostUSDMicros: 10_000_000,
		DefaultMaxWallClock: 60 * time.Minute, OperationChecker: demoOperationChecker{},
	})
	if err != nil {
		t.Fatal(err)
	}
	issued, err := tokenService.Issue(t.Context(), agentrelay.IssueInput{
		OperationID: "spring-cold-demo", WorkspaceID: "demo-workspace", UserID: "demo-user",
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

	binary, template := openCodePaths(t)
	agent, err := agentruntime.NewOpenCodeRuntime(agentruntime.OpenCodeConfig{
		BinaryPath: binary, RuntimeTemplate: template, RelayBaseURL: relayServer.URL + "/v1",
		RelayToken: issued.Token, Model: cfg.Relay.PublicModel,
	})
	if err != nil {
		t.Fatal(err)
	}
	preparer := &Preparer{
		Runtime:     agent,
		Provisioner: HostProvisioner{},
		TurnCeiling: 32,
		Timeout:     40 * time.Minute,
	}
	result, err := preparer.Prepare(t.Context(), BuildRequest{
		Technology: "Spring Boot",
		Objective:  "practice dependency injection with services",
	})
	if err != nil {
		t.Fatalf("cold preparation error: %v; diagnostics: %#v", err, result.Diagnostics)
	}
	if result.ProposedManifest == nil {
		t.Fatalf("no manifest drafted; diagnostics: %#v", result.Diagnostics)
	}
	if len(result.ProposedManifest.Workspace.LearnerEditable) == 0 {
		t.Fatalf("no learner files produced: %#v", result.ProposedManifest.Workspace)
	}
	t.Logf("cold build complete: artifact=%s editable=%d protected=%d commands=%+v telemetry=%+v",
		result.ProposedManifest.Artifact.ID,
		len(result.ProposedManifest.Workspace.LearnerEditable),
		len(result.ProposedManifest.Workspace.Protected),
		result.ProposedManifest.Commands, result.Telemetry)
	record := map[string]any{
		"technology":  "Spring Boot",
		"objective":   "practice dependency injection with services",
		"manifest":    result.ProposedManifest,
		"telemetry":   result.Telemetry,
		"diagnostics": result.Diagnostics,
	}
	payload, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	reportPath := strings.TrimSpace(os.Getenv("CODEGYM_SPRING_DEMO_REPORT"))
	if reportPath == "" {
		reportPath = filepath.Join("testdata", "spring-cold-build.json")
	}
	if dir := filepath.Dir(reportPath); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(reportPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Logf("cold build record=%s", reportPath)
	if os.Getenv("CODEGYM_SPRING_DEMO_KEEP_WORKSPACE") == "1" {
		t.Logf("kept workspace for inspection: %s", result.Workspace.Root)
		return
	}
	if err := preparer.Provisioner.Destroy(t.Context(), result.Workspace); err != nil {
		t.Fatal(err)
	}
}

type demoOperationChecker struct{}

func (demoOperationChecker) OperationActive(context.Context, string, string, string) (bool, error) {
	return true, nil
}

// openCodePaths resolves the pinned binary and tool runtime, preferring
// explicit environment overrides so demos are not coupled to checkout
// layout. It skips cleanly when the binary is absent.
func openCodePaths(t *testing.T) (string, string) {
	t.Helper()
	if binary := strings.TrimSpace(os.Getenv("CODEGYM_OPENCODE_BINARY")); binary != "" {
		template := strings.TrimSpace(os.Getenv("CODEGYM_OPENCODE_TEMPLATE"))
		if template == "" {
			t.Fatal("CODEGYM_OPENCODE_TEMPLATE is required with CODEGYM_OPENCODE_BINARY")
		}
		return binary, template
	}
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve demo test path")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
	binary := filepath.Join(repoRoot, "spikes", "opencode", ".bin", "opencode")
	template := filepath.Join(repoRoot, "spikes", "opencode", ".bin", "home-template", ".config", "opencode")
	if info, err := os.Stat(binary); err != nil || info.Mode()&0o111 == 0 {
		t.Skip("pinned opencode binary is absent; run spikes/opencode/download-opencode.sh")
	}
	return binary, template
}
