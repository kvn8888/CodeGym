package session

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func newMemoryStatusTestSession(t *testing.T) (context.Context, *InMemoryStore, Session) {
	t.Helper()
	ctx := context.Background()
	store := NewInMemoryStore()
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	created, err := store.Create(ctx, Session{
		ID: "sess_memory_status", WorkspaceID: "workspace-1", UserID: "user-1",
		Kind: KindWorkspace, Status: StatusActive, ProblemID: "two-sum",
		State: json.RawMessage(`{"schema_version":1}`), CreatedAt: now, UpdatedAt: now, LastActivityAt: now,
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return ctx, store, created
}

func readMemoryStatus(t *testing.T, session Session) (string, string) {
	t.Helper()
	state := map[string]any{}
	if len(session.State) > 0 {
		if err := json.Unmarshal(session.State, &state); err != nil {
			t.Fatalf("decode session state: %v", err)
		}
	}
	status, _ := state["memory_update_status"].(string)
	submissionID, _ := state["last_submission_id"].(string)
	return status, submissionID
}

func TestInMemoryStoreMemoryStatusClaimAndConditionalWrites(t *testing.T) {
	ctx, store, created := newMemoryStatusTestSession(t)

	claimed, err := store.SetMemoryUpdateStatus(ctx, created.WorkspaceID, created.UserID, created.ID, "exec-older", "pending")
	if err != nil {
		t.Fatalf("claim status: %v", err)
	}
	if status, submissionID := readMemoryStatus(t, claimed); status != "pending" || submissionID != "exec-older" {
		t.Fatalf("claimed status=%q submission=%q", status, submissionID)
	}

	applied, ok, err := store.SetMemoryUpdateStatusIfCurrent(ctx, created.WorkspaceID, created.UserID, created.ID, "exec-older", "synced")
	if err != nil || !ok {
		t.Fatalf("owner conditional write applied=%t err=%v", ok, err)
	}
	if status, _ := readMemoryStatus(t, applied); status != "synced" {
		t.Fatalf("owner status = %q", status)
	}

	newer, err := store.SetMemoryUpdateStatus(ctx, created.WorkspaceID, created.UserID, created.ID, "exec-newer", "pending")
	if err != nil {
		t.Fatalf("claim newer status: %v", err)
	}
	if _, submissionID := readMemoryStatus(t, newer); submissionID != "exec-newer" {
		t.Fatalf("newer submission = %q", submissionID)
	}

	stale, ok, err := store.SetMemoryUpdateStatusIfCurrent(ctx, created.WorkspaceID, created.UserID, created.ID, "exec-older", "failed")
	if err != nil {
		t.Fatalf("stale conditional write: %v", err)
	}
	if ok {
		t.Fatalf("stale conditional write applied: %#v", stale)
	}
	if status, submissionID := readMemoryStatus(t, stale); status != "pending" || submissionID != "exec-newer" {
		t.Fatalf("stale write corrupted status=%q submission=%q", status, submissionID)
	}

	legacy, ok, err := store.SetMemoryUpdateStatusIfCurrent(ctx, created.WorkspaceID, created.UserID, created.ID, "", "idle")
	if err != nil || !ok {
		t.Fatalf("legacy conditional write applied=%t err=%v", ok, err)
	}
	if status, submissionID := readMemoryStatus(t, legacy); status != "idle" || submissionID != "exec-newer" {
		t.Fatalf("legacy write status=%q submission=%q", status, submissionID)
	}

	if _, _, err := store.SetMemoryUpdateStatusIfCurrent(ctx, "workspace-1", "user-1", "missing", "exec-older", "synced"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing session error = %v", err)
	}
}

func TestValidateMemoryUpdateStatus(t *testing.T) {
	for _, status := range []string{"idle", "pending", "synced", "failed"} {
		if err := validateMemoryUpdateStatus(status); err != nil {
			t.Fatalf("status %q rejected: %v", status, err)
		}
	}
	if err := validateMemoryUpdateStatus("bogus"); err == nil {
		t.Fatal("unknown status accepted")
	}
}
