package integration_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/execution"
	"github.com/kvn8888/codegym/backend/internal/identity"
)

func TestNeonExecutionBootstrap(t *testing.T) {
	databaseURL := os.Getenv("CODEGYM_TEST_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("NEON_CONNECTION_STRING")
	}
	if databaseURL == "" {
		t.Skip("set CODEGYM_TEST_DATABASE_URL or NEON_CONNECTION_STRING to run Neon integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("configure Postgres pool: %v", err)
	}
	defer pool.Close()

	identityStore := identity.NewPostgresStore(pool)
	if err := identityStore.EnsureSchema(ctx); err != nil {
		t.Fatalf("ensure identity schema: %v", err)
	}
	executionStore := execution.NewPostgresStore(pool)
	// Twice: EnsureSchema must be idempotent.
	for i := 0; i < 2; i++ {
		if err := executionStore.EnsureSchema(ctx); err != nil {
			t.Fatalf("ensure execution schema (pass %d): %v", i+1, err)
		}
	}
	assertExecutionConstraints(t, ctx, pool)

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	userID := "test-user-exec-" + suffix
	workspaceID := "test-workspace-exec-" + suffix
	defer cleanupExecutionRows(t, pool, workspaceID, userID)

	identityService := identity.NewService(identityStore)
	if err := identityService.EnsurePersonalWorkspace(ctx, auth.Principal{
		UserID:             userID,
		DefaultWorkspaceID: workspaceID,
		WorkspaceIDs:       []string{workspaceID},
	}); err != nil {
		t.Fatalf("ensure personal workspace: %v", err)
	}

	created := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	run := execution.Run{
		ID:          "exec_run_test_" + suffix,
		WorkspaceID: workspaceID,
		UserID:      userID,
		ProblemID:   "two-sum",
		Language:    "python",
		Entrypoint:  "test_solution.py",
		Files: []execution.File{
			{Path: "solution.py", Content: "def two_sum(): ..."},
			{Path: "test_solution.py", Content: "import solution"},
		},
		Status:    execution.StatusQueued,
		CreatedAt: created,
	}
	if err := executionStore.CreateRun(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}

	// Queued: nullable columns are nil.
	fetched, err := executionStore.GetRun(ctx, workspaceID, userID, run.ID)
	if err != nil {
		t.Fatalf("get queued run: %v", err)
	}
	if fetched.ExitCode != nil || fetched.CompletedAt != nil {
		t.Fatalf("expected nil exit_code/completed_at on queued run, got %v/%v", fetched.ExitCode, fetched.CompletedAt)
	}
	if len(fetched.Files) != 2 || fetched.Files[0].Path != "solution.py" {
		t.Fatalf("files did not round-trip: %+v", fetched.Files)
	}

	// Terminal update: nullable columns set.
	exitCode := 0
	completed := created.Add(3 * time.Second)
	run.Status = execution.StatusPassed
	run.ExitCode = &exitCode
	run.Output = "PASS 3 cases\n"
	run.JudgeResult = &execution.JudgeResult{
		Schema: execution.JudgeSchema, Status: execution.JudgeStatusPassed,
		ExitCode: &exitCode, DurationMs: 1500, Stdout: "debug\n",
		Cases: []execution.CaseResult{{Name: "case-1", Status: "pass", DurationMs: 2}},
	}
	run.DurationMs = 1500
	run.CompletedAt = &completed
	if err := executionStore.UpdateRun(ctx, run); err != nil {
		t.Fatalf("update run: %v", err)
	}

	fetched, err = executionStore.GetRun(ctx, workspaceID, userID, run.ID)
	if err != nil {
		t.Fatalf("get completed run: %v", err)
	}
	if fetched.Status != execution.StatusPassed || fetched.ExitCode == nil || *fetched.ExitCode != 0 {
		t.Fatalf("terminal state did not round-trip: %+v", fetched)
	}
	if fetched.JudgeResult == nil || fetched.JudgeResult.Status != execution.JudgeStatusPassed || fetched.JudgeResult.Stdout != "debug\n" {
		t.Fatalf("judge result did not round-trip: %+v", fetched.JudgeResult)
	}
	if fetched.CompletedAt == nil || !fetched.CompletedAt.Equal(completed) {
		t.Fatalf("expected completed_at %s, got %v", completed, fetched.CompletedAt)
	}

	// Scoped reads.
	if _, err := executionStore.GetRun(ctx, "other-workspace", userID, run.ID); err != execution.ErrRunNotFound {
		t.Fatalf("expected ErrRunNotFound for cross-workspace get, got %v", err)
	}
	runs, err := executionStore.ListRuns(ctx, workspaceID, userID)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != run.ID {
		t.Fatalf("expected the created run in list, got %+v", runs)
	}

	// Updating a nonexistent run reports ErrRunNotFound.
	missing := run
	missing.ID = "exec_run_missing_" + suffix
	if err := executionStore.UpdateRun(ctx, missing); err != execution.ErrRunNotFound {
		t.Fatalf("expected ErrRunNotFound for missing update, got %v", err)
	}

	assertExecutionOrphanRejected(t, ctx, pool)
}

func assertExecutionConstraints(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()

	for _, name := range []string{"fk_execution_runs_membership", "chk_execution_runs_status"} {
		var exists bool
		err := pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM pg_constraint
				WHERE conname = $1
					AND conrelid = 'execution_runs'::regclass
			)
		`, name).Scan(&exists)
		if err != nil {
			t.Fatalf("check constraint %s: %v", name, err)
		}
		if !exists {
			t.Fatalf("expected constraint %s to exist", name)
		}
	}
}

func assertExecutionOrphanRejected(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()

	_, err := pool.Exec(ctx, `
		INSERT INTO execution_runs (
			id, workspace_id, user_id, problem_id, language, entrypoint,
			files, status, created_at
		)
		VALUES ($1, 'missing-workspace', 'missing-user', '', 'python', 'x.py', '[]'::jsonb, 'queued', now())
	`, "exec_run_orphan_"+fmt.Sprint(time.Now().UnixNano()))
	if err == nil {
		t.Fatal("expected orphan execution run insert to fail")
	}
}

func cleanupExecutionRows(t *testing.T, pool *pgxpool.Pool, workspaceID, userID string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	statements := []struct {
		sql  string
		args []any
	}{
		{sql: `DELETE FROM execution_runs WHERE workspace_id = $1 AND user_id = $2`, args: []any{workspaceID, userID}},
		{sql: `DELETE FROM workspace_memberships WHERE workspace_id = $1 AND user_id = $2`, args: []any{workspaceID, userID}},
		{sql: `DELETE FROM workspaces WHERE id = $1`, args: []any{workspaceID}},
		{sql: `DELETE FROM app_users WHERE id = $1`, args: []any{userID}},
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Logf("cleanup failed for %q: %v", statement.sql, err)
		}
	}
}
