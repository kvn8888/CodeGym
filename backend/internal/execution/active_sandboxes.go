package execution

import (
	"context"
	"sync"
)

// activeRunRegistry is process-local protection for the orphan sweeper. A run
// is registered before any create begins and remains active until the winning
// sandbox's normal cleanup finishes, regardless of request cancellation.
type activeRunRegistry struct {
	mu     sync.RWMutex
	runIDs map[string]struct{}
}

func (r *activeRunRegistry) add(runID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.runIDs == nil {
		r.runIDs = make(map[string]struct{})
	}
	r.runIDs[runID] = struct{}{}
}

func (r *activeRunRegistry) remove(runID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.runIDs, runID)
}

func (r *activeRunRegistry) contains(runID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.runIDs[runID]
	return ok
}

type activeTrackingSandbox struct {
	daytonaRunnerSandbox
	runID    string
	registry *activeRunRegistry
}

func (s *activeTrackingSandbox) Delete(ctx context.Context) error {
	defer s.registry.remove(s.runID)
	return s.daytonaRunnerSandbox.Delete(ctx)
}
