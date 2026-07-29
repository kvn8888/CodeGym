package memory

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Summarize derives a user's memory Profile from their append-only event log.
//
// ─────────────────────────────────────────────────────────────────────────────
// LEARNING GOAL
// ─────────────────────────────────────────────────────────────────────────────
// This is the heart of CodeGym's "memory" feature. Events are raw evidence
// ("solved a two-pointer problem", "failed an SQL-join MCQ"). The Profile is the
// *derived* summary that generation / chat / marathon read to personalize
// problems. This deterministic v0 turns recorded events into stable skill,
// strength, growth-edge, and note signals.
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
// The v0 derivation below:
//   - Count events per Source/Type to find most- and least-practiced areas.
//   - Roll recent activity into Strengths and GrowthEdges.
//   - Update each SkillProficiency's LastPracticed / Level / Trend from payloads.
//   - Write a one-paragraph Summary describing the user.
func Summarize(current Profile, events []Event, now time.Time) Profile {
	now = now.UTC()

	// Start from the current profile so fields you don't recompute survive.
	next := current
	next.UpdatedAt = now
	next.NextReviewAt = now.Add(24 * time.Hour)

	// The interesting part: walk `events` and derive:
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
	statsByID := map[string]*skillStats{}
	notesByID := existingNotes(current.Notes)

	for _, event := range events {
		payload := payloadObject(event.Payload)
		skills := inferSkills(event, payload)
		outcome := inferOutcome(event, payload)
		practicedAt := event.OccurredAt.UTC()
		if practicedAt.IsZero() {
			practicedAt = event.CreatedAt.UTC()
		}
		if practicedAt.IsZero() {
			practicedAt = now
		}

		for _, label := range skills {
			stat := statsFor(statsByID, label)
			stat.Attempts++
			if outcome > 0 {
				stat.Positive++
			} else if outcome < 0 {
				stat.Negative++
			}
			if practicedAt.After(stat.LastPracticed) {
				stat.LastPracticed = practicedAt
			}
		}

		if note, ok := noteFromEvent(event, payload, skills, outcome); ok {
			notesByID[note.ID] = note
		}
	}

	stats := sortedStats(statsByID)
	next.Skills = buildSkillProficiencies(stats)
	next.Strengths = buildStrengths(stats)
	next.GrowthEdges = buildGrowthEdges(stats)
	next.Notes = sortedNotes(notesByID)
	next.Summary = buildSummary(len(events), stats, next.Strengths, next.GrowthEdges)

	return next
}

type skillStats struct {
	ID            string
	Label         string
	Area          string
	Attempts      int
	Positive      int
	Negative      int
	LastPracticed time.Time
}

func existingNotes(notes []Note) map[string]Note {
	indexed := map[string]Note{}
	for _, note := range notes {
		if note.ID == "" {
			continue
		}
		indexed[note.ID] = note
	}
	return indexed
}

func payloadObject(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	return payload
}

func inferSkills(event Event, payload map[string]any) []string {
	seen := map[string]struct{}{}
	labels := []string{}
	add := func(value string) {
		label := humanLabel(value)
		if label == "" {
			return
		}
		id := slug(label)
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		labels = append(labels, label)
	}

	for _, key := range []string{"skill", "area", "topic", "concept", "category", "framework", "language", "problem_type"} {
		add(stringValue(payload, key))
	}
	for _, key := range []string{"skills", "tags", "concepts", "topics"} {
		for _, value := range stringSliceValue(payload, key) {
			add(value)
		}
	}
	for _, label := range labelsFromSummary(event.Summary) {
		add(label)
	}
	if len(labels) == 0 {
		add(event.Source)
	}
	if len(labels) == 0 {
		add("General Practice")
	}
	return labels
}

