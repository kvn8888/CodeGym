package memory

import (
	"context"
	"sync"
)

type InMemoryStore struct {
	mu       sync.RWMutex
	profiles map[string]Profile
	events   map[string][]Event
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		profiles: map[string]Profile{},
		events:   map[string][]Event{},
	}
}

func (s *InMemoryStore) GetProfile(_ context.Context, tenantID, userID string) (Profile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	profile, ok := s.profiles[key(tenantID, userID)]
	if !ok {
		return Profile{}, ErrProfileNotFound
	}
	return profile, nil
}

func (s *InMemoryStore) UpsertProfile(_ context.Context, tenantID, userID string, profile Profile) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.profiles[key(tenantID, userID)] = profile
	return nil
}

func (s *InMemoryStore) AppendEvent(_ context.Context, event Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	k := key(event.TenantID, event.UserID)
	s.events[k] = append(s.events[k], event)
	return nil
}

func (s *InMemoryStore) ListEvents(_ context.Context, tenantID, userID string) ([]Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	events := s.events[key(tenantID, userID)]
	copied := make([]Event, len(events))
	copy(copied, events)
	return copied, nil
}

func key(tenantID, userID string) string {
	return tenantID + "\x00" + userID
}
