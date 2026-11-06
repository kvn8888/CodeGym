package agentruntime

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SandboxEntry is one directory entry inside a sandbox, in sandbox-native
// terms. Keeping this neutral lets callers walk a sandbox tree without
// importing any cloud SDK types.
type SandboxEntry struct {
	Path        string
	IsDirectory bool
	Size        int64
}

// SandboxExecutor is the remote execution surface of one build sandbox. It
// mirrors the sandbox seams the Daytona adapter already implements, so the
// same agent code runs whether the sandbox is remote or a test fake.
type SandboxExecutor interface {
	UploadFile(ctx context.Context, content []byte, path string) error
	ExecuteCommand(ctx context.Context, command string, timeout time.Duration) (exitCode int, output string, err error)
	DownloadFile(ctx context.Context, path string) ([]byte, error)
	ListFiles(ctx context.Context, dir string) ([]SandboxEntry, error)
}

// SandboxConfig configures one sandbox-backed opencode runtime. The custom
// tool runtime (the pinned @opencode-ai/plugin package) is prebaked in the
// sandbox image at ConfigDir; only the per-build config and progress tool are
// written on each run.
type SandboxConfig struct {
	Executor     SandboxExecutor
	BinaryPath   string
	ConfigDir    string
	HomeDir      string
	ScratchDir   string
	RelayBaseURL string
	RelayToken   string
	Model        string
	Progress     ProgressReporter
	Now          func() time.Time
	// ManifestDir is a local scratch directory used to stage the downloaded
	// result manifest so the shared manifest loader can read it.
	ManifestDir string
}

// SandboxRuntime runs the pinned opencode agent inside a build sandbox. It is
// the production agent path: the agent and every tool it calls execute in the
// sandbox, and the model is reached only through the relay.
type SandboxRuntime struct {
	config SandboxConfig
}

