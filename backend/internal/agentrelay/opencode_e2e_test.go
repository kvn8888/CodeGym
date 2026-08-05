package agentrelay

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/environment"
	"github.com/kvn8888/codegym/backend/internal/generation/openaicompat"
	"github.com/kvn8888/codegym/backend/internal/usage"
)

func TestOpenCodeThroughRelayToolRoundTrip(t *testing.T) {
	repoRoot := relayRepoRoot(t)
	opencodeBinary := filepath.Join(repoRoot, "spikes", "opencode", ".bin", "opencode")
	if info, err := os.Stat(opencodeBinary); err != nil || info.Mode()&0o111 == 0 {
		t.Skip("pinned opencode binary is absent; run spikes/opencode/download-opencode.sh")
	}
	timeoutBinary, err := exec.LookPath("timeout")
	if err != nil {
		t.Skip("GNU timeout is absent; install coreutils to run the opencode acceptance gate")
	}
	homeTemplate := filepath.Join(repoRoot, "spikes", "opencode", ".bin", "home-template", ".config", "opencode")
	if _, err := os.Stat(filepath.Join(homeTemplate, "node_modules", "@opencode-ai", "plugin")); err != nil {
		t.Fatalf("opencode binary exists but its pinned custom-tool runtime is missing: %v", err)
	}

	testRoot, err := os.MkdirTemp("/tmp", "codegym-opencode-relay-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(testRoot) })
	home := filepath.Join(testRoot, "home")
	workspaceDir := filepath.Join(testRoot, "workspace")
	configDir := filepath.Join(home, ".config", "opencode")
	workspaceConfig := filepath.Join(workspaceDir, ".opencode")
	for _, parent := range []string{configDir, workspaceConfig, workspaceDir} {
		if err := os.MkdirAll(parent, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	copyPinnedRuntime(t, homeTemplate, configDir)
	copyPinnedRuntime(t, homeTemplate, workspaceConfig)
	toolDir := filepath.Join(workspaceConfig, "tools")
	if err := os.MkdirAll(toolDir, 0o700); err != nil {
		t.Fatal(err)
	}
	toolSource := filepath.Join(repoRoot, "spikes", "opencode", ".opencode", "tools", "report_progress.ts")
	toolBytes, err := os.ReadFile(toolSource)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(toolDir, "report_progress.ts"), toolBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	gitInit := exec.Command("git", "init", "-q", workspaceDir)
	gitInit.Env = append(os.Environ(), "HOME="+home)
	if output, err := gitInit.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}

	fakeAzure := newOpenCodeFakeAzure(t)
	defer fakeAzure.server.Close()
	checker := &operationChecker{active: map[string]bool{"ws-e2e\x00user-e2e\x00op-e2e": true}}
	relayStore := NewInMemoryStore()
	tokenService, err := NewService(relayStore, ServiceConfig{
		Environment: environment.Dev,
		TokenSecret: testSecret, TokenTTL: 2 * time.Minute,
		DefaultMaxTotalTokens: 100_000, DefaultMaxCostUSDMicros: 1_000_000,
		DefaultMaxWallClock: 2 * time.Minute,
		Clock:               time.Now, OperationChecker: checker,
	})
	if err != nil {
		t.Fatal(err)
	}
	issued, err := tokenService.Issue(t.Context(), IssueInput{
		OperationID: "op-e2e", WorkspaceID: "ws-e2e", UserID: "user-e2e",
	})
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := openaicompat.New(openaicompat.Config{
		Name: "azure", BaseURL: fakeAzure.server.URL, APIKey: fakeAzure.apiKey,
		Model: "fake-azure-deployment", AuthStyle: openaicompat.AuthAzureAPIKey,
		APIVersion: "test-version", HTTPClient: http.DefaultClient,
	})
	if err != nil {
		t.Fatal(err)
	}
	usageStore := usage.NewInMemoryStore()
	relayHandler, err := NewHTTPHandler(tokenService, adapter, usage.NewService(usageStore, time.Now), "codegym-agent")
	if err != nil {
		t.Fatal(err)
	}
	relayMux := http.NewServeMux()
	relayMux.HandleFunc("GET /api/v1/agent-relay/v1/models", relayHandler.Models)
	relayMux.HandleFunc("POST /api/v1/agent-relay/v1/chat/completions", relayHandler.ChatCompletions)
	relayServer := httptest.NewServer(relayMux)
	defer relayServer.Close()

	configPath := filepath.Join(configDir, "opencode.json")
	config := map[string]any{
		"$schema": "https://opencode.ai/config.json", "autoupdate": false, "share": "disabled",
		"model": "codegym-relay/codegym-agent",
		"provider": map[string]any{"codegym-relay": map[string]any{
			"npm": "@ai-sdk/openai-compatible", "name": "CodeGym relay acceptance",
			"options": map[string]any{
				"baseURL": relayServer.URL + "/api/v1/agent-relay/v1", "apiKey": issued.Token,
			},
			"models": map[string]any{"codegym-agent": map[string]any{
				"name": "CodeGym agent", "limit": map[string]any{"context": 32768, "output": 4096},
			}},
		}},
		"permission": map[string]any{"*": "deny", "report_progress": "allow"},
	}
	encodedConfig, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, encodedConfig, 0o600); err != nil {
		t.Fatal(err)
	}
	toolLog := filepath.Join(testRoot, "tool-invocations.jsonl")
	if err := os.WriteFile(toolLog, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	commandContext, cancel := context.WithTimeout(t.Context(), 75*time.Second)
	defer cancel()
	command := exec.CommandContext(
		commandContext, timeoutBinary, "--signal=TERM", "--kill-after=5s", "60s",
		opencodeBinary, "run", "--format", "json", "--auto", "-m", "codegym-relay/codegym-agent",
		"Call the report_progress tool with step_id 'environment' and label 'Setting up', then reply DONE.",
	)
	command.Dir = workspaceDir
	command.Env = []string{
		"HOME=" + home, "PATH=" + os.Getenv("PATH"), "TMPDIR=/tmp", "OPENCODE_CONFIG=" + configPath,
		"OPENCODE_DISABLE_AUTOUPDATE=1", "OPENCODE_DISABLE_MODELS_FETCH=1",
		"OPENCODE_DISABLE_DEFAULT_PLUGINS=1", "OPENCODE_DISABLE_LSP_DOWNLOAD=1",
		"OPENCODE_DISABLE_CLAUDE_CODE=1", "OPENCODE_ENABLE_EXA=0", "OPENCODE_AUTO_SHARE=0",
		"OPENCODE_SPIKE_TOOL_LOG=" + toolLog,
	}
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("opencode acceptance run: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), `"type":"tool_use"`) || !strings.Contains(string(output), "DONE") {
		t.Fatalf("opencode output omitted tool completion or final text:\n%s", output)
	}
	toolInvocation, err := os.ReadFile(toolLog)
	if err != nil {
		t.Fatal(err)
	}
	var progress struct {
		StepID string `json:"step_id"`
		Label  string `json:"label"`
	}
	lines := strings.Split(strings.TrimSpace(string(toolInvocation)), "\n")
	if len(lines) != 1 || json.Unmarshal([]byte(lines[0]), &progress) != nil {
		t.Fatalf("tool invocation log = %q", toolInvocation)
	}
	if progress.StepID != "environment" || progress.Label != "Setting up" {
		t.Fatalf("tool invocation = %#v", progress)
	}
	fakeAzure.assertComplete(t)

	records, err := usageStore.List(t.Context(), "ws-e2e", "user-e2e")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) < 3 {
		t.Fatalf("relay usage records = %#v, want title + tool + final calls", records)
	}
	for _, record := range records {
		if record.OperationID != "op-e2e" || record.TotalTokens <= 0 || record.CostUSDMicros <= 0 {
			t.Fatalf("unscoped or unpriced relay usage = %#v", record)
		}
	}
	budget, err := relayStore.Get(t.Context(), "ws-e2e", "user-e2e", "op-e2e")
	if err != nil {
		t.Fatal(err)
	}
	if budget.UsedTotalTokens <= 0 || budget.UsedCostUSDMicros <= 0 {
		t.Fatalf("operation budget was not accumulated: %#v", budget)
	}
}

type openCodeFakeAzure struct {
	server         *httptest.Server
	apiKey         string
	mu             sync.Mutex
	requests       int
	sawToolRequest bool
	sawToolResult  bool
}

func newOpenCodeFakeAzure(t *testing.T) *openCodeFakeAzure {
	t.Helper()
	fake := &openCodeFakeAzure{apiKey: "fake-azure-provider-secret"}
	fake.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" || r.URL.Query().Get("api-version") != "test-version" {
			t.Errorf("fake Azure URL = %s", r.URL.String())
			http.Error(w, "wrong URL", http.StatusNotFound)
			return
		}
		if r.Header.Get("api-key") != fake.apiKey {
			t.Errorf("fake Azure api-key = %q", r.Header.Get("api-key"))
			http.Error(w, "wrong credential", http.StatusUnauthorized)
			return
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode fake Azure request: %v", err)
			http.Error(w, "bad JSON", http.StatusBadRequest)
			return
		}
		if request["model"] != "fake-azure-deployment" || request["stream"] != true {
			t.Errorf("fake Azure request routing = %#v", request)
		}
		streamOptions, _ := request["stream_options"].(map[string]any)
		if streamOptions["include_usage"] != true {
			t.Errorf("stream_options = %#v", streamOptions)
		}
		hasProgressTool := requestHasProgressTool(request)
		hasToolResult := requestHasToolResult(request, "call_opencode_e2e_progress")
		fake.mu.Lock()
		fake.requests++
		fake.sawToolRequest = fake.sawToolRequest || hasProgressTool
		fake.sawToolResult = fake.sawToolResult || hasToolResult
		fake.mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		switch {
		case hasToolResult:
			writeOpenCodeTextStream(w, "DONE — the relay tool round trip completed.", 53, 3)
		case hasProgressTool:
			writeOpenCodeToolStream(w)
		default:
			writeOpenCodeTextStream(w, "Relay acceptance", 5, 2)
		}
	}))
	return fake
}

