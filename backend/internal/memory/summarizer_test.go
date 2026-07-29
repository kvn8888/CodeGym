package memory

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

// TestSummarizePlumbing covers the pure derivation path. Keep the assertions
// focused on behavior that must stay stable as the summarizer grows smarter:
// empty logs are valid, event signal becomes profile signal, and reruns are
// idempotent.
func TestSummarizePlumbing(t *testing.T) {
	now := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)

	// An empty event log must not panic and must yield a valid, timestamped profile.
	got := Summarize(Profile{}, nil, now)
	if !got.UpdatedAt.Equal(now) {
		t.Fatalf("UpdatedAt = %v, want %v", got.UpdatedAt, now)
	}
	if got.NextReviewAt.IsZero() {
		t.Fatal("expected NextReviewAt to be set for a fresh profile")
	}

	events := []Event{
		{
			ID:         "evt-1",
			Source:     "workspace",
			Type:       "attempt_solved",
			Summary:    "Solved pagination cache edge cases.",
			Payload:    mustJSON(t, map[string]any{"skill": "API Patterns", "problem_id": "pagination-api-cache", "passed": true}),
			OccurredAt: now.Add(-2 * time.Hour),
			CreatedAt:  now.Add(-2 * time.Hour),
		},
		{
			ID:         "evt-2",
			Source:     "mcq",
			Type:       "answer_incorrect",
			Summary:    "Missed SQL join cardinality.",
			Payload:    mustJSON(t, map[string]any{"skill": "SQL", "problem_id": "sql-joins", "correct": false}),
			OccurredAt: now.Add(-1 * time.Hour),
			CreatedAt:  now.Add(-1 * time.Hour),
		},
	}

	got = Summarize(Profile{}, events, now)
	if !contains(got.Strengths, "API Patterns") {
		t.Fatalf("expected API Patterns strength, got %#v", got.Strengths)
	}
	if !contains(got.GrowthEdges, "SQL") {
		t.Fatalf("expected SQL growth edge, got %#v", got.GrowthEdges)
	}
	if len(got.Skills) < 2 {
		t.Fatalf("expected at least 2 skills, got %d", len(got.Skills))
	}
	if got.Skills[0].LastPracticed.IsZero() {
		t.Fatal("expected LastPracticed to be derived")
	}
	if len(got.Notes) != 2 {
		t.Fatalf("expected 2 notes, got %d", len(got.Notes))
	}
	if got.Summary == "" || got.Summary == "No memory events have been recorded yet. Future chat, generation, marathon, and workspace activity will shape this profile." {
		t.Fatalf("expected event-derived summary, got %q", got.Summary)
	}

	again := Summarize(Profile{}, events, now)
	if !reflect.DeepEqual(got, again) {
		t.Fatal("expected Summarize to be idempotent for same inputs")
	}
}

func TestSummarizeKeepsMixedSignalNeutral(t *testing.T) {
	now := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	events := []Event{
		{
			ID:         "evt-1",
			Source:     "workspace",
			Type:       "attempt_solved",
			Summary:    "Solved a cache problem.",
			Payload:    mustJSON(t, map[string]any{"skill": "Caching", "passed": true}),
			OccurredAt: now.Add(-2 * time.Hour),
			CreatedAt:  now.Add(-2 * time.Hour),
		},
		{
			ID:         "evt-2",
			Source:     "workspace",
			Type:       "attempt_failed",
			Summary:    "Missed a cache invalidation case.",
			Payload:    mustJSON(t, map[string]any{"skill": "Caching", "passed": false}),
			OccurredAt: now.Add(-1 * time.Hour),
			CreatedAt:  now.Add(-1 * time.Hour),
		},
	}

	got := Summarize(Profile{}, events, now)
	if contains(got.Strengths, "Caching") {
		t.Fatalf("expected mixed Caching signal not to be a strength, got %#v", got.Strengths)
	}
	if contains(got.GrowthEdges, "Caching") {
		t.Fatalf("expected mixed Caching signal not to be a growth edge, got %#v", got.GrowthEdges)
	}
}

func TestSummarizeTreatsGovernedConceptAsSkillEvidence(t *testing.T) {
	now := time.Date(2026, 7, 29, 22, 0, 0, 0, time.UTC)
	got := Summarize(Profile{}, []Event{{
		ID:         "evt-concept",
		Source:     "workspace",
		Type:       "attempt_solved",
		Summary:    "Solved a coding problem.",
		Payload:    mustJSON(t, map[string]any{"concept": "Monotonic Stack", "language": "Python", "passed": true}),
		OccurredAt: now.Add(-time.Minute),
		CreatedAt:  now.Add(-time.Minute),
	}}, now)

	if !contains(got.Strengths, "Monotonic Stack") {
		t.Fatalf("expected governed concept to support a skill, got %#v", got.Strengths)
	}
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return payload
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
