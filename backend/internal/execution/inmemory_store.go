package execution

import (
	"context"
	"sync"
)

type InMemoryStore struct {
	mu      sync.RWMutex
	runs    map[string][]Run
	timings []RunTiming
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{runs: map[string][]Run{}, timings: make([]RunTiming, 0, 64)}
}

func (s *InMemoryStore) EnsureSchema(context.Context) error { return nil }

func (s *InMemoryStore) CreateRun(_ context.Context, run Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	k := key(run.WorkspaceID, run.UserID)
	s.runs[k] = append(s.runs[k], run)
	return nil
}

func (s *InMemoryStore) UpdateRun(_ context.Context, run Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	runs := s.runs[key(run.WorkspaceID, run.UserID)]
	for i := range runs {
		if runs[i].ID == run.ID {
			runs[i] = run
			return nil
		}
	}
	return ErrRunNotFound
}

func (s *InMemoryStore) GetRun(_ context.Context, workspaceID, userID, id string) (Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, run := range s.runs[key(workspaceID, userID)] {
		if run.ID == id {
			return run, nil
		}
	}
	return Run{}, ErrRunNotFound
}

func (s *InMemoryStore) ListRuns(_ context.Context, workspaceID, userID string) ([]Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	runs := s.runs[key(workspaceID, userID)]
	copied := make([]Run, len(runs))
	for i, run := range runs {
		copied[len(runs)-1-i] = run
	}
	return copied, nil
}

func (s *InMemoryStore) AppendRunTiming(_ context.Context, timing RunTiming) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.timings = append(s.timings, timing)
	return nil
}

func (s *InMemoryStore) ListRunTimings(_ context.Context, workspaceID, userID string) ([]RunTiming, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]RunTiming, 0)
	for _, timing := range s.timings {
		if timing.WorkspaceID == workspaceID && timing.UserID == userID {
			out = append(out, timing)
		}
	}
	return out, nil
}

func key(workspaceID, userID string) string {
	return workspaceID + "\x00" + userID
}
