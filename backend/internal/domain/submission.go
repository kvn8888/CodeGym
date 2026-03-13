package domain

import "time"

type SubmissionStatus string

const (
	SubmissionStatusPending SubmissionStatus = "pending"
	SubmissionStatusRunning SubmissionStatus = "running"
	SubmissionStatusPass    SubmissionStatus = "pass"
	SubmissionStatusFail    SubmissionStatus = "fail"
	SubmissionStatusError   SubmissionStatus = "error"
	SubmissionStatusTimeout SubmissionStatus = "timeout"
)

type Submission struct {
	ID          string           `json:"id"`
	UserID      string           `json:"user_id"`
	ProblemID   string           `json:"problem_id"`
	Status      SubmissionStatus `json:"status"`
	Language    string           `json:"language"`
	SubmittedAt time.Time        `json:"submitted_at"`
	CompletedAt *time.Time       `json:"completed_at,omitempty"`
	DurationMS  *int             `json:"duration_ms,omitempty"`
	TotalTests  *int             `json:"total_tests,omitempty"`
	PassedTests *int             `json:"passed_tests,omitempty"`
	Result      *TestResult      `json:"result,omitempty"`
}

type SubmissionFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type TestResult struct {
	Status       SubmissionStatus  `json:"status"`
	Total        int               `json:"total"`
	Passed       int               `json:"passed"`
	Failed       int               `json:"failed"`
	DurationMS   int               `json:"duration_ms"`
	TestCases    []TestCaseResult  `json:"test_cases"`
	CompileError *string           `json:"compile_error,omitempty"`
	Stderr       string            `json:"stderr,omitempty"`
	Stdout       string            `json:"stdout,omitempty"`
}

type TestCaseResult struct {
	Name       string `json:"name"`
	Status     string `json:"status"` // "pass", "fail", "error", "skip"
	DurationMS int    `json:"duration_ms"`
	Expected   string `json:"expected,omitempty"`
	Actual     string `json:"actual,omitempty"`
	Output     string `json:"output,omitempty"`
	Error      string `json:"error,omitempty"`
}

// ExecutionEvent is streamed over WebSocket during execution.
type ExecutionEvent struct {
	Type    string `json:"type"` // "log", "test_case", "compile_error", "done"
	Payload any    `json:"payload"`
}
