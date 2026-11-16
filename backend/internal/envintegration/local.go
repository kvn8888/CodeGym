package envintegration

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"

	"github.com/kvn8888/codegym/backend/internal/environmentverify"
	"github.com/kvn8888/codegym/backend/internal/procgroup"
)

// DefaultLocalOutputCap bounds captured stage output for local verification.
const DefaultLocalOutputCap = 64 * 1024

// ErrLocalOutputExceeded marks a stage command that produced more output than
// the configured ceiling. The process group is terminated and the caller
// treats the run as an execution failure rather than a normal stage exit.
var ErrLocalOutputExceeded = errors.New("envintegration: local command output exceeded the configured ceiling")

// LocalCommandRunner executes stage commands on the host for development and
// CI verification. Production verification runs inside sandboxes through the
// Daytona-backed runner; this runner exists so the handoff is testable
// without one. It honors context cancellation and reports nonzero exits as
// normal stage failures.
type LocalCommandRunner struct {
	// Calls records every executed spec in order. Tests use it to prove
	// rejected runs execute nothing.
	Calls          []environmentverify.CommandSpec
	MaxOutputBytes int
}

func (r *LocalCommandRunner) outputCap() int {
	if r.MaxOutputBytes > 0 {
		return r.MaxOutputBytes
	}
	return DefaultLocalOutputCap
}

// Run executes one stage command. A nonzero exit is a normal stage failure
// reported in the result; infrastructure faults and output-ceiling exhaustion
// return errors, and a context deadline reports a timeout rather than failing
// the stage silently.
// The command runs in its own process group so cancellation kills orphaned
// children too: without this, a killed shell's surviving children hold the
// output pipes open and the call outlives its own timeout. This is Unix-only,
// matching the runtime package; production verification runs in Daytona.
func (r *LocalCommandRunner) Run(ctx context.Context, spec environmentverify.CommandSpec) (environmentverify.CommandResult, error) {
	r.Calls = append(r.Calls, spec)
	workDir := ""
	if strings.TrimSpace(spec.WorkDir) != "" {
		workDir = spec.WorkDir
	}
	exitCode, output, timedOut, err := runLocalStageCommand(ctx, spec.Command, workDir, r.outputCap())
	if err != nil {
		return environmentverify.CommandResult{}, err
	}
	return environmentverify.CommandResult{ExitCode: exitCode, Output: output, TimedOut: timedOut}, nil
}

// runLocalStageCommand executes one shell command with process-group
// termination so cancellation kills orphaned children too. A nonzero exit is
// a normal result; only infrastructure faults return errors.
func runLocalStageCommand(ctx context.Context, command, workDir string, capBytes int) (int, string, bool, error) {
	if capBytes <= 0 {
		capBytes = DefaultLocalOutputCap
	}
	commandCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	buffer := procgroup.NewCappedBuffer(capBytes, cancel)
	cmd := exec.CommandContext(commandCtx, "sh", "-c", command)
	if strings.TrimSpace(workDir) != "" {
		cmd.Dir = workDir
	}
	cmd.Stdout = buffer
	cmd.Stderr = buffer
	procgroup.Setup(cmd)
	cmd.WaitDelay = 2 * time.Second
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return procgroup.Kill(cmd)
	}
	err := cmd.Run()
	output := buffer.String()
	if buffer.Truncated() {
		return -1, output, false, ErrLocalOutputExceeded
	}
	if ctx.Err() == context.DeadlineExceeded {
		return -1, output, true, nil
	}
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			return exitError.ExitCode(), output, false, nil
		}
		if ctx.Err() != nil {
			return -1, output, true, nil
		}
		return 0, "", false, err
	}
	return 0, output, false, nil
}
