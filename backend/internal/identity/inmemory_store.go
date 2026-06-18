package identity

import (
	"context"
	"sync"
)

type InMemoryStore struct {
	mu          sync.RWMutex
	users       map[string]struct{}
	tenants     map[string]struct{}
	memberships map[string]PersonalTenant
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		users:       map[string]struct{}{},
		tenants:     map[string]struct{}{},
		memberships: map[string]PersonalTenant{},
	}
}

func (s *InMemoryStore) EnsurePersonalTenant(_ context.Context, tenant PersonalTenant) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.users[tenant.UserID] = struct{}{}
	s.tenants[tenant.TenantID] = struct{}{}
	s.memberships[membershipKey(tenant.TenantID, tenant.UserID)] = tenant
	return nil
}

func membershipKey(tenantID, userID string) string {
	return tenantID + "\x00" + userID
}
