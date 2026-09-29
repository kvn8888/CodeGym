package execution

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/daytona/clients/sdk-go/pkg/types"
)

type fakeSweepClient struct {
	sandboxes []sweepSandbox
	err       error
}

func (f *fakeSweepClient) ListSubmissionSandboxes(context.Context) ([]sweepSandbox, error) {
	return append([]sweepSandbox(nil), f.sandboxes...), f.err
}

type deletionRecord struct {
	mu       sync.Mutex
	count    int
	ctxError error
}

func (r *deletionRecord) delete(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.count++
	r.ctxError = ctx.Err()
	return nil
}

func (r *deletionRecord) snapshot() (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.count, r.ctxError
}

func TestSweeperDeletesOldLabelledSandboxesAndLeavesRecentOnes(t *testing.T) {
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	oldDelete := &deletionRecord{}
	recentDelete := &deletionRecord{}
	unlabelledDelete := &deletionRecord{}
	client := &fakeSweepClient{sandboxes: []sweepSandbox{
		{
			id: "old", providerCreatedAt: now.Add(-20 * time.Minute).Format(time.RFC3339Nano),
			labels: map[string]string{codegymSandboxLabel: codegymSandboxLabelValue, codegymRunIDLabel: "old-run"},
			delete: oldDelete.delete,
		},
		{
			id: "recent", providerCreatedAt: now.Add(-5 * time.Minute).Format(time.RFC3339Nano),
			labels: map[string]string{codegymSandboxLabel: codegymSandboxLabelValue, codegymRunIDLabel: "recent-run"},
			delete: recentDelete.delete,
		},
		{
			id: "not-ours", providerCreatedAt: now.Add(-time.Hour).Format(time.RFC3339Nano),
			labels: map[string]string{codegymSandboxLabel: "other"}, delete: unlabelledDelete.delete,
		},
	}}
	runner := &DaytonaRunner{client: combinedSweepClient{daytonaSandboxClient: noopCreateClient{}, sweep: client}}
	sweeper := NewSweeper(runner, 15*time.Minute, time.Minute, func() time.Time { return now })

	removed, err := sweeper.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	if count, _ := oldDelete.snapshot(); count != 1 {
		t.Fatalf("old delete count = %d, want 1", count)
	}
	if count, _ := recentDelete.snapshot(); count != 0 {
		t.Fatalf("recent delete count = %d, want 0", count)
	}
	if count, _ := unlabelledDelete.snapshot(); count != 0 {
		t.Fatalf("unlabelled delete count = %d, want 0", count)
	}
}

func TestSweeperNeverDeletesActiveRun(t *testing.T) {
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	activeDelete := &deletionRecord{}
	client := &fakeSweepClient{sandboxes: []sweepSandbox{{
		id: "active-old", providerCreatedAt: now.Add(-time.Hour).Format(time.RFC3339Nano),
		labels: map[string]string{codegymSandboxLabel: codegymSandboxLabelValue, codegymRunIDLabel: "active-run"},
		delete: activeDelete.delete,
	}}}
	runner := &DaytonaRunner{client: combinedSweepClient{daytonaSandboxClient: noopCreateClient{}, sweep: client}}
	runner.activeRuns.add("active-run")
	sweeper := NewSweeper(runner, 15*time.Minute, time.Minute, func() time.Time { return now })

	removed, err := sweeper.RunOnce(context.Background())
	if err != nil || removed != 0 {
		t.Fatalf("RunOnce removed=%d err=%v", removed, err)
	}
	if count, _ := activeDelete.snapshot(); count != 0 {
		t.Fatalf("active delete count = %d, want 0", count)
	}
}

func TestSweeperUsesIndependentDeleteContext(t *testing.T) {
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	deleted := &deletionRecord{}
	client := &fakeSweepClient{sandboxes: []sweepSandbox{{
		id: "old", providerCreatedAt: now.Add(-time.Hour).Format(time.RFC3339Nano),
		labels: map[string]string{codegymSandboxLabel: codegymSandboxLabelValue},
		delete: deleted.delete,
	}}}
	runner := &DaytonaRunner{client: combinedSweepClient{daytonaSandboxClient: noopCreateClient{}, sweep: client}}
	sweeper := NewSweeper(runner, 15*time.Minute, time.Minute, func() time.Time { return now })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	removed, err := sweeper.RunOnce(ctx)
	if err != nil || removed != 1 {
		t.Fatalf("RunOnce removed=%d err=%v", removed, err)
	}
	if count, contextErr := deleted.snapshot(); count != 1 || contextErr != nil {
		t.Fatalf("delete count=%d context_err=%v, want 1 and nil", count, contextErr)
	}
}

func TestSweeperLeavesUnknownTimestampAlone(t *testing.T) {
	deleted := &deletionRecord{}
	client := &fakeSweepClient{sandboxes: []sweepSandbox{{
		id:     "unknown-age",
		labels: map[string]string{codegymSandboxLabel: codegymSandboxLabelValue, codegymCreatedAtLabel: "corrupt"},
		delete: deleted.delete,
	}}}
	runner := &DaytonaRunner{client: combinedSweepClient{daytonaSandboxClient: noopCreateClient{}, sweep: client}}
	sweeper := NewSweeper(runner, 15*time.Minute, time.Minute, time.Now)
	removed, err := sweeper.RunOnce(context.Background())
	if err != nil || removed != 0 {
		t.Fatalf("RunOnce removed=%d err=%v", removed, err)
	}
	if count, _ := deleted.snapshot(); count != 0 {
		t.Fatalf("delete count = %d, want 0", count)
	}
}

type noopCreateClient struct{}

func (noopCreateClient) Create(context.Context, types.SnapshotParams) (daytonaRunnerSandbox, error) {
	return nil, errors.New("unused")
}

type combinedSweepClient struct {
	daytonaSandboxClient
	sweep *fakeSweepClient
}

func (c combinedSweepClient) ListSubmissionSandboxes(ctx context.Context) ([]sweepSandbox, error) {
	return c.sweep.ListSubmissionSandboxes(ctx)
}
