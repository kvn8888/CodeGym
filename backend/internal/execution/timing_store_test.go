package execution

import (
	"context"
	"testing"
	"time"
)

func TestInMemoryStoreRunTimingInsertAndScopedQuery(t *testing.T) {
	store := NewInMemoryStore()
	if err := store.EnsureSchema(context.Background()); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	created := time.Date(2026, 8, 4, 22, 15, 0, 0, time.UTC)
	records := []RunTiming{
		{
			ID: "timing-1", RunID: "run-1", WorkspaceID: "workspace-1", UserID: "user-1",
			Snapshot: GoSnapshotName, Language: "go", Strategy: TestStrategyHTTP,
			CreateRetried: true, CreateMs: 22000, UploadMs: 450, ExecMs: 1600, TotalMs: 24050,
			CreatedAt: created,
		},
		{
			ID: "timing-2", RunID: "run-2", WorkspaceID: "workspace-2", UserID: "user-2",
			Language: "python", Strategy: TestStrategyUnit, CreateMs: 70, TotalMs: 300,
			CreatedAt: created.Add(time.Second),
		},
	}
	for _, record := range records {
		if err := store.AppendRunTiming(context.Background(), record); err != nil {
			t.Fatalf("AppendRunTiming: %v", err)
		}
	}

	got, err := store.ListRunTimings(context.Background(), "workspace-1", "user-1")
	if err != nil {
		t.Fatalf("ListRunTimings: %v", err)
	}
	if len(got) != 1 || got[0].ID != "timing-1" {
		t.Fatalf("scoped timings = %#v", got)
	}
	if !got[0].CreateRetried || got[0].Snapshot != GoSnapshotName {
		t.Fatalf("inserted timing lost dimensions: %#v", got[0])
	}
}