func labelsFromSummary(summary string) []string {
	lower := strings.ToLower(summary)
	keywords := []struct {
		Needle string
		Label  string
	}{
		{"api", "API Patterns"},
		{"cache", "Caching"},
		{"concurr", "Concurrency"},
		{"goroutine", "Concurrency"},
		{"graph", "Graphs"},
		{"tree", "Trees"},
		{"sql", "SQL"},
		{"join", "SQL"},
		{"dynamic program", "Dynamic Programming"},
		{"two pointer", "Two Pointers"},
		{"sliding window", "Sliding Window"},
		{"rate limit", "Rate Limiting"},
		{"pagination", "Pagination"},
	}
	labels := []string{}
	for _, keyword := range keywords {
		if strings.Contains(lower, keyword.Needle) {
			labels = append(labels, keyword.Label)
		}
	}
	return labels
}

func inferOutcome(event Event, payload map[string]any) int {
	for _, key := range []string{"passed", "correct", "success", "solved"} {
		if value, ok := boolValue(payload, key); ok {
			if value {
				return 1
			}
			return -1
		}
	}
	for _, key := range []string{"failed", "wrong", "incorrect"} {
		if value, ok := boolValue(payload, key); ok && value {
			return -1
		}
	}

	eventType := strings.ToLower(event.Type)
	summary := strings.ToLower(event.Summary)
	if containsAny(eventType, "passed", "correct", "success", "solved", "completed") {
		return 1
	}
	if containsAny(eventType, "failed", "wrong", "incorrect", "missed", "struggle") {
		return -1
	}
	if containsAny(summary, "failed", "wrong", "missed", "struggled", "confused") {
		return -1
	}
	return 0
}

func statsFor(statsByID map[string]*skillStats, label string) *skillStats {
	id := slug(label)
	stat, ok := statsByID[id]
	if ok {
		return stat
	}
	stat = &skillStats{
		ID:    id,
		Label: label,
		Area:  areaFor(label),
	}
	statsByID[id] = stat
	return stat
}

func sortedStats(statsByID map[string]*skillStats) []*skillStats {
	stats := make([]*skillStats, 0, len(statsByID))
	for _, stat := range statsByID {
		stats = append(stats, stat)
	}
	sort.Slice(stats, func(i, j int) bool {
		if stats[i].Attempts == stats[j].Attempts {
			return stats[i].Label < stats[j].Label
		}
		return stats[i].Attempts > stats[j].Attempts
	})
	return stats
}

func buildSkillProficiencies(stats []*skillStats) []SkillProficiency {
	skills := make([]SkillProficiency, 0, len(stats))
	for _, stat := range stats {
		level := clamp(2+stat.Positive-stat.Negative, 1, 5)
		confidence := clamp(25+(stat.Attempts*15), 25, 95)
		trend := "flat"
		if stat.Positive > stat.Negative {
			trend = "up"
		} else if stat.Negative > stat.Positive {
			trend = "down"
		}
		skills = append(skills, SkillProficiency{
			ID:            stat.ID,
			Label:         stat.Label,
			Area:          stat.Area,
			Level:         level,
			Confidence:    confidence,
			Trend:         trend,
			LastPracticed: stat.LastPracticed,
		})
	}
	return skills
}

func buildStrengths(stats []*skillStats) []string {
	strengths := []string{}
	for _, stat := range stats {
		if stat.Positive > stat.Negative {
			strengths = append(strengths, stat.Label)
		}
		if len(strengths) == 5 {
			break
		}
	}
	return strengths
}

func buildGrowthEdges(stats []*skillStats) []string {
	edges := []string{}
	for _, stat := range stats {
		if stat.Negative > stat.Positive {
			edges = append(edges, stat.Label)
		}
		if len(edges) == 5 {
			break
		}
	}
	return edges
}

func noteFromEvent(event Event, payload map[string]any, skills []string, outcome int) (Note, bool) {
	problemID := stringValue(payload, "problem_id")
	if problemID == "" {
		problemID = stringValue(payload, "problemId")
	}
	if problemID == "" {
		return Note{}, false
	}

	title := firstNonEmpty(
		stringValue(payload, "title"),
		stringValue(payload, "problem_title"),
		problemID,
	)
	createdAt := event.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = event.OccurredAt.UTC()
	}
	action := "review"
	if outcome > 0 {
		action = "keep"
	} else if outcome < 0 {
		action = "review"
	}
	return Note{
		ID:        "note_" + firstNonEmpty(event.ID, slug(problemID)),
		ProblemID: problemID,
		Title:     title,
		Summary:   firstNonEmpty(event.Summary, "Memory event recorded."),
		CreatedAt: createdAt,
		Tags:      skills,
		Action:    action,
	}, true
}

