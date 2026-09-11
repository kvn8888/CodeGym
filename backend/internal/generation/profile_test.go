package generation

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/memory"
	"github.com/kvn8888/codegym/backend/internal/workflow"
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

	notePayload := json.RawMessage(`{"actions":[{"op":"create","note":{"id":"note_sql-joins","title":"SQL join direction","summary":"Review which side preserves unmatched rows.","tags":["sql","joins"],"action":"review"}}]}`)
	profilePayload := json.RawMessage(`{
		"summary":"SQL join direction is the current priority.",
		"strengths":[],
		"growth_edges":["SQL Joins"],
		"skills":[{"id":"sql-joins","label":"SQL Joins","area":"Data Systems","level":2,"confidence":62,"trend":"down"}],
		"notes":[{"id":"note_sql-joins","title":"SQL join direction","summary":"Review which side preserves unmatched rows.","tags":["sql","joins"],"action":"review"}]
	}`)
	generator := &scriptedGenerator{payloads: []json.RawMessage{notePayload, profilePayload}}
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
	if len(generator.requests) != 2 {
		t.Fatalf("requests = %#v", generator.requests)
	}
	if generator.requests[0].Kind != KindNotes || generator.requests[0].Schema.Name != "note_actions" {
		t.Fatalf("note request = %#v", generator.requests[0])
	}
	request := generator.requests[1]
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

func TestProfilePromptIncludesEvidenceInterpretationGuardrails(t *testing.T) {
	for _, phrase := range []string{
		"single incorrect answer is limited evidence",
		"Assisted success",
		"not independent mastery",
		"used_help=true",
	} {
		if !strings.Contains(profileSystemPrompt, phrase) {
			t.Fatalf("profile prompt missing %q", phrase)
		}
	}
}

func TestProfileSynthesizerDedupesReplayedLogicalEvidence(t *testing.T) {
	ctx := scopedContext()
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	service := memory.NewService(memory.NewInMemoryStore(), func() time.Time { return now })
	for _, input := range []memory.RecordEventInput{
		{Source: "mcq", Type: "answer_incorrect", Summary: "Missed SQL joins.", OccurredAt: now.Add(-4 * time.Minute), Payload: json.RawMessage(`{"session_id":"mcq_r1","question_id":"q_sql_1","correct":false}`)},
		{Source: "mcq", Type: "answer_incorrect", Summary: "Missed SQL joins replay.", OccurredAt: now.Add(-3 * time.Minute), Payload: json.RawMessage(`{"session_id":"mcq_r1","question_id":"q_sql_1","topic":"SQL Joins","correct":false}`)},
		{Source: "mcq", Type: "session_completed", Summary: "Finished round 1.", OccurredAt: now.Add(-2 * time.Minute), Payload: json.RawMessage(`{"session_id":"mcq_r1","round":1}`)},
		{Source: "mcq", Type: "session_completed", Summary: "Finished round 1 replay.", OccurredAt: now.Add(-time.Minute), Payload: json.RawMessage(`{"session_id":"mcq_r1","round":1,"question_count":1,"answered_count":1,"correct_count":0}`)},
	} {
		if _, err := service.RecordEvent(ctx, input); err != nil {
			t.Fatalf("seed event: %v", err)
		}
	}
	generator := &scriptedGenerator{payloads: []json.RawMessage{json.RawMessage(`{"actions":[]}`), json.RawMessage(`{
		"summary":"SQL joins need review.","strengths":[],"growth_edges":["SQL Joins"],
		"skills":[{"id":"sql-joins","label":"SQL Joins","area":"Data Systems","level":2,"confidence":55,"trend":"down"}],
		"notes":[]
	}`)}}
	synthesizer := NewProfileSynthesizer(NewOrchestrator(service, generator), service).WithClock(func() time.Time { return now })

	result, err := synthesizer.RefreshProfile(ctx, ProfileRefreshInput{SessionID: "mcq_r1", Trigger: "set-completion"})
	if err != nil {
		t.Fatalf("RefreshProfile: %v", err)
	}
	if result.Profile.Provenance == nil || result.Profile.Provenance.EventCount != 2 {
		t.Fatalf("provenance = %#v", result.Profile.Provenance)
	}
	if len(result.Profile.Provenance.EvidenceSources) != 2 {
		t.Fatalf("evidence sources = %#v", result.Profile.Provenance.EvidenceSources)
	}
	keys := map[string]bool{}
	for _, source := range result.Profile.Provenance.EvidenceSources {
		if keys[source.Key] {
			t.Fatalf("duplicate evidence source key saved: %#v", result.Profile.Provenance.EvidenceSources)
		}
		keys[source.Key] = true
	}

	var evidence profileEvidence
	if err := json.Unmarshal(generator.requests[1].Spec, &evidence); err != nil {
		t.Fatalf("decode evidence: %v", err)
	}
	if evidence.TotalEvents != 2 || len(evidence.Recent) != 2 {
		t.Fatalf("evidence = %#v", evidence)
	}
	if evidence.Recent[0].Details["topic"] != "SQL Joins" {
		t.Fatalf("less complete question replay was kept: %#v", evidence.Recent[0].Details)
	}
	if evidence.Recent[1].Details["correct_count"] == nil {
		t.Fatalf("less complete round replay was kept: %#v", evidence.Recent[1].Details)
	}
}

