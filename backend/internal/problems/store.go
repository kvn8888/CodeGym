package problems

import (
	"context"
	"errors"
)

var ErrNotFound = errors.New("problem not found")

type Store interface {
	EnsureSchema(ctx context.Context) error
	Upsert(ctx context.Context, definition Definition) error
	Get(ctx context.Context, id, workspaceID, userID string) (Definition, error)
	List(ctx context.Context, workspaceID, userID string) ([]Summary, error)
}
