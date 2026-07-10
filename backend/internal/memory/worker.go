package memory

import (
	"context"
	"log"
	"time"
)

// Worker is the *async* boundary for memory summarization — issue #2 build
// step 4 ("async memory update worker") and the scope spec's "daily cron job".
// It periodically re-derives profiles so the request path (Service.RecordEvent)
// stays fast and never blocks on summarization.
//
// This is the async side of the isolated refresh boundary. The synchronous
// Service.RefreshProfile endpoint is still useful for manual refreshes and smoke
// tests; this Worker is what lets production refresh profiles on a schedule.
//
// LEARNING GOAL: how Go runs background work — a goroutine driving a
// time.Ticker, with context cancellation for clean shutdown.
type Worker struct {
	service  *Service
	interval time.Duration
}

func NewWorker(service *Service, interval time.Duration) *Worker {
	if interval <= 0 {
		interval = time.Hour
	}
	return &Worker{service: service, interval: interval}
}

// Run blocks until ctx is cancelled, refreshing on each tick. Start it from
// main.go with:  go memory.NewWorker(memoryService, time.Hour).Run(ctx)
func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// The key design problem: Service.RefreshProfile reads the
			// identity (workspace_id, user_id) from the *request* context via
			// auth/workspace middleware — but a background tick has no request. Two ways:
			//
			//   (a) Add a Store method to list (workspace_id, user_id) pairs that have
			//       events, then for each pair build a context with
			//       auth.WithPrincipal(...) + workspace.WithScope(...) and call
			//       RefreshProfile. (See internal/integration/neon_test.go for how
			//       a context is seeded this way.)
			//
			//   (b) Add a Service method like RefreshProfileFor(ctx, workspaceID, userID)
			//       that takes the identity explicitly instead of from context, and
			//       call it directly. Simpler — recommended for v0.
			//
			// We use option (b): Service.RefreshAllProfiles asks the store for active
			// event scopes, then calls RefreshProfileFor for each pair. The worker owns
			// scheduling and logging only; the service owns memory semantics.
			count, err := w.RunOnce(ctx)
			if err != nil {
				log.Printf("memory worker tick failed: %v", err)
				continue
			}
			log.Printf("memory worker refreshed %d profile(s)", count)
		}
	}
}

func (w *Worker) RunOnce(ctx context.Context) (int, error) {
	return w.service.RefreshAllProfiles(ctx)
}
