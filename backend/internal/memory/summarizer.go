package memory

import "time"

// Summarize derives a user's memory Profile from their append-only event log.
//
// ─────────────────────────────────────────────────────────────────────────────
// LEARNING GOAL
// ─────────────────────────────────────────────────────────────────────────────
// This is the heart of CodeGym's "memory" feature. Events are raw evidence
// ("solved a two-pointer problem", "failed an SQL-join MCQ"). The Profile is the
// *derived* summary that generation / chat / marathon read to personalize
// problems. Right now events accumulate but nothing reads them — that's your job.
//
// Keep this function PURE: inputs -> output, with no DB, no clock, and no
// globals. Purity makes it trivial to unit-test (see summarizer_test.go) and
// safe to re-run over the entire event history at any time (it is idempotent:
// the same events always produce the same profile).
//
//	current -> the existing profile, so you can preserve fields you don't recompute
//	events  -> the user's events, oldest first (exactly as Store.ListEvents returns)
//	now     -> the timestamp to stamp on the refreshed profile (clock is injected)
//
// A reasonable v0 to build up to:
//   - Count events per Source/Type to find most- and least-practiced areas.
//   - Roll recent activity into Strengths and GrowthEdges.
//   - Update each SkillProficiency's LastPracticed / Level / Trend from payloads.
//   - Write a one-paragraph Summary describing the user.
func Summarize(current Profile, events []Event, now time.Time) Profile {
	now = now.UTC()

	// Start from the current profile so fields you don't recompute survive.
	next := current
	next.UpdatedAt = now
	if next.NextReviewAt.IsZero() {
		next.NextReviewAt = now.Add(24 * time.Hour)
	}

	// TODO(you): the interesting part. Walk `events` and derive:
	//   next.Summary      — one paragraph; mention activity volume + focus areas
	//   next.Strengths    — []string, areas with strong recent signal
	//   next.GrowthEdges  — []string, areas the user struggles with
	//   next.Skills       — []SkillProficiency, one per skill area you track
	//   next.Notes        — optional problem-specific notes (these can be pruned)
	//
	// Hints:
	//   - Group events first: a map[string]int keyed by Type or Source is enough for v0.
	//   - event.Payload is json.RawMessage. When you need structured signal,
	//     json.Unmarshal it into a small struct, e.g.:
	//         struct{ Skill string `json:"skill"`; Passed bool `json:"passed"` }
	//     (This is where the memory event naming guide, issue #7, pays off —
	//     consistent Source/Type/payload shapes make this aggregation sane.)
	//   - Empty `events` must NOT panic: return a valid empty-but-timestamped profile.
	//   - Never mutate the input slice or the elements it points to.

	return next
}