func TestProfileSynthesizerKeepsDistinctFreeResponseOutcomes(t *testing.T) {
	ctx := scopedContext()
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	service := memory.NewService(memory.NewInMemoryStore(), func() time.Time { return now })
	for _, input := range []memory.RecordEventInput{
		{Source: "mcq", Type: "free_response_evaluated", Summary: "Written answer was incorrect.", Payload: json.RawMessage(`{"session_id":"round-1","question_id":"q1","topic":"Indexes","correct":false}`)},
		{Source: "mcq", Type: "free_response_evaluated", Summary: "Written answer was correct.", Payload: json.RawMessage(`{"session_id":"round-1","question_id":"q1","topic":"Indexes","correct":true}`)},
	} {
		if _, err := service.RecordEvent(ctx, input); err != nil {
			t.Fatalf("seed event: %v", err)
		}
	}
	generator := &scriptedGenerator{payloads: []json.RawMessage{json.RawMessage(`{"actions":[]}`), json.RawMessage(`{
		"summary":"Index evidence changed across written evaluations.","strengths":[],"growth_edges":[],
		"skills":[{"id":"indexes","label":"Indexes","area":"Data Systems","level":2,"confidence":45,"trend":"flat"}],
		"notes":[]
	}`)}}
	synthesizer := NewProfileSynthesizer(NewOrchestrator(service, generator), service).WithClock(func() time.Time { return now })

	result, err := synthesizer.RefreshProfile(ctx, ProfileRefreshInput{SessionID: "round-1", Trigger: "set-completion"})
	if err != nil {
		t.Fatalf("RefreshProfile: %v", err)
	}
	if result.Profile.Provenance == nil || len(result.Profile.Provenance.EvidenceSources) != 2 {
		t.Fatalf("provenance = %#v", result.Profile.Provenance)
	}

	var evidence profileEvidence
	if err := json.Unmarshal(generator.requests[1].Spec, &evidence); err != nil {
		t.Fatalf("decode evidence: %v", err)
	}
	outcomes := map[string]bool{}
	for _, event := range evidence.Recent {
		outcomes[event.Outcome] = true
	}
	if !outcomes["incorrect"] || !outcomes["correct"] {
		t.Fatalf("outcomes = %#v", evidence.Recent)
	}
}

func TestProfileEvidenceDedupesExactReplayWithoutReplacingTimestamp(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	payload := json.RawMessage(`{"session_id":"mcq_r1","round":1,"question_count":1,"answered_count":1,"correct_count":0}`)
	evidence := profileEvidenceEvents([]memory.Event{
		{Source: "mcq", Type: "session_completed", Summary: "Finished round.", Payload: payload, OccurredAt: now},
		{Source: "mcq", Type: "session_completed", Summary: "Finished round replay.", Payload: payload, OccurredAt: now.Add(time.Hour)},
	})
	if len(evidence) != 1 {
		t.Fatalf("evidence = %#v", evidence)
	}
	if !evidence[0].OccurredAt.Equal(now) || evidence[0].Summary != "Finished round." {
		t.Fatalf("exact replay replaced original event: %#v", evidence[0])
	}
}

