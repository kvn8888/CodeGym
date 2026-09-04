package integration_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/identity"
	"github.com/kvn8888/codegym/backend/internal/workflow"
	"github.com/kvn8888/codegym/backend/internal/workspace"
)

// TestNeonProblemGenerationWorkflow proves the Postgres workflow store accepts
// problem_generation operations, including on databases created before that
// kind existed. It first reinstalls the legacy three-kind chk_workflow_kind
// constraint, then runs EnsureSchema and requires the repair to admit the new
// kind. Without the repair, Create fails the CHECK and this test fails.
func TestNeonProblemGenerationWorkflow(t *testing.T) {
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

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping Postgres: %v", err)
	}

	identityStore := identity.NewPostgresStore(pool)
	workflowStore := workflow.NewPostgresStore(pool)
	if err := identityStore.EnsureSchema(ctx); err != nil {
		t.Fatalf("ensure identity schema: %v", err)
	}
	if err := workflowStore.EnsureSchema(ctx); err != nil {
		t.Fatalf("ensure workflow schema: %v", err)
	}

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	userID := "test-problem-user-" + suffix
	workspaceID := "test-problem-workspace-" + suffix
	defer cleanupRows(t, pool, workspaceID, userID)

	principal := auth.Principal{
		UserID:             userID,
		DefaultWorkspaceID: workspaceID,
		WorkspaceIDs:       []string{workspaceID},
	}
	if err := identity.NewService(identityStore).EnsurePersonalWorkspace(ctx, principal); err != nil {
		t.Fatalf("ensure personal workspace: %v", err)
	}

	// Simulate a database created before problem_generation existed.
	if _, err := pool.Exec(ctx, `ALTER TABLE workflow_operations DROP CONSTRAINT IF EXISTS chk_workflow_kind`); err != nil {
		t.Fatalf("drop kind constraint: %v", err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE workflow_operations
		ADD CONSTRAINT chk_workflow_kind
		CHECK (kind IN ('mcq_generation', 'memory_reflection', 'mcq_next_round'))`); err != nil {
		t.Fatalf("install legacy kind constraint: %v", err)
	}

	// A legacy operation must still succeed before the repair runs.
	requestCtx := auth.WithPrincipal(ctx, principal)
	requestCtx = workspace.WithScope(requestCtx, workspace.Scope{WorkspaceID: workspaceID})
	legacyService := workflow.NewService(workflowStore, nil)
	if _, err := legacyService.Create(requestCtx, workflow.CreateInput{Kind: workflow.KindMCQGeneration}); err != nil {
		t.Fatalf("create legacy workflow operation: %v", err)
	}

	// The repair runs as part of normal schema bootstrap.
	if err := workflowStore.EnsureSchema(ctx); err != nil {
		t.Fatalf("ensure repaired workflow schema: %v", err)
	}
	var definition string
	if err := pool.QueryRow(ctx, `SELECT pg_get_constraintdef(oid)
		FROM pg_constraint
		WHERE conname = 'chk_workflow_kind'
			AND conrelid = 'workflow_operations'::regclass`).Scan(&definition); err != nil {
		t.Fatalf("read kind constraint definition: %v", err)
	}
	if !strings.Contains(definition, "problem_generation") {
		t.Fatalf("kind constraint was not repaired: %s", definition)
	}

	service := workflow.NewService(workflowStore, nil)
	created, err := service.Create(requestCtx, workflow.CreateInput{Kind: workflow.KindProblemGeneration})
	if err != nil {
		t.Fatalf("create problem_generation workflow operation: %v", err)
	}
	if len(created.Events) != 5 {
		t.Fatalf("got %d seeded events, want 5", len(created.Events))
	}
	reporter, err := service.Attach(requestCtx, created.Operation.ID)
	if err != nil {
		t.Fatalf("attach problem_generation workflow operation: %v", err)
	}
	if err := reporter.Report(requestCtx, "load_context", workflow.StatusRunning, nil, false); err != nil {
		t.Fatalf("append problem_generation workflow event: %v", err)
	}
	events, err := service.Events(requestCtx, created.Operation.ID, 5)
	if err != nil {
		t.Fatalf("list problem_generation workflow events: %v", err)
	}
	if len(events) != 1 || events[0].StepID != "load_context" || events[0].Sequence != 6 {
		t.Fatalf("workflow events = %#v", events)
	}
}
