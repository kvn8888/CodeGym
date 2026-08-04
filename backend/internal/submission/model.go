package submission

import "github.com/kvn8888/codegym/backend/internal/execution"

type Status string

type Mode string

const (
	StatusPending     Status = "pending"
	StatusRunning     Status = "running"
	StatusCompleted   Status = "completed"
	StatusTimeout     Status = "timeout"
	StatusOutOfMemory Status = "out_of_memory"
	StatusCrashed     Status = "crashed"
	StatusError       Status = "error"
)

const (
	ModeRun    Mode = "run"
	ModeSubmit Mode = "submit"
)

type SubmitInput struct {
	ProblemID string           `json:"problem_id"`
	SessionID string           `json:"session_id,omitempty"`
	Files     []execution.File `json:"files"`
	Mode      Mode             `json:"mode,omitempty"`
}

type Accepted struct {
	SubmissionID       string `json:"submission_id"`
	MemoryUpdateStatus string `json:"memory_update_status,omitempty"`
}

type View struct {
	Status          Status      `json:"status"`
	Mode            Mode        `json:"mode"`
	ExecutedCount   int         `json:"executed_count"`
	Result          *TestResult `json:"result,omitempty"`
	FailureDetail   *string     `json:"failure_detail,omitempty"`
	Stdout          string      `json:"stdout"`
	OutputTruncated bool        `json:"output_truncated"`
}

type TestResult struct {
	Status       string           `json:"status"`
	Total        int              `json:"total"`
	Passed       int              `json:"passed"`
	Failed       int              `json:"failed"`
	DurationMs   int64            `json:"duration_ms"`
	TestCases    []TestCaseResult `json:"test_cases"`
	CompileError *string          `json:"compile_error,omitempty"`
}

type TestCaseResult struct {
	Name       string  `json:"name"`
	Status     string  `json:"status"`
	DurationMs int64   `json:"duration_ms"`
	Error      *string `json:"error,omitempty"`
}
