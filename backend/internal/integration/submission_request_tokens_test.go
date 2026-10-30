package integration_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kvn8888/codegym/backend/internal/execution"
)

func TestPostgresSubmissionRequestTokenLifecycle(t *testing.T) {
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

	store := execution.NewPostgresStore(pool)
	if err := store.EnsureSchema(ctx); err != nil {
		t.Fatalf("ensure execution schema: %v", err)
	}

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	workspaceID := "test-tenant-token-" + suffix
	userID := "test-user-token-" + suffix
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx,
			`DELETE FROM submission_request_tokens WHERE workspace_id = $1 AND user_id = $2`,
			workspaceID, userID); err != nil {
			t.Logf("cleanup submission request tokens: %v", err)
		}
	}()

	first, err := store.ReserveSubmissionRequest(ctx, workspaceID, userID, "token-1", "fingerprint-1")
	if err != nil {
		t.Fatalf("reserve fresh token: %v", err)
	}
	if !first.Created || first.Status != execution.SubmissionRequestStatusInProgress || first.RunID != "" || first.SubmissionID != "" {
		t.Fatalf("fresh reservation = %#v", first)
	}

	replay, err := store.ReserveSubmissionRequest(ctx, workspaceID, userID, "token-1", "fingerprint-1")
	if err != nil {
		t.Fatalf("reserve identical token: %v", err)
	}
	if replay.Created {
		t.Fatalf("identical reservation was reported as created: %#v", replay)
	}

	if _, err := store.ReserveSubmissionRequest(ctx, workspaceID, userID, "token-1", "fingerprint-2"); !errors.Is(err, execution.ErrSubmissionRequestConflict) {
		t.Fatalf("conflicting fingerprint error = %v", err)
	}

	if err := store.BindSubmissionRequestRun(ctx, workspaceID, userID, "token-1", "exec-1"); err != nil {
		t.Fatalf("bind run: %v", err)
	}
	bound, err := store.GetSubmissionRequest(ctx, workspaceID, userID, "token-1")
	if err != nil {
		t.Fatalf("get bound reservation: %v", err)
	}
	if bound.RunID != "exec-1" || bound.SubmissionID != "" || bound.Status != execution.SubmissionRequestStatusInProgress {
		t.Fatalf("bound reservation = %#v", bound)
	}

	if err := store.CompleteSubmissionRequest(ctx, workspaceID, userID, "token-1", "exec-1"); err != nil {
		t.Fatalf("complete reservation: %v", err)
	}
	completed, err := store.GetSubmissionRequest(ctx, workspaceID, userID, "token-1")
	if err != nil {
		t.Fatalf("get completed reservation: %v", err)
	}
	if completed.Status != execution.SubmissionRequestStatusCompleted || completed.SubmissionID != "exec-1" || completed.RunID != "exec-1" {
		t.Fatalf("completed reservation = %#v", completed)
	}

	afterCompletion, err := store.ReserveSubmissionRequest(ctx, workspaceID, userID, "token-1", "fingerprint-1")
	if err != nil {
		t.Fatalf("reserve completed token: %v", err)
	}
	if afterCompletion.Created || afterCompletion.Status != execution.SubmissionRequestStatusCompleted || afterCompletion.SubmissionID != "exec-1" {
		t.Fatalf("completed replay reservation = %#v", afterCompletion)
	}

	otherScope, err := store.ReserveSubmissionRequest(ctx, workspaceID, "another-user-"+suffix, "token-1", "fingerprint-1")
	if err != nil {
		t.Fatalf("reserve token in another scope: %v", err)
	}
	if !otherScope.Created {
		t.Fatalf("token leaked across user scope: %#v", otherScope)
	}
	if _, err := pool.Exec(ctx,
		`DELETE FROM submission_request_tokens WHERE workspace_id = $1 AND user_id = $2`,
		workspaceID, "another-user-"+suffix); err != nil {
		t.Logf("cleanup cross-scope token: %v", err)
	}
}
