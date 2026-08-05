package agentruntime

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const OpenCodePinnedVersion = "1.18.11"

type OpenCodeConfig struct {
	BinaryPath      string
	RuntimeTemplate string
	RelayBaseURL    string
	RelayToken      string
	Model           string
	Progress        ProgressReporter
	TempRoot        string
	Now             func() time.Time
}

type OpenCodeRuntime struct {
	config OpenCodeConfig
}

func NewOpenCodeRuntime(config OpenCodeConfig) (*OpenCodeRuntime, error) {
	config.BinaryPath = strings.TrimSpace(config.BinaryPath)
	config.RelayBaseURL = strings.TrimRight(strings.TrimSpace(config.RelayBaseURL), "/")
	config.RelayToken = strings.TrimSpace(config.RelayToken)
	config.Model = strings.TrimSpace(config.Model)
	if config.BinaryPath == "" || config.RelayBaseURL == "" || config.RelayToken == "" || config.Model == "" {
		return nil, errors.New("agentruntime: opencode binary, relay URL, relay token, and model are required")
	}
	if config.RuntimeTemplate == "" {
		config.RuntimeTemplate = filepath.Join(filepath.Dir(config.BinaryPath), "home-template", ".config", "opencode")
	}
	if config.TempRoot == "" {
		config.TempRoot = "/tmp"
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &OpenCodeRuntime{config: config}, nil
}

func (r *OpenCodeRuntime) Name() string { return "opencode" }

func (r *OpenCodeRuntime) Run(ctx context.Context, task TaskSpec) (result RunResult, err error) {
	if r == nil {
		return result, errors.New("agentruntime: opencode runtime is nil")
	}
	startedAt := r.config.Now()
	result.Telemetry.Runtime = r.Name()
	result.Telemetry.RuntimeVersion = OpenCodePinnedVersion
	result.Telemetry.Termination = TerminationRuntimeFailure
	defer func() {
		result.Telemetry.WallTime = r.config.Now().Sub(startedAt)
		result.Manifest = LoadManifest(task.WorkingDirectory)
	}()
	if err := task.Validate(startedAt); err != nil {
		return result, err
	}
	if err := r.verifyVersion(ctx); err != nil {
		return result, err
	}

	runRoot, err := os.MkdirTemp(r.config.TempRoot, "codegym-opencode-runtime-")
	if err != nil {
		return result, fmt.Errorf("agentruntime: create isolated opencode home: %w", err)
	}
	defer func() {
		if cleanupErr := os.RemoveAll(runRoot); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("agentruntime: remove isolated opencode home: %w", cleanupErr))
		}
	}()
	home := filepath.Join(runRoot, "home")
	configDirectory := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(configDirectory, 0o700); err != nil {
		return result, fmt.Errorf("agentruntime: create opencode config directory: %w", err)
	}
	if err := os.CopyFS(configDirectory, os.DirFS(r.config.RuntimeTemplate)); err != nil && !errors.Is(err, fs.ErrExist) {
		return result, fmt.Errorf("agentruntime: copy pinned opencode tool runtime: %w", err)
	}
	toolDirectory := filepath.Join(configDirectory, "tools")
	if err := os.MkdirAll(toolDirectory, 0o700); err != nil {
		return result, fmt.Errorf("agentruntime: create opencode tool directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(toolDirectory, "report_progress.ts"), []byte(openCodeProgressTool), 0o600); err != nil {
		return result, fmt.Errorf("agentruntime: write opencode progress tool: %w", err)
	}
	progressLog := filepath.Join(runRoot, "progress.jsonl")
	if err := os.WriteFile(progressLog, nil, 0o600); err != nil {
		return result, fmt.Errorf("agentruntime: initialize opencode progress log: %w", err)
	}
	configPath := filepath.Join(configDirectory, "opencode.json")
	configPayload, err := r.buildConfig(task)
	if err != nil {
		return result, err
	}
	if err := os.WriteFile(configPath, configPayload, 0o600); err != nil {
		return result, fmt.Errorf("agentruntime: write opencode config: %w", err)
	}

	runContext, cancel := context.WithDeadline(ctx, task.Deadline)
	defer cancel()
	prompt := fmt.Sprintf(
		"%s\n\n%s",
		task.Goal, resultManifestInstructions(),
	)
	command := exec.CommandContext(
		runContext, r.config.BinaryPath, "run", "--format", "json", "--auto",
		"--agent", "codegym", "--model", "codegym-relay/"+r.config.Model, "--title", "CodeGym agent task", prompt,
	)
	command.Dir = task.WorkingDirectory
	command.Env = append(safeRuntimeEnvironment(home, r.config.TempRoot),
		"OPENCODE_CONFIG="+configPath, "OPENCODE_DISABLE_AUTOUPDATE=1",
		"OPENCODE_DISABLE_MODELS_FETCH=1", "OPENCODE_DISABLE_DEFAULT_PLUGINS=1",
		"OPENCODE_DISABLE_LSP_DOWNLOAD=1", "OPENCODE_DISABLE_CLAUDE_CODE=1",
		"OPENCODE_ENABLE_EXA=0", "OPENCODE_AUTO_SHARE=0",
		"CODEGYM_PROGRESS_LOG="+progressLog,
	)
	output, exitCode, truncated, commandErr := runBoundedProcess(runContext, command, task.EffectiveOutputCap())
	result.Telemetry.ExitCode = &exitCode
	result.Telemetry.OutputTruncated = truncated
	parseOpenCodeEvents(output, &result)
	if progressErr := r.deliverProgress(ctx, progressLog, &result); progressErr != nil {
		return result, progressErr
	}
	if truncated {
		result.Telemetry.Termination = TerminationOutputCeiling
		return result, nil
	}
	if reason, contextErr := contextTermination(ctx, runContext); reason != "" {
		result.Telemetry.Termination = reason
		return result, contextErr
	}
	if commandErr != nil {
		result.Telemetry.Termination = TerminationRuntimeFailure
		return result, commandErr
	}
	if exitCode != 0 {
		result.Telemetry.Termination = TerminationRuntimeFailure
		return result, nil
	}
	result.Manifest = LoadManifest(task.WorkingDirectory)
	if result.Telemetry.Turns >= task.TurnCeiling && result.Manifest.Status != ManifestPresent {
		result.Telemetry.Termination = TerminationTurnCeiling
		return result, nil
	}
	result.Telemetry.Termination = TerminationCompleted
	return result, nil
}

