package usage

import "context"

// Store persists GenAI usage records.
type Store interface {
	EnsureSchema(ctx context.Context) error
	Append(ctx context.Context, record Record) error
	List(ctx context.Context, workspaceID, userID string) ([]Record, error)
}
