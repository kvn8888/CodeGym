package identity

import "context"

type PersonalTenant struct {
	UserID      string
	TenantID    string
	Role        string
	Email       string
	DisplayName string
}

type Store interface {
	EnsurePersonalTenant(ctx context.Context, tenant PersonalTenant) error
}