// NewSandboxRuntime validates and normalises a sandbox runtime.
func NewSandboxRuntime(config SandboxConfig) (*SandboxRuntime, error) {
	config.BinaryPath = strings.TrimSpace(config.BinaryPath)
	config.ConfigDir = strings.TrimRight(strings.TrimSpace(config.ConfigDir), "/")
	config.HomeDir = strings.TrimRight(strings.TrimSpace(config.HomeDir), "/")
	config.ScratchDir = strings.TrimRight(strings.TrimSpace(config.ScratchDir), "/")
	config.RelayBaseURL = strings.TrimRight(strings.TrimSpace(config.RelayBaseURL), "/")
	config.RelayToken = strings.TrimSpace(config.RelayToken)
	config.Model = strings.TrimSpace(config.Model)
	if config.Executor == nil {
		return nil, errors.New("agentruntime: sandbox executor is required")
	}
	if config.BinaryPath == "" || config.ConfigDir == "" || config.HomeDir == "" || config.ScratchDir == "" {
		return nil, errors.New("agentruntime: sandbox binary, config, home, and scratch paths are required")
	}
	if config.RelayBaseURL == "" || config.RelayToken == "" || config.Model == "" {
		return nil, errors.New("agentruntime: relay URL, relay token, and model are required")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.ManifestDir == "" {
		config.ManifestDir = os.TempDir()
	}
	return &SandboxRuntime{config: config}, nil
}

// Name identifies the runtime for telemetry.
func (r *SandboxRuntime) Name() string { return "opencode-sandbox" }

// Run prepares the sandbox, runs the agent inside it, and reads back the
// result manifest. Every failure mode mirrors the host runtime so downstream
// termination handling is identical.
func (r *SandboxRuntime) Run(ctx context.Context, task TaskSpec) (result RunResult, err error) {
	if r == nil {
		return result, errors.New("agentruntime: sandbox runtime is nil")
	}
	startedAt := r.config.Now()
	result.Telemetry.Runtime = r.Name()
	result.Telemetry.RuntimeVersion = OpenCodePinnedVersion
	result.Telemetry.Termination = TerminationRuntimeFailure
	defer func() {
		result.Telemetry.WallTime = r.config.Now().Sub(startedAt)
		result.Manifest = r.loadManifest(ctx, task)
	}()
	if err := task.ValidateSandbox(startedAt); err != nil {
		return result, err
	}
	if err := r.verifyVersion(ctx); err != nil {
		return result, err
	}

	configPath, progressLog, err := r.writeSandboxConfig(ctx, task)
	if err != nil {
		return result, err
	}
	runContext, cancel := context.WithDeadline(ctx, task.Deadline)
	defer cancel()
	timeout := time.Until(task.Deadline)
	if timeout <= 0 {
		return result, context.DeadlineExceeded
	}
	prompt := fmt.Sprintf("%s\n\n%s", task.Goal, resultManifestInstructions())
	command := r.agentCommand(task, configPath, progressLog, prompt)
	log.Printf("agentruntime: starting agent command in %s", task.WorkingDirectory)
	exitCode, output, executeErr := r.runCommand(runContext, command, timeout)
	result.Telemetry.ExitCode = &exitCode
	truncated := len(output) > task.EffectiveOutputCap()
	if truncated {
		output = output[:task.EffectiveOutputCap()]
	}
	result.Telemetry.OutputTruncated = truncated
	parseOpenCodeEvents(output, &result)
	if progressErr := r.deliverProgress(ctx, progressLog, &result); progressErr != nil {
		log.Printf("agentruntime: deliver progress failed: %v", progressErr)
		return result, progressErr
	}
	if truncated {
		result.Telemetry.Termination = TerminationOutputCeiling
		log.Printf("agentruntime: agent command output ceiling exceeded")
		return result, nil
	}
	if reason, contextErr := contextTermination(ctx, runContext); reason != "" {
		result.Telemetry.Termination = reason
		return result, contextErr
	}
	if executeErr != nil {
		result.Telemetry.Termination = TerminationRuntimeFailure
		result.OutputTail = failureOutputTail(output)
		log.Printf("agentruntime: agent command failed: %v output_tail=%q", executeErr, result.OutputTail)
		return result, executeErr
	}
	if exitCode != 0 {
		result.Telemetry.Termination = TerminationRuntimeFailure
		result.OutputTail = failureOutputTail(output)
		log.Printf("agentruntime: agent command exited %d output_tail=%q", exitCode, result.OutputTail)
		return result, nil
	}
	result.Manifest = r.loadManifest(ctx, task)
	if result.Telemetry.Turns >= task.TurnCeiling && result.Manifest.Status != ManifestPresent {
		result.Telemetry.Termination = TerminationTurnCeiling
		return result, nil
	}
	result.Telemetry.Termination = TerminationCompleted
	log.Printf("agentruntime: agent command completed (exit=%d, turns=%d, output_tail=%q)", *result.Telemetry.ExitCode, result.Telemetry.Turns, result.OutputTail)
	return result, nil
}

// failureOutputTail keeps the end of the agent output, which is where a
// runtime failure usually explains itself, bounded for logs and telemetry.
func failureOutputTail(output string) string {
	const limit = 4000
	output = strings.TrimSpace(output)
	if len(output) <= limit {
		return output
	}
	return "..." + output[len(output)-limit:]
}

// verifyVersion runs the pinned binary's version check inside the sandbox.
func (r *SandboxRuntime) verifyVersion(ctx context.Context) error {
	versionContext, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	exitCode, output, err := r.config.Executor.ExecuteCommand(versionContext, shellQuote(r.config.BinaryPath)+" --version", 30*time.Second)
	if err != nil {
		return fmt.Errorf("agentruntime: inspect sandbox opencode version: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("agentruntime: sandbox opencode version check exited %d: %s", exitCode, strings.TrimSpace(output))
	}
	version := strings.TrimSpace(output)
	if version != OpenCodePinnedVersion {
		return fmt.Errorf("agentruntime: sandbox opencode version %q does not match pinned %s", version, OpenCodePinnedVersion)
	}
	return nil
}

// writeSandboxConfig uploads the per-build opencode config and progress tool,
// and initialises the progress log. It returns the config path and the
// progress log path inside the sandbox.
func (r *SandboxRuntime) writeSandboxConfig(ctx context.Context, task TaskSpec) (string, string, error) {
	toolsDir := r.config.ConfigDir + "/tools"
	progressLog := r.config.ScratchDir + "/progress.jsonl"
	// The sandbox image only guarantees the home directory exists. Two
	// Daytona toolbox limits shape this command: an upload rejects a missing
	// parent, and neither upload nor download works for a zero-byte file.
	// Create the tool, scratch and working directories and seed the progress
	// log with one newline, which the reader skips as an empty line. The
	// working directory must exist because the agent command starts with cd.
	prepare := "mkdir -p " + shellQuote(toolsDir) + " " + shellQuote(r.config.ScratchDir) + " " + shellQuote(task.WorkingDirectory) +
		` && printf '\n' > ` + shellQuote(progressLog)
	if _, _, err := r.config.Executor.ExecuteCommand(ctx, prepare, 30*time.Second); err != nil {
		return "", "", fmt.Errorf("agentruntime: prepare sandbox config directory: %w", err)
	}
	payload, err := openCodeConfigPayload(r.config.Model, r.config.RelayBaseURL, r.config.RelayToken, task.TurnCeiling, task.AllowedTools)
	if err != nil {
		return "", "", err
	}
	configPath := r.config.ConfigDir + "/opencode.json"
	if err := r.config.Executor.UploadFile(ctx, payload, configPath); err != nil {
		return "", "", fmt.Errorf("agentruntime: upload sandbox opencode config: %w", err)
	}
	if err := r.config.Executor.UploadFile(ctx, []byte(openCodeProgressTool), toolsDir+"/report_progress.ts"); err != nil {
		return "", "", fmt.Errorf("agentruntime: upload sandbox progress tool: %w", err)
	}
	return configPath, progressLog, nil
}

// runCommand executes one command by running it in the background inside the
// sandbox and polling for a completion marker file. Daytona's SDK has a
// hardcoded 60s HTTP client timeout for synchronous ExecuteCommand, and the
// session API does not reliably report exit codes for long-running interactive
// processes like opencode, so background execution is the only reliable path
// for builds that take longer than a minute.
func (r *SandboxRuntime) runCommand(ctx context.Context, command string, timeout time.Duration) (int, string, error) {
	donePath := r.config.ScratchDir + "/agent.done"
	exitCodePath := r.config.ScratchDir + "/agent.exitcode"
	outputPath := r.config.ScratchDir + "/agent.output"
	scriptPath := r.config.ScratchDir + "/agent-run.sh"
	launchLogPath := r.config.ScratchDir + "/agent.launch.log"

	// Write the command pipeline to a script: the command writes its output
	// to outputPath, its exit code to exitCodePath, and touches donePath
	// when finished. The poll loop below watches for donePath.
	script := command + " > " + shellQuote(outputPath) + " 2>&1\n" +
		"echo $? > " + shellQuote(exitCodePath) + "\n" +
		"touch " + shellQuote(donePath) + "\n"
	if err := r.config.Executor.UploadFile(ctx, []byte(script), scriptPath); err != nil {
		return 0, "", fmt.Errorf("agentruntime: upload agent run script: %w", err)
	}

	// Launch the script fully detached from the command session. A plain
	// `... &` is not enough on Daytona: the background job inherits the
	// session's stdout and stderr pipes, the toolbox waits for every writer
	// of those pipes to exit, and the launch call dies at the server-side
	// command timeout (Daytona error 408). setsid puts the script in its
	// own session, nohup ignores the hangup when the launching shell
	// exits, and redirecting all three streams means nothing the script
	// spawns holds the session's pipes open, so ExecuteCommand returns
	// immediately after backgrounding.
	launch := "setsid nohup /bin/sh " + shellQuote(scriptPath) +
		" < /dev/null > " + shellQuote(launchLogPath) + " 2>&1 &"
	if _, _, err := r.config.Executor.ExecuteCommand(ctx, launch, 30*time.Second); err != nil {
		return 0, "", fmt.Errorf("agentruntime: start agent command in background: %w", err)
	}

	deadline := time.Now().Add(timeout)
	poll := 2 * time.Second
	for i := 0; ; i++ {
		if err := ctx.Err(); err != nil {
			return 0, "", err
		}
		if time.Now().After(deadline) {
			return 0, "", fmt.Errorf("agentruntime: agent command still running after %v", timeout)
		}
		_, err := r.config.Executor.DownloadFile(ctx, donePath)
		if err == nil {
			break
		}
		if i%30 == 0 {
			log.Printf("agentruntime: agent command still running...")
		}
		select {
		case <-ctx.Done():
			return 0, "", ctx.Err()
		case <-time.After(poll):
		}
	}

	exitCodeData, err := r.config.Executor.DownloadFile(ctx, exitCodePath)
	exitCode := 1
	if err == nil {
		if n, err := fmt.Sscanf(string(exitCodeData), "%d", &exitCode); err != nil || n != 1 {
			exitCode = 1
		}
	}

	outputData, err := r.config.Executor.DownloadFile(ctx, outputPath)
	output := ""
	if err == nil {
		output = string(outputData)
	}

	return exitCode, output, nil
}

// agentCommand builds the shell command that runs the agent inside the
// sandbox: the working directory is the workspace, the environment carries
// only what the pinned runtime needs, and the prompt is quoted as one word.
func (r *SandboxRuntime) agentCommand(task TaskSpec, configPath, progressLog, prompt string) string {
	assignments := []string{
		"HOME=" + r.config.HomeDir,
		"OPENCODE_CONFIG=" + configPath,
		"OPENCODE_DISABLE_AUTOUPDATE=1",
		"OPENCODE_DISABLE_MODELS_FETCH=1",
		"OPENCODE_DISABLE_DEFAULT_PLUGINS=1",
		"OPENCODE_DISABLE_LSP_DOWNLOAD=1",
		"OPENCODE_DISABLE_CLAUDE_CODE=1",
		"OPENCODE_ENABLE_EXA=0",
		"OPENCODE_AUTO_SHARE=0",
		"CODEGYM_PROGRESS_LOG=" + progressLog,
	}
	quoted := make([]string, 0, len(assignments))
	for _, assignment := range assignments {
		key, value, _ := strings.Cut(assignment, "=")
		quoted = append(quoted, key+"="+shellQuote(value))
	}
	invocation := []string{
		shellQuote(r.config.BinaryPath), "run", "--format", "json", "--auto",
		"--agent", "codegym", "--model", "codegym-relay/" + r.config.Model,
		"--title", shellQuote("CodeGym agent task"), shellQuote(prompt),
	}
	return "cd " + shellQuote(task.WorkingDirectory) + " && env " + strings.Join(quoted, " ") + " " + strings.Join(invocation, " ")
}

// deliverProgress downloads the sandbox progress log and delivers it exactly
// as the host runtime does.
func (r *SandboxRuntime) deliverProgress(ctx context.Context, path string, result *RunResult) error {
	payload, err := r.config.Executor.DownloadFile(ctx, path)
	if err != nil {
		return fmt.Errorf("agentruntime: read sandbox progress log: %w", err)
	}
	scanner := bufio.NewScanner(strings.NewReader(string(payload)))
	scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event ProgressEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return fmt.Errorf("agentruntime: decode sandbox progress: %w", err)
		}
		if event.At.IsZero() {
			event.At = r.config.Now()
		}
		result.Telemetry.Progress = append(result.Telemetry.Progress, event)
		if r.config.Progress != nil {
			if err := r.config.Progress(ctx, event); err != nil {
				return fmt.Errorf("agentruntime: deliver sandbox progress: %w", err)
			}
		}
	}
	return scanner.Err()
}

