package memory

import (
	"context"
	"errors"
)

var ErrProfileNotFound = errors.New("memory profile not found")

type Scope struct {
	TenantID string
	UserID   string
}

type Store interface {
	GetProfile(ctx context.Context, tenantID, userID string) (Profile, error)
	UpsertProfile(ctx context.Context, tenantID, userID string, profile Profile) error
	AppendEvent(ctx context.Context, event Event) error
	ListEvents(ctx context.Context, tenantID, userID string) ([]Event, error)
	ListEventScopes(ctx context.Context) ([]Scope, error)
}
