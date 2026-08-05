package agentruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestOpenCodeRuntimeFakeRelay(t *testing.T) {
	binary, template := pinnedOpenCodePaths(t)
	workingDirectory := t.TempDir()
	fake := newOpenCodeRuntimeRelay(t, func(_ string) string {
		return `mkdir -p .codegym && printf '%s' '{"version":1,"completed":true,"summary":"claimed success","artifacts":[]}' > .codegym/agent-result.json`
	})
	defer fake.Close()
	var delivered []ProgressEvent
	runtimeAdapter, err := NewOpenCodeRuntime(OpenCodeConfig{
		BinaryPath: binary, RuntimeTemplate: template, RelayBaseURL: fake.URL + "/v1",
		RelayToken: "fake-operation-token", Model: "codegym-agent",
		Progress: func(_ context.Context, event ProgressEvent) error {
			delivered = append(delivered, event)
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := runtimeAdapter.Run(t.Context(), TaskSpec{
		Goal: "Exercise progress and shell continuation.", WorkingDirectory: workingDirectory,
		AllowedTools: []Tool{ToolShell, ToolReportProgress}, TurnCeiling: 5,
		Deadline: time.Now().Add(30 * time.Second), OutputCapBytes: 256 * 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Telemetry.RuntimeVersion != OpenCodePinnedVersion || result.Telemetry.ExitCode == nil || *result.Telemetry.ExitCode != 0 {
		t.Fatalf("telemetry = %#v", result.Telemetry)
	}
	if result.Manifest.Status != ManifestPresent || result.Manifest.Manifest == nil || !result.Manifest.Manifest.Completed {
		t.Fatalf("manifest = %#v; telemetry=%#v; prose=%q", result.Manifest, result.Telemetry, result.Prose)
	}
	if len(delivered) != 1 || delivered[0].StepID != "environment" || delivered[0].Label != "Scaffolding" {
		t.Fatalf("delivered progress = %#v", delivered)
	}
	requests, progressContinuation, shellContinuation, unexpectedTools, _ := fake.Snapshot()
	if requests < 3 || !progressContinuation || !shellContinuation || len(unexpectedTools) != 0 {
		t.Fatalf("requests=%d progress=%t shell=%t unexpected_tools=%v", requests, progressContinuation, shellContinuation, unexpectedTools)
	}

	verification, err := Evaluate(t.Context(), VerifierFunc(func(context.Context, TaskSpec, RunResult) (Verification, error) {
		return Verification{Passed: false, Detail: "black-box verifier rejected the implementation"}, nil
	}), TaskSpec{}, result)
	if err != nil {
		t.Fatal(err)
	}
	if verification.Passed {
		t.Fatal("opencode exit zero or manifest claim overrode semantic verification")
	}
}

func TestOpenCodeRuntimeManifestDiagnostic(t *testing.T) {
	binary, template := pinnedOpenCodePaths(t)
	fake := newOpenCodeRuntimeRelay(t, func(string) string {
		return `mkdir -p .codegym && printf '%s' '{"version":1,"completed":' > .codegym/agent-result.json`
	})
	defer fake.Close()
	runtimeAdapter, err := NewOpenCodeRuntime(OpenCodeConfig{
		BinaryPath: binary, RuntimeTemplate: template, RelayBaseURL: fake.URL + "/v1",
		RelayToken: "fake-operation-token", Model: "codegym-agent",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := runtimeAdapter.Run(t.Context(), TaskSpec{
		Goal: "write a completion claim", WorkingDirectory: t.TempDir(),
		AllowedTools: []Tool{ToolShell, ToolReportProgress}, TurnCeiling: 5,
		Deadline: time.Now().Add(30 * time.Second), OutputCapBytes: 256 * 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Manifest.Status != ManifestMalformed || !strings.Contains(result.Manifest.Error, "decode result manifest") {
		t.Fatalf("manifest diagnostic = %#v; telemetry=%#v; prose=%q", result.Manifest, result.Telemetry, result.Prose)
	}
	verification, err := (FixtureVerifier{}).Verify(t.Context(), TaskSpec{}, result)
	if err != nil {
		t.Fatal(err)
	}
	if verification.Passed || !strings.Contains(verification.Detail, result.Manifest.Error) {
		t.Fatalf("verification = %#v", verification)
	}
}

func TestOpenCodeRuntimeClassifiesStepExhaustionWithProse(t *testing.T) {
	binary, template := pinnedOpenCodePaths(t)
	fake := newOpenCodeRuntimeRelay(t, func(string) string { return `true` })
	defer fake.Close()
	runtimeAdapter, err := NewOpenCodeRuntime(OpenCodeConfig{
		BinaryPath: binary, RuntimeTemplate: template, RelayBaseURL: fake.URL + "/v1",
		RelayToken: "fake-operation-token", Model: "codegym-agent",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := runtimeAdapter.Run(t.Context(), TaskSpec{
		Goal: "use every step without writing a manifest", WorkingDirectory: t.TempDir(),
		AllowedTools: []Tool{ToolShell, ToolReportProgress}, TurnCeiling: 3,
		Deadline: time.Now().Add(30 * time.Second), OutputCapBytes: 256 * 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Telemetry.Turns != 3 || result.Telemetry.Termination != TerminationTurnCeiling || strings.TrimSpace(result.Prose) == "" {
		t.Fatalf("result = %#v", result)
	}
}

func TestOpenCodeConfigAllowsOnlyIsolatedToolOutputExternally(t *testing.T) {
	runtimeAdapter := &OpenCodeRuntime{config: OpenCodeConfig{Model: "codegym-agent"}}
	payload, err := runtimeAdapter.buildConfig(TaskSpec{AllowedTools: []Tool{ToolReadFile, ToolWriteFile, ToolShell}})
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Permission map[string]any `json:"permission"`
		Agent      map[string]struct {
			Permission map[string]any `json:"permission"`
		} `json:"agent"`
	}
	if err := json.Unmarshal(payload, &config); err != nil {
		t.Fatal(err)
	}
	for name, permissions := range map[string]map[string]any{"top-level": config.Permission, "agent": config.Agent["codegym"].Permission} {
		external, ok := permissions["external_directory"].(map[string]any)
		if !ok || external["*"] != "deny" || external["~/.local/share/opencode/tool-output/*"] != "allow" {
			t.Fatalf("%s permissions = %#v", name, permissions)
		}
	}
}

func TestOpenCodeRuntimeUsesFreshHomeEveryRun(t *testing.T) {
	binary, template := pinnedOpenCodePaths(t)
	fake := newOpenCodeRuntimeRelay(t, func(goal string) string {
		manifest := `mkdir -p .codegym && printf '%s' '{"version":1,"completed":true,"summary":"state checked","artifacts":[]}' > .codegym/agent-result.json`
		if strings.Contains(goal, "seed isolated state") {
			return `printf CANARY > "$HOME/previous-run" && ` + manifest
		}
		return `test ! -e "$HOME/previous-run" && ` + manifest
	})
	defer fake.Close()
	runtimeAdapter, err := NewOpenCodeRuntime(OpenCodeConfig{
		BinaryPath: binary, RuntimeTemplate: template, RelayBaseURL: fake.URL + "/v1",
		RelayToken: "fake-operation-token", Model: "codegym-agent",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, goal := range []string{"seed isolated state", "check isolated state"} {
		result, runErr := runtimeAdapter.Run(t.Context(), TaskSpec{
			Goal: goal, WorkingDirectory: t.TempDir(),
			AllowedTools: []Tool{ToolShell, ToolReportProgress}, TurnCeiling: 5,
			Deadline: time.Now().Add(30 * time.Second), OutputCapBytes: 256 * 1024,
		})
		if runErr != nil {
			t.Fatalf("%s: %v", goal, runErr)
		}
		if result.Manifest.Status != ManifestPresent {
			t.Fatalf("%s manifest = %#v; tools = %#v; prose=%q", goal, result.Manifest, result.Telemetry.ToolInvocations, result.Prose)
		}
		for _, invocation := range result.Telemetry.ToolInvocations {
			if invocation.Tool == ToolShell && invocation.ExitCode != nil && *invocation.ExitCode != 0 {
				t.Fatalf("%s shell exit = %d", goal, *invocation.ExitCode)
			}
		}
	}
	_, _, _, _, sessionIDs := fake.Snapshot()
	if len(sessionIDs) != 2 {
		t.Fatalf("session IDs = %#v, want two fresh sessions", sessionIDs)
	}
}

func TestOpenCodeRuntimeBoundedTermination(t *testing.T) {
	binary, template := pinnedOpenCodePaths(t)
	workingDirectory := t.TempDir()
	fake := newOpenCodeRuntimeRelay(t, func(string) string {
		return `sleep 30 & child=$!; echo $child > child.pid; wait`
	})
	defer fake.Close()
	runtimeAdapter, err := NewOpenCodeRuntime(OpenCodeConfig{
		BinaryPath: binary, RuntimeTemplate: template, RelayBaseURL: fake.URL + "/v1",
		RelayToken: "fake-operation-token", Model: "codegym-agent",
	})
	if err != nil {
		t.Fatal(err)
	}
	startedAt := time.Now()
	result, err := runtimeAdapter.Run(t.Context(), TaskSpec{
		Goal: "run until bounded", WorkingDirectory: workingDirectory,
		AllowedTools: []Tool{ToolShell, ToolReportProgress}, TurnCeiling: 5,
		Deadline: time.Now().Add(1500 * time.Millisecond), OutputCapBytes: 256 * 1024,
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run error = %v", err)
	}
	if result.Telemetry.Termination != TerminationDeadline || time.Since(startedAt) > 6*time.Second {
		t.Fatalf("bounded result = %#v elapsed=%s", result, time.Since(startedAt))
	}
	assertRecordedProcessGoneIfPresent(t, filepath.Join(workingDirectory, "child.pid"))
	requests, _, shellContinuation, _, _ := fake.Snapshot()
	if requests > 2 || shellContinuation {
		t.Fatalf("relay continued after deadline: requests=%d shell_continuation=%t", requests, shellContinuation)
	}
}

type openCodeRuntimeRelay struct {
	*httptest.Server
	t                 *testing.T
	commandForGoal    func(string) string
	mu                sync.Mutex
	requests          int
	progressContinued bool
	shellContinued    bool
	unexpectedTools   []string
	sessionIDs        map[string]struct{}
}

func newOpenCodeRuntimeRelay(t *testing.T, commandForGoal func(string) string) *openCodeRuntimeRelay {
	t.Helper()
	fake := &openCodeRuntimeRelay{t: t, commandForGoal: commandForGoal, sessionIDs: map[string]struct{}{}}
	fake.Server = httptest.NewServer(http.HandlerFunc(fake.handle))
	return fake
}

func (f *openCodeRuntimeRelay) handle(w http.ResponseWriter, request *http.Request) {
	if request.URL.Path != "/v1/chat/completions" || request.Header.Get("Authorization") != "Bearer fake-operation-token" {
		f.t.Errorf("unexpected opencode relay request %s auth=%q", request.URL.Path, request.Header.Get("Authorization"))
		http.Error(w, "unexpected request", http.StatusBadRequest)
		return
	}
	var body struct {
		Messages []ChatMessage `json:"messages"`
		Tools    []struct {
			Function ToolDefinitionFunction `json:"function"`
		} `json:"tools"`
	}
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		f.t.Errorf("decode opencode relay body: %v", err)
		http.Error(w, "bad JSON", http.StatusBadRequest)
		return
	}
	goal := openCodeGoal(body.Messages)
	hasProgress := messageHasToolResult(body.Messages, "call-runtime-progress")
	hasShell := messageHasToolResult(body.Messages, "call-runtime-shell")
	unexpectedTools := make([]string, 0)
	for _, tool := range body.Tools {
		if tool.Function.Name != "bash" && tool.Function.Name != "report_progress" {
			unexpectedTools = append(unexpectedTools, tool.Function.Name)
		}
	}
	f.mu.Lock()
	f.requests++
	f.progressContinued = f.progressContinued || hasProgress
	f.shellContinued = f.shellContinued || hasShell
	f.unexpectedTools = append(f.unexpectedTools, unexpectedTools...)
	if sessionID := request.Header.Get("X-Session-Id"); sessionID != "" {
		f.sessionIDs[sessionID] = struct{}{}
	}
	f.mu.Unlock()

	w.Header().Set("Content-Type", "text/event-stream")
	switch {
	case hasShell:
		writeRuntimeTextStream(w, "DONE", 11, 2)
	case hasProgress:
		writeRuntimeToolStream(w, "call-runtime-shell", "bash", map[string]any{"command": f.commandForGoal(goal)})
	default:
		writeRuntimeToolStream(w, "call-runtime-progress", "report_progress", map[string]any{"step_id": "environment", "label": "Scaffolding"})
	}
}

func (f *openCodeRuntimeRelay) Snapshot() (int, bool, bool, []string, []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	sessions := make([]string, 0, len(f.sessionIDs))
	for sessionID := range f.sessionIDs {
		sessions = append(sessions, sessionID)
	}
	return f.requests, f.progressContinued, f.shellContinued, append([]string(nil), f.unexpectedTools...), sessions
}

func openCodeGoal(messages []ChatMessage) string {
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index].Role == "user" {
			return messages[index].Content
		}
	}
	return ""
}

func messageHasToolResult(messages []ChatMessage, id string) bool {
	for _, message := range messages {
		if message.Role == "tool" && message.ToolCallID == id {
			return true
		}
	}
	return false
}

func writeRuntimeToolStream(w http.ResponseWriter, id, name string, arguments map[string]any) {
	payload, _ := json.Marshal(arguments)
	writeRuntimeSSE(w, map[string]any{
		"id": "chatcmpl-runtime", "object": "chat.completion.chunk", "created": time.Now().Unix(), "model": "codegym-agent",
		"choices": []any{map[string]any{"index": 0, "finish_reason": nil, "delta": map[string]any{
			"role": "assistant", "content": nil, "tool_calls": []any{map[string]any{
				"index": 0, "id": id, "type": "function", "function": map[string]any{"name": name, "arguments": string(payload)},
			}},
		}}},
	})
	writeRuntimeSSE(w, map[string]any{
		"id": "chatcmpl-runtime", "object": "chat.completion.chunk", "created": time.Now().Unix(), "model": "codegym-agent",
		"choices": []any{map[string]any{"index": 0, "finish_reason": "tool_calls", "delta": map[string]any{}}},
		"usage":   map[string]any{"prompt_tokens": 13, "completion_tokens": 7, "total_tokens": 20},
	})
	_, _ = io.WriteString(w, "data: [DONE]\n\n")
}

func writeRuntimeTextStream(w http.ResponseWriter, content string, inputTokens, outputTokens int) {
	writeRuntimeSSE(w, map[string]any{
		"id": "chatcmpl-runtime", "object": "chat.completion.chunk", "created": time.Now().Unix(), "model": "codegym-agent",
		"choices": []any{map[string]any{"index": 0, "finish_reason": nil, "delta": map[string]any{"role": "assistant", "content": content}}},
	})
	writeRuntimeSSE(w, map[string]any{
		"id": "chatcmpl-runtime", "object": "chat.completion.chunk", "created": time.Now().Unix(), "model": "codegym-agent",
		"choices": []any{map[string]any{"index": 0, "finish_reason": "stop", "delta": map[string]any{}}},
		"usage":   map[string]any{"prompt_tokens": inputTokens, "completion_tokens": outputTokens, "total_tokens": inputTokens + outputTokens},
	})
	_, _ = io.WriteString(w, "data: [DONE]\n\n")
}

func writeRuntimeSSE(w http.ResponseWriter, value any) {
	payload, _ := json.Marshal(value)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func pinnedOpenCodePaths(t *testing.T) (string, string) {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve opencode test path")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
	binary := filepath.Join(repoRoot, "spikes", "opencode", ".bin", "opencode")
	template := filepath.Join(repoRoot, "spikes", "opencode", ".bin", "home-template", ".config", "opencode")
	if info, err := os.Stat(binary); err != nil || info.Mode()&0o111 == 0 {
		t.Skip("pinned opencode binary is absent; run spikes/opencode/download-opencode.sh")
	}
	if _, err := os.Stat(filepath.Join(template, "node_modules", "@opencode-ai", "plugin")); err != nil {
		t.Skip("pinned opencode custom-tool runtime is absent; run spikes/opencode/download-opencode.sh")
	}
	return binary, template
}

func assertRecordedProcessGoneIfPresent(t *testing.T, path string) {
	t.Helper()
	payload, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(payload)))
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		err = syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("opencode child process %d survived process-group cancellation", pid)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
