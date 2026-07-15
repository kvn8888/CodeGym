package memory

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/tenant"
)

func TestServiceReturnsDefaultProfileWhenMissing(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	service := NewService(NewInMemoryStore(), func() time.Time { return now })

	profile, err := service.GetProfile(scopedContext())
	if err != nil {
		t.Fatalf("GetProfile returned error: %v", err)
	}
	if profile.Summary == "" {
		t.Fatal("expected default summary")
	}
	if !profile.UpdatedAt.Equal(now) {
		t.Fatalf("expected UpdatedAt %s, got %s", now, profile.UpdatedAt)
	}
}

func TestServiceRecordsEventForScopedUser(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	service := NewService(NewInMemoryStore(), func() time.Time { return now })
	payload := json.RawMessage(`{"problem_id":"pagination-api-cache"}`)

	event, err := service.RecordEvent(scopedContext(), RecordEventInput{
		Source:  "generate",
		Type:    "intake_started",
		Summary: "Asked for pagination cache practice.",
		Payload: payload,
	})
	if err != nil {
		t.Fatalf("RecordEvent returned error: %v", err)
	}
	if event.ID == "" {
		t.Fatal("expected event id")
	}
	if event.TenantID != "tenant-1" || event.UserID != "user-1" {
		t.Fatalf("unexpected scope: %s/%s", event.TenantID, event.UserID)
	}

	events, err := service.ListEvents(scopedContext())
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
}

func TestServiceRefreshProfilePersistsDerivedMemory(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	store := NewInMemoryStore()
	service := NewService(store, func() time.Time { return now })
	ctx := scopedContext()

	_, err := service.RecordEvent(ctx, RecordEventInput{
		Source:  "workspace",
		Type:    "attempt_solved",
		Summary: "Solved graph traversal.",
		Payload: json.RawMessage(`{"skill":"Graphs","problem_id":"graph-traversal","passed":true}`),
	})
	if err != nil {
		t.Fatalf("RecordEvent returned error: %v", err)
	}

	profile, err := service.RefreshProfile(ctx)
	if err != nil {
		t.Fatalf("RefreshProfile returned error: %v", err)
	}
	if !contains(profile.Strengths, "Graphs") {
		t.Fatalf("expected Graphs strength, got %#v", profile.Strengths)
	}

	persisted, err := service.GetProfile(ctx)
	if err != nil {
		t.Fatalf("GetProfile returned error: %v", err)
	}
	if persisted.Summary != profile.Summary {
		t.Fatal("expected refreshed profile to be persisted")
	}
}

func TestServiceRefreshAllProfilesUsesEventScopes(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	store := NewInMemoryStore()
	service := NewService(store, func() time.Time { return now })

	if _, err := service.RecordEvent(scopedContext(), RecordEventInput{
		Source:  "mcq",
		Type:    "answer_incorrect",
		Summary: "Missed SQL join.",
		Payload: json.RawMessage(`{"skill":"SQL","problem_id":"sql-join","correct":false}`),
	}); err != nil {
		t.Fatalf("RecordEvent returned error: %v", err)
	}

	count, err := service.RefreshAllProfiles(context.Background())
	if err != nil {
		t.Fatalf("RefreshAllProfiles returned error: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 refreshed profile, got %d", count)
	}

	profile, err := store.GetProfile(context.Background(), "tenant-1", "user-1")
	if err != nil {
		t.Fatalf("GetProfile returned error: %v", err)
	}
	if !contains(profile.GrowthEdges, "SQL") {
		t.Fatalf("expected SQL growth edge, got %#v", profile.GrowthEdges)
	}
}

func TestWorkerRunOnceRefreshesActiveProfiles(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	store := NewInMemoryStore()
	service := NewService(store, func() time.Time { return now })
	worker := NewWorker(service, time.Minute)

	if _, err := service.RecordEvent(scopedContext(), RecordEventInput{
		Source:  "memory",
		Type:    "note_created",
		Summary: "Asked for concurrency practice.",
		Payload: json.RawMessage(`{"skill":"Concurrency","problem_id":"goroutine-worker"}`),
	}); err != nil {
		t.Fatalf("RecordEvent returned error: %v", err)
	}

	count, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 refreshed profile, got %d", count)
	}
}

func TestServiceRejectsUnknownEventName(t *testing.T) {
	service := NewService(NewInMemoryStore(), time.Now)
	_, err := service.RecordEvent(scopedContext(), RecordEventInput{
		Source:  SourceGenerate,
		Type:    "not_a_real_event",
		Summary: "bad event",
	})
	if err == nil {
		t.Fatal("expected validation error for unknown event type")
	}
}

func scopedContext() context.Context {
	ctx := context.Background()
	ctx = auth.WithPrincipal(ctx, auth.Principal{
		UserID:          "user-1",
		DefaultTenantID: "tenant-1",
		TenantIDs:       []string{"tenant-1"},
	})
	return tenant.WithScope(ctx, tenant.Scope{TenantID: "tenant-1"})
}
