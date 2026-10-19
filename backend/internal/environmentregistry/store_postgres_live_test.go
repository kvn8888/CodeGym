package environmentregistry

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestPostgresStoreRoundTrip exercises the real SQL against a live database.
// It never runs unasked: set CODEGYM_TEST_DATABASE_URL to the development
// database to run it. Every row it writes is namespaced and removed again, so
// it leaves the database as it found it.
func TestPostgresStoreRoundTrip(t *testing.T) {
	url := strings.TrimSpace(os.Getenv("CODEGYM_TEST_DATABASE_URL"))
	if url == "" {
		t.Skip("set CODEGYM_TEST_DATABASE_URL to the development database to run the registry round trip")
	}
	ctx := t.Context()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	// Registered first so it runs last: cleanup needs the pool alive, and it
	// cannot reuse the test context, which is already cancelled by then.
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}

	store := NewPostgresStore(pool)
	if err := store.EnsureSchema(ctx); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}

	artifactID := "codegym-registry-roundtrip-test"
	technology := "codegym-registry-roundtrip-technology"
	cleanup := func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM environment_artifacts WHERE artifact_id = $1`, artifactID); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	}
	cleanup()
	defer cleanup()

	if _, err := store.GetArtifact(ctx, artifactID, "1.0.0"); !errors.Is(err, ErrArtifactNotFound) {
		t.Fatalf("expected ErrArtifactNotFound before insert, got %v", err)
	}

	record := fullyPopulatedRecord()
	record.ArtifactID = artifactID
	record.ArtifactVersion = "1.0.0"
	record.Technology = technology
	record.CreatedAt = time.Now().UTC().Truncate(time.Microsecond)
	record.LastBootCheckAt = time.Time{}
	if err := store.SaveArtifact(ctx, record); err != nil {
		t.Fatalf("save artifact: %v", err)
	}

	loaded, err := store.GetArtifact(ctx, artifactID, "1.0.0")
	if err != nil {
		t.Fatalf("get artifact: %v", err)
	}
	if !sameRecord(loaded, record) {
		t.Fatalf("loaded record differs:\n got %+v\nwant %+v", loaded, record)
	}
	if !loaded.LastBootCheckAt.IsZero() {
		t.Fatalf("expected zero boot-check time, got %s", loaded.LastBootCheckAt)
	}

	listed, err := store.ListByTechnology(ctx, technology)
	if err != nil {
		t.Fatalf("list by technology: %v", err)
	}
	if len(listed) != 1 || listed[0].ArtifactID != artifactID {
		t.Fatalf("expected the saved artifact in the technology listing, got %+v", listed)
	}

	bootedAt := time.Now().UTC().Truncate(time.Microsecond)
	if err := store.UpdateValidation(ctx, artifactID, "1.0.0", ValidationBootCheckFailed, bootedAt); err != nil {
		t.Fatalf("update validation: %v", err)
	}
	updated, err := store.GetArtifact(ctx, artifactID, "1.0.0")
	if err != nil {
		t.Fatalf("get artifact after update: %v", err)
	}
	if updated.ValidationState != ValidationBootCheckFailed || !updated.LastBootCheckAt.Equal(bootedAt) {
		t.Fatalf("validation update not persisted: state=%s booted=%s", updated.ValidationState, updated.LastBootCheckAt)
	}

	// Same id/version replaces rather than forks, matching InMemoryStore.
	record.BuildCommand = "mvn -q verify"
	record.ValidationState = ValidationPassed
	record.LastBootCheckAt = bootedAt
	if err := store.SaveArtifact(ctx, record); err != nil {
		t.Fatalf("re-save artifact: %v", err)
	}
	replaced, err := store.GetArtifact(ctx, artifactID, "1.0.0")
	if err != nil {
		t.Fatalf("get replaced artifact: %v", err)
	}
	if replaced.BuildCommand != "mvn -q verify" || replaced.ValidationState != ValidationPassed {
		t.Fatalf("replacement did not overwrite: %+v", replaced)
	}
	if err := store.UpdateValidation(ctx, artifactID, "9.9.9", ValidationPassed, bootedAt); !errors.Is(err, ErrArtifactNotFound) {
		t.Fatalf("expected ErrArtifactNotFound for unknown version, got %v", err)
	}
}

// sameRecord compares everything that survives a database round trip. Times
// are compared as instants so Postgres timezone handling never matters, then
// blanked so the remaining fields compare structurally.
func sameRecord(got, want ArtifactRecord) bool {
	if !got.CreatedAt.UTC().Equal(want.CreatedAt.UTC()) {
		return false
	}
	if !got.LastBootCheckAt.UTC().Equal(want.LastBootCheckAt.UTC()) {
		return false
	}
	got.CreatedAt, want.CreatedAt = time.Time{}, time.Time{}
	got.LastBootCheckAt, want.LastBootCheckAt = time.Time{}, time.Time{}
	return reflect.DeepEqual(got, want)
}