func (r *OpenCodeRuntime) verifyVersion(ctx context.Context) error {
	versionContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	command := exec.CommandContext(versionContext, r.config.BinaryPath, "--version")
	output, err := command.Output()
	if err != nil {
		return fmt.Errorf("agentruntime: inspect opencode version: %w", err)
	}
	version := strings.TrimSpace(string(output))
	if version != OpenCodePinnedVersion {
		return fmt.Errorf("agentruntime: opencode version %q does not match pinned %s", version, OpenCodePinnedVersion)
	}
	return nil
}

func (r *OpenCodeRuntime) buildConfig(task TaskSpec) ([]byte, error) {
	permissions := map[string]any{
		"*": "deny",
		"external_directory": map[string]string{
			"*":                                     "deny",
			"~/.local/share/opencode/tool-output/*": "allow",
		},
	}
	for _, allowed := range task.AllowedTools {
		name := openCodeToolName(allowed)
		permissions[name] = "allow"
	}
	config := map[string]any{
		"$schema": "https://opencode.ai/config.json", "autoupdate": false, "share": "disabled",
		"model": "codegym-relay/" + r.config.Model,
		"provider": map[string]any{"codegym-relay": map[string]any{
			"npm": "@ai-sdk/openai-compatible", "name": "CodeGym operation relay",
			"options": map[string]any{"baseURL": r.config.RelayBaseURL, "apiKey": r.config.RelayToken},
			"models": map[string]any{r.config.Model: map[string]any{
				"name": "CodeGym agent", "limit": map[string]any{"context": 32768, "output": 4096},
			}},
		}},
		"permission": permissions,
		"agent": map[string]any{"codegym": map[string]any{
			"description": "Bounded CodeGym coding-agent runtime", "mode": "primary",
			"model": "codegym-relay/" + r.config.Model, "steps": task.TurnCeiling,
			"permission": permissions,
		}},
	}
	payload, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("agentruntime: encode opencode config: %w", err)
	}
	return payload, nil
}

func openCodeToolName(tool Tool) string {
	switch tool {
	case ToolReadFile:
		return "read"
	case ToolWriteFile:
		return "write"
	case ToolShell:
		return "bash"
	case ToolReportProgress:
		return "report_progress"
	default:
		return string(tool)
	}
}

