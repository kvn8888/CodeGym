package execution

import (
	"context"
	"sync"
)

type InMemoryStore struct {
	mu   sync.RWMutex
	runs map[string][]Run
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{runs: map[string][]Run{}}
}

func (s *InMemoryStore) CreateRun(_ context.Context, run Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	k := key(run.TenantID, run.UserID)
	s.runs[k] = append(s.runs[k], run)
	return nil
}

func (s *InMemoryStore) UpdateRun(_ context.Context, run Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	runs := s.runs[key(run.TenantID, run.UserID)]
	for i := range runs {
		if runs[i].ID == run.ID {
			runs[i] = run
			return nil
		}
	}
	return ErrRunNotFound
}

func (s *InMemoryStore) GetRun(_ context.Context, tenantID, userID, id string) (Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, run := range s.runs[key(tenantID, userID)] {
		if run.ID == id {
			return run, nil
		}
	}
	return Run{}, ErrRunNotFound
}

func (s *InMemoryStore) ListRuns(_ context.Context, tenantID, userID string) ([]Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	runs := s.runs[key(tenantID, userID)]
	copied := make([]Run, len(runs))
	for i, run := range runs {
		copied[len(runs)-1-i] = run
	}
	return copied, nil
}

func key(tenantID, userID string) string {
	return tenantID + "\x00" + userID
}
