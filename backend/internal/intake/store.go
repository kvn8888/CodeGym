package intake

import (
	"context"
	"errors"
)

var ErrNotFound = errors.New("practice intake not found")

type Store interface {
	Create(ctx context.Context, record Record) (Record, error)
	Get(ctx context.Context, workspaceID, userID, id string) (Record, error)
	FindLatestByTopic(ctx context.Context, workspaceID, userID, normalizedTopic string) (Record, error)
	ListPending(ctx context.Context, workspaceID, userID string, limit int) ([]Record, error)
	Update(ctx context.Context, record Record) (Record, error)
}
