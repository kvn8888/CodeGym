package generation

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/memory"
)

func TestProfileSynthesizerCuratesAndPersistsFullProfile(t *testing.T) {
	ctx := scopedContext()
	now := time.Date(2026, 7, 15, 18, 0, 0, 0, time.UTC)
	service := memory.NewService(memory.NewInMemoryStore(), func() time.Time { return now })
	if _, err := service.RecordEvent(ctx, memory.RecordEventInput{
		Source: "mcq", Type: "answer_incorrect", Summary: "Missed a SQL join question.",
		OccurredAt: now.Add(-time.Hour),
		Payload:    json.RawMessage(`{"session_id":"round-1","topic":"SQL Joins","correct":false,"secret":"drop-me"}`),
	}); err != nil {
		t.Fatalf("seed event: %v", err)
	}

	payload := json.RawMessage(`{
		"summary":"SQL join direction is the current priority.",
		"strengths":[],
		"growth_edges":["SQL Joins"],
		"skills":[{"id":"sql-joins","label":"SQL Joins","area":"Data Systems","level":2,"confidence":62,"trend":"down"}],
		"notes":[{"id":"note_sql-joins","title":"SQL join direction","summary":"Review which side preserves unmatched rows.","tags":["sql","joins"],"action":"review"}]
	}`)
	generator := &scriptedGenerator{payloads: []json.RawMessage{payload}}
	synthesizer := NewProfileSynthesizer(NewOrchestrator(service, generator), service).WithClock(func() time.Time { return now })

	result, err := synthesizer.RefreshProfile(ctx, ProfileRefreshInput{SessionID: "round-1"})
	if err != nil {
		t.Fatalf("RefreshProfile: %v", err)
	}
	if result.Skipped != "" {
		t.Fatalf("unexpected fallback: %s", result.Skipped)
	}
	if result.Profile.Summary != "SQL join direction is the current priority." {
		t.Fatalf("summary = %q", result.Profile.Summary)
	}
	if len(result.Profile.Skills) != 1 || result.Profile.Skills[0].Confidence != 62 {
		t.Fatalf("skills = %#v", result.Profile.Skills)
	}
	if !result.Profile.Skills[0].LastPracticed.Equal(now.Add(-time.Hour)) {
		t.Fatalf("last practiced = %s", result.Profile.Skills[0].LastPracticed)
	}
	if len(result.AppliedActions) != 1 || result.AppliedActions[0].Op != "create" {
		t.Fatalf("note actions = %#v", result.AppliedActions)
	}
	if result.Profile.Provenance == nil || result.Profile.Provenance.Trigger != "manual" ||
		result.Profile.Provenance.Provider != "scripted" || result.Profile.Provenance.EventCount != 1 {
		t.Fatalf("provenance = %#v", result.Profile.Provenance)
	}

	persisted, err := service.GetProfile(ctx)
	if err != nil || persisted.Summary != result.Profile.Summary {
		t.Fatalf("persisted profile = %#v, err=%v", persisted, err)
	}
	request := generator.requests[0]
	if request.Kind != KindProfile || request.Schema.Name != "memory_profile" {
		t.Fatalf("request = %#v", request)
	}
	if strings.Contains(string(request.Spec), "round-1") && !strings.Contains(string(request.Spec), `"session_id":"round-1"`) {
		t.Fatalf("session id should only appear as the explicit focus id: %s", request.Spec)
	}
	if strings.Contains(string(request.Spec), "drop-me") {
		t.Fatalf("unapproved payload key leaked into evidence: %s", request.Spec)
	}
}