func (f *openCodeFakeAzure) assertComplete(t *testing.T) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.requests < 3 || !f.sawToolRequest || !f.sawToolResult {
		t.Fatalf("fake Azure requests=%d tool_request=%t tool_result=%t", f.requests, f.sawToolRequest, f.sawToolResult)
	}
}

func requestHasProgressTool(request map[string]any) bool {
	tools, _ := request["tools"].([]any)
	for _, raw := range tools {
		tool, _ := raw.(map[string]any)
		function, _ := tool["function"].(map[string]any)
		if function["name"] == "report_progress" {
			return true
		}
	}
	return false
}

func requestHasToolResult(request map[string]any, expectedID string) bool {
	messages, _ := request["messages"].([]any)
	for _, raw := range messages {
		message, _ := raw.(map[string]any)
		if message["role"] == "tool" && message["tool_call_id"] == expectedID && message["content"] != nil {
			return true
		}
	}
	return false
}

func writeOpenCodeToolStream(w http.ResponseWriter) {
	writeOpenCodeSSE(w, map[string]any{
		"id": "chatcmpl-opencode-e2e", "object": "chat.completion.chunk", "created": time.Now().Unix(),
		"model": "fake-azure-deployment", "choices": []any{map[string]any{
			"index": 0, "finish_reason": nil, "delta": map[string]any{
				"role": "assistant", "content": nil, "tool_calls": []any{map[string]any{
					"index": 0, "id": "call_opencode_e2e_progress", "type": "function",
					"function": map[string]any{"name": "report_progress", "arguments": `{"step_id":"environment","label":"Setting up"}`},
				}},
			},
		}},
	})
	writeOpenCodeSSE(w, map[string]any{
		"id": "chatcmpl-opencode-e2e", "object": "chat.completion.chunk", "created": time.Now().Unix(),
		"model": "fake-azure-deployment", "choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{}, "finish_reason": "tool_calls",
		}},
		"usage": map[string]any{"prompt_tokens": 37, "completion_tokens": 11, "total_tokens": 48},
	})
	_, _ = io.WriteString(w, "data: [DONE]\n\n")
}

