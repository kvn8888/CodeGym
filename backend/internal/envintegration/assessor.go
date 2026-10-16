package envintegration

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/kvn8888/codegym/backend/internal/environmentverify"
)

// TestOutcomeProtocol is the harness protocol generated tests speak. It
// lives in environmentverify so both the verification assessor and browser
// practice Submit evaluate the same lines; this alias keeps existing
// references compiling.
const TestOutcomeProtocol = environmentverify.TestOutcomeProtocol

// DefaultAssessorOutputCap bounds captured assessment output.
const DefaultAssessorOutputCap = 64 * 1024

// ParseTestOutcomes parses protocol lines into structured test outcomes.
// The parser lives in environmentverify; this wrapper preserves the
// existing call shape. Malformed lines are skipped; an empty input yields
// no outcomes, never a fabricated pass.
func ParseTestOutcomes(output string) []environmentverify.TestOutcome {
	return environmentverify.ParseTestOutcomes(output)
}

// LocalAssessor implements the assessment runner by executing the work's
// test command and parsing the outcome protocol. A returned error means
// timeout, cancellation, or platform fault, never a test failure.
type LocalAssessor struct {
	MaxOutputBytes int
}

func (a *LocalAssessor) outputCap() int {
	if a.MaxOutputBytes > 0 {
		return a.MaxOutputBytes
	}
	return DefaultAssessorOutputCap
}

// Assess runs one solution's tests and returns the parsed outcomes.
func (a *LocalAssessor) Assess(ctx context.Context, work environmentverify.AssessmentWork) (environmentverify.AssessmentReport, error) {
	if strings.TrimSpace(work.TestCommand) == "" {
		return environmentverify.AssessmentReport{}, fmt.Errorf("envintegration: assessment test command is required")
	}
	command := exec.CommandContext(ctx, "sh", "-c", work.TestCommand)
	if strings.TrimSpace(work.WorkDir) != "" {
		command.Dir = work.WorkDir
	}
	output, err := command.CombinedOutput()
	if len(output) > a.outputCap() {
		output = output[:a.outputCap()]
	}
	if ctx.Err() != nil {
		return environmentverify.AssessmentReport{}, ctx.Err()
	}
	outcomes := ParseTestOutcomes(string(output))
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			return environmentverify.AssessmentReport{}, err
		}
		if len(outcomes) == 0 {
			return environmentverify.AssessmentReport{Tests: []environmentverify.TestOutcome{{
				TestID:       "unstructured",
				Passed:       false,
				FailureClass: "unstructured",
				Message:      strings.TrimSpace(string(output)),
			}}}, nil
		}
		return environmentverify.AssessmentReport{Tests: outcomes}, nil
	}
	return environmentverify.AssessmentReport{Tests: outcomes}, nil
}