func TestProfileSynthesizerKeepsDistinctSkipAndAnswerEvidence(t *testing.T) {
	ctx := scopedContext()
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	service := memory.NewService(memory.NewInMemoryStore(), func() time.Time { return now })
	for _, input := range []memory.RecordEventInput{
		{Source: "mcq", Type: "question_skipped", Summary: "Skipped a Spring Profiles question.", Payload: json.RawMessage(`{"session_id":"round-1","question_id":"q1","topic":"Spring Profiles","correct":true,"skipped":true}`)},
		{Source: "mcq", Type: "question_answered", Summary: "Answered a Spring Profiles question.", Payload: json.RawMessage(`{"session_id":"round-1","question_id":"q1","topic":"Spring Profiles","correct":true}`)},
	} {
		if _, err := service.RecordEvent(ctx, input); err != nil {
			t.Fatalf("seed event: %v", err)
		}
	}
	generator := &scriptedGenerator{payloads: []json.RawMessage{json.RawMessage(`{"actions":[]}`), json.RawMessage(`{
		"summary":"Spring Profiles evidence is mixed and limited.","strengths":[],"growth_edges":[],
		"skills":[{"id":"spring-profiles","label":"Spring Profiles","area":"Backend","level":2,"confidence":45,"trend":"flat"}],
		"notes":[]
	}`)}}
	synthesizer := NewProfileSynthesizer(NewOrchestrator(service, generator), service).WithClock(func() time.Time { return now })

	result, err := synthesizer.RefreshProfile(ctx, ProfileRefreshInput{SessionID: "round-1", Trigger: "set-completion"})
	if err != nil {
		t.Fatalf("RefreshProfile: %v", err)
	}
	if result.Profile.Provenance == nil || result.Profile.Provenance.EventCount != 2 || len(result.Profile.Provenance.EvidenceSources) != 2 {
		t.Fatalf("provenance = %#v", result.Profile.Provenance)
	}

	var evidence profileEvidence
	if err := json.Unmarshal(generator.requests[1].Spec, &evidence); err != nil {
		t.Fatalf("decode evidence: %v", err)
	}
	seen := map[string]profileEvidenceEvent{}
	for _, event := range evidence.Recent {
		seen[event.Type] = event
	}
	if _, ok := seen["question_skipped"]; !ok {
		t.Fatalf("skip evidence missing: %#v", evidence.Recent)
	}
	if _, ok := seen["question_answered"]; !ok {
		t.Fatalf("answer evidence missing: %#v", evidence.Recent)
	}
	if _, leaked := seen["question_skipped"].Details["correct"]; leaked {
		t.Fatalf("skip evidence leaked revealed correctness: %#v", seen["question_skipped"].Details)
	}
}

