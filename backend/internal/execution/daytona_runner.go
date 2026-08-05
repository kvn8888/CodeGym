package execution

// DaytonaRunner is the production Runner backed by Daytona sandboxes. It is
// the ONLY file in the backend that imports the Daytona SDK — everything
// else talks to the Runner interface.
//
// The adapter follows the validated spike at spikes/daytona/main.go and is
// covered by the gated integration suite in daytona_runner_test.go:
//
//	doppler run -p codegym -c dev -- go test ./internal/execution -run TestDaytonaRunner -count=1

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/daytona/clients/sdk-go/pkg/daytona"
	"github.com/daytona/clients/sdk-go/pkg/options"
	"github.com/daytona/clients/sdk-go/pkg/types"
)

const (
	supervisorRemotePath   = "work/.codegym/supervisor.py"
	resultRemotePath       = "work/.codegym/result.json"
	casesRemotePath        = "work/.codegym/cases.jsonl"
	defaultOutputCapBytes  = 64 * 1024
	supervisorExecOverhead = 15 * time.Second
	timeoutReadbackBudget  = 5 * time.Second
	defaultCreateBackoff   = 250 * time.Millisecond
	sandboxCreateAttempts  = 2
)

var ErrInfrastructureBusy = errors.New("execution infrastructure is busy; please retry")

type DaytonaRunner struct {
	client        daytonaSandboxClient
	createBackoff time.Duration
}

type daytonaSandboxClient interface {
	Create(context.Context, types.SnapshotParams) (daytonaRunnerSandbox, error)
}

type daytonaRunnerSandbox interface {
	CreateFolder(context.Context, string) error
	UploadFile(context.Context, []byte, string) error
	DownloadFile(context.Context, string) ([]byte, error)
	ExecuteCommand(context.Context, string, time.Duration) (*types.ExecuteResponse, error)
	Delete(context.Context) error
}

type daytonaClientAdapter struct {
	client *daytona.Client
}

func (a daytonaClientAdapter) Create(ctx context.Context, params types.SnapshotParams) (daytonaRunnerSandbox, error) {
	sandbox, err := a.client.Create(ctx, params)
	if err != nil {
		return nil, err
	}
	return daytonaSandboxAdapter{sandbox: sandbox}, nil
}

type daytonaSandboxAdapter struct {
	sandbox *daytona.Sandbox
}

func (a daytonaSandboxAdapter) CreateFolder(ctx context.Context, path string) error {
	return a.sandbox.FileSystem.CreateFolder(ctx, path)
}

func (a daytonaSandboxAdapter) UploadFile(ctx context.Context, content []byte, path string) error {
	return a.sandbox.FileSystem.UploadFile(ctx, content, path)
}

func (a daytonaSandboxAdapter) DownloadFile(ctx context.Context, path string) ([]byte, error) {
	return a.sandbox.FileSystem.DownloadFile(ctx, path, nil)
}

func (a daytonaSandboxAdapter) ExecuteCommand(ctx context.Context, command string, timeout time.Duration) (*types.ExecuteResponse, error) {
	return a.sandbox.Process.ExecuteCommand(ctx, command, options.WithExecuteTimeout(timeout))
}

func (a daytonaSandboxAdapter) Delete(ctx context.Context) error {
	return a.sandbox.Delete(ctx)
}

// NewDaytonaRunner builds a runner from explicit credentials (loaded from
// config, not raw env, so tests and main stay explicit).
func NewDaytonaRunner(apiKey, apiURL string) (*DaytonaRunner, error) {
	client, err := daytona.NewClientWithConfig(&types.DaytonaConfig{
		APIKey: apiKey,
		APIUrl: apiURL,
	})
	if err != nil {
		return nil, fmt.Errorf("create Daytona client: %w", err)
	}
	return &DaytonaRunner{
		client:        daytonaClientAdapter{client: client},
		createBackoff: defaultCreateBackoff,
	}, nil
}

