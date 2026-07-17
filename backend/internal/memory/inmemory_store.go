package memory

import (
	"context"
	"sort"
	"strings"
	"sync"
)

// InMemoryStore is a process-local memory store used for local development.
type InMemoryStore struct {
	mu       sync.RWMutex
	profiles map[string]Profile
	events   map[string][]Event
}

// NewInMemoryStore creates an empty in-memory memory store.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		profiles: map[string]Profile{},
		events:   map[string][]Event{},
	}
}

func (s *InMemoryStore) GetProfile(_ context.Context, workspaceID, userID string) (Profile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	profile, ok := s.profiles[key(workspaceID, userID)]
	if !ok {
		return Profile{}, ErrProfileNotFound
	}
	return profile, nil
}

func (s *InMemoryStore) UpsertProfile(_ context.Context, workspaceID, userID string, profile Profile) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.profiles[key(workspaceID, userID)] = profile
	return nil
}

func (s *InMemoryStore) AppendEvent(_ context.Context, event Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	k := key(event.WorkspaceID, event.UserID)
	s.events[k] = append(s.events[k], event)
	return nil
}

func (s *InMemoryStore) ListEvents(_ context.Context, workspaceID, userID string) ([]Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	events := s.events[key(workspaceID, userID)]
	copied := make([]Event, len(events))
	copy(copied, events)
	return copied, nil
}

func (s *InMemoryStore) ListEventScopes(_ context.Context) ([]Scope, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	scopes := make([]Scope, 0, len(s.events))
	for k, events := range s.events {
		if len(events) == 0 {
			continue
		}
		workspaceID, userID, ok := splitKey(k)
		if !ok {
			continue
		}
		scopes = append(scopes, Scope{WorkspaceID: workspaceID, UserID: userID})
	}
	sort.Slice(scopes, func(i, j int) bool {
		if scopes[i].WorkspaceID == scopes[j].WorkspaceID {
			return scopes[i].UserID < scopes[j].UserID
		}
		return scopes[i].WorkspaceID < scopes[j].WorkspaceID
	})
	return scopes, nil
}

func key(workspaceID, userID string) string {
	return workspaceID + "\x00" + userID
}

func splitKey(value string) (string, string, bool) {
	parts := strings.SplitN(value, "\x00", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}
