package memory

import (
	"encoding/json"
	"time"
)

// Profile is the long-lived memory summary for a user within a workspace.
type Profile struct {
	Summary      string             `json:"summary"`
	UpdatedAt    time.Time          `json:"updated_at"`
	NextReviewAt time.Time          `json:"next_review_at"`
	Strengths    []string           `json:"strengths"`
	GrowthEdges  []string           `json:"growth_edges"`
	Skills       []SkillProficiency `json:"skills"`
	Notes        []Note             `json:"notes"`
}

// SkillProficiency captures skill-level observations in the user profile.
type SkillProficiency struct {
	ID            string    `json:"id"`
	Label         string    `json:"label"`
	Area          string    `json:"area"`
	Level         int       `json:"level"`
	Confidence    int       `json:"confidence"`
	Trend         string    `json:"trend"`
	LastPracticed time.Time `json:"last_practiced"`
}

// Note stores problem-specific learning notes linked to user activity.
type Note struct {
	ID        string    `json:"id"`
	ProblemID string    `json:"problem_id"`
	Title     string    `json:"title"`
	Summary   string    `json:"summary"`
	CreatedAt time.Time `json:"created_at"`
	Tags      []string  `json:"tags"`
	Action    string    `json:"action"`
}

// Event is an append-only memory event emitted from product surfaces.
type Event struct {
	ID         string          `json:"id"`
	WorkspaceID   string          `json:"workspace_id"`
	UserID     string          `json:"user_id"`
	Source     string          `json:"source"`
	Type       string          `json:"type"`
	Summary    string          `json:"summary"`
	Payload    json.RawMessage `json:"payload,omitempty"`
	OccurredAt time.Time       `json:"occurred_at"`
	CreatedAt  time.Time       `json:"created_at"`
}

// RecordEventInput is the HTTP/service input payload for appending events.
type RecordEventInput struct {
	Source     string          `json:"source"`
	Type       string          `json:"type"`
	Summary    string          `json:"summary"`
	Payload    json.RawMessage `json:"payload,omitempty"`
	OccurredAt time.Time       `json:"occurred_at"`
}
