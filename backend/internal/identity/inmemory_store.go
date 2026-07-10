package identity

import (
	"context"
	"strings"
	"sync"
)

// InMemoryStore is a process-local identity store for local development.
type InMemoryStore struct {
	mu          sync.RWMutex
	users       map[string]PersonalTenant
	tenants     map[string]struct{}
	memberships map[string]PersonalTenant
}

// NewInMemoryStore creates an empty in-memory identity store.
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
	user.Email = strings.TrimSpace(user.Email)
	user.DisplayName = strings.TrimSpace(user.DisplayName)
	user.DisplayNameSource = normalizeDisplayNameSource(user.DisplayNameSource)
	if user.DisplayName == "" {
		user.DisplayName = tenant.UserID
		user.DisplayNameSource = "fallback"
	}

	if existing, ok := s.users[tenant.UserID]; ok {
		if user.Email == "" {
			user.Email = existing.Email
		}
		if existing.DisplayNameSource == "user" {
			user.DisplayName = existing.DisplayName
			user.DisplayNameSource = existing.DisplayNameSource
		} else if strings.TrimSpace(tenant.DisplayName) == "" && existing.DisplayName != "" {
			user.DisplayName = existing.DisplayName
			user.DisplayNameSource = normalizeDisplayNameSource(existing.DisplayNameSource)
		}
	}
	s.users[tenant.UserID] = user
	s.tenants[tenant.TenantID] = struct{}{}
	s.memberships[membershipKey(tenant.TenantID, tenant.UserID)] = tenant
	return nil
}

func (s *InMemoryStore) GetUserProfile(_ context.Context, userID string) (UserProfile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, ok := s.users[userID]
	if !ok {
		return UserProfile{}, ErrUserNotFound
	}

	return profileFromPersonalTenant(user), nil
}

func (s *InMemoryStore) UpdateDisplayName(_ context.Context, userID, displayName string) (UserProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.users[userID]
	if !ok {
		return UserProfile{}, ErrUserNotFound
	}

	user.DisplayName = strings.TrimSpace(displayName)
	user.DisplayNameSource = "user"
	s.users[userID] = user

	return profileFromPersonalTenant(user), nil
}

func profileFromPersonalTenant(user PersonalTenant) UserProfile {
	return UserProfile{
		UserID:            user.UserID,
		Email:             user.Email,
		DisplayName:       user.DisplayName,
		DisplayNameSource: normalizeDisplayNameSource(user.DisplayNameSource),
		DefaultTenantID:   user.TenantID,
	}
}

func normalizeDisplayNameSource(source string) string {
	switch strings.TrimSpace(source) {
	case "oauth", "user", "fallback":
		return strings.TrimSpace(source)
	default:
		return "fallback"
	}
}

func membershipKey(tenantID, userID string) string {
	return tenantID + "\x00" + userID
}
