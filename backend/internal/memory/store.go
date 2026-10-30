package memory

import (
	"context"
	"errors"
)

// ErrProfileNotFound indicates a scoped memory profile is not yet persisted.
var ErrProfileNotFound = errors.New("memory profile not found")

// ErrStaleProfile indicates a profile save was based on an older profile version.
var ErrStaleProfile = errors.New("memory profile version is stale")

// ErrCompletionAlreadyProcessed indicates that a graded completion has
// already been atomically applied to the scoped profile.
var ErrCompletionAlreadyProcessed = errors.New("memory completion already processed")

// Store defines persistence operations required by the memory service.
type Scope struct {
	WorkspaceID string
	UserID      string
}

type Store interface {
	GetProfile(ctx context.Context, workspaceID, userID string) (Profile, error)
	UpsertProfile(ctx context.Context, workspaceID, userID string, profile Profile) error
	ReplaceProfileIfVersion(ctx context.Context, workspaceID, userID string, profile Profile, expectedVersion int64) error
	ReplaceProfileAndMarkCompletionIfVersion(ctx context.Context, workspaceID, userID string, profile Profile, expectedVersion int64, completion ProcessedCompletion) error
	IsCompletionProcessed(ctx context.Context, workspaceID, userID, sessionID, problemID, submissionID string) (bool, error)
	AppendEvent(ctx context.Context, event Event) error
	ListEvents(ctx context.Context, workspaceID, userID string) ([]Event, error)
	ListEventScopes(ctx context.Context) ([]Scope, error)
}
