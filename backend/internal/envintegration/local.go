package envintegration

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/kvn8888/codegym/backend/internal/environmentverify"
)

// DefaultLocalOutputCap bounds captured stage output for local verification.
const DefaultLocalOutputCap = 64 * 1024

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
// reported in the result; only infrastructure faults return errors, and a
// context deadline reports a timeout rather than failing the stage silently.
// The command runs in its own process group so cancellation kills orphaned
// children too: without this, a killed shell's surviving children hold the
// output pipes open and the call outlives its own timeout. This is Unix-only,
// matching the runtime package; production verification runs in Daytona.
func (r *LocalCommandRunner) Run(ctx context.Context, spec environmentverify.CommandSpec) (environmentverify.CommandResult, error) {
	r.Calls = append(r.Calls, spec)
	command := exec.CommandContext(ctx, "sh", "-c", spec.Command)
	if strings.TrimSpace(spec.WorkDir) != "" {
		command.Dir = spec.WorkDir
	}
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
	output, err := command.CombinedOutput()
	if len(output) > r.outputCap() {
		output = output[:r.outputCap()]
	}
	if ctx.Err() == context.DeadlineExceeded {
		return environmentverify.CommandResult{ExitCode: -1, Output: string(output), TimedOut: true}, nil
	}
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			return environmentverify.CommandResult{ExitCode: exitError.ExitCode(), Output: string(output)}, nil
		}
		if ctx.Err() != nil {
			return environmentverify.CommandResult{ExitCode: -1, Output: string(output), TimedOut: true}, nil
		}
		return environmentverify.CommandResult{}, err
	}
	return environmentverify.CommandResult{ExitCode: 0, Output: string(output)}, nil
}