func sortedNotes(notesByID map[string]Note) []Note {
	notes := make([]Note, 0, len(notesByID))
	for _, note := range notesByID {
		notes = append(notes, note)
	}
	sort.Slice(notes, func(i, j int) bool {
		if notes[i].CreatedAt.Equal(notes[j].CreatedAt) {
			return notes[i].ID < notes[j].ID
		}
		return notes[i].CreatedAt.After(notes[j].CreatedAt)
	})
	if len(notes) > 20 {
		return notes[:20]
	}
	return notes
}

func buildSummary(eventCount int, stats []*skillStats, strengths, growthEdges []string) string {
	if eventCount == 0 {
		return "No memory events have been recorded yet. Future chat, generation, marathon, and workspace activity will shape this profile."
	}

	focus := labelsFromStats(stats, 3)
	parts := []string{
		"Memory is based on " + countLabel(eventCount, "event") + ".",
	}
	if len(focus) > 0 {
		parts = append(parts, "Recent focus areas: "+strings.Join(focus, ", ")+".")
	}
	if len(strengths) > 0 {
		parts = append(parts, "Current strengths: "+strings.Join(strengths, ", ")+".")
	}
	if len(growthEdges) > 0 {
		parts = append(parts, "Growth edges: "+strings.Join(growthEdges, ", ")+".")
	}
	return strings.Join(parts, " ")
}

func labelsFromStats(stats []*skillStats, limit int) []string {
	labels := []string{}
	for _, stat := range stats {
		labels = append(labels, stat.Label)
		if len(labels) == limit {
			break
		}
	}
	return labels
}

func stringValue(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	value, ok := payload[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	default:
		return ""
	}
}

func stringSliceValue(payload map[string]any, key string) []string {
	if payload == nil {
		return nil
	}
	value, ok := payload[key]
	if !ok || value == nil {
		return nil
	}
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		values := []string{}
		for _, item := range typed {
			if text, ok := item.(string); ok {
				values = append(values, text)
			}
		}
		return values
	case string:
		return []string{typed}
	default:
		return nil
	}
}

func boolValue(payload map[string]any, key string) (bool, bool) {
	if payload == nil {
		return false, false
	}
	value, ok := payload[key]
	if !ok {
		return false, false
	}
	typed, ok := value.(bool)
	return typed, ok
}

func humanLabel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, "_", " ")
	value = strings.ReplaceAll(value, "-", " ")
	words := strings.Fields(value)
	for i, word := range words {
		lower := strings.ToLower(word)
		switch lower {
		case "api", "sql", "dsa", "ui", "ux":
			words[i] = strings.ToUpper(lower)
		default:
			words[i] = strings.ToUpper(lower[:1]) + lower[1:]
		}
	}
	return strings.Join(words, " ")
}

func areaFor(label string) string {
	lower := strings.ToLower(label)
	switch {
	case containsAny(lower, "api", "cache", "pagination", "rate limiting"):
		return "API Patterns"
	case containsAny(lower, "concurrency", "goroutine"):
		return "Concurrency"
	case containsAny(lower, "graph", "tree", "dynamic programming", "two pointers", "sliding window"):
		return "DSA"
	case containsAny(lower, "sql", "database", "postgres"):
		return "Data Systems"
	default:
		return "General"
	}
}

func slug(value string) string {
	var builder strings.Builder
	previousDash := false
	for _, r := range strings.ToLower(value) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
			previousDash = false
			continue
		}
		if !previousDash && builder.Len() > 0 {
			builder.WriteRune('-')
			previousDash = true
		}
	}
	return strings.Trim(builder.String(), "-")
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func countLabel(count int, singular string) string {
	if count == 1 {
		return "1 " + singular
	}
	return strconv.Itoa(count) + " " + singular + "s"
}

func clamp(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
