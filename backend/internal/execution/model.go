package execution

import "time"

// Status is the lifecycle of a submission run. Runs are executed
// synchronously in v0, but every transition is persisted so the same shape
// works when execution moves behind a worker queue.
type Status string

const (
	StatusQueued  Status = "queued"
	StatusRunning Status = "running"
	// StatusPassed means the run command exited 0.
	StatusPassed Status = "passed"
	// StatusFailed means the run command exited nonzero — a normal result
	// (e.g. an assertion failure), not an infrastructure problem.
	StatusFailed Status = "failed"
	// StatusError means the runner itself failed (sandbox create, upload,
	// timeout, …); details are in Run.Error.
	StatusError Status = "error"
)

// File is one file of a submission. Path is relative (it becomes a
// home-relative path inside the sandbox, e.g. "solution.py").
type File struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// Run is one execution of a submission in an isolated sandbox.
//
// Per-case test counts (v1 had total_tests/passed_tests) are deferred until
// a structured test-output protocol exists; Output carries the raw combined
// stdout+stderr for now.
type Run struct {
	ID       string `json:"id"`
	TenantID string `json:"tenant_id"`
	UserID   string `json:"user_id"`
	// ProblemID is a free-form reference; there is no problems domain in v2 yet.
	ProblemID   string     `json:"problem_id,omitempty"`
	Language    string     `json:"language"`
	Entrypoint  string     `json:"entrypoint"`
	Files       []File     `json:"files"`
	Status      Status     `json:"status"`
	ExitCode    *int       `json:"exit_code,omitempty"`
	Output      string     `json:"output"`
	Error       string     `json:"error,omitempty"`
	DurationMs  int64      `json:"duration_ms"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// SubmitRunInput is the request payload for running a submission.
type SubmitRunInput struct {
	ProblemID  string `json:"problem_id"`
	Language   string `json:"language"`
	Entrypoint string `json:"entrypoint"`
	Files      []File `json:"files"`
}
