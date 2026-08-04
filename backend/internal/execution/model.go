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
	// These statuses are supervisor-reported user-code deaths, not platform
	// faults. Partial case progress remains available in Run.JudgeResult.
	StatusTimeout     Status = "timeout"
	StatusOutOfMemory Status = "out_of_memory"
	StatusCrashed     Status = "crashed"
	// StatusError means the runner itself failed (sandbox create, upload,
	// result readback, …); details are in Run.Error.
	StatusError Status = "error"
)

// File is one file of a submission. Path is relative (it becomes a
// home-relative path inside the sandbox, e.g. "solution.py").
type File struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

const NetworkModeBlockAll = "block-all"

// Limits are mandatory per-run capabilities supplied by the problem runtime.
// They intentionally have no permissive defaults: zero values are rejected so
// callers cannot silently drop a problem's execution policy.
type Limits struct {
	TimeoutSeconds int    `json:"timeout_seconds"`
	MemoryMB       int    `json:"memory_mb"`
	NetworkMode    string `json:"network_mode"`
}

// Run is one execution of a submission in an isolated sandbox.
//
// Per-case test counts (v1 had total_tests/passed_tests) are deferred until
// a structured test-output protocol exists; Output carries the raw combined
// stdout+stderr for now.
type Run struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	UserID      string `json:"user_id"`
	// ProblemID references the global problem catalog when the run comes
	// through /submissions; raw /executions callers may use another label.
	ProblemID  string `json:"problem_id,omitempty"`
	Language   string `json:"language"`
	Entrypoint string `json:"entrypoint"`
	// Files are persisted for execution diagnostics but never serialized:
	// submission runs include server-only hidden tests.
	Files    []File `json:"-"`
	Status   Status `json:"status"`
	ExitCode *int   `json:"exit_code,omitempty"`
	Output   string `json:"output"`
	// JudgeResult is persisted for submission views but never serialized from
	// raw execution endpoints; it can contain server-owned hidden case names.
	JudgeResult *JudgeResult `json:"-"`
	Error       string       `json:"error,omitempty"`
	DurationMs  int64        `json:"duration_ms"`
	CreatedAt   time.Time    `json:"created_at"`
	CompletedAt *time.Time   `json:"completed_at,omitempty"`
}

// SubmitRunInput is the request payload for running a submission.
type SubmitRunInput struct {
	ProblemID  string `json:"problem_id"`
	Language   string `json:"language"`
	Entrypoint string `json:"entrypoint"`
	Files      []File `json:"files"`
	Limits     Limits `json:"limits"`
}
