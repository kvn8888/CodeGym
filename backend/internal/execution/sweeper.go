package execution

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/kvn8888/codegym/backend/internal/environment"
)

const (
	codegymSandboxLabel         = "codegym"
	codegymSandboxLabelValue    = "submission"
	codegymEnvironmentLabel     = "codegym-environment"
	codegymRunIDLabel           = "codegym-run-id"
	codegymCreatedAtLabel       = "codegym-created-at"
	defaultSandboxSweepAge      = 15 * time.Minute
	defaultSandboxSweepInterval = 5 * time.Minute
	sandboxSweepDeleteTimeout   = 30 * time.Second
)

type sweepSandbox struct {
	id                string
	labels            map[string]string
	providerCreatedAt string
	delete            func(context.Context) error
}

type daytonaSweepClient interface {
	ListSubmissionSandboxes(ctx context.Context) ([]sweepSandbox, error)
}

// Sweeper conservatively removes old CodeGym-labelled sandboxes that are not
// tracked as active by this runner. The age margin is a second safety boundary
// around the exact active-run check.
type Sweeper struct {
	client      daytonaSweepClient
	active      *activeRunRegistry
	environment environment.Name
	maxAge      time.Duration
	interval    time.Duration
	now         Clock
}

// NewSweeper follows the background-worker construction used by memory.Worker.
// The Daytona runner supplies both the provider client and its active registry.
func NewSweeper(runner *DaytonaRunner, maxAge, interval time.Duration, clock Clock) *Sweeper {
	if maxAge <= 0 {
		maxAge = defaultSandboxSweepAge
	}
	if interval <= 0 {
		interval = defaultSandboxSweepInterval
	}
	if clock == nil {
		clock = time.Now
	}
	var client daytonaSweepClient
	var active *activeRunRegistry
	var codegymEnvironment environment.Name
	if runner != nil {
		client, _ = runner.client.(daytonaSweepClient)
		active = &runner.activeRuns
		codegymEnvironment = runner.environment
	}
	return &Sweeper{
		client: client, active: active, environment: codegymEnvironment,
		maxAge: maxAge, interval: interval, now: clock,
	}
}

// Run blocks until ctx is canceled and sweeps on each tick.
func (s *Sweeper) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			removed, err := s.RunOnce(ctx)
			if err != nil {
				log.Printf("daytona orphan sweeper tick failed removed=%d err=%v", removed, err)
				continue
			}
			log.Printf("daytona orphan sweeper tick ok removed=%d", removed)
		}
	}
}

// RunOnce lists only CodeGym submission sandboxes and removes inactive entries
// strictly older than the safety margin. Unknown timestamps are skipped.
func (s *Sweeper) RunOnce(ctx context.Context) (int, error) {
	if s == nil || s.client == nil || s.active == nil {
		return 0, errors.New("Daytona orphan sweeper is not configured")
	}
	if !s.environment.Valid() {
		return 0, errors.New("Daytona orphan sweeper environment is not configured")
	}
	sandboxes, err := s.client.ListSubmissionSandboxes(ctx)
	if err != nil {
		return 0, fmt.Errorf("list CodeGym sandboxes: %w", err)
	}

	cutoff := s.now().UTC().Add(-s.maxAge)
	removed := 0
	var cleanupErrors error
	for _, sandbox := range sandboxes {
		if sandbox.labels[codegymSandboxLabel] != codegymSandboxLabelValue {
			continue
		}
		sandboxEnvironment, environmentErr := environment.Parse(sandbox.labels[codegymEnvironmentLabel])
		if environmentErr != nil {
			log.Printf("daytona orphan sweeper skipped sandbox_id=%s reason=missing_or_invalid_environment", sandbox.id)
			continue
		}
		if sandboxEnvironment != s.environment {
			log.Printf("daytona orphan sweeper skipped sandbox_id=%s reason=environment_mismatch sandbox_environment=%s sweeper_environment=%s",
				sandbox.id, sandboxEnvironment, s.environment)
			continue
		}
		createdAt, ok := conservativeCreatedAt(sandbox)
		if !ok {
			log.Printf("daytona orphan sweeper skipped sandbox_id=%s reason=unknown_created_at", sandbox.id)
			continue
		}
		if !createdAt.Before(cutoff) {
			continue
		}
		runID := sandbox.labels[codegymRunIDLabel]
		if runID != "" && s.active.contains(runID) {
			continue
		}

		// Re-check immediately before the destructive call. Registration happens
		// before creates begin, so a matching active run always wins this race.
		if runID != "" && s.active.contains(runID) {
			continue
		}
		if sandbox.delete == nil {
			cleanupErrors = errors.Join(cleanupErrors, fmt.Errorf("sandbox %s has no delete operation", sandbox.id))
			continue
		}
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sandboxSweepDeleteTimeout)
		deleteErr := sandbox.delete(cleanupCtx)
		cancel()
		if deleteErr != nil {
			cleanupErrors = errors.Join(cleanupErrors, fmt.Errorf("delete sandbox %s: %w", sandbox.id, deleteErr))
			continue
		}
		removed++
		log.Printf("daytona orphan sweeper removed sandbox_id=%s run_id=%s created_at=%s",
			sandbox.id, runID, createdAt.Format(time.RFC3339))
	}
	return removed, cleanupErrors
}

// conservativeCreatedAt uses the later of Daytona's timestamp and our label.
// A forged or malformed older marker therefore cannot make a recent sandbox
// eligible early.
func conservativeCreatedAt(sandbox sweepSandbox) (time.Time, bool) {
	var createdAt time.Time
	if sandbox.providerCreatedAt != "" {
		parsed, err := time.Parse(time.RFC3339Nano, sandbox.providerCreatedAt)
		if err == nil {
			createdAt = parsed.UTC()
		}
	}
	if raw := sandbox.labels[codegymCreatedAtLabel]; raw != "" {
		unixSeconds, err := strconv.ParseInt(raw, 10, 64)
		if err == nil {
			labelCreatedAt := time.Unix(unixSeconds, 0).UTC()
			if createdAt.IsZero() || labelCreatedAt.After(createdAt) {
				createdAt = labelCreatedAt
			}
		}
	}
	return createdAt, !createdAt.IsZero()
}
