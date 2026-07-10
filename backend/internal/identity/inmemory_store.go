package identity

import (
	"context"
	"strings"
	"sync"
)

// InMemoryStore is a process-local identity store for local development.
type InMemoryStore struct {
	mu          sync.RWMutex
	users       map[string]PersonalWorkspace
	workspaces  map[string]struct{}
	memberships map[string]PersonalWorkspace
}

// NewInMemoryStore creates an empty in-memory identity store.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		users:       map[string]PersonalWorkspace{},
		workspaces:  map[string]struct{}{},
		memberships: map[string]PersonalWorkspace{},
	}
}

func (s *InMemoryStore) EnsurePersonalWorkspace(_ context.Context, workspace PersonalWorkspace) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	user := workspace
	user.Email = strings.TrimSpace(user.Email)
	user.DisplayName = strings.TrimSpace(user.DisplayName)
	user.DisplayNameSource = normalizeDisplayNameSource(user.DisplayNameSource)
	if user.DisplayName == "" {
		user.DisplayName = workspace.UserID
		user.DisplayNameSource = "fallback"
	}

	if existing, ok := s.users[workspace.UserID]; ok {
		if user.Email == "" {
			user.Email = existing.Email
		}
		if existing.DisplayNameSource == "user" {
			user.DisplayName = existing.DisplayName
			user.DisplayNameSource = existing.DisplayNameSource
		} else if strings.TrimSpace(workspace.DisplayName) == "" && existing.DisplayName != "" {
			user.DisplayName = existing.DisplayName
			user.DisplayNameSource = normalizeDisplayNameSource(existing.DisplayNameSource)
		}
	}
	s.users[workspace.UserID] = user
	s.workspaces[workspace.WorkspaceID] = struct{}{}
	s.memberships[membershipKey(workspace.WorkspaceID, workspace.UserID)] = workspace
	return nil
}

func (s *InMemoryStore) GetUserProfile(_ context.Context, userID string) (UserProfile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, ok := s.users[userID]
	if !ok {
		return UserProfile{}, ErrUserNotFound
	}

	return profileFromPersonalWorkspace(user), nil
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

	return profileFromPersonalWorkspace(user), nil
}

func profileFromPersonalWorkspace(user PersonalWorkspace) UserProfile {
	return UserProfile{
		UserID:             user.UserID,
		Email:              user.Email,
		DisplayName:        user.DisplayName,
		DisplayNameSource:  normalizeDisplayNameSource(user.DisplayNameSource),
		DefaultWorkspaceID: user.WorkspaceID,
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

func membershipKey(workspaceID, userID string) string {
	return workspaceID + "\x00" + userID
}