func TestProfileSynthesizerReportsEvidenceThroughPersistence(t *testing.T) {
	ctx := scopedContext()
	now := time.Date(2026, 7, 15, 18, 0, 0, 0, time.UTC)
	service := memory.NewService(memory.NewInMemoryStore(), func() time.Time { return now })
	if _, err := service.RecordEvent(ctx, memory.RecordEventInput{
		Source: "mcq", Type: "answer_incorrect", Summary: "Missed a graph question.",
	}); err != nil {
		t.Fatal(err)
	}
	generator := &scriptedGenerator{payloads: []json.RawMessage{json.RawMessage(`{"actions":[]}`), json.RawMessage(`{
		"summary":"Graphs need practice.","strengths":[],"growth_edges":["Graphs"],
		"skills":[{"id":"graphs","label":"Graphs","area":"DSA","level":2,"confidence":60,"trend":"flat"}],
		"notes":[{"id":"note_graphs","title":"Graphs","summary":"Review traversal.","tags":["graphs"],"action":"review"}]
	}`)}}
	progress := workflow.NewService(workflow.NewInMemoryStore(), nil)
	created, err := progress.Create(ctx, workflow.CreateInput{Kind: workflow.KindMemoryReflection})
	if err != nil {
		t.Fatal(err)
	}
	reporter, err := progress.Attach(ctx, created.Operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	ctx = workflow.WithReporter(ctx, reporter)
	synthesizer := NewProfileSynthesizer(NewOrchestrator(service, generator), service).
		WithClock(func() time.Time { return now })
	if _, err := synthesizer.RefreshProfile(ctx, ProfileRefreshInput{}); err != nil {
		t.Fatal(err)
	}
	events, err := progress.Events(ctx, created.Operation.ID, 5)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"load_evidence", "load_evidence", "synthesize_profile", "synthesize_profile",
		"validate_profile", "validate_profile", "save_profile", "save_profile",
	}
	if len(events) != len(want) {
		t.Fatalf("events=%#v", events)
	}
	for index, stepID := range want {
		if events[index].StepID != stepID {
			t.Fatalf("event %d=%#v, want %s", index, events[index], stepID)
		}
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

func TestProfileSynthesizerDoesNotPersistNotesWhenProfileStageFails(t *testing.T) {
	ctx := scopedContext()
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	service := memory.NewService(memory.NewInMemoryStore(), func() time.Time { return now })

	previous := memory.Profile{
		Summary:      "Known-good curated profile.",
		UpdatedAt:    now.Add(-time.Hour),
		NextReviewAt: now.Add(time.Hour),
		Strengths:    []string{},
		GrowthEdges:  []string{},
		Skills:       []memory.SkillProficiency{},
		Notes:        []memory.Note{},
	}
	previous, err := service.ReplaceProfile(ctx, previous)
	if err != nil {
		t.Fatalf("seed profile: %v", err)
	}

	if _, err := service.RecordEvent(ctx, memory.RecordEventInput{
		Source:  "mcq",
		Type:    "answer_incorrect",
		Summary: "Missed a SQL joins question.",
		Payload: json.RawMessage(`{
			"session_id":"mcq_r1",
			"question_id":"q_sql_1",
			"round":1,
			"topic":"SQL Joins",
			"correct":false
		}`),
	}); err != nil {
		t.Fatalf("seed answer event: %v", err)
	}

	if _, err := service.RecordEvent(ctx, memory.RecordEventInput{
		Source:  "mcq",
		Type:    "session_completed",
		Summary: "Finished round 1.",
		Payload: json.RawMessage(`{
			"session_id":"mcq_r1",
			"round":1,
			"question_count":1,
			"answered_count":1,
			"correct_count":0
		}`),
	}); err != nil {
		t.Fatalf("seed completion event: %v", err)
	}

	noteCandidate := json.RawMessage(`{
		"actions":[
			{
				"op":"create",
				"note":{
					"id":"note_sql_joins",
					"title":"SQL joins",
					"summary":"Review join direction and unmatched-row behavior.",
					"tags":["sql","joins"],
					"action":"review"
				}
			}
		],
		"reason":"missed SQL joins"
	}`)

	invalidProfileCandidate := json.RawMessage(`{
		"summary":"",
		"strengths":[],
		"growth_edges":[],
		"skills":[],
		"notes":[
			{
				"id":"note_sql_joins",
				"title":"SQL joins",
				"summary":"Review join direction and unmatched-row behavior.",
				"tags":["sql","joins"],
				"action":"review"
			}
		]
	}`)

	generator := &scriptedGenerator{
		payloads: []json.RawMessage{noteCandidate, invalidProfileCandidate},
	}
	synthesizer := NewProfileSynthesizer(NewOrchestrator(service, generator), service).
		WithClock(func() time.Time { return now })

	result, err := synthesizer.RefreshProfile(ctx, ProfileRefreshInput{
		SessionID: "mcq_r1",
		Trigger:   "set-completion",
	})
	if err != nil {
		t.Fatalf("RefreshProfile: %v", err)
	}
	if result.Skipped == "" {
		t.Fatal("expected skipped result after invalid profile stage")
	}
	if len(generator.requests) != 2 {
		t.Fatalf("model calls = %d, want note stage and profile stage", len(generator.requests))
	}
	if generator.requests[0].Kind != KindNotes {
		t.Fatalf("first model call kind = %q, want %q", generator.requests[0].Kind, KindNotes)
	}
	if generator.requests[1].Kind != KindProfile {
		t.Fatalf("second model call kind = %q, want %q", generator.requests[1].Kind, KindProfile)
	}

	persisted, err := service.GetProfile(ctx)
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if persisted.Summary != previous.Summary {
		t.Fatalf("summary changed after failed profile stage: %#v", persisted)
	}
	if len(persisted.Notes) != 0 {
		t.Fatalf("note candidate was persisted before profile stage succeeded: %#v", persisted.Notes)
	}
	if persisted.Version != previous.Version {
		t.Fatalf("profile version changed after failed profile stage: got %d want %d", persisted.Version, previous.Version)
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
			{"id":"note_new_id","problem_id":"problem-sql-1","title":"SQL joins","summary":"Updated summary with the earlier unmatched-row confusion still noted.","tags":["sql"],"action":"review"},
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
	if profile.Notes[0].ID != "note_sql_original" ||
		!strings.Contains(profile.Notes[0].Summary, "Updated summary") {
		t.Fatalf("note = %#v", profile.Notes[0])
	}
	if !profile.Notes[0].CreatedAt.Equal(createdAt) {
		t.Fatalf("created_at = %s", profile.Notes[0].CreatedAt)
	}
}

func TestParseCuratedProfileRejectsSummaryRegression(t *testing.T) {
	now := time.Date(2026, 7, 15, 18, 0, 0, 0, time.UTC)
	current := memory.Profile{
		Summary: strings.Repeat("Prior skill detail. ", 20), // >200 runes
	}
	raw := json.RawMessage(`{
		"summary":"Latest set went okay.",
		"strengths":[],"growth_edges":[],"skills":[],"notes":[]
	}`)
	_, err := ParseCuratedProfile(raw, current, memory.Profile{}, now)
	if err == nil || !strings.Contains(err.Error(), "regresses living document") {
		t.Fatalf("expected summary regression, got %v", err)
	}
}

func TestParseCuratedProfileRejectsNoteSummaryRegression(t *testing.T) {
	now := time.Date(2026, 7, 15, 18, 0, 0, 0, time.UTC)
	current := memory.Profile{Notes: []memory.Note{{
		ID: "note_sql", Title: "SQL joins",
		Summary:   "Earlier sets showed confusion on unmatched rows and LEFT vs INNER; keep reviewing cardinality examples before the next SQL set.",
		CreatedAt: now.Add(-time.Hour), Action: "review",
	}}}
	raw := json.RawMessage(`{
		"summary":"SQL joins remain an open growth edge after the latest set.",
		"strengths":[],"growth_edges":["SQL Joins"],"skills":[],
		"notes":[{"id":"note_sql","title":"SQL joins","summary":"Review joins.","tags":["sql"],"action":"review"}]
	}`)
	_, err := ParseCuratedProfile(raw, current, memory.Profile{}, now)
	if err == nil || !strings.Contains(err.Error(), "note summary regresses") {
		t.Fatalf("expected note regression, got %v", err)
	}
}

func TestProfileSystemPromptRequiresLivingSummary(t *testing.T) {
	if !strings.Contains(profileSystemPrompt, "living document") ||
		!strings.Contains(profileSystemPrompt, "three-sentence status blurb") {
		t.Fatalf("profile prompt lost living-document guidance")
	}
	if strings.Contains(profileSystemPrompt, "summary is a short paragraph") {
		t.Fatalf("profile prompt still asks for a short replacement paragraph")
	}
}

func TestParseCuratedProfileDropsSkillsWithoutDeterministicEvidence(t *testing.T) {
	now := time.Date(2026, 7, 29, 23, 0, 0, 0, time.UTC)
	fallback := memory.Profile{Skills: []memory.SkillProficiency{{
		ID: "monotonic-stack", Label: "Monotonic Stack", LastPracticed: now.Add(-time.Minute),
	}}}
	raw := json.RawMessage(`{
		"summary":"Monotonic-stack practice is the current focus.",
		"strengths":["Monotonic Stack"],"growth_edges":[],
		"skills":[
			{"id":"monotonic-stack","label":"Monotonic Stack","area":"DSA","level":3,"confidence":70,"trend":"up"},
			{"id":"iterative-debugging","label":"Iterative Debugging","area":"General","level":3,"confidence":60,"trend":"flat"}
		],
		"notes":[]
	}`)

	profile, err := ParseCuratedProfile(raw, memory.Profile{}, fallback, now)
	if err != nil {
		t.Fatalf("ParseCuratedProfile: %v", err)
	}
	if len(profile.Skills) != 1 || profile.Skills[0].Label != "Monotonic Stack" {
		t.Fatalf("skills = %#v", profile.Skills)
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
	generator := &scriptedGenerator{payloads: []json.RawMessage{json.RawMessage(`{"actions":[]}`), json.RawMessage(`{
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
	if len(generator.requests) != 2 || generator.requests[0].Kind != KindNotes || generator.requests[1].Kind != KindProfile {
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
	notePayload := json.RawMessage(`{"actions":[]}`)
	profilePayload := json.RawMessage(`{
		"summary":"Queue ordering needs reinforcement.","strengths":[],"growth_edges":["Queues"],
		"skills":[{"id":"queues","label":"Queues","area":"DSA","level":2,"confidence":50,"trend":"down"}],
		"notes":[]
	}`)
	generator := &scriptedGenerator{payloads: []json.RawMessage{notePayload, profilePayload, notePayload, profilePayload}}
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
	if len(generator.requests) != 2 {
		t.Fatalf("unchanged daily refresh used model tokens; requests=%d", len(generator.requests))
	}

	if _, err := service.RecordEvent(ctx, memory.RecordEventInput{
		Source: "mcq", Type: "question_answered", Summary: "Practiced queues again.",
		Payload: json.RawMessage(`{"topic":"Queues","correct":true}`),
	}); err != nil {
		t.Fatalf("append evidence: %v", err)
	}
	third, err := synthesizer.RefreshProfile(ctx, ProfileRefreshInput{Trigger: "daily"})
	if err != nil || !third.Changed || len(generator.requests) != 4 {
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
	if len(evidenceEvents) != 3 {
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
	if len(evidence.Recent) != 3 || evidence.Recent[0].Outcome != "skipped" || evidence.Recent[1].Outcome != "correct" || evidence.Recent[2].Outcome != "correct" {
		t.Fatalf("outcomes = %#v", evidence.Recent)
	}
}

func TestCompactEvidencePayloadKeepsOnlyCoarseInterviewAssessment(t *testing.T) {
	payload := compactEvidencePayload(json.RawMessage(`{
		"topic":"Graphs",
		"mode":"coding",
		"strengths":["Clear decomposition"],
		"growth_edges":["Tradeoff depth"],
		"turn_count":4,
		"duration_seconds":120,
		"transcript":"must not survive",
		"code":"must not survive"
	}`))
	if payload["topic"] != "Graphs" || payload["mode"] != "coding" {
		t.Fatalf("payload = %#v", payload)
	}
	strengths, ok := payload["strengths"].([]string)
	if !ok || len(strengths) != 1 || strengths[0] != "Clear decomposition" {
		t.Fatalf("strengths = %#v", payload["strengths"])
	}
	if _, exists := payload["transcript"]; exists {
		t.Fatal("transcript survived compact evidence hygiene")
	}
	if _, exists := payload["code"]; exists {
		t.Fatal("code survived compact evidence hygiene")
	}
}
