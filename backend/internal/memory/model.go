package memory

import (
	"encoding/json"
	"time"
)

type Profile struct {
	Summary      string             `json:"summary"`
	UpdatedAt    time.Time          `json:"updated_at"`
	NextReviewAt time.Time          `json:"next_review_at"`
	Strengths    []string           `json:"strengths"`
	GrowthEdges  []string           `json:"growth_edges"`
	Skills       []SkillProficiency `json:"skills"`
	Notes        []Note             `json:"notes"`
}

type SkillProficiency struct {
	ID            string    `json:"id"`
	Label         string    `json:"label"`
	Area          string    `json:"area"`
	Level         int       `json:"level"`
	Confidence    int       `json:"confidence"`
	Trend         string    `json:"trend"`
	LastPracticed time.Time `json:"last_practiced"`
}

type Note struct {
	ID        string    `json:"id"`
	ProblemID string    `json:"problem_id"`
	Title     string    `json:"title"`
	Summary   string    `json:"summary"`
	CreatedAt time.Time `json:"created_at"`
	Tags      []string  `json:"tags"`
	Action    string    `json:"action"`
}

type Event struct {
	ID         string          `json:"id"`
	TenantID   string          `json:"tenant_id"`
	UserID     string          `json:"user_id"`
	Source     string          `json:"source"`
	Type       string          `json:"type"`
	Summary    string          `json:"summary"`
	Payload    json.RawMessage `json:"payload,omitempty"`
	OccurredAt time.Time       `json:"occurred_at"`
	CreatedAt  time.Time       `json:"created_at"`
}

type RecordEventInput struct {
	Source     string          `json:"source"`
	Type       string          `json:"type"`
	Summary    string          `json:"summary"`
	Payload    json.RawMessage `json:"payload,omitempty"`
	OccurredAt time.Time       `json:"occurred_at"`
}
