package submission

import "github.com/kvn8888/codegym/backend/internal/execution"

type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusError     Status = "error"
)

type SubmitInput struct {
	ProblemID string           `json:"problem_id"`
	SessionID string           `json:"session_id,omitempty"`
	Files     []execution.File `json:"files"`
}

type Accepted struct {
	SubmissionID string `json:"submission_id"`
}

type View struct {
	Status Status      `json:"status"`
	Result *TestResult `json:"result,omitempty"`
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
