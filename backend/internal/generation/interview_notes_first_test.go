package generation

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/memory"
)

func TestInterviewNotesFirstRequiresFinishedInterview(t *testing.T) {
	ctx := scopedContext()
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	service := memory.NewService(memory.NewInMemoryStore(), func() time.Time { return now })
	_, err := service.ReplaceProfile(ctx, memory.Profile{
		Summary: "base", UpdatedAt: now, NextReviewAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.RecordEvent(ctx, memory.RecordEventInput{
		Source: memory.SourceChat, Type: memory.TypeInterviewExited, Summary: "Exited.",
		Payload: json.RawMessage(`{"thread_id":"thread-1","session_id":"interview_r1","schema_version":1}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	generator := &scriptedGenerator{payloads: []json.RawMessage{json.RawMessage(`{"actions":[]}`), json.RawMessage(`{"summary":"x","strengths":[],"growth_edges":[],"skills":[],"notes":[]}`)}}
	synthesizer := NewProfileSynthesizer(NewOrchestrator(service, generator), service).WithClock(func() time.Time { return now })
	_, err = synthesizer.RefreshProfile(ctx, ProfileRefreshInput{
		SessionID: "interview_r1", ThreadID: "thread-1", Trigger: "set-completion",
	})
	if err == nil {
		t.Fatalf("expected error for unfinished interview, got nil")
	}
	if !errors.Is(err, ErrCodingCompletionEvidenceNotFound) && !strings.Contains(err.Error(), "completion evidence") {
		t.Fatalf("unexpected error: %v", err)
	}
}
