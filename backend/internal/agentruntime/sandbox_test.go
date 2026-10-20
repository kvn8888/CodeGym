package agentruntime

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"
)

var _ AgentRuntime = (*SandboxRuntime)(nil)

// commandResult is one canned sandbox command outcome.
type commandResult struct {
	exitCode int
	output   string
	err      error
}

// fakeExecutor is an in-memory sandbox: it records uploads and commands, and
// answers commands by substring match so a test reads as a script.
type fakeExecutor struct {
	uploads  map[string][]byte
	commands []string
	files    map[string][]byte
	results  map[string]commandResult
	fallback commandResult
}

func (e *fakeExecutor) UploadFile(_ context.Context, content []byte, path string) error {
	if e.uploads == nil {
		e.uploads = map[string][]byte{}
	}
	e.uploads[path] = append([]byte(nil), content...)
	return nil
}

func (e *fakeExecutor) ExecuteCommand(_ context.Context, command string, _ time.Duration) (int, string, error) {
	e.commands = append(e.commands, command)
	// Model `printf '\n' > 'path'`: the sandbox seeds the progress log
	// with one newline, so downloads work.
	if idx := strings.Index(command, "> '"); idx >= 0 {
		if end := strings.Index(command[idx+len("> '"):], "'"); end >= 0 {
			if e.files == nil {
				e.files = map[string][]byte{}
			}
			path := command[idx+len("> '") : idx+len("> '")+end]
			if _, ok := e.files[path]; !ok {
				e.files[path] = []byte("\n")
			}
		}
	}
	// Background execution: `... &` starts a command and returns immediately.
	// The fake simulates the sandbox completing it by writing the done, exit
	// code, and output files that runCommand polls for.
	if strings.HasSuffix(strings.TrimSpace(command), "&") {
		exitCode := 1
		output := ""
		for needle, result := range e.results {
			if strings.Contains(command, needle) {
				exitCode = result.exitCode
				output = result.output
				break
			}
		}
		if e.files == nil {
			e.files = map[string][]byte{}
		}
		e.files["/home/agent/.codegym-scratch/agent.done"] = []byte{}
		e.files["/home/agent/.codegym-scratch/agent.exitcode"] = []byte(strconv.Itoa(exitCode))
		e.files["/home/agent/.codegym-scratch/agent.output"] = []byte(output)
		return 0, "", nil
	}
	// Synchronous execution: version check, mkdir, etc.
	for needle, result := range e.results {
		if strings.Contains(command, needle) {
			return result.exitCode, result.output, result.err
		}
	}
	return e.fallback.exitCode, e.fallback.output, e.fallback.err
}

func (e *fakeExecutor) DownloadFile(_ context.Context, path string) ([]byte, error) {
	// Uploaded files are readable again, exactly as in a real sandbox.
	if payload, ok := e.files[path]; ok {
		return payload, nil
	}
	if payload, ok := e.uploads[path]; ok {
		return payload, nil
	}
	return nil, errors.New("no such file: " + path)
}

func (e *fakeExecutor) ListFiles(_ context.Context, dir string) ([]SandboxEntry, error) {
	prefix := strings.TrimRight(dir, "/") + "/"
	seen := map[string]struct{}{}
	var entries []SandboxEntry
	for _, source := range []map[string][]byte{e.files, e.uploads} {
		for path := range source {
			if !strings.HasPrefix(path, prefix) {
				continue
			}
			rest := strings.TrimPrefix(path, prefix)
			if rest == "" {
				continue
			}
			name := rest
			isDir := false
			if idx := strings.Index(rest, "/"); idx >= 0 {
				name = rest[:idx]
				isDir = true
			}
			child := prefix + name
			if _, ok := seen[child]; ok {
				continue
			}
			seen[child] = struct{}{}
			entries = append(entries, SandboxEntry{Path: child, IsDirectory: isDir})
		}
	}
	return entries, nil
}

