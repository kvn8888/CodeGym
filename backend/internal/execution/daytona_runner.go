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
	defaultOutputCapBytes  = 64 * 1024
	supervisorExecOverhead = 15 * time.Second
)

type DaytonaRunner struct {
	client *daytona.Client
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
	return &DaytonaRunner{client: client}, nil
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
	lang := spec.Language.Name
	timeout := time.Duration(spec.Limits.TimeoutSeconds) * time.Second
	log.Printf("daytona run start language=%s entrypoint=%s files=%d timeout=%s memory_mb=%d network_mode=%s",
		lang, spec.Entrypoint, len(spec.Files), timeout, spec.Limits.MemoryMB, spec.Limits.NetworkMode)

	createStarted := time.Now()
	sb, err := r.client.Create(ctx, types.SnapshotParams{
		Snapshot: spec.Language.Snapshot,
		SandboxBaseParams: types.SandboxBaseParams{
			Labels:          map[string]string{"codegym": "submission"},
			NetworkBlockAll: spec.Limits.NetworkMode == NetworkModeBlockAll,
			Ephemeral:       true,
		},
	})
	if err != nil {
		log.Printf("daytona run failed stage=create language=%s elapsed_ms=%d err=%v",
			lang, time.Since(startedAt).Milliseconds(), err)
		return RunOutcome{}, fmt.Errorf("create Daytona sandbox: %w", err)
	}
	createMS := time.Since(createStarted).Milliseconds()
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

	uploadStarted := time.Now()
	if err := sb.FileSystem.CreateFolder(ctx, "work"); err != nil {
		log.Printf("daytona run failed stage=mkdir language=%s create_ms=%d elapsed_ms=%d err=%v",
			lang, createMS, time.Since(startedAt).Milliseconds(), err)
		return RunOutcome{}, fmt.Errorf("create Daytona work directory: %w", err)
	}
	if err := sb.FileSystem.CreateFolder(ctx, "work/.codegym"); err != nil {
		log.Printf("daytona run failed stage=mkdir-protocol language=%s create_ms=%d elapsed_ms=%d err=%v",
			lang, createMS, time.Since(startedAt).Milliseconds(), err)
		return RunOutcome{}, fmt.Errorf("create Daytona judge protocol directory: %w", err)
	}
	for _, file := range spec.Files {
		if err := sb.FileSystem.UploadFile(ctx, []byte(file.Content), "work/"+file.Path); err != nil {
			log.Printf("daytona run failed stage=upload language=%s create_ms=%d elapsed_ms=%d file=%q err=%v",
				lang, createMS, time.Since(startedAt).Milliseconds(), file.Path, err)
			return RunOutcome{}, fmt.Errorf("upload %q to Daytona sandbox: %w", file.Path, err)
		}
	}
	if err := sb.FileSystem.UploadFile(ctx, SupervisorScript, supervisorRemotePath); err != nil {
		log.Printf("daytona run failed stage=upload-supervisor language=%s create_ms=%d elapsed_ms=%d err=%v",
			lang, createMS, time.Since(startedAt).Milliseconds(), err)
		return RunOutcome{}, fmt.Errorf("upload judge supervisor to Daytona sandbox: %w", err)
	}
	uploadMS := time.Since(uploadStarted).Milliseconds()

	execStarted := time.Now()
	execResult, err := sb.Process.ExecuteCommand(
		ctx,
		buildSupervisorCommand(spec),
		options.WithExecuteTimeout(timeout+supervisorExecOverhead),
	)
	execMS := time.Since(execStarted).Milliseconds()
	if err != nil {
		log.Printf("daytona run failed stage=exec language=%s create_ms=%d upload_ms=%d exec_ms=%d elapsed_ms=%d err=%v",
			lang, createMS, uploadMS, execMS, time.Since(startedAt).Milliseconds(), err)
		return RunOutcome{}, fmt.Errorf("execute submission in Daytona sandbox: %w", err)
	}

	resultData, err := sb.FileSystem.DownloadFile(ctx, resultRemotePath, nil)
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
