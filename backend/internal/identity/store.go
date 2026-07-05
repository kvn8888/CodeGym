package identity

import "context"

// PersonalTenant defines the minimum fields required to ensure a personal tenant membership for an authenticated user.
type PersonalTenant struct {
	UserID   string
	TenantID string
	Role     string
}

// Store defines persistence operations for identity bootstrap behavior.
type Store interface {
	EnsurePersonalTenant(ctx context.Context, tenant PersonalTenant) error
}
