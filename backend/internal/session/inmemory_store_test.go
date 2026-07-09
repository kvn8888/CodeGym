package session

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestInMemoryStoreSessionLifecycle(t *testing.T) {
	ctx := context.Background()
	var store Store = NewInMemoryStore()
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)

	first, err := store.Create(ctx, Session{
		ID:             "sess_first",
		TenantID:       "personal-kevin",
		UserID:         "kevin",
		Kind:           KindWorkspace,
		Status:         StatusActive,
		Title:          "Graph traversal",
		ProblemID:      "prob_graph",
		State:          json.RawMessage(`{"schema_version":1,"language":"go"}`),
		CreatedAt:      now,
		UpdatedAt:      now,
		LastActivityAt: now,
	})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}

	later := now.Add(time.Hour)
	_, err = store.Create(ctx, Session{
		ID:             "sess_second",
		TenantID:       "personal-kevin",
		UserID:         "kevin",
		Kind:           KindMCQ,
		Status:         StatusCompleted,
		Title:          "Cloud MCQ",
		State:          json.RawMessage(`{"schema_version":1,"current_index":3}`),
		CreatedAt:      later,
		UpdatedAt:      later,
		LastActivityAt: later,
	})
	if err != nil {
		t.Fatalf("create second: %v", err)
	}

	list, err := store.List(ctx, "personal-kevin", "kevin", ListFilter{Limit: 10})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("list len = %d, want 2", len(list))
	}
	if list[0].ID != "sess_second" || list[1].ID != "sess_first" {
		t.Fatalf("list order = %s, %s", list[0].ID, list[1].ID)
	}

	active, err := store.List(ctx, "personal-kevin", "kevin", ListFilter{
		Status: StatusActive,
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("list active: %v", err)
	}
	if len(active) != 1 || active[0].ID != first.ID {
		t.Fatalf("active list = %#v", active)
	}

	first.Title = "Updated graph traversal"
	first.Status = StatusCompleted
	completedAt := now.Add(2 * time.Hour)
	first.CompletedAt = &completedAt
	first.UpdatedAt = completedAt
	first.LastActivityAt = completedAt
	updated, err := store.Update(ctx, first)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Title != "Updated graph traversal" || updated.Status != StatusCompleted {
		t.Fatalf("updated session = %#v", updated)
	}
	if updated.CompletedAt == nil || !updated.CompletedAt.Equal(completedAt) {
		t.Fatalf("CompletedAt = %v, want %s", updated.CompletedAt, completedAt)
	}

	withFiles, err := store.UpsertFiles(ctx, "personal-kevin", "kevin", first.ID, []File{
		{Path: "main.go", Content: "package main\n", UpdatedAt: completedAt.Add(time.Minute)},
		{Path: "README.md", Content: "notes", UpdatedAt: completedAt.Add(time.Minute)},
	})
	if err != nil {
		t.Fatalf("upsert files: %v", err)
	}
	if len(withFiles.Files) != 2 {
		t.Fatalf("files len = %d, want 2", len(withFiles.Files))
	}
	if withFiles.Files[0].Path != "README.md" || withFiles.Files[1].Path != "main.go" {
		t.Fatalf("files not sorted by path: %#v", withFiles.Files)
	}
}

func TestInMemoryStoreScopesSessions(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore()
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)

	_, err := store.Create(ctx, Session{
		ID:             "sess_scoped",
		TenantID:       "personal-kevin",
		UserID:         "kevin",
		Kind:           KindWorkspace,
		Status:         StatusActive,
		State:          json.RawMessage(`{"schema_version":1}`),
		CreatedAt:      now,
		UpdatedAt:      now,
		LastActivityAt: now,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if _, err := store.Get(ctx, "personal-alec", "alec", "sess_scoped"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-scope get err = %v, want ErrNotFound", err)
	}
}
