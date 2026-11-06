package execution

import (
	"context"
	"sync"
	"time"
)

type InMemoryStore struct {
	mu       sync.RWMutex
	runs     map[string][]Run
	timings  []RunTiming
	requests map[string]SubmissionRequestReservation
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{runs: map[string][]Run{}, timings: make([]RunTiming, 0, 64), requests: map[string]SubmissionRequestReservation{}}
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

func (s *InMemoryStore) ReserveSubmissionRequest(_ context.Context, workspaceID, userID, token, fingerprint string) (SubmissionRequestReservation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := submissionRequestKey(workspaceID, userID, token)
	existing, exists := s.requests[key]
	if exists {
		if existing.Fingerprint != fingerprint {
			return SubmissionRequestReservation{}, ErrSubmissionRequestConflict
		}
		existing.Created = false
		return existing, nil
	}
	now := time.Now().UTC()
	reservation := SubmissionRequestReservation{
		Token: token, Fingerprint: fingerprint, Status: SubmissionRequestStatusInProgress, Created: true,
		CreatedAt: now, UpdatedAt: now,
	}
	s.requests[key] = reservation
	return reservation, nil
}

func (s *InMemoryStore) BindSubmissionRequestRun(_ context.Context, workspaceID, userID, token, runID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := submissionRequestKey(workspaceID, userID, token)
	existing, exists := s.requests[key]
	if !exists {
		return ErrSubmissionRequestNotFound
	}
	existing.RunID = runID
	existing.UpdatedAt = time.Now().UTC()
	s.requests[key] = existing
	return nil
}

func (s *InMemoryStore) CompleteSubmissionRequest(_ context.Context, workspaceID, userID, token, runID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := submissionRequestKey(workspaceID, userID, token)
	existing, exists := s.requests[key]
	if !exists {
		return ErrSubmissionRequestNotFound
	}
	existing.Status = SubmissionRequestStatusCompleted
	existing.SubmissionID = runID
	existing.RunID = runID
	existing.UpdatedAt = time.Now().UTC()
	s.requests[key] = existing
	return nil
}

func (s *InMemoryStore) GetSubmissionRequest(_ context.Context, workspaceID, userID, token string) (SubmissionRequestReservation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	reservation, exists := s.requests[submissionRequestKey(workspaceID, userID, token)]
	if !exists {
		return SubmissionRequestReservation{}, ErrSubmissionRequestNotFound
	}
	reservation.Created = false
	return reservation, nil
}

func submissionRequestKey(workspaceID, userID, token string) string {
	return workspaceID + "\x00" + userID + "\x00" + token
}

func key(workspaceID, userID string) string {
	return workspaceID + "\x00" + userID
}