// Run executes one submission in an ephemeral, fully network-blocked sandbox
// and reports the outcome. Remember the contract from runner.go: a nonzero
// exit code is a NORMAL result (return it in RunOutcome with a nil error);
// a returned error means the infrastructure itself failed.
func (r *DaytonaRunner) Run(ctx context.Context, spec RunSpec) (outcome RunOutcome, err error) {
	if r == nil || r.client == nil {
		return RunOutcome{}, errors.New("Daytona runner is not initialized")
	}
	if err := validateLimits(spec.Limits); err != nil {
		return RunOutcome{}, err
	}
	strategy, err := normalizeTestStrategy(string(spec.Strategy))
	if err != nil {
		return RunOutcome{}, err
	}
	spec.Strategy = strategy

	startedAt := time.Now()
	stages := StageDurations{}
	lang := spec.Language.Name
	timeout := time.Duration(spec.Limits.TimeoutSeconds) * time.Second
	log.Printf("daytona run start language=%s entrypoint=%s files=%d timeout=%s memory_mb=%d network_mode=%s",
		lang, spec.Entrypoint, len(spec.Files), timeout, spec.Limits.MemoryMB, spec.Limits.NetworkMode)

	createStarted := time.Now()
	sb, createAttempts, err := r.createSandbox(ctx, types.SnapshotParams{
		Snapshot: spec.Language.Snapshot,
		SandboxBaseParams: types.SandboxBaseParams{
			Labels:          map[string]string{"codegym": "submission"},
			NetworkBlockAll: spec.Limits.NetworkMode == NetworkModeBlockAll,
			Ephemeral:       true,
		},
	})
	stages.CreateMs = time.Since(createStarted).Milliseconds()
	stages.CreateRetried = createAttempts > 1
	if err != nil {
		log.Printf("daytona run failed stage=create language=%s elapsed_ms=%d err=%v",
			lang, time.Since(startedAt).Milliseconds(), err)
		stages.TotalMs = time.Since(startedAt).Milliseconds()
		return RunOutcome{StageDurations: stages}, err
	}
	createMS := stages.CreateMs
	defer func() {
		// Execution timeouts cancel ctx. Cleanup still needs a short,
		// independent window so a timed-out run cannot strand a billable
		// sandbox.
		cleanupStarted := time.Now()
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		if deleteErr := sb.Delete(cleanupCtx); deleteErr != nil {
			log.Printf("daytona cleanup failed language=%s elapsed_ms=%d err=%v",
				lang, time.Since(cleanupStarted).Milliseconds(), deleteErr)
			err = errors.Join(err, fmt.Errorf("delete Daytona sandbox: %w", deleteErr))
			return
		}
		log.Printf("daytona cleanup ok language=%s elapsed_ms=%d",
			lang, time.Since(cleanupStarted).Milliseconds())
	}()
	defer func() {
		stages.TotalMs = time.Since(startedAt).Milliseconds()
		outcome.StageDurations = stages
	}()

	uploadStarted := time.Now()
	uploadActive := true
	defer func() {
		if uploadActive {
			stages.UploadMs = time.Since(uploadStarted).Milliseconds()
		}
	}()
	if err := sb.CreateFolder(ctx, "work"); err != nil {
		log.Printf("daytona run failed stage=mkdir language=%s create_ms=%d elapsed_ms=%d err=%v",
			lang, createMS, time.Since(startedAt).Milliseconds(), err)
		return RunOutcome{}, fmt.Errorf("create Daytona work directory: %w", err)
	}
	if err := sb.CreateFolder(ctx, "work/.codegym"); err != nil {
		log.Printf("daytona run failed stage=mkdir-protocol language=%s create_ms=%d elapsed_ms=%d err=%v",
			lang, createMS, time.Since(startedAt).Milliseconds(), err)
		return RunOutcome{}, fmt.Errorf("create Daytona judge protocol directory: %w", err)
	}
	for _, file := range spec.Files {
		if err := sb.UploadFile(ctx, []byte(file.Content), "work/"+file.Path); err != nil {
			log.Printf("daytona run failed stage=upload language=%s create_ms=%d elapsed_ms=%d file=%q err=%v",
				lang, createMS, time.Since(startedAt).Milliseconds(), file.Path, err)
			return RunOutcome{}, fmt.Errorf("upload %q to Daytona sandbox: %w", file.Path, err)
		}
	}
	if err := sb.UploadFile(ctx, SupervisorScript, supervisorRemotePath); err != nil {
		log.Printf("daytona run failed stage=upload-supervisor language=%s create_ms=%d elapsed_ms=%d err=%v",
			lang, createMS, time.Since(startedAt).Milliseconds(), err)
		return RunOutcome{}, fmt.Errorf("upload judge supervisor to Daytona sandbox: %w", err)
	}
	stages.UploadMs = time.Since(uploadStarted).Milliseconds()
	uploadActive = false
	uploadMS := stages.UploadMs

	execStarted := time.Now()
	execResult, err := sb.ExecuteCommand(ctx, buildSupervisorCommand(spec), timeout+supervisorExecOverhead)
	stages.ExecMs = time.Since(execStarted).Milliseconds()
	execMS := stages.ExecMs
	if err != nil {
		if executionDeadlineExceeded(ctx, err) {
			return timeoutRunOutcome(ctx, sb, spec, execMS, lang, createMS, uploadMS, startedAt), nil
		}
		log.Printf("daytona run failed stage=exec language=%s create_ms=%d upload_ms=%d exec_ms=%d elapsed_ms=%d err=%v",
			lang, createMS, uploadMS, execMS, time.Since(startedAt).Milliseconds(), err)
		return RunOutcome{}, fmt.Errorf("execute submission in Daytona sandbox: %w", err)
	}

	resultData, err := sb.DownloadFile(ctx, resultRemotePath)
	if err != nil {
		log.Printf("daytona run failed stage=download-result language=%s supervisor_exit_code=%d elapsed_ms=%d err=%v",
			lang, execResult.ExitCode, time.Since(startedAt).Milliseconds(), err)
		return RunOutcome{}, fmt.Errorf("download judge result from Daytona sandbox: %w", err)
	}
	judgeResult, err := ParseJudgeResult(resultData)
	if err != nil {
		return RunOutcome{}, err
	}
	exitCode := -1
	if judgeResult.ExitCode != nil {
		exitCode = *judgeResult.ExitCode
	}

	total := time.Since(startedAt)
	log.Printf("daytona run ok language=%s entrypoint=%s status=%s exit_code=%d create_ms=%d upload_ms=%d exec_ms=%d total_ms=%d output_truncated=%t",
		lang, spec.Entrypoint, judgeResult.Status, exitCode, createMS, uploadMS, execMS, total.Milliseconds(), judgeResult.OutputTruncated)

	return RunOutcome{
		ExitCode: exitCode,
		Result:   judgeResult,
		Stdout:   judgeResult.Stdout,
		Stderr:   judgeResult.Stderr,
		Output:   judgeResult.Stdout + judgeResult.Stderr,
		Duration: time.Duration(judgeResult.DurationMs) * time.Millisecond,
	}, nil
}

