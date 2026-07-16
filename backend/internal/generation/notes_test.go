package generation

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/memory"
)

func seedRoundEvents(t *testing.T, ctx context.Context, service *memory.Service, sessionID string, outcomes map[string]bool) {
	t.Helper()
	for concept, correct := range outcomes {
		eventType := "question_answered"
		if !correct {
			eventType = "answer_incorrect"
		}
		payload, _ := json.Marshal(map[string]any{
			"session_id": sessionID,
			"topic":      concept,
			"correct":    correct,
		})
		if _, err := service.RecordEvent(ctx, memory.RecordEventInput{
			Source:  "mcq",
			Type:    eventType,
			Summary: fmt.Sprintf("%s on %s", eventType, concept),
			Payload: payload,
		}); err != nil {
			t.Fatalf("seed event: %v", err)
		}
	}
}

func TestMaintainNotesAppliesLLMActions(t *testing.T) {
	ctx := scopedContext()
	store := memory.NewInMemoryStore()
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	service := memory.NewService(store, func() time.Time { return now })

	seedRoundEvents(t, ctx, service, "mcq_r1", map[string]bool{"SQL Joins": false, "Two Pointers": true})

	actionsJSON := `{"actions":[
		{"op":"create","note":{"id":"note_sql-joins","title":"SQL joins","summary":"Missed LEFT JOIN NULL semantics; revisit join direction.","tags":["sql"],"action":"review"}}
	],"reason":"missed sql"}`
	generator := &scriptedGenerator{payloads: []json.RawMessage{json.RawMessage(actionsJSON)}}
	orchestrator := NewOrchestrator(service, generator)

	result, err := MaintainNotes(ctx, orchestrator, service, MaintainNotesInput{SessionID: "mcq_r1", Now: now})
	if err != nil {
		t.Fatalf("MaintainNotes: %v", err)
	}
	if result.Skipped != "" {
		t.Fatalf("unexpected skip: %s", result.Skipped)
	}
	if len(result.AppliedActions) != 1 {
		t.Fatalf("applied = %d, want 1", len(result.AppliedActions))
	}
	if len(result.Profile.Notes) != 1 || result.Profile.Notes[0].ID != "note_sql-joins" {
		t.Fatalf("notes = %#v", result.Profile.Notes)
	}
	if result.Profile.Notes[0].Action != "review" {
		t.Fatalf("note action = %q", result.Profile.Notes[0].Action)
	}

	// The engagement digest must have reached the model.
	request := generator.requests[0]
	if request.Kind != KindNotes {
		t.Fatalf("kind = %q", request.Kind)
	}
	var engagement sessionEngagement
	if err := json.Unmarshal(request.Spec, &engagement); err != nil {
		t.Fatalf("decode engagement: %v", err)
	}
	if engagement.SessionID != "mcq_r1" || engagement.Total != 2 || engagement.CorrectCount != 1 {
		t.Fatalf("engagement = %+v", engagement)
	}

	// The persisted profile is what the next generation round reads.
	profile, err := service.GetProfile(ctx)
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if len(profile.Notes) != 1 || profile.Notes[0].ID != "note_sql-joins" {
		t.Fatalf("persisted notes = %#v", profile.Notes)
	}
}

func TestMaintainNotesSurvivesDeterministicRefresh(t *testing.T) {
	ctx := scopedContext()
	store := memory.NewInMemoryStore()
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	service := memory.NewService(store, func() time.Time { return now })

	seedRoundEvents(t, ctx, service, "mcq_r1", map[string]bool{"Caching": false})
	if _, err := service.ReplaceNotes(ctx, []memory.Note{{
		ID: "note_caching", Title: "Caching", Summary: "Revisit LRU eviction.", CreatedAt: now, Action: "review",
	}}); err != nil {
		t.Fatalf("ReplaceNotes: %v", err)
	}

	refreshed, err := service.RefreshProfile(ctx)
	if err != nil {
		t.Fatalf("RefreshProfile: %v", err)
	}
	if len(refreshed.Notes) != 1 || refreshed.Notes[0].ID != "note_caching" {
		t.Fatalf("deterministic refresh dropped LLM notes: %#v", refreshed.Notes)
	}
}

func TestMaintainNotesSkipsWithoutOrchestrator(t *testing.T) {
	ctx := scopedContext()
	service := memory.NewService(memory.NewInMemoryStore(), nil)
	seedRoundEvents(t, ctx, service, "mcq_r1", map[string]bool{"Graphs": false})

	result, err := MaintainNotes(ctx, nil, service, MaintainNotesInput{SessionID: "mcq_r1"})
	if err != nil {
		t.Fatalf("MaintainNotes: %v", err)
	}
	if result.Skipped == "" {
		t.Fatal("expected skip reason without orchestrator")
	}
	if len(result.AppliedActions) != 0 {
		t.Fatal("no actions should apply without orchestrator")
	}
}

func TestMaintainNotesSkipsWithoutEngagement(t *testing.T) {
	ctx := scopedContext()
	service := memory.NewService(memory.NewInMemoryStore(), nil)
	generator := &scriptedGenerator{payloads: []json.RawMessage{json.RawMessage(`{"actions":[]}`)}}
	orchestrator := NewOrchestrator(service, generator)

	result, err := MaintainNotes(ctx, orchestrator, service, MaintainNotesInput{})
	if err != nil {
		t.Fatalf("MaintainNotes: %v", err)
	}
	if result.Skipped == "" {
		t.Fatal("expected skip with empty event log")
	}
	if len(generator.requests) != 0 {
		t.Fatal("model should not be called without engagement")
	}
}