func TestProfileSynthesizerPreservesPersistedProfileOnInvalidOutput(t *testing.T) {
	ctx := scopedContext()
	now := time.Date(2026, 7, 15, 18, 0, 0, 0, time.UTC)
	service := memory.NewService(memory.NewInMemoryStore(), func() time.Time { return now })
	previous := memory.Profile{
		Summary: "Known-good curated profile.", UpdatedAt: now.Add(-time.Hour), NextReviewAt: now.Add(time.Hour),
		Strengths: []string{}, GrowthEdges: []string{"Graphs"}, Skills: []memory.SkillProficiency{}, Notes: []memory.Note{},
	}
	if _, err := service.ReplaceProfile(ctx, previous); err != nil {
		t.Fatalf("seed profile: %v", err)
	}
	if _, err := service.RecordEvent(ctx, memory.RecordEventInput{Source: "mcq", Type: "answer_incorrect", Summary: "Missed graphs."}); err != nil {
		t.Fatalf("seed event: %v", err)
	}
	generator := &scriptedGenerator{payloads: []json.RawMessage{json.RawMessage(`{"summary":"","strengths":[],"growth_edges":[],"skills":[],"notes":[]}`)}}
	synthesizer := NewProfileSynthesizer(NewOrchestrator(service, generator), service).WithClock(func() time.Time { return now })

	result, err := synthesizer.RefreshProfile(ctx, ProfileRefreshInput{})
	if err != nil {
		t.Fatalf("RefreshProfile: %v", err)
	}
	if result.Skipped == "" || result.Profile.Summary != previous.Summary {
		t.Fatalf("result = %#v", result)
	}
	persisted, _ := service.GetProfile(ctx)
	if persisted.Summary != previous.Summary {
		t.Fatalf("invalid output replaced profile: %#v", persisted)
	}
}

func TestProfileSynthesizerDoesNotPersistDeterministicFallbackForFirstProfile(t *testing.T) {
	ctx := scopedContext()
	now := time.Date(2026, 7, 15, 18, 0, 0, 0, time.UTC)
	store := memory.NewInMemoryStore()
	service := memory.NewService(store, func() time.Time { return now })
	if _, err := service.RecordEvent(ctx, memory.RecordEventInput{
		Source: "mcq", Type: "answer_incorrect", Summary: "Missed graphs.",
		Payload: json.RawMessage(`{"topic":"Graphs","correct":false}`),
	}); err != nil {
		t.Fatalf("seed event: %v", err)
	}

	result, err := NewProfileSynthesizer(nil, service).WithClock(func() time.Time { return now }).RefreshProfile(ctx, ProfileRefreshInput{})
	if err != nil {
		t.Fatalf("RefreshProfile: %v", err)
	}
	if result.Skipped == "" || result.Changed || len(result.Profile.GrowthEdges) != 0 {
		t.Fatalf("fallback result = %#v", result)
	}
	if _, err := store.GetProfile(ctx, "workspace-1", "user-1"); !errors.Is(err, memory.ErrProfileNotFound) {
		t.Fatalf("deterministic fallback was persisted: %v", err)
	}
}

func TestParseCuratedProfileReusesExistingNoteForDuplicateConcept(t *testing.T) {
	now := time.Date(2026, 7, 15, 18, 0, 0, 0, time.UTC)
	createdAt := now.Add(-48 * time.Hour)
	current := memory.Profile{Notes: []memory.Note{{
		ID: "note_sql_original", ProblemID: "problem-sql-1", Title: "SQL joins",
		Summary: "Old summary.", CreatedAt: createdAt, Action: "review",
	}}}
	fallback := memory.Profile{Skills: []memory.SkillProficiency{{
		ID: "sql-joins", Label: "SQL Joins", LastPracticed: now.Add(-time.Hour),
	}}}
	raw := json.RawMessage(`{
		"summary":"Join direction needs another pass.","strengths":[],"growth_edges":["SQL joins"],
		"skills":[{"id":"sql-joins","label":"SQL Joins","area":"Data Systems","level":2,"confidence":60,"trend":"flat"}],
		"notes":[
			{"id":"note_new_id","problem_id":"problem-sql-1","title":"SQL joins","summary":"Updated summary.","tags":["sql"],"action":"review"},
			{"id":"note_duplicate","title":"SQL joins","summary":"Duplicate summary.","tags":["sql"],"action":"review"}
		]
	}`)

	profile, err := ParseCuratedProfile(raw, current, fallback, now)
	if err != nil {
		t.Fatalf("ParseCuratedProfile: %v", err)
	}
	if len(profile.Notes) != 1 {
		t.Fatalf("notes = %#v", profile.Notes)
	}
	if profile.Notes[0].ID != "note_sql_original" || profile.Notes[0].Summary != "Updated summary." {
		t.Fatalf("note = %#v", profile.Notes[0])
	}
	if !profile.Notes[0].CreatedAt.Equal(createdAt) {
		t.Fatalf("created_at = %s", profile.Notes[0].CreatedAt)
	}
}