// loadManifest downloads the agent result manifest from the sandbox and loads
// it through the shared loader so the claim matches the host path exactly.
func (r *SandboxRuntime) loadManifest(ctx context.Context, task TaskSpec) ManifestClaim {
	remotePath := strings.TrimRight(task.WorkingDirectory, "/") + "/" + ManifestRelativePath
	payload, err := r.config.Executor.DownloadFile(ctx, remotePath)
	if err != nil {
		return ManifestClaim{Status: ManifestAbsent, Error: "result manifest is absent"}
	}
	dir, err := os.MkdirTemp(r.config.ManifestDir, "codegym-sandbox-manifest-")
	if err != nil {
		return ManifestClaim{Status: ManifestMalformed, Error: fmt.Sprintf("stage result manifest: %v", err)}
	}
	defer func() { _ = os.RemoveAll(dir) }()
	staged := filepath.Join(dir, filepath.FromSlash(ManifestRelativePath))
	if err := os.MkdirAll(filepath.Dir(staged), 0o700); err != nil {
		return ManifestClaim{Status: ManifestMalformed, Error: fmt.Sprintf("stage result manifest: %v", err)}
	}
	if err := os.WriteFile(staged, payload, 0o600); err != nil {
		return ManifestClaim{Status: ManifestMalformed, Error: fmt.Sprintf("stage result manifest: %v", err)}
	}
	return LoadManifest(dir)
}

// shellQuote wraps one shell argument in single quotes, escaping embedded
// single quotes, so any prompt or path survives a POSIX shell unchanged.
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
