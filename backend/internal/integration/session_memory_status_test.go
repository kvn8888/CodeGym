package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/identity"
	"github.com/kvn8888/codegym/backend/internal/memory"
	"github.com/kvn8888/codegym/backend/internal/session"
)

func TestPostgresSessionMemoryStatusIsAtomicAndPreservesSnapshots(t *testing.T) {
	databaseURL := os.Getenv("CODEGYM_TEST_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("NEON_CONNECTION_STRING")
	}
	if databaseURL == "" {
		t.Skip("set CODEGYM_TEST_DATABASE_URL or NEON_CONNECTION_STRING to run Postgres integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("configure Postgres pool: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping Postgres: %v", err)
	}

	identityStore := identity.NewPostgresStore(pool)
	sessionStore := session.NewPostgresStore(pool)
	memoryStore := memory.NewPostgresStore(pool)
	if err := identityStore.EnsureSchema(ctx); err != nil {
		t.Fatalf("ensure identity schema: %v", err)
	}
	if err := sessionStore.EnsureSchema(ctx); err != nil {
		t.Fatalf("ensure session schema: %v", err)
	}
	if err := memoryStore.EnsureSchema(ctx); err != nil {
		t.Fatalf("ensure memory schema: %v", err)
	}

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	userID := "test-user-status-" + suffix
	workspaceID := "test-workspace-status-" + suffix
	defer cleanupRows(t, pool, workspaceID, userID)

	principal := auth.Principal{
		UserID:             userID,
		DefaultWorkspaceID: workspaceID,
		WorkspaceIDs:       []string{workspaceID},
		UserMetadata: auth.UserMetadata{
			Email:       userID + "@example.com",
			DisplayName: "Status User " + suffix,
		},
	}
	if err := identity.NewService(identityStore).EnsurePersonalWorkspace(ctx, principal); err != nil {
		t.Fatalf("ensure personal workspace: %v", err)
	}

	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	created, err := sessionStore.Create(ctx, session.Session{
		ID:             "sess_status_" + suffix,
		WorkspaceID:    workspaceID,
		UserID:         userID,
		Kind:           session.KindWorkspace,
		Status:         session.StatusActive,
		ProblemID:      "two-sum",
		State:          json.RawMessage(`{"schema_version":1,"hints_revealed":0}`),
		CreatedAt:      now,
		UpdatedAt:      now,
		LastActivityAt: now,
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	claimed, err := sessionStore.SetMemoryUpdateStatus(ctx, workspaceID, userID, created.ID, "exec-older", "pending")
	if err != nil {
		t.Fatalf("claim status: %v", err)
	}
	if memoryStatus(claimed) != "pending" || lastSubmissionID(claimed) != "exec-older" {
		t.Fatalf("claimed session = %#v", claimed)
	}

	applied, ok, err := sessionStore.SetMemoryUpdateStatusIfCurrent(ctx, workspaceID, userID, created.ID, "exec-older", "synced")
	if err != nil || !ok {
		t.Fatalf("conditional write for owner applied=%t err=%v", ok, err)
	}
	if memoryStatus(applied) != "synced" {
		t.Fatalf("owner status = %#v", applied)
	}

	newer, err := sessionStore.SetMemoryUpdateStatus(ctx, workspaceID, userID, created.ID, "exec-newer", "pending")
	if err != nil {
		t.Fatalf("claim newer status: %v", err)
	}
	if lastSubmissionID(newer) != "exec-newer" {
		t.Fatalf("newer session = %#v", newer)
	}

	stale, ok, err := sessionStore.SetMemoryUpdateStatusIfCurrent(ctx, workspaceID, userID, created.ID, "exec-older", "failed")
	if err != nil {
		t.Fatalf("conditional stale write: %v", err)
	}
	if ok {
		t.Fatalf("stale write unexpectedly applied: %#v", stale)
	}
	if memoryStatus(stale) != "pending" || lastSubmissionID(stale) != "exec-newer" {
		t.Fatalf("stale write corrupted newer status: %#v", stale)
	}

	if _, _, err := sessionStore.SetMemoryUpdateStatusIfCurrent(ctx, workspaceID, userID, "sess_missing_"+suffix, "exec-older", "synced"); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("missing session error = %v", err)
	}

	note := memory.Note{
		ID: "note_snapshot", ProblemID: "two-sum", Title: "Two Sum",
		Summary: "Snapshot retained for the coding summary.", Tags: []string{"dsa"},
		Action: "review", CreatedAt: now,
	}
	profile := memory.Profile{
		Summary:      "Snapshot profile",
		UpdatedAt:    now,
		NextReviewAt: now.Add(24 * time.Hour),
		Strengths:    []string{},
		GrowthEdges:  []string{},
		Skills:       []memory.SkillProficiency{},
		Notes:        []memory.Note{note},
		Provenance: &memory.ProfileProvenance{
			SchemaVersion:   1,
			Trigger:         "set-completion",
			Provider:        "test",
			Model:           "test-model",
			SynthesizedAt:   now,
			EvidenceThrough: now,
			MaintainedNotes: []memory.Note{note},
		},
	}
	if err := memoryStore.ReplaceProfileIfVersion(ctx, workspaceID, userID, profile, 0); err != nil {
		t.Fatalf("save profile snapshot: %v", err)
	}
	stored, err := memoryStore.GetProfile(ctx, workspaceID, userID)
	if err != nil {
		t.Fatalf("get profile snapshot: %v", err)
	}
	if len(stored.Notes) != 1 || stored.Notes[0].ID != note.ID {
		t.Fatalf("stored notes = %#v", stored.Notes)
	}
	if stored.Provenance == nil || len(stored.Provenance.MaintainedNotes) != 1 || stored.Provenance.MaintainedNotes[0].ID != note.ID {
		t.Fatalf("stored maintained-note snapshot = %#v", stored.Provenance)
	}
}

func memoryStatus(session session.Session) string {
	state := map[string]any{}
	if len(session.State) > 0 {
		_ = json.Unmarshal(session.State, &state)
	}
	status, _ := state["memory_update_status"].(string)
	return status
}

func lastSubmissionID(session session.Session) string {
	state := map[string]any{}
	if len(session.State) > 0 {
		_ = json.Unmarshal(session.State, &state)
	}
	id, _ := state["last_submission_id"].(string)
	return id
}
