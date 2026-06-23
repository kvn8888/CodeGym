package identity

import "context"

type PersonalTenant struct {
	UserID   string
	TenantID string
	Role     string
}

type Store interface {
	EnsurePersonalTenant(ctx context.Context, tenant PersonalTenant) error
}