func TestProfileSynthesizerRefreshAllProfilesUsesLLM(t *testing.T) {
	now := time.Date(2026, 7, 15, 18, 0, 0, 0, time.UTC)
	store := memory.NewInMemoryStore()
	service := memory.NewService(store, func() time.Time { return now })
	if err := store.AppendEvent(t.Context(), memory.Event{
		ID: "evt-1", WorkspaceID: "ws-1", UserID: "user-1", Source: "mcq", Type: "answer_incorrect",
		Summary: "Missed queues.", Payload: json.RawMessage(`{"topic":"Queues","correct":false}`),
		OccurredAt: now.Add(-time.Minute), CreatedAt: now.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("seed event: %v", err)
	}
	generator := &scriptedGenerator{payloads: []json.RawMessage{json.RawMessage(`{
		"summary":"Queue ordering needs reinforcement.","strengths":[],"growth_edges":["Queues"],
		"skills":[{"id":"queues","label":"Queues","area":"DSA","level":2,"confidence":50,"trend":"down"}],
		"notes":[]
	}`)}}
	synthesizer := NewProfileSynthesizer(NewOrchestrator(service, generator), service).WithClock(func() time.Time { return now })

	count, err := synthesizer.RefreshAllProfiles(t.Context())
	if err != nil || count != 1 {
		t.Fatalf("RefreshAllProfiles count=%d err=%v", count, err)
	}
	profile, err := store.GetProfile(t.Context(), "ws-1", "user-1")
	if err != nil || profile.Summary != "Queue ordering needs reinforcement." {
		t.Fatalf("profile=%#v err=%v", profile, err)
	}
	if len(generator.requests) != 1 || generator.requests[0].Kind != KindProfile {
		t.Fatalf("worker requests = %#v", generator.requests)
	}
}

func TestProfileSynthesizerSkipsUnchangedDailyEvidence(t *testing.T) {
	ctx := scopedContext()
	now := time.Date(2026, 7, 15, 18, 0, 0, 0, time.UTC)
	service := memory.NewService(memory.NewInMemoryStore(), func() time.Time { return now })
	if _, err := service.RecordEvent(ctx, memory.RecordEventInput{
		Source: "mcq", Type: "answer_incorrect", Summary: "Missed queues.",
		Payload: json.RawMessage(`{"topic":"Queues","correct":false}`),
	}); err != nil {
		t.Fatalf("seed event: %v", err)
	}
	payload := json.RawMessage(`{
		"summary":"Queue ordering needs reinforcement.","strengths":[],"growth_edges":["Queues"],
		"skills":[{"id":"queues","label":"Queues","area":"DSA","level":2,"confidence":50,"trend":"down"}],
		"notes":[]
	}`)
	generator := &scriptedGenerator{payloads: []json.RawMessage{payload, payload}}
	synthesizer := NewProfileSynthesizer(NewOrchestrator(service, generator), service).WithClock(func() time.Time { return now })

	first, err := synthesizer.RefreshProfile(ctx, ProfileRefreshInput{Trigger: "daily"})
	if err != nil || !first.Changed || first.Profile.Provenance == nil || first.Profile.Provenance.EvidenceDigest == "" {
		t.Fatalf("first refresh = %#v, err=%v", first, err)
	}
	now = now.Add(24 * time.Hour)
	second, err := synthesizer.RefreshProfile(ctx, ProfileRefreshInput{Trigger: "daily"})
	if err != nil || second.Changed || second.Skipped != "memory evidence is unchanged" {
		t.Fatalf("second refresh = %#v, err=%v", second, err)
	}
	if len(generator.requests) != 1 {
		t.Fatalf("unchanged daily refresh used model tokens; requests=%d", len(generator.requests))
	}

	if _, err := service.RecordEvent(ctx, memory.RecordEventInput{
		Source: "mcq", Type: "question_answered", Summary: "Practiced queues again.",
		Payload: json.RawMessage(`{"topic":"Queues","correct":true}`),
	}); err != nil {
		t.Fatalf("append evidence: %v", err)
	}
	third, err := synthesizer.RefreshProfile(ctx, ProfileRefreshInput{Trigger: "daily"})
	if err != nil || !third.Changed || len(generator.requests) != 2 {
		t.Fatalf("changed refresh = %#v requests=%d err=%v", third, len(generator.requests), err)
	}
}

func TestProfileEvidenceExcludesAuditEventsAndSessionMarkersFromSignals(t *testing.T) {
	now := time.Date(2026, 7, 15, 18, 0, 0, 0, time.UTC)
	events := []memory.Event{
		{Source: "memory", Type: "note_updated", Summary: "Updated SQL note.", OccurredAt: now},
		{Source: "system", Type: "worker_profile_refreshed", Summary: "Worker refreshed.", OccurredAt: now},
		{Source: "mcq", Type: "session_completed", Summary: "Finished round.", OccurredAt: now},
		{Source: "mcq", Type: "answer_incorrect", Summary: "Missed queues.", Payload: json.RawMessage(`{"topic":"Queues","correct":false}`), OccurredAt: now},
	}
	evidence := profileEvidenceEvents(events)
	if len(evidence) != 2 {
		t.Fatalf("evidence = %#v", evidence)
	}
	signals := profileSignalEvents(evidence)
	if len(signals) != 1 || signals[0].Type != "answer_incorrect" {
		t.Fatalf("signals = %#v", signals)
	}
	profile := memory.Summarize(memory.Profile{}, signals, now)
	if len(profile.Skills) != 1 || profile.Skills[0].Label != "Queues" {
		t.Fatalf("skills = %#v", profile.Skills)
	}
}

func TestProfileEvidenceCanonicalizesSkipAndSuppressesRevealCorrectness(t *testing.T) {
	now := time.Date(2026, 7, 16, 3, 0, 0, 0, time.UTC)
	events := []memory.Event{
		{
			Source: "mcq", Type: "answer_incorrect", Summary: "Skipped Spring profiles.",
			Payload:    json.RawMessage(`{"session_id":"round-1","question_id":"q1","topic":"Spring Profiles","correct":false,"skipped":true}`),
			OccurredAt: now,
		},
		{
			Source: "mcq", Type: "question_answered", Summary: "UI revealed the correct answer.",
			Payload:    json.RawMessage(`{"session_id":"round-1","question_id":"q1","topic":"Spring Profiles","correct":true}`),
			OccurredAt: now.Add(time.Second),
		},
		{
			Source: "mcq", Type: "question_answered", Summary: "Answered a later Spring profiles question correctly.",
			Payload:    json.RawMessage(`{"session_id":"round-1","question_id":"q2","topic":"Spring Profiles","correct":true}`),
			OccurredAt: now.Add(2 * time.Second),
		},
	}

	evidenceEvents := profileEvidenceEvents(events)
	if len(evidenceEvents) != 2 {
		t.Fatalf("evidence events = %#v", evidenceEvents)
	}
	if evidenceEvents[0].Type != "question_skipped" {
		t.Fatalf("legacy skip type = %q", evidenceEvents[0].Type)
	}
	var skipPayload map[string]any
	if err := json.Unmarshal(evidenceEvents[0].Payload, &skipPayload); err != nil {
		t.Fatalf("decode skip payload: %v", err)
	}
	if skipPayload["skipped"] != true || skipPayload["answer_revealed"] != true {
		t.Fatalf("skip payload = %#v", skipPayload)
	}
	if _, exists := skipPayload["correct"]; exists {
		t.Fatalf("skip retained correctness: %#v", skipPayload)
	}

	evidence := buildProfileEvidence(evidenceEvents, memory.Profile{}, "round-1")
	if len(evidence.Recent) != 2 || evidence.Recent[0].Outcome != "skipped" || evidence.Recent[1].Outcome != "correct" {
		t.Fatalf("outcomes = %#v", evidence.Recent)
	}
}