func (r *DaytonaRunner) createSandbox(ctx context.Context, params types.SnapshotParams) (daytonaRunnerSandbox, int, error) {
	var lastErr error
	attempts := 0
	for attempt := 1; attempt <= sandboxCreateAttempts; attempt++ {
		attempts = attempt
		sandbox, err := r.client.Create(ctx, params)
		if err == nil {
			return sandbox, attempts, nil
		}
		lastErr = err
		log.Printf("daytona sandbox create attempt failed attempt=%d/%d err=%v", attempt, sandboxCreateAttempts, err)
		if attempt == sandboxCreateAttempts {
			break
		}
		if r.createBackoff <= 0 {
			continue
		}
		timer := time.NewTimer(r.createBackoff)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			lastErr = ctx.Err()
			return nil, attempts, fmt.Errorf("%w: sandbox creation failed after %d attempts: %v",
				ErrInfrastructureBusy, attempts, lastErr)
		case <-timer.C:
		}
	}
	return nil, attempts, fmt.Errorf("%w: sandbox creation failed after %d attempts: %v",
		ErrInfrastructureBusy, attempts, lastErr)
}

func executionDeadlineExceeded(ctx context.Context, err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return true
	}
	// The Daytona SDK converts toolbox transport errors into its own error type
	// without preserving the wrapped context error. Keep this check scoped to
	// the execute stage so create/upload/API failures still remain platform
	// errors.
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "deadline exceeded") ||
		strings.Contains(message, "timed out") ||
		strings.Contains(message, "timeout")
}

