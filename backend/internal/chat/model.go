package chat

import (
	"encoding/json"
	"time"

	"github.com/kvn8888/codegym/backend/internal/generation"
)

type Kind string

const (
	KindInterview Kind = "interview"
	KindCoach     Kind = "coach"
)

type Status string

const (
	StatusActive    Status = "active"
	StatusClosed    Status = "closed"
	StatusCompleted Status = "completed"
)

type MessageStatus string

const (
	MessageComplete    MessageStatus = "complete"
	MessageInterrupted MessageStatus = "interrupted"
)

type ContextEnvelope struct {
	Version    int    `json:"version"`
	SessionID  string `json:"session_id"`
	ProblemID  string `json:"problem_id,omitempty"`
	QuestionID string `json:"question_id,omitempty"`
}

type Thread struct {
	ID          string          `json:"id"`
	WorkspaceID string          `json:"workspace_id"`
	UserID      string          `json:"user_id"`
	SessionID   string          `json:"session_id"`
	Kind        Kind            `json:"kind"`
	Mode        string          `json:"mode,omitempty"`
	Status      Status          `json:"status"`
	Context     ContextEnvelope `json:"context"`
	SuccessorID string          `json:"successor_id,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	ClosedAt    *time.Time      `json:"closed_at,omitempty"`
}

type Message struct {
	ID               string        `json:"id"`
	ThreadID         string        `json:"thread_id"`
	Role             string        `json:"role"`
	Content          string        `json:"content"`
	Status           MessageStatus `json:"status"`
	ClientMessageID  string        `json:"client_message_id,omitempty"`
	ReplyToMessageID string        `json:"reply_to_message_id,omitempty"`
	CreatedAt        time.Time     `json:"created_at"`
}

type ThreadWithMessages struct {
	Thread
	Messages []Message `json:"messages"`
}

type CreateThreadInput struct {
	Kind       Kind   `json:"kind"`
	Mode       string `json:"mode,omitempty"`
	SessionID  string `json:"session_id"`
	QuestionID string `json:"question_id,omitempty"`
}

type ThreadFilter struct {
	Kind      Kind
	Status    Status
	SessionID string
	Limit     int
}

type TurnInput struct {
	ClientMessageID string `json:"client_message_id"`
	Message         string `json:"message"`
}

type SSEEvent struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

type FinishResult struct {
	Thread             Thread                         `json:"thread"`
	Assessment         generation.InterviewAssessment `json:"assessment"`
	MemoryUpdateStatus string                         `json:"memory_update_status"`
}

type ExitResult struct {
	Thread Thread `json:"thread"`
}

type threadContextData struct {
	Session json.RawMessage `json:"session"`
	Problem json.RawMessage `json:"problem,omitempty"`
}
