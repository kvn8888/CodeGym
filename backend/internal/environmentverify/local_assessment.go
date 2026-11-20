package environmentverify

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/kvn8888/codegym/backend/internal/procgroup"
)

// DefaultLocalAssessmentOutputCap bounds captured developer-machine
// assessment output. Reports carry structured outcomes, not unbounded logs.
const DefaultLocalAssessmentOutputCap = 64 * 1024

// ErrAssessmentOutputExceeded marks an assessment command that produced more
// output than the configured ceiling. The command and its process group are
// terminated, partial test evidence is discarded, and the caller treats the
// run as an execution failure rather than an assessment verdict.
var ErrAssessmentOutputExceeded = errors.New("environmentverify: assessment output exceeded the configured ceiling")

// LocalProtocolAssessor runs a manifest-provided test command on the local
// machine and parses the harness's CODEGYM_TEST protocol. It is
// technology-neutral: the shell command comes from the artifact manifest and
// no language-specific dispatch happens here. It is intended for local
// development and CI only, not as a sandbox.
type LocalProtocolAssessor struct {
	MaxOutputBytes int
}

// Assess runs the test command in the prepared workspace, honoring context
// cancellation by terminating the whole process group. A nonzero exit is a
// normal assessment result; only cancellation, timeout, or infrastructure
// faults return an error.
func (a *LocalProtocolAssessor) Assess(ctx context.Context, work AssessmentWork) (AssessmentReport, error) {
	command := strings.TrimSpace(work.TestCommand)
	if command == "" {
		return AssessmentReport{}, fmt.Errorf("environmentverify: manifest declares no test command")
	}
	started := time.Now()
	exitCode, output, timedOut, exhausted, err := runLocalAssessmentCommand(ctx, command, work.WorkDir, a.outputCap())
	elapsed := time.Since(started).Milliseconds()
	if err != nil {
		return AssessmentReport{}, err
	}
	report := AssessmentReport{
		Tests:     ParseTestOutcomes(output),
		ExitCode:  intPointer(exitCode),
		ElapsedMS: int64Pointer(elapsed),
	}
	if exhausted {
		// The command was killed mid-stream. Any protocol lines captured
		// before the ceiling are incomplete evidence, so none are accepted.
		report.Tests = []TestOutcome{}
		report.OutputTail = tailAssessmentOutput(output, 1000)
		return report, ErrAssessmentOutputExceeded
	}
	if timedOut {
		if report.Tests == nil {
			report.Tests = []TestOutcome{}
		}
		report.OutputTail = tailAssessmentOutput(output, 1000)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return report, ctxErr
		}
		return report, context.DeadlineExceeded
	}
	if len(report.Tests) == 0 {
		report.OutputTail = tailAssessmentOutput(output, 1000)
	}
	return report, nil
}

func (a *LocalProtocolAssessor) outputCap() int {
	if a != nil && a.MaxOutputBytes > 0 {
		return a.MaxOutputBytes
	}
	return DefaultLocalAssessmentOutputCap
}

// runLocalAssessmentCommand executes one shell command with process-group
// termination so cancellation kills orphaned children too. Output is captured
// through a bounded buffer: crossing the ceiling cancels the command context,
// which kills the whole process group rather than letting the child write
// without limit. A nonzero exit is a normal result; only infrastructure
// faults return an error. exhausted reports that the ceiling, not the command,
// ended the run.
func runLocalAssessmentCommand(ctx context.Context, command, workDir string, capBytes int) (exitCode int, output string, timedOut, exhausted bool, err error) {
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
	runErr := cmd.Run()
	output = buffer.String()
	if buffer.Truncated() {
		return -1, output, false, true, nil
	}
	if ctx.Err() == context.DeadlineExceeded {
		return -1, output, true, false, nil
	}
	if runErr != nil {
		var exitError *exec.ExitError
		if errors.As(runErr, &exitError) {
			return exitError.ExitCode(), output, false, false, nil
		}
		if ctx.Err() != nil {
			return -1, output, true, false, nil
		}
		return 0, "", false, false, runErr
	}
	return 0, output, false, false, nil
}

func tailAssessmentOutput(output string, limit int) string {
	output = strings.TrimSpace(output)
	if limit <= 0 || len(output) <= limit {
		return output
	}
	return output[len(output)-limit:]
}
