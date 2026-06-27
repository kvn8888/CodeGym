package identity

import (
	"context"
	"sync"
)

type InMemoryStore struct {
	mu          sync.RWMutex
	users       map[string]PersonalTenant
	tenants     map[string]struct{}
	memberships map[string]PersonalTenant
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		users:       map[string]PersonalTenant{},
		tenants:     map[string]struct{}{},
		memberships: map[string]PersonalTenant{},
	}
}

func (s *InMemoryStore) EnsurePersonalTenant(_ context.Context, tenant PersonalTenant) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	user := tenant
	if existing, ok := s.users[tenant.UserID]; ok {
		if user.Email == "" {
			user.Email = existing.Email
		}
		if user.DisplayName == "" {
			user.DisplayName = existing.DisplayName
		}
	}
	if user.DisplayName == "" {
		user.DisplayName = tenant.UserID
	}
	s.users[tenant.UserID] = user
	s.tenants[tenant.TenantID] = struct{}{}
	s.memberships[membershipKey(tenant.TenantID, tenant.UserID)] = tenant
	return nil
}

func membershipKey(tenantID, userID string) string {
	return tenantID + "\x00" + userID
}
