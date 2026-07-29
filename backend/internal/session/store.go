package session

import (
	"context"
	"errors"
)

var ErrNotFound = errors.New("session not found")

type Store interface {
	EnsureSchema(ctx context.Context) error
	Create(ctx context.Context, session Session) (Session, error)
	Get(ctx context.Context, workspaceID, userID, id string) (Session, error)
	List(ctx context.Context, workspaceID, userID string, filter ListFilter) ([]Summary, error)
	Update(ctx context.Context, session Session) (Session, error)
	UpsertFiles(ctx context.Context, workspaceID, userID, sessionID string, files []File) (Session, error)
	HasPendingMemoryUpdate(ctx context.Context, workspaceID, userID string) (bool, error)
}