func writeOpenCodeTextStream(w http.ResponseWriter, content string, inputTokens, outputTokens int) {
	writeOpenCodeSSE(w, map[string]any{
		"id": "chatcmpl-opencode-e2e", "object": "chat.completion.chunk", "created": time.Now().Unix(),
		"model": "fake-azure-deployment", "choices": []any{map[string]any{
			"index": 0, "finish_reason": nil, "delta": map[string]any{"role": "assistant", "content": content},
		}},
	})
	writeOpenCodeSSE(w, map[string]any{
		"id": "chatcmpl-opencode-e2e", "object": "chat.completion.chunk", "created": time.Now().Unix(),
		"model": "fake-azure-deployment", "choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{}, "finish_reason": "stop",
		}},
		"usage": map[string]any{
			"prompt_tokens": inputTokens, "completion_tokens": outputTokens,
			"total_tokens": inputTokens + outputTokens,
		},
	})
	_, _ = io.WriteString(w, "data: [DONE]\n\n")
}

func writeOpenCodeSSE(w http.ResponseWriter, value any) {
	encoded, _ := json.Marshal(value)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", encoded)
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func relayRepoRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve relay test source path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
}

func copyPinnedRuntime(t *testing.T, source, destination string) {
	t.Helper()
	command := exec.Command("cp", "-R", source+string(os.PathSeparator)+".", destination)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("copy pinned opencode runtime: %v: %s", err, output)
	}
}
