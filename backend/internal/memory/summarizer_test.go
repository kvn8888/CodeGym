package memory

import (
	"testing"
	"time"
)

// TestSummarizePlumbing is a GREEN baseline: it proves the wiring works before
// you add real intelligence. It already passes against the skeleton. As you
// teach Summarize to read events, add cases below and assert on the derived
// fields (Strengths, GrowthEdges, Skills, Summary).
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

	// TODO(you): grow this test. Example to aim for once Summarize derives signal:
	//
	//   events := []Event{
	//       {Source: "workspace", Type: "attempt_passed", Summary: "Solved two-sum", OccurredAt: now},
	//       {Source: "mcq",       Type: "answer_wrong",   Summary: "Missed SQL join", OccurredAt: now},
	//   }
	//   got := Summarize(Profile{}, events, now)
	//   // expect "sql" to surface in got.GrowthEdges after a wrong SQL answer, etc.
}
