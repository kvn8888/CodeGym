package execution

import (
	"context"
	"errors"
)

var ErrRunNotFound = errors.New("execution run not found")

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
}