func runBoundedProcess(ctx context.Context, command *exec.Cmd, capBytes int) (string, int, bool, error) {
	killProcessGroup := func() {
		if command.Process != nil {
			_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		}
	}
	buffer := newCappedBuffer(capBytes, killProcessGroup)
	command.Stdout = buffer
	command.Stderr = buffer
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.WaitDelay = 2 * time.Second
	command.Cancel = func() error {
		if command.Process == nil {
			return nil
		}
		err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	commandErr := command.Run()
	exitCode := 0
	if commandErr != nil {
		exitCode = -1
		var exitError *exec.ExitError
		if errors.As(commandErr, &exitError) {
			exitCode = exitError.ExitCode()
			if ctx.Err() == nil && !buffer.Truncated() {
				commandErr = nil
			}
		}
	}
	return buffer.String(), exitCode, buffer.Truncated(), commandErr
}

func parseOpenCodeEvents(output string, result *RunResult) {
	scanner := bufio.NewScanner(strings.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64<<10), DefaultOutputCap)
	var prose strings.Builder
	for scanner.Scan() {
		var event struct {
			Type string `json:"type"`
			Part struct {
				Type   string `json:"type"`
				Text   string `json:"text"`
				Tool   string `json:"tool"`
				Tokens struct {
					Total     int64 `json:"total"`
					Input     int64 `json:"input"`
					Output    int64 `json:"output"`
					Reasoning int64 `json:"reasoning"`
					Cache     struct {
						Read  int64 `json:"read"`
						Write int64 `json:"write"`
					} `json:"cache"`
				} `json:"tokens"`
				Cost  float64 `json:"cost"`
				State struct {
					Status   string `json:"status"`
					Error    string `json:"error"`
					Metadata struct {
						Exit      int  `json:"exit"`
						Truncated bool `json:"truncated"`
					} `json:"metadata"`
					Time struct {
						Start int64 `json:"start"`
						End   int64 `json:"end"`
					} `json:"time"`
				} `json:"state"`
			} `json:"part"`
		}
		if json.Unmarshal(scanner.Bytes(), &event) != nil {
			continue
		}
		switch event.Type {
		case "step_finish":
			result.Telemetry.Turns++
			addTokenUsage(&result.Telemetry.Tokens, TokenUsage{
				Total: event.Part.Tokens.Total, Input: event.Part.Tokens.Input,
				Output: event.Part.Tokens.Output, Reasoning: event.Part.Tokens.Reasoning,
				CacheRead: event.Part.Tokens.Cache.Read, CacheWrite: event.Part.Tokens.Cache.Write,
			})
			result.Telemetry.CostUSDMicros += int64(event.Part.Cost * 1_000_000)
		case "text":
			prose.WriteString(event.Part.Text)
		case "tool_use":
			tool := fromOpenCodeTool(event.Part.Tool)
			invocation := ToolInvocation{Tool: tool, Error: event.Part.State.Error, Truncated: event.Part.State.Metadata.Truncated}
			if event.Part.State.Time.Start > 0 {
				invocation.StartedAt = time.UnixMilli(event.Part.State.Time.Start)
			}
			if event.Part.State.Time.End >= event.Part.State.Time.Start {
				invocation.Duration = time.Duration(event.Part.State.Time.End-event.Part.State.Time.Start) * time.Millisecond
			}
			if tool == ToolShell {
				exitCode := event.Part.State.Metadata.Exit
				invocation.ExitCode = &exitCode
				if exitCode != 0 {
					result.Telemetry.RepairIterations++
				}
			}
			result.Telemetry.ToolInvocations = append(result.Telemetry.ToolInvocations, invocation)
			if event.Part.State.Status == "error" && strings.Contains(strings.ToLower(event.Part.State.Error), "permission") {
				result.Telemetry.PolicyViolations = append(result.Telemetry.PolicyViolations, event.Part.State.Error)
			}
		}
	}
	result.Prose = prose.String()
}

func fromOpenCodeTool(name string) Tool {
	switch name {
	case "read":
		return ToolReadFile
	case "write", "edit":
		return ToolWriteFile
	case "bash":
		return ToolShell
	case "report_progress":
		return ToolReportProgress
	default:
		return Tool(name)
	}
}

func (r *OpenCodeRuntime) deliverProgress(ctx context.Context, path string, result *RunResult) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("agentruntime: open opencode progress log: %w", err)
	}
	defer func() { _ = file.Close() }()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event ProgressEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return fmt.Errorf("agentruntime: decode opencode progress: %w", err)
		}
		if event.At.IsZero() {
			event.At = r.config.Now()
		}
		result.Telemetry.Progress = append(result.Telemetry.Progress, event)
		if r.config.Progress != nil {
			if err := r.config.Progress(ctx, event); err != nil {
				return fmt.Errorf("agentruntime: deliver opencode progress: %w", err)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("agentruntime: read opencode progress: %w", err)
	}
	return nil
}

const openCodeProgressTool = `import { appendFileSync } from "node:fs"
import { tool } from "@opencode-ai/plugin"

export default tool({
  description: "Report optional detail within the backend-owned workflow step.",
  args: {
    step_id: tool.schema.string().describe("Workflow step identifier"),
    label: tool.schema.string().describe("Human-readable progress label"),
  },
  async execute(args) {
    const path = process.env.CODEGYM_PROGRESS_LOG
    if (!path) throw new Error("CODEGYM_PROGRESS_LOG is not set")
    appendFileSync(path, JSON.stringify(args) + "\n", { encoding: "utf8" })
    return JSON.stringify({ accepted: true, ...args })
  },
})
`
