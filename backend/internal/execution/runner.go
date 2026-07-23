package execution

import (
	"context"
	"time"
)

// RunSpec is a validated, language-resolved submission ready to execute.
type RunSpec struct {
	Language   Language
	Files      []File
	Entrypoint string
}

// RunOutcome is the result of executing a submission in a sandbox.
type RunOutcome struct {
	ExitCode int
	// Output is the combined stdout+stderr of the run command.
	Output string
	// Duration is the sandbox-side wall time (create through exec).
	Duration time.Duration
}

// Runner executes one submission in an isolated sandbox.
//
// A returned error means infrastructure failure (the run gets StatusError);
// a nonzero RunOutcome.ExitCode is a normal result (StatusFailed).
type Runner interface {
	Run(ctx context.Context, spec RunSpec) (RunOutcome, error)
}
