package memory

import (
	"context"
	"log"
	"time"
)

// Worker is the async boundary for scheduled memory profile refreshes. It
// periodically invokes a ProfileRefresher so event-recording request paths stay
// fast and never block on synthesis.
//
// Production passes the model-backed ProfileSynthesizer; the deterministic
// memory Service remains supported for cold-start and compatibility tests.
//
// LEARNING GOAL: how Go runs background work — a goroutine driving a
// time.Ticker, with context cancellation for clean shutdown.
type Worker struct {
	service  ProfileRefresher
	interval time.Duration
}

// ProfileRefresher is implemented by both the deterministic memory service and
// the model-backed profile synthesizer.
type ProfileRefresher interface {
	RefreshAllProfiles(ctx context.Context) (int, error)
}

func NewWorker(service ProfileRefresher, interval time.Duration) *Worker {
	if interval <= 0 {
		interval = time.Hour
	}
	return &Worker{service: service, interval: interval}
}

// Run blocks until ctx is cancelled, refreshing on each tick.
func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
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
