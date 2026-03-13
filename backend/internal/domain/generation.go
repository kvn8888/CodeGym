package domain

import "time"

type GenerationStatus string

const (
	GenerationStatusPending    GenerationStatus = "pending"
	GenerationStatusGenerating GenerationStatus = "generating"
	GenerationStatusValidating GenerationStatus = "validating"
	GenerationStatusReady      GenerationStatus = "ready"
	GenerationStatusFailed     GenerationStatus = "failed"
)

type GenerationJob struct {
	ID          string           `json:"id"`
	UserID      string           `json:"user_id"`
	Prompt      string           `json:"prompt"`
	Status      GenerationStatus `json:"status"`
	ProblemID   *string          `json:"problem_id,omitempty"`
	Error       *string          `json:"error,omitempty"`
	Attempts    int              `json:"attempts"`
	CreatedAt   time.Time        `json:"created_at"`
	CompletedAt *time.Time       `json:"completed_at,omitempty"`
}
