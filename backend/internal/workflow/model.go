package workflow

import "time"

type Kind string

const (
	KindMCQGeneration    Kind = "mcq_generation"
	KindMemoryReflection Kind = "memory_reflection"
	KindMCQNextRound     Kind = "mcq_next_round"
)

type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
)

type Operation struct {
	ID           string     `json:"id"`
	WorkspaceID  string     `json:"workspace_id"`
	UserID       string     `json:"user_id"`
	Kind         Kind       `json:"kind"`
	Status       Status     `json:"status"`
	LastSequence int64      `json:"last_sequence"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
}

type Event struct {
	OperationID string         `json:"operation_id"`
	Sequence    int64          `json:"sequence"`
	StepID      string         `json:"step_id"`
	Label       string         `json:"label"`
	Status      Status         `json:"status"`
	Timestamp   time.Time      `json:"timestamp"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type CreateInput struct {
	Kind Kind `json:"kind"`
}

type CreateResult struct {
	Operation Operation `json:"operation"`
	Events    []Event   `json:"events"`
}

type stepDefinition struct {
	ID    string
	Label string
}

var kindSteps = map[Kind][]stepDefinition{
	KindMCQGeneration: {
		{ID: "load_context", Label: "Load personalization"},
		{ID: "generate_questions", Label: "Generate questions"},
		{ID: "validate_questions", Label: "Validate question set"},
		{ID: "repair_questions", Label: "Repair invalid output if needed"},
		{ID: "questions_ready", Label: "Question set ready"},
	},
	KindMemoryReflection: {
		{ID: "load_evidence", Label: "Load learning evidence"},
		{ID: "synthesize_profile", Label: "Update memory profile"},
		{ID: "validate_profile", Label: "Validate memory update"},
		{ID: "save_profile", Label: "Save updated memory"},
		{ID: "memory_ready", Label: "Memory update ready"},
	},
	KindMCQNextRound: {
		{ID: "load_evidence", Label: "Load learning evidence"},
		{ID: "synthesize_profile", Label: "Update memory profile"},
		{ID: "validate_profile", Label: "Validate memory update"},
		{ID: "save_profile", Label: "Save updated memory"},
		{ID: "load_context", Label: "Load refreshed personalization"},
		{ID: "generate_questions", Label: "Generate next questions"},
		{ID: "validate_questions", Label: "Validate question set"},
		{ID: "repair_questions", Label: "Repair invalid output if needed"},
		{ID: "questions_ready", Label: "Next question set ready"},
	},
}