func TestMaintainNotesToleratesBadModelOutput(t *testing.T) {
	ctx := scopedContext()
	service := memory.NewService(memory.NewInMemoryStore(), nil)
	seedRoundEvents(t, ctx, service, "mcq_r1", map[string]bool{"Graphs": false})
	generator := &scriptedGenerator{payloads: []json.RawMessage{json.RawMessage(`"not actions"`)}}
	orchestrator := NewOrchestrator(service, generator)

	result, err := MaintainNotes(ctx, orchestrator, service, MaintainNotesInput{SessionID: "mcq_r1"})
	if err != nil {
		t.Fatalf("MaintainNotes should be best-effort, got: %v", err)
	}
	if result.Skipped == "" {
		t.Fatal("expected skip reason for unusable output")
	}
}

func TestParseNoteActionsSanitizes(t *testing.T) {
	raw := `{"actions":[
		{"op":"CREATE","note":{"title":"SQL joins","summary":"Revisit LEFT JOIN.","action":"Review"}},
		{"op":"create","note":{"title":"","summary":"missing title dropped"}},
		{"op":"update","note":{"id":"","summary":"missing id dropped"}},
		{"op":"prune","note":{"id":"note_old"}},
		{"op":"explode","note":{"id":"note_x"}}
	]}`
	actions, err := ParseNoteActions(json.RawMessage(raw))
	if err != nil {
		t.Fatalf("ParseNoteActions: %v", err)
	}
	if len(actions) != 2 {
		t.Fatalf("actions = %#v, want create+prune", actions)
	}
	if actions[0].Op != "create" || actions[0].Note.ID != "note_sql-joins" || actions[0].Note.Action != "review" {
		t.Fatalf("create action = %#v", actions[0])
	}
	if actions[1].Op != "prune" || actions[1].Note.ID != "note_old" {
		t.Fatalf("prune action = %#v", actions[1])
	}
}

func TestApplyNoteActionsCRUDAndCap(t *testing.T) {
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	existing := []memory.Note{
		{ID: "note_a", Title: "A", Summary: "old", CreatedAt: now.Add(-2 * time.Hour), Action: "review"},
		{ID: "note_b", Title: "B", Summary: "keep me", CreatedAt: now.Add(-1 * time.Hour), Action: "keep"},
	}
	actions := []NoteAction{
		{Op: "update", Note: NoteChange{ID: "note_a", Summary: "updated summary", Action: "keep"}},
		{Op: "prune", Note: NoteChange{ID: "note_b"}},
		{Op: "create", Note: NoteChange{ID: "note_c", Title: "C", Summary: "new gap", Action: "review", Tags: []string{"sql"}}},
		{Op: "prune", Note: NoteChange{ID: "note_unknown"}}, // no-op
	}

	next := ApplyNoteActions(existing, actions, now)
	if len(next) != 2 {
		t.Fatalf("next = %#v", next)
	}
	// Newest first: note_c (created now) before note_a.
	if next[0].ID != "note_c" || next[1].ID != "note_a" {
		t.Fatalf("order = %s, %s", next[0].ID, next[1].ID)
	}
	if next[1].Summary != "updated summary" || next[1].Action != "keep" || next[1].Title != "A" {
		t.Fatalf("updated note = %#v", next[1])
	}

	// Cap: 25 creates keep only the newest 20.
	many := []NoteAction{}
	for i := 0; i < 25; i++ {
		many = append(many, NoteAction{Op: "create", Note: NoteChange{
			ID: fmt.Sprintf("note_%02d", i), Title: fmt.Sprintf("N%d", i), Summary: "s",
		}})
	}
	capped := ApplyNoteActions(nil, many, now)
	if len(capped) != maxNotes {
		t.Fatalf("capped = %d, want %d", len(capped), maxNotes)
	}
}

func TestApplyNoteActionsUpdatesMatchingConceptInsteadOfAppending(t *testing.T) {
	now := time.Date(2026, 7, 15, 18, 0, 0, 0, time.UTC)
	createdAt := now.Add(-24 * time.Hour)
	existing := []memory.Note{{
		ID: "note_sql_original", ProblemID: "problem-sql-1", Title: "SQL joins",
		Summary: "Old summary.", CreatedAt: createdAt, Action: "review",
	}}
	actions := []NoteAction{{
		Op: "create",
		Note: NoteChange{
			ID: "note_sql_duplicate", ProblemID: "problem-sql-1", Title: "SQL joins",
			Summary: "Updated summary.", Action: "keep",
		},
	}}

	next := ApplyNoteActions(existing, actions, now)
	if len(next) != 1 {
		t.Fatalf("notes = %#v", next)
	}
	if next[0].ID != "note_sql_original" || next[0].Summary != "Updated summary." || next[0].Action != "keep" {
		t.Fatalf("note = %#v", next[0])
	}
	if !next[0].CreatedAt.Equal(createdAt) {
		t.Fatalf("created_at = %s", next[0].CreatedAt)
	}
}
