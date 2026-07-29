package intake

import (
	"encoding/json"
	"time"

	"github.com/kvn8888/codegym/backend/internal/generation"
)

type Status string

const (
	StatusPending   Status = "pending"
	StatusCompleted Status = "completed"
	StatusSkipped   Status = "skipped"
)

type Record struct {
	ID                string                      `json:"id"`
	WorkspaceID       string                      `json:"workspace_id"`
	UserID            string                      `json:"user_id"`
	NormalizedTopic   string                      `json:"normalized_topic"`
	OriginalTopic     string                      `json:"original_topic"`
	PracticeSeed      json.RawMessage             `json:"practice_seed"`
	Questions         []generation.IntakeQuestion `json:"questions"`
	Answers           map[string]string           `json:"answers"`
	Status            Status                      `json:"status"`
	SuppressionReason string                      `json:"suppression_reason,omitempty"`
	GenerationError   string                      `json:"generation_error,omitempty"`
	CreatedAt         time.Time                   `json:"created_at"`
	UpdatedAt         time.Time                   `json:"updated_at"`
	CompletedAt       *time.Time                  `json:"completed_at,omitempty"`
}

type PrepareInput struct {
	Topic        string          `json:"topic"`
	PracticeSeed json.RawMessage `json:"practice_seed"`
	Restart      bool            `json:"restart,omitempty"`
}

type UpdateInput struct {
	Answers map[string]string `json:"answers,omitempty"`
	Status  *Status           `json:"status,omitempty"`
}