func timeoutRunOutcome(
	ctx context.Context,
	sandbox daytonaRunnerSandbox,
	spec RunSpec,
	execMS int64,
	language string,
	createMS int64,
	uploadMS int64,
	startedAt time.Time,
) RunOutcome {
	readbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeoutReadbackBudget)
	defer cancel()
	casesJSONL, downloadErr := sandbox.DownloadFile(readbackCtx, casesRemotePath)
	if downloadErr != nil {
		casesJSONL = nil
		log.Printf("daytona timeout progress unavailable language=%s elapsed_ms=%d err=%v",
			language, time.Since(startedAt).Milliseconds(), downloadErr)
	}
	detail := "run exceeded its overall execution budget before a final judge result was written"
	judgeResult := ReconstructJudgeResult(ReconstructionInput{
		CasesJSONL:    casesJSONL,
		DeathStatus:   JudgeStatusTimeout,
		FailureDetail: detail,
		DurationMs:    execMS,
	})
	log.Printf("daytona run timeout language=%s entrypoint=%s status=%s create_ms=%d upload_ms=%d exec_ms=%d total_ms=%d progress_recovered=%t",
		language, spec.Entrypoint, judgeResult.Status, createMS, uploadMS, execMS,
		time.Since(startedAt).Milliseconds(), downloadErr == nil)
	return outcomeFromJudgeResult(judgeResult)
}

func outcomeFromJudgeResult(judgeResult JudgeResult) RunOutcome {
	exitCode := -1
	if judgeResult.ExitCode != nil {
		exitCode = *judgeResult.ExitCode
	}
	return RunOutcome{
		ExitCode: exitCode,
		Result:   judgeResult,
		Stdout:   judgeResult.Stdout,
		Stderr:   judgeResult.Stderr,
		Output:   judgeResult.Stdout + judgeResult.Stderr,
		Duration: time.Duration(judgeResult.DurationMs) * time.Millisecond,
	}
}

func buildSupervisorCommand(spec RunSpec) string {
	args := []string{
		"python3", ".codegym/supervisor.py",
		"--work-dir", ".codegym",
		"--timeout-seconds", strconv.Itoa(spec.Limits.TimeoutSeconds),
		"--memory-mb", strconv.Itoa(spec.Limits.MemoryMB),
		"--output-cap-bytes", strconv.Itoa(defaultOutputCapBytes),
		"--",
	}
	args = append(args, childCommandForStrategy(spec.Language, spec.Strategy, spec.Entrypoint)...)
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, shellQuote(arg))
	}
	return "cd ~/work && " + strings.Join(quoted, " ")
}

func childCommandForStrategy(language Language, strategy TestStrategy, entrypoint string) []string {
	switch strategy {
	case TestStrategyHTTP:
		// HTTP entrypoints are server-owned compile-and-harness launchers. They
		// use the same language command seam while selecting a different hidden
		// artifact in submission assembly.
		return language.ChildCommand(entrypoint)
	case TestStrategyUnit, "":
		return language.ChildCommand(entrypoint)
	default:
		return nil
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
