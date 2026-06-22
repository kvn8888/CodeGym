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
// This is the STRETCH goal. For M1 you can satisfy issue #2's acceptance
// criterion ("profile updates happen asynchronously OR behind a clearly isolated
// worker/service boundary") with the synchronous Service.RefreshProfile alone.
// Reach for this Worker once RefreshProfile works.
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
			// TODO(you): refresh every active user/tenant.
			//
			// The design problem to solve first: Service.RefreshProfile reads the
			// identity (tenant_id, user_id) from the *request* context via
			// auth/tenant middleware — but a background tick has no request. Two ways:
			//
			//   (a) Add a Store method to list (tenant_id, user_id) pairs that have
			//       events, then for each pair build a context with
			//       auth.WithPrincipal(...) + tenant.WithScope(...) and call
			//       RefreshProfile. (See internal/integration/neon_test.go for how
			//       a context is seeded this way.)
			//
			//   (b) Add a Service method like RefreshProfileFor(ctx, tenantID, userID)
			//       that takes the identity explicitly instead of from context, and
			//       call it directly. Simpler — recommended for v0.
			//
			// Until then, this is a no-op heartbeat so the loop is observable.
			log.Printf("memory worker tick (every %s): TODO refresh active profiles", w.interval)
		}
	}
}
