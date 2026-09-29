package settings

import "context"

// Store persists workspace-scoped runtime settings.
type Store interface {
	EnsureSchema(ctx context.Context) error
	Get(ctx context.Context, workspaceID, key string) (Setting, error)
	Upsert(ctx context.Context, setting Setting) error
}
