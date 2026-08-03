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
	Limits     Limits
}

// RunOutcome is the result of executing a submission in a sandbox.
type RunOutcome struct {
	ExitCode int
	// Result is the authoritative out-of-band supervisor result. A zero Schema
	// means a compatibility Runner returned only the legacy fields below.
	Result JudgeResult
	Stdout string
	Stderr string
	// Output retains combined stdout+stderr compatibility for callers that have
	// not moved to Result yet. New code should use Result and the split streams.
	Output string
	// Duration is the child-process wall time reported by the supervisor.
	Duration time.Duration
}

// Runner executes one submission in an isolated sandbox.
//
// A returned error means infrastructure failure (the run gets StatusError);
// a nonzero RunOutcome.ExitCode is a normal result (StatusFailed).
type Runner interface {
	Run(ctx context.Context, spec RunSpec) (RunOutcome, error)
}