func newSandboxRuntimeForTest(t *testing.T, executor SandboxExecutor, progress ProgressReporter) *SandboxRuntime {
	t.Helper()
	runtime, err := NewSandboxRuntime(SandboxConfig{
		Executor:     executor,
		BinaryPath:   "/usr/local/bin/opencode",
		ConfigDir:    "/home/agent/.config/opencode",
		HomeDir:      "/home/agent",
		ScratchDir:   "/home/agent/.codegym-scratch",
		RelayBaseURL: "https://relay.example.com/v1",
		RelayToken:   "token-123",
		Model:        "codegym-agent",
		Progress:     progress,
		ManifestDir:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("new sandbox runtime: %v", err)
	}
	return runtime
}

func testTask() TaskSpec {
	return TaskSpec{
		Goal:             "Prepare a Spring Boot exercise.",
		WorkingDirectory: "/home/agent/work",
		AllowedTools:     []Tool{ToolReadFile, ToolWriteFile, ToolShell, ToolReportProgress},
		TurnCeiling:      4,
		Deadline:         time.Now().Add(2 * time.Minute),
	}
}

// TestSandboxRuntimeRunsAgentInSandbox proves the whole remote run: the agent
// is invoked inside the sandbox with the relay configuration, the manifest is
// read back, and progress reaches the reporter.
func TestSandboxRuntimeRunsAgentInSandbox(t *testing.T) {
	const manifestPath = "/home/agent/work/.codegym/agent-result.json"
	executor := &fakeExecutor{
		results: map[string]commandResult{
			"--version": {output: OpenCodePinnedVersion + "\n"},
			" run ": {
				output: `{"type":"step_finish","part":{"tokens":{"total":120,"input":100,"output":20},"cost":0.002}}
{"type":"text","part":{"type":"text","text":"exercise ready"}}
{"type":"tool_use","part":{"tool":"bash","state":{"status":"completed","metadata":{"exit":0}}}}
`,
			},
		},
		files: map[string][]byte{
			manifestPath: []byte(`{"version":1,"completed":true,"summary":"did it"}`),
			"/home/agent/.codegym-scratch/progress.jsonl": []byte(`{"step_id":"s1","label":"compiling"}` + "\n"),
		},
	}
	var delivered []ProgressEvent
	runtime := newSandboxRuntimeForTest(t, executor, func(_ context.Context, event ProgressEvent) error {
		delivered = append(delivered, event)
		return nil
	})

	result, err := runtime.Run(context.Background(), testTask())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Telemetry.Termination != TerminationCompleted {
		t.Fatalf("termination = %s", result.Telemetry.Termination)
	}
	if result.Telemetry.Turns != 1 {
		t.Fatalf("turns = %d", result.Telemetry.Turns)
	}
	if !strings.Contains(result.Prose, "exercise ready") {
		t.Fatalf("prose = %q", result.Prose)
	}
	if result.Manifest.Status != ManifestPresent {
		t.Fatalf("manifest status = %s (%s)", result.Manifest.Status, result.Manifest.Error)
	}
	if len(delivered) != 1 || delivered[0].StepID != "s1" {
		t.Fatalf("progress not delivered: %+v", delivered)
	}

	configPayload, ok := executor.uploads["/home/agent/.config/opencode/opencode.json"]
	if !ok {
		t.Fatal("opencode config was not uploaded")
	}
	var config struct {
		Model    string `json:"model"`
		Provider map[string]struct {
			Options struct {
				BaseURL string `json:"baseURL"`
				APIKey  string `json:"apiKey"`
			} `json:"options"`
		} `json:"provider"`
	}
	if err := json.Unmarshal(configPayload, &config); err != nil {
		t.Fatalf("decode uploaded config: %v", err)
	}
	if config.Model != "codegym-relay/codegym-agent" {
		t.Fatalf("model = %q", config.Model)
	}
	relay := config.Provider["codegym-relay"].Options
	if relay.BaseURL != "https://relay.example.com/v1" || relay.APIKey != "token-123" {
		t.Fatalf("relay options = %+v", relay)
	}
	if _, ok := executor.uploads["/home/agent/.config/opencode/tools/report_progress.ts"]; !ok {
		t.Fatal("progress tool was not uploaded")
	}

	agentCommand := executor.commands[len(executor.commands)-1]
	for _, fragment := range []string{
		"cd '/home/agent/work'", "HOME='/home/agent'",
		"OPENCODE_CONFIG='/home/agent/.config/opencode/opencode.json'",
		"CODEGYM_PROGRESS_LOG='/home/agent/.codegym-scratch/progress.jsonl'",
		"'/usr/local/bin/opencode' run", "--format json",
	} {
		if !strings.Contains(agentCommand, fragment) {
			t.Fatalf("agent command missing %q: %s", fragment, agentCommand)
		}
	}
}

// TestSandboxRuntimeRejectsVersionMismatch proves a wrong pinned binary is a
// runtime fault rather than a silent run.
func TestSandboxRuntimeRejectsVersionMismatch(t *testing.T) {
	executor := &fakeExecutor{results: map[string]commandResult{"--version": {output: "9.9.9\n"}}}
	runtime := newSandboxRuntimeForTest(t, executor, nil)
	if _, err := runtime.Run(context.Background(), testTask()); err == nil {
		t.Fatal("expected a version mismatch error")
	}
}

// TestSandboxRuntimeRejectsInvalidTask proves task validation still applies.
func TestSandboxRuntimeRejectsInvalidTask(t *testing.T) {
	executor := &fakeExecutor{results: map[string]commandResult{"--version": {output: OpenCodePinnedVersion}}}
	runtime := newSandboxRuntimeForTest(t, executor, nil)
	task := testTask()
	task.WorkingDirectory = "relative/path"
	if _, err := runtime.Run(context.Background(), task); err == nil {
		t.Fatal("expected a validation error")
	}
}

// TestSandboxRuntimeReportsNonZeroExit proves an agent that exits nonzero is
// a runtime failure with no manifest.
func TestSandboxRuntimeReportsNonZeroExit(t *testing.T) {
	executor := &fakeExecutor{results: map[string]commandResult{
		"--version": {output: OpenCodePinnedVersion},
		" run ":     {exitCode: 2, output: ""},
	}}
	runtime := newSandboxRuntimeForTest(t, executor, nil)
	result, err := runtime.Run(context.Background(), testTask())
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if result.Telemetry.Termination != TerminationRuntimeFailure {
		t.Fatalf("termination = %s", result.Telemetry.Termination)
	}
}

// TestSandboxRuntimeKeepsFailureOutput proves a runtime failure carries the
// tail of the agent output so operators can see why the run failed.
func TestSandboxRuntimeKeepsFailureOutput(t *testing.T) {
	executor := &fakeExecutor{results: map[string]commandResult{
		"--version": {output: OpenCodePinnedVersion},
		" run ":     {exitCode: 1, output: "error: failed to load plugin"},
	}}
	runtime := newSandboxRuntimeForTest(t, executor, nil)
	result, err := runtime.Run(context.Background(), testTask())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Telemetry.Termination != TerminationRuntimeFailure {
		t.Fatalf("termination = %s", result.Telemetry.Termination)
	}
	if !strings.Contains(result.OutputTail, "failed to load plugin") {
		t.Fatalf("output tail = %q", result.OutputTail)
	}
}

// TestSandboxRuntimeTruncatesOutput proves the output ceiling is enforced on
// the captured sandbox output.
func TestSandboxRuntimeTruncatesOutput(t *testing.T) {
	executor := &fakeExecutor{results: map[string]commandResult{
		"--version": {output: OpenCodePinnedVersion},
		" run ":     {output: strings.Repeat("x", 256)},
	}}
	runtime := newSandboxRuntimeForTest(t, executor, nil)
	task := testTask()
	task.OutputCapBytes = 64
	result, err := runtime.Run(context.Background(), task)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Telemetry.Termination != TerminationOutputCeiling {
		t.Fatalf("termination = %s", result.Telemetry.Termination)
	}
}

// TestShellQuoteNeutralisesQuotes proves a prompt with quotes cannot break out
// of its argument. The expected escaping is the POSIX-safe close/escape/reopen
// sequence for every embedded single quote.
func TestShellQuoteNeutralisesQuotes(t *testing.T) {
	quoted := shellQuote("it's a 'quoted' prompt")
	want := `'it'\''s a '\''quoted'\'' prompt'`
	if quoted != want {
		t.Fatalf("shellQuote = %s, want %s", quoted, want)
	}
}

// TestSandboxRuntimeRequiresConfiguration proves construction refuses an
// incomplete configuration.
func TestSandboxRuntimeRequiresConfiguration(t *testing.T) {
	cases := map[string]SandboxConfig{
		"missing executor": {BinaryPath: "/b", ConfigDir: "/c", HomeDir: "/h", ScratchDir: "/s", RelayBaseURL: "u", RelayToken: "t", Model: "m"},
		"missing relay":    {BinaryPath: "/b", ConfigDir: "/c", HomeDir: "/h", ScratchDir: "/s"},
		"missing paths":    {RelayBaseURL: "u", RelayToken: "t", Model: "m"},
	}
	for name, config := range cases {
		if _, err := NewSandboxRuntime(config); err == nil {
			t.Fatalf("%s: expected an error", name)
		}
	}
}
