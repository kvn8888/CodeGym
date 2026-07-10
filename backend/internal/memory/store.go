package memory

import (
	"context"
	"errors"
)

// ErrProfileNotFound indicates a scoped memory profile is not yet persisted.
var ErrProfileNotFound = errors.New("memory profile not found")

// Store defines persistence operations required by the memory service.
type Scope struct {
	WorkspaceID string
	UserID   string
}

type Store interface {
	GetProfile(ctx context.Context, workspaceID, userID string) (Profile, error)
	UpsertProfile(ctx context.Context, workspaceID, userID string, profile Profile) error
	AppendEvent(ctx context.Context, event Event) error
	ListEvents(ctx context.Context, workspaceID, userID string) ([]Event, error)
	ListEventScopes(ctx context.Context) ([]Scope, error)
}
