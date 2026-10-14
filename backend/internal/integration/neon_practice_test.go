package integration_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/identity"
	"github.com/kvn8888/codegym/backend/internal/practice"
)

// TestNeonPracticeSavedReopenResume proves durable save/reopen/resume
// against a real Postgres: a saved exercise written through one service
// instance is resumed through a fresh instance sharing only the Postgres
// store, restoring exact edits and artifact/environment identity. It
// skips without a disposable test database and never substitutes shared
// in-memory state as durability proof.
func TestNeonPracticeSavedReopenResume(t *testing.T) {
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
	savedStore := practice.NewPostgresSavedExerciseStore(pool)
	for i := 0; i < 2; i++ {
		if err := savedStore.EnsureSchema(ctx); err != nil {
			t.Fatalf("ensure practice saved schema (pass %d): %v", i+1, err)
		}
	}

	suffix := time.Now().UTC().Format("20060102150405.000000000")
	userID := "test-user-practice-" + suffix
	workspaceID := "test-workspace-practice-" + suffix
	defer cleanupPracticeRows(t, pool, workspaceID, userID)

	identityService := identity.NewService(identityStore)
	if err := identityService.EnsurePersonalWorkspace(ctx, auth.Principal{
		UserID:             userID,
		DefaultWorkspaceID: workspaceID,
		WorkspaceIDs:       []string{workspaceID},
	}); err != nil {
		t.Fatalf("ensure personal workspace: %v", err)
	}

	newService := func() *practice.Service {
		return &practice.Service{
			Artifacts: practice.SampleArtifactSource{},
			Contents:  practice.SampleContentStore{},
			Runner:    practice.FakeSampleRunner{},
			Records:   practice.NewInMemoryRecordStore(),
			Saved:     savedStore,
			Simulated: true,
		}
	}

	// First instance: open the pinned sample, edit, durably save.
	first := newService()
	view, err := first.OpenBrowserSession(ctx, userID, workspaceID, practice.ArtifactRef{
		ArtifactID: practice.SampleArtifactID,
		Technology: "python",
		Version:    practice.SampleArtifactVersion,
	})
	if err != nil {
		t.Fatalf("open browser session: %v", err)
	}
	const edited = "def greet(name):\n    return 'Hello, Ada!'\n"
	if err := first.WriteBrowserFiles(view.SessionID, userID, workspaceID, map[string]string{"solution.py": edited}); err != nil {
		t.Fatalf("write browser files: %v", err)
	}
	saved, err := first.SaveBrowserExercise(ctx, view.SessionID, userID, workspaceID)
	if err != nil {
		t.Fatalf("save browser exercise: %v", err)
	}
	if saved.ArtifactID != practice.SampleArtifactID || saved.ArtifactVersion != practice.SampleArtifactVersion {
		t.Fatalf("saved identity = %s %s", saved.ArtifactID, saved.ArtifactVersion)
	}

	// Restart: a fresh service sharing only the Postgres store lists and
	// resumes the saved exercise with exact edits and identity.
	second := newService()
	summaries, err := second.ListBrowserSaved(ctx, userID, workspaceID)
	if err != nil {
		t.Fatalf("list saved: %v", err)
	}
	found := false
	for _, summary := range summaries {
		if summary.ID == saved.ID {
			found = true
			if summary.ArtifactID != practice.SampleArtifactID {
				t.Fatalf("summary artifact = %q", summary.ArtifactID)
			}
		}
	}
	if !found {
		t.Fatalf("saved exercise %q missing after reopen", saved.ID)
	}
	resumed, err := second.ResumeBrowserSaved(ctx, userID, workspaceID, saved.ID)
	if err != nil {
		t.Fatalf("resume saved: %v", err)
	}
	if resumed.ArtifactID != practice.SampleArtifactID || resumed.ArtifactVersion != practice.SampleArtifactVersion {
		t.Fatalf("resumed identity = %s %s", resumed.ArtifactID, resumed.ArtifactVersion)
	}
	if len(resumed.Files) != 1 || resumed.Files[0].Content != edited {
		t.Fatalf("resumed files = %+v, want exact saved edits", resumed.Files)
	}
	if resumed.SessionID == view.SessionID {
		t.Fatalf("resume reused the pre-restart session ID")
	}
}

func cleanupPracticeRows(t *testing.T, pool *pgxpool.Pool, workspaceID, userID string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	statements := []struct {
		sql  string
		args []any
	}{
		{sql: `DELETE FROM practice_saved_exercises WHERE workspace_id = $1 AND user_id = $2`, args: []any{workspaceID, userID}},
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
