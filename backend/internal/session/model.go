package session

import (
	"encoding/json"
	"time"
)

type Kind string

const (
	KindWorkspace Kind = "workspace"
	KindMCQ       Kind = "mcq"
	KindInterview Kind = "interview"
)

type Status string

const (
	StatusActive    Status = "active"
	StatusCompleted Status = "completed"
	StatusAbandoned Status = "abandoned"
)

// Session is the durable resume record for a bounded practice activity.
type Session struct {
	ID              string          `json:"id"`
	WorkspaceID        string          `json:"workspace_id"`
	UserID          string          `json:"user_id"`
	Kind            Kind            `json:"kind"`
	Status          Status          `json:"status"`
	Title           string          `json:"title"`
	ProblemID       string          `json:"problem_id,omitempty"`
	GenerationJobID string          `json:"generation_job_id,omitempty"`
	State           json.RawMessage `json:"state"`
	Files           []File          `json:"files,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
	LastActivityAt  time.Time       `json:"last_activity_at"`
	CompletedAt     *time.Time      `json:"completed_at,omitempty"`
}

// Summary is the history-list representation. It deliberately excludes
// heavyweight resume state and draft files.
type Summary struct {
	ID              string     `json:"id"`
	WorkspaceID        string     `json:"workspace_id"`
	UserID          string     `json:"user_id"`
	Kind            Kind       `json:"kind"`
	Status          Status     `json:"status"`
	Title           string     `json:"title"`
	ProblemID       string     `json:"problem_id,omitempty"`
	GenerationJobID string     `json:"generation_job_id,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	LastActivityAt  time.Time  `json:"last_activity_at"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
}

type File struct {
	Path      string    `json:"file_path"`
	Content   string    `json:"content"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateInput struct {
	Kind            Kind            `json:"kind"`
	Title           string          `json:"title,omitempty"`
	ProblemID       string          `json:"problem_id,omitempty"`
	GenerationJobID string          `json:"generation_job_id,omitempty"`
	State           json.RawMessage `json:"state,omitempty"`
}

type UpdateInput struct {
	Title  *string          `json:"title,omitempty"`
	Status *Status          `json:"status,omitempty"`
	State  *json.RawMessage `json:"state,omitempty"`
}

type ListFilter struct {
	Kind   Kind
	Status Status
	Limit  int
	Before time.Time
}

type UpsertFilesInput struct {
	Files []FileInput `json:"files"`
}

type FileInput struct {
	Path    string `json:"file_path"`
	Content string `json:"content"`
}

func (s Session) Summary() Summary {
	return Summary{
		ID:              s.ID,
		WorkspaceID:        s.WorkspaceID,
		UserID:          s.UserID,
		Kind:            s.Kind,
		Status:          s.Status,
		Title:           s.Title,
		ProblemID:       s.ProblemID,
		GenerationJobID: s.GenerationJobID,
		CreatedAt:       s.CreatedAt,
		UpdatedAt:       s.UpdatedAt,
		LastActivityAt:  s.LastActivityAt,
		CompletedAt:     s.CompletedAt,
	}
}
