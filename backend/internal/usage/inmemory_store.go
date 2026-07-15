package usage

import (
	"context"
	"sync"
)

// InMemoryStore keeps usage records in process memory.
type InMemoryStore struct {
	mu      sync.RWMutex
	records []Record
}

// NewInMemoryStore creates an empty in-memory usage store.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{records: make([]Record, 0, 64)}
}

// EnsureSchema is a no-op for the in-memory store.
func (s *InMemoryStore) EnsureSchema(context.Context) error {
	return nil
}

// Append stores a usage record.
func (s *InMemoryStore) Append(_ context.Context, record Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, record)
	return nil
}

// List returns records for the given scope (oldest first).
func (s *InMemoryStore) List(_ context.Context, workspaceID, userID string) ([]Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Record, 0)
	for _, record := range s.records {
		if record.WorkspaceID == workspaceID && record.UserID == userID {
			out = append(out, record)
		}
	}
	return out, nil
}
