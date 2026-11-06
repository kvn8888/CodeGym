package execution

import (
	"context"
	"errors"
	"time"
)

var ErrRunNotFound = errors.New("execution run not found")

// ErrSubmissionRequestConflict indicates a submission request token was
// reused with content that differs from the originally accepted request.
var ErrSubmissionRequestConflict = errors.New("submission request token conflict")

// ErrSubmissionRequestNotFound indicates no reservation exists for a token.
var ErrSubmissionRequestNotFound = errors.New("submission request token not found")

// ErrSubmissionRequestInProgress indicates a logical submission is still being
// processed by another request. Callers should retry with the same token.
var ErrSubmissionRequestInProgress = errors.New("submission request is still in progress")

// SubmissionRequestStatusInProgress marks a logical submission request that
// reserved execution capacity but has not produced a terminal outcome yet.
const SubmissionRequestStatusInProgress = "in_progress"

// SubmissionRequestStatusCompleted marks a logical submission request whose
// terminal execution outcome was accepted.
const SubmissionRequestStatusCompleted = "completed"

// SubmissionRequestReservation is the durable, user/workspace-scoped state of
// one logical coding submission request claimed by a client request token.
type SubmissionRequestReservation struct {
	Token        string
	Fingerprint  string
	RunID        string
	SubmissionID string
	Status       string
	Created      bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Store interface {
	EnsureSchema(ctx context.Context) error
	CreateRun(ctx context.Context, run Run) error
	// UpdateRun replaces the run matched by ID, WorkspaceID, and UserID;
	// it returns ErrRunNotFound when no such run exists.
	UpdateRun(ctx context.Context, run Run) error
	GetRun(ctx context.Context, workspaceID, userID, id string) (Run, error)
	// ListRuns returns the scope's runs, newest first.
	ListRuns(ctx context.Context, workspaceID, userID string) ([]Run, error)
	AppendRunTiming(ctx context.Context, timing RunTiming) error
	// ListRunTimings returns structured timing observations for the scope,
	// oldest first, so callers can compute time-windowed percentiles.
	ListRunTimings(ctx context.Context, workspaceID, userID string) ([]RunTiming, error)
	// ReserveSubmissionRequest atomically claims a logical submission request
	// token inside one workspace/user scope. Reusing a token with different
	// content conflicts; reusing it with identical content replays the reserved
	// logical request without starting another execution.
	ReserveSubmissionRequest(ctx context.Context, workspaceID, userID, token, fingerprint string) (SubmissionRequestReservation, error)
	// BindSubmissionRequestRun records the run reserved by an in-progress
	// submission request token.
	BindSubmissionRequestRun(ctx context.Context, workspaceID, userID, token, runID string) error
	// CompleteSubmissionRequest marks a submitted logical request completed
	// against the run that produced its terminal outcome.
	CompleteSubmissionRequest(ctx context.Context, workspaceID, userID, token, runID string) error
	// GetSubmissionRequest returns the reservation for one token inside scope.
	GetSubmissionRequest(ctx context.Context, workspaceID, userID, token string) (SubmissionRequestReservation, error)
}
