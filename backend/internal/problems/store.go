package problems

import (
	"context"
	"errors"
)

var ErrNotFound = errors.New("problem not found")

type Store interface {
	EnsureSchema(ctx context.Context) error
	Upsert(ctx context.Context, definition Definition) error
	Get(ctx context.Context, id string) (Definition, error)
	List(ctx context.Context) ([]Summary, error)
}
