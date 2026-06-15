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
		Type:    "problem_requested",
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

func scopedContext() context.Context {
	ctx := context.Background()
	ctx = auth.WithPrincipal(ctx, auth.Principal{
		UserID:          "user-1",
		DefaultTenantID: "tenant-1",
		TenantIDs:       []string{"tenant-1"},
	})
	return tenant.WithScope(ctx, tenant.Scope{TenantID: "tenant-1"})
}
