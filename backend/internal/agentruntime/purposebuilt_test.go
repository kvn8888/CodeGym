package agentruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestPurposeBuiltRuntimeFakeRelayRepairsBuild(t *testing.T) {
	workingDirectory := t.TempDir()
	writeTestFile(t, workingDirectory, "go.mod", "module fixture\n\ngo 1.25.4\n")
	writeTestFile(t, workingDirectory, "main_test.go", `package fixture

import "testing"

func TestValue(t *testing.T) {
	if Value() != "ok" { t.Fatalf("Value() = %q", Value()) }
}
`)
	steps := []fakeRelayStep{
		toolStep("write-1", ToolWriteFile, map[string]any{"path": "main.go", "content": "package fixture\n\nfunc Value() string { return \"broken\" }\n"}),
		toolStep("build-1", ToolShell, map[string]any{"command": "go test ./..."}),
		toolStep("write-2", ToolWriteFile, map[string]any{"path": "main.go", "content": "package fixture\n\nfunc Value() string { return \"ok\" }\n"}),
		toolStep("progress-1", ToolReportProgress, map[string]any{"step_id": "environment", "label": "Fixed failing build"}),
		toolStep("build-2", ToolShell, map[string]any{"command": "go test ./..."}),
		toolStep("manifest-1", ToolWriteFile, map[string]any{
			"path":    ManifestRelativePath,
			"content": `{"version":1,"completed":true,"summary":"fixture passes","artifacts":["main.go"],"checks":[{"name":"go test","passed":true}]}`,
		}),
		{content: "DONE"},
	}
	relay := newFakeRelay(t, steps)
	defer relay.Close()
	client := newTestRelayClient(t, relay.URL)
	var progress []ProgressEvent
	runtime := &PurposeBuiltRuntime{
		Client: client,
		ReportProgress: func(_ context.Context, event ProgressEvent) error {
			progress = append(progress, event)
			return nil
		},
	}
	result, err := runtime.Run(t.Context(), TaskSpec{
		Goal: "Implement Value so the checked-in test passes.", WorkingDirectory: workingDirectory,
		AllowedTools: []Tool{ToolReadFile, ToolWriteFile, ToolShell, ToolReportProgress},
		TurnCeiling:  8, Deadline: time.Now().Add(20 * time.Second), OutputCapBytes: 16 * 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Telemetry.Termination != TerminationCompleted || result.Telemetry.Turns != 7 {
		t.Fatalf("telemetry = %#v", result.Telemetry)
	}
	if result.Telemetry.RepairIterations != 1 {
		t.Fatalf("repair iterations = %d", result.Telemetry.RepairIterations)
	}
	if result.Telemetry.Tokens.Total != 35 || result.Telemetry.Tokens.Input != 21 || result.Telemetry.Tokens.Output != 14 {
		t.Fatalf("token telemetry = %#v", result.Telemetry.Tokens)
	}
	if len(progress) != 1 || progress[0].Label != "Fixed failing build" {
		t.Fatalf("progress = %#v", progress)
	}
	if result.Manifest.Status != ManifestPresent || result.Manifest.Manifest == nil || !result.Manifest.Manifest.Completed {
		t.Fatalf("manifest claim = %#v", result.Manifest)
	}
	content, err := os.ReadFile(filepath.Join(workingDirectory, "main.go"))
	if err != nil || !strings.Contains(string(content), `return "ok"`) {
		t.Fatalf("final main.go = %q, %v", content, err)
	}
	if relay.Requests() != 7 {
		t.Fatalf("relay requests = %d", relay.Requests())
	}
}

func TestPurposeBuiltRuntimeManifestDiagnostic(t *testing.T) {
	workingDirectory := t.TempDir()
	relay := newFakeRelay(t, []fakeRelayStep{
		toolStep("manifest", ToolWriteFile, map[string]any{"path": ManifestRelativePath, "content": `{"version":1,"completed":`}),
		{content: "DONE"},
	})
	defer relay.Close()
	runtimeAdapter := &PurposeBuiltRuntime{Client: newTestRelayClient(t, relay.URL)}
	result, err := runtimeAdapter.Run(t.Context(), TaskSpec{
		Goal: "write a completion claim", WorkingDirectory: workingDirectory,
		AllowedTools: []Tool{ToolWriteFile}, TurnCeiling: 3,
		Deadline: time.Now().Add(10 * time.Second), OutputCapBytes: DefaultOutputCap,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Manifest.Status != ManifestMalformed || !strings.Contains(result.Manifest.Error, "decode result manifest") {
		t.Fatalf("manifest diagnostic = %#v", result.Manifest)
	}
	verification, err := (FixtureVerifier{}).Verify(t.Context(), TaskSpec{}, result)
	if err != nil {
		t.Fatal(err)
	}
	if verification.Passed || !strings.Contains(verification.Detail, result.Manifest.Error) {
		t.Fatalf("verification = %#v", verification)
	}
}

func TestPurposeBuiltRuntimeCeilingsStopToolDispatch(t *testing.T) {
	t.Run("turn ceiling", func(t *testing.T) {
		workingDirectory := t.TempDir()
		relay := newFakeRelay(t, []fakeRelayStep{
			toolStep("late-write", ToolWriteFile, map[string]any{"path": "must-not-exist", "content": "bad"}),
		})
		defer relay.Close()
		runtime := &PurposeBuiltRuntime{Client: newTestRelayClient(t, relay.URL)}
		result, err := runtime.Run(t.Context(), TaskSpec{
			Goal: "test", WorkingDirectory: workingDirectory, AllowedTools: []Tool{ToolWriteFile},
			TurnCeiling: 1, Deadline: time.Now().Add(time.Second),
		})
		if err != nil {
			t.Fatal(err)
		}
		if result.Telemetry.Termination != TerminationTurnCeiling || len(result.Telemetry.ToolInvocations) != 0 {
			t.Fatalf("telemetry = %#v", result.Telemetry)
		}
		if _, err := os.Stat(filepath.Join(workingDirectory, "must-not-exist")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("tool executed after turn ceiling: %v", err)
		}
	})

	t.Run("wall clock kills process group", func(t *testing.T) {
		workingDirectory := t.TempDir()
		relay := newFakeRelay(t, []fakeRelayStep{
			toolStep("runaway", ToolShell, map[string]any{"command": "sleep 30 & child=$!; echo $child > child.pid; wait"}),
			toolStep("late-write", ToolWriteFile, map[string]any{"path": "must-not-exist", "content": "bad"}),
		})
		defer relay.Close()
		runtime := &PurposeBuiltRuntime{Client: newTestRelayClient(t, relay.URL)}
		result, err := runtime.Run(t.Context(), TaskSpec{
			Goal: "test", WorkingDirectory: workingDirectory, AllowedTools: []Tool{ToolShell, ToolWriteFile},
			TurnCeiling: 4, Deadline: time.Now().Add(300 * time.Millisecond),
		})
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Run error = %v", err)
		}
		if result.Telemetry.Termination != TerminationDeadline || relay.Requests() != 1 || len(result.Telemetry.ToolInvocations) != 1 {
			t.Fatalf("result = %#v, requests = %d", result, relay.Requests())
		}
		assertRecordedProcessGone(t, filepath.Join(workingDirectory, "child.pid"))
		if _, err := os.Stat(filepath.Join(workingDirectory, "must-not-exist")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("tool executed after deadline: %v", err)
		}
	})

	t.Run("output ceiling", func(t *testing.T) {
		workingDirectory := t.TempDir()
		relay := newFakeRelay(t, []fakeRelayStep{
			toolStep("noisy", ToolShell, map[string]any{"command": "yes runaway"}),
			toolStep("late-write", ToolWriteFile, map[string]any{"path": "must-not-exist", "content": "bad"}),
		})
		defer relay.Close()
		runtime := &PurposeBuiltRuntime{Client: newTestRelayClient(t, relay.URL)}
		result, err := runtime.Run(t.Context(), TaskSpec{
			Goal: "test", WorkingDirectory: workingDirectory, AllowedTools: []Tool{ToolShell, ToolWriteFile},
			TurnCeiling: 4, Deadline: time.Now().Add(3 * time.Second), OutputCapBytes: 512,
		})
		if err != nil {
			t.Fatal(err)
		}
		if result.Telemetry.Termination != TerminationOutputCeiling || !result.Telemetry.OutputTruncated || relay.Requests() != 1 {
			t.Fatalf("result = %#v, requests = %d", result, relay.Requests())
		}
		if len(result.Telemetry.ToolInvocations) != 1 || !result.Telemetry.ToolInvocations[0].Truncated {
			t.Fatalf("tool telemetry = %#v", result.Telemetry.ToolInvocations)
		}
	})

	t.Run("relay token budget", func(t *testing.T) {
		relay := newFakeRelay(t, []fakeRelayStep{{status: http.StatusTooManyRequests, errorCode: "token_budget_exceeded", content: "token budget exhausted"}})
		defer relay.Close()
		runtime := &PurposeBuiltRuntime{Client: newTestRelayClient(t, relay.URL)}
		result, err := runtime.Run(t.Context(), TaskSpec{
			Goal: "test", WorkingDirectory: t.TempDir(), AllowedTools: []Tool{ToolShell},
			TurnCeiling: 2, Deadline: time.Now().Add(time.Second),
		})
		if err != nil {
			t.Fatal(err)
		}
		if result.Telemetry.Termination != TerminationTokenBudget || len(result.Telemetry.ToolInvocations) != 0 {
			t.Fatalf("telemetry = %#v", result.Telemetry)
		}
	})
}

func TestPurposeBuiltRuntimeHardFailureAndConfinement(t *testing.T) {
	t.Run("relay hard failure", func(t *testing.T) {
		relay := newFakeRelay(t, []fakeRelayStep{{status: http.StatusBadGateway, content: "provider unavailable"}})
		defer relay.Close()
		runtime := &PurposeBuiltRuntime{Client: newTestRelayClient(t, relay.URL)}
		result, err := runtime.Run(t.Context(), TaskSpec{
			Goal: "test", WorkingDirectory: t.TempDir(), AllowedTools: []Tool{ToolShell},
			TurnCeiling: 2, Deadline: time.Now().Add(time.Second),
		})
		if err == nil || result.Telemetry.Termination != TerminationRuntimeFailure || len(result.Telemetry.ToolInvocations) != 0 {
			t.Fatalf("result = %#v, err = %v", result, err)
		}
	})

	t.Run("write cannot escape root", func(t *testing.T) {
		parent := t.TempDir()
		workingDirectory := filepath.Join(parent, "workspace")
		if err := os.Mkdir(workingDirectory, 0o700); err != nil {
			t.Fatal(err)
		}
		relay := newFakeRelay(t, []fakeRelayStep{
			toolStep("escape", ToolWriteFile, map[string]any{"path": "../escaped", "content": "bad"}),
			{content: "DONE"},
		})
		defer relay.Close()
		runtime := &PurposeBuiltRuntime{Client: newTestRelayClient(t, relay.URL)}
		result, err := runtime.Run(t.Context(), TaskSpec{
			Goal: "test", WorkingDirectory: workingDirectory, AllowedTools: []Tool{ToolWriteFile},
			TurnCeiling: 3, Deadline: time.Now().Add(time.Second),
		})
		if err != nil {
			t.Fatal(err)
		}
		if result.Telemetry.Termination != TerminationCompleted {
			t.Fatalf("termination = %q", result.Telemetry.Termination)
		}
		if _, err := os.Stat(filepath.Join(parent, "escaped")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("write escaped root: %v", err)
		}
		if len(result.Telemetry.ToolInvocations) != 1 || result.Telemetry.ToolInvocations[0].Error == "" {
			t.Fatalf("tool invocation = %#v", result.Telemetry.ToolInvocations)
		}
	})
}

type fakeRelayStep struct {
	tool      *ToolCall
	content   string
	status    int
	errorCode string
}

func toolStep(id string, tool Tool, arguments map[string]any) fakeRelayStep {
	payload, _ := json.Marshal(arguments)
	return fakeRelayStep{tool: &ToolCall{ID: id, Type: "function", Function: ToolFunction{Name: string(tool), Arguments: string(payload)}}}
}

type fakeRelay struct {
	*httptest.Server
	mu       sync.Mutex
	requests int
	steps    []fakeRelayStep
}

func newFakeRelay(t *testing.T, steps []fakeRelayStep) *fakeRelay {
	t.Helper()
	relay := &fakeRelay{steps: steps}
	relay.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/chat/completions" || request.Header.Get("Authorization") != "Bearer fake-operation-token" {
			t.Errorf("unexpected relay request %s auth=%q", request.URL.Path, request.Header.Get("Authorization"))
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		var body struct {
			Model    string        `json:"model"`
			Messages []ChatMessage `json:"messages"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode relay request: %v", err)
			http.Error(w, "bad JSON", http.StatusBadRequest)
			return
		}
		if body.Model != "codegym-agent" || len(body.Messages) < 2 {
			t.Errorf("relay body = %#v", body)
		}
		relay.mu.Lock()
		index := relay.requests
		relay.requests++
		relay.mu.Unlock()
		if index >= len(relay.steps) {
			t.Errorf("unexpected relay request %d", index+1)
			http.Error(w, "script exhausted", http.StatusInternalServerError)
			return
		}
		step := relay.steps[index]
		status := step.status
		if status == 0 {
			status = http.StatusOK
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status < 200 || status >= 300 {
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": step.content, "code": step.errorCode}})
			return
		}
		message := ChatMessage{Role: "assistant", Content: step.content}
		if step.tool != nil {
			message.ToolCalls = []ToolCall{*step.tool}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": message}},
			"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 2, "total_tokens": 5},
		})
	}))
	return relay
}

func (r *fakeRelay) Requests() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.requests
}

func newTestRelayClient(t *testing.T, baseURL string) *OpenAICompatibleClient {
	t.Helper()
	client, err := NewOpenAICompatibleClient(OpenAICompatibleClientConfig{
		BaseURL: baseURL + "/v1", APIKey: "fake-operation-token", Model: "codegym-agent", MaxTokens: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func writeTestFile(t *testing.T, root, path, content string) {
	t.Helper()
	fullPath := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertRecordedProcessGone(t *testing.T, path string) {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read child pid: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(payload)))
	if err != nil {
		t.Fatalf("parse child pid %q: %v", payload, err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		err = syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("child process %d survived process-group cancellation: %v", pid, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (s fakeRelayStep) String() string {
	return fmt.Sprintf("status=%d code=%s content=%s", s.status, s.errorCode, s.content)
}
