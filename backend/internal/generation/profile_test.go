package generation

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
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
	if _, err := service.RecordEvent(ctx, memory.RecordEventInput{
		Source: "mcq", Type: "session_completed", Summary: "Finished round 1.",
		OccurredAt: now.Add(-30 * time.Minute),
		Payload:    json.RawMessage(`{"session_id":"round-1","round":1,"question_count":1,"answered_count":1,"correct_count":0}`),
	}); err != nil {
		t.Fatalf("seed completion: %v", err)
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
		result.Profile.Provenance.Provider != "scripted" || result.Profile.Provenance.EventCount != 2 {
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

func TestCodingEvidenceUsesDurableSubmissionIdentityAndAssistance(t *testing.T) {
	ctx := scopedContext()
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	store := memory.NewInMemoryStore()
	service := memory.NewService(store, func() time.Time { return now })
	inputs := []memory.RecordEventInput{
		{Source: memory.SourceWorkspace, Type: memory.TypeAttemptSolved, Summary: "Solved.", OccurredAt: now.Add(-5 * time.Minute), Payload: json.RawMessage(`{"session_id":"coding-1","problem_id":"queue-1","submission_id":"exec-1","graded":true,"passed":true,"assistance_status":"unknown"}`)},
		{Source: memory.SourceWorkspace, Type: memory.TypeAttemptSolved, Summary: "Solved replay.", OccurredAt: now.Add(-4 * time.Minute), Payload: json.RawMessage(`{"session_id":"coding-1","problem_id":"queue-1","submission_id":"exec-1","graded":true,"passed":true,"assistance_status":"used","used_help":true,"hint_count":1}`)},
		{Source: memory.SourceWorkspace, Type: memory.TypeAttemptSolved, Summary: "Solved again.", OccurredAt: now.Add(-3 * time.Minute), Payload: json.RawMessage(`{"session_id":"coding-1","problem_id":"queue-1","submission_id":"exec-2","graded":true,"passed":true,"assistance_status":"not_used","used_help":false,"hint_count":0,"coaching_used":false}`)},
		{Source: memory.SourceWorkspace, Type: memory.TypeAttemptSolved, Summary: "Legacy solved event.", OccurredAt: now.Add(-2 * time.Minute), Payload: json.RawMessage(`{"session_id":"coding-1","problem_id":"queue-1","passed":true}`)},
		{Source: memory.SourceMCQ, Type: memory.TypeQuestionAnswered, Summary: "Answered MCQ.", OccurredAt: now.Add(-time.Minute), Payload: json.RawMessage(`{"session_id":"mcq-1","question_id":"q1","topic":"Queues","correct":true}`)},
	}
	var firstEventID string
	for index, input := range inputs {
		recorded, err := service.RecordEvent(ctx, input)
		if err != nil {
			t.Fatal(err)
		}
		if index == 0 {
			firstEventID = recorded.ID
		}
	}
	// Reconstruct the service over the same durable store before deduplication.
	service = memory.NewService(store, func() time.Time { return now })
	_, events, _, err := service.ProfileInputs(ctx)
	if err != nil {
		t.Fatal(err)
	}

	canonical := profileEvidenceEvents(events)
	if len(canonical) != 3 {
		t.Fatalf("canonical evidence = %#v", canonical)
	}
	evidence := buildProfileEvidence(canonical, memory.Profile{}, "")
	if evidence.TotalEvents != 3 || len(evidence.Recent) != 3 {
		t.Fatalf("evidence = %#v", evidence)
	}
	if evidence.Recent[0].Details["submission_id"] != "exec-1" || evidence.Recent[0].Details["assistance_status"] != "unknown" {
		t.Fatalf("replayed completion rewrote the original assistance snapshot: %#v", evidence.Recent[0])
	}
	if _, leaked := evidence.Recent[0].Details["used_help"]; leaked {
		t.Fatalf("replayed assistance fields leaked into original snapshot: %#v", evidence.Recent[0])
	}
	if evidence.Recent[1].Details["submission_id"] != "exec-2" || evidence.Recent[1].Details["assistance_status"] != "not_used" {
		t.Fatalf("distinct completion was collapsed: %#v", evidence.Recent[1])
	}
	if evidence.Recent[2].Source != memory.SourceMCQ || evidence.Recent[2].Details["topic"] != "Queues" {
		t.Fatalf("MCQ evidence changed: %#v", evidence.Recent[2])
	}

	sources := profileEvidenceSources(canonical)
	if sources[0].SubmissionID != "exec-1" || sources[0].ProblemID != "queue-1" || sources[0].EventID != firstEventID {
		t.Fatalf("coding source provenance = %#v", sources[0])
	}
	if sources[1].SubmissionID != "exec-2" || sources[0].Key == sources[1].Key {
		t.Fatalf("distinct attempt provenance = %#v", sources)
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
	generator := &scriptedGenerator{payloads: []json.RawMessage{json.RawMessage(`{
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
	if err := json.Unmarshal(generator.requests[0].Spec, &evidence); err != nil {
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
	generator := &scriptedGenerator{payloads: []json.RawMessage{json.RawMessage(`{
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
	if err := json.Unmarshal(generator.requests[0].Spec, &evidence); err != nil {
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
	generator := &scriptedGenerator{payloads: []json.RawMessage{json.RawMessage(`{
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
	result, err := synthesizer.RefreshProfile(ctx, ProfileRefreshInput{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Profile.Provenance == nil || result.Profile.Provenance.OperationID != created.Operation.ID {
		t.Fatalf("operation provenance=%#v", result.Profile.Provenance)
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

func TestProfileSynthesizerRetriesOnceAfterStaleSave(t *testing.T) {
	ctx := scopedContext()
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	store := &staleOnceProfileStore{InMemoryStore: memory.NewInMemoryStore()}
	service := memory.NewService(store, func() time.Time { return now })
	if _, err := service.RecordEvent(ctx, memory.RecordEventInput{
		Source: "mcq", Type: "answer_incorrect", Summary: "Missed SQL joins.",
		Payload: json.RawMessage(`{"session_id":"mcq_r1","question_id":"q1","topic":"SQL Joins","correct":false}`),
	}); err != nil {
		t.Fatalf("seed event: %v", err)
	}
	if _, err := service.RecordEvent(ctx, memory.RecordEventInput{
		Source: "mcq", Type: "session_completed", Summary: "Finished round 1.",
		Payload: json.RawMessage(`{"session_id":"mcq_r1","round":1,"question_count":1,"answered_count":1,"correct_count":0}`),
	}); err != nil {
		t.Fatalf("seed completion: %v", err)
	}
	generator := &scriptedGenerator{payloads: []json.RawMessage{
		json.RawMessage(`{"actions":[]}`),
		json.RawMessage(`{"summary":"first attempt","strengths":[],"growth_edges":["SQL Joins"],"skills":[{"id":"sql-joins","label":"SQL Joins","area":"Data Systems","level":2,"confidence":50,"trend":"down"}],"notes":[]}`),
		json.RawMessage(`{"actions":[]}`),
		json.RawMessage(`{"summary":"retried attempt","strengths":[],"growth_edges":["SQL Joins"],"skills":[{"id":"sql-joins","label":"SQL Joins","area":"Data Systems","level":2,"confidence":55,"trend":"down"}],"notes":[]}`),
	}}
	synthesizer := NewProfileSynthesizer(NewOrchestrator(service, generator), service).WithClock(func() time.Time { return now })

	result, err := synthesizer.RefreshProfile(ctx, ProfileRefreshInput{SessionID: "mcq_r1", Trigger: "set-completion"})
	if err != nil {
		t.Fatalf("RefreshProfile: %v", err)
	}
	if result.Profile.Summary != "retried attempt" {
		t.Fatalf("profile = %#v", result.Profile)
	}
	if len(generator.requests) != 4 {
		t.Fatalf("model calls = %d, want first attempt plus retry", len(generator.requests))
	}
	if store.replaceAttempts != 2 {
		t.Fatalf("replace attempts = %d, want retry after stale save", store.replaceAttempts)
	}
}

func TestProfileSynthesizerRevisesExistingNoteIdentity(t *testing.T) {
	ctx := scopedContext()
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	service := memory.NewService(memory.NewInMemoryStore(), func() time.Time { return now })
	previous, err := service.ReplaceProfile(ctx, memory.Profile{
		Summary:      "SQL joins look uncertain from an earlier round.",
		UpdatedAt:    now.Add(-time.Hour),
		NextReviewAt: now.Add(time.Hour),
		Strengths:    []string{},
		GrowthEdges:  []string{"SQL Joins"},
		Skills:       []memory.SkillProficiency{},
		Notes: []memory.Note{{
			ID: "note_sql_joins", Title: "SQL joins", Summary: "Review join direction.",
			Tags: []string{"sql"}, Action: "review", CreatedAt: now.Add(-time.Hour),
		}},
	})
	if err != nil {
		t.Fatalf("seed profile: %v", err)
	}
	if _, err := service.RecordEvent(ctx, memory.RecordEventInput{
		Source: "mcq", Type: "question_answered", Summary: "Answered SQL joins correctly without help.",
		Payload: json.RawMessage(`{"session_id":"mcq_r2","question_id":"q1","topic":"SQL Joins","correct":true}`),
	}); err != nil {
		t.Fatalf("seed event: %v", err)
	}
	if _, err := service.RecordEvent(ctx, memory.RecordEventInput{
		Source: "mcq", Type: "session_completed", Summary: "Finished round 2.",
		Payload: json.RawMessage(`{"session_id":"mcq_r2","round":2,"question_count":1,"answered_count":1,"correct_count":1}`),
	}); err != nil {
		t.Fatalf("seed completion: %v", err)
	}
	generator := &scriptedGenerator{payloads: []json.RawMessage{
		json.RawMessage(`{"actions":[{"op":"update","note":{"id":"note_sql_joins","title":"SQL joins","summary":"Join direction is improving; keep an eye on unmatched rows.","tags":["sql"],"action":"keep"}}]}`),
		json.RawMessage(`{"summary":"SQL joins improved after later correct evidence.","strengths":["SQL Joins"],"growth_edges":[],"skills":[{"id":"sql-joins","label":"SQL Joins","area":"Data Systems","level":3,"confidence":65,"trend":"up"}],"notes":[{"id":"note_sql_joins","title":"SQL joins","summary":"Join direction is improving; keep an eye on unmatched rows.","tags":["sql"],"action":"keep"}]}`),
	}}
	synthesizer := NewProfileSynthesizer(NewOrchestrator(service, generator), service).WithClock(func() time.Time { return now })

	result, err := synthesizer.RefreshProfile(ctx, ProfileRefreshInput{SessionID: "mcq_r2", Trigger: "set-completion"})
	if err != nil {
		t.Fatalf("RefreshProfile: %v", err)
	}
	if result.Profile.Summary == previous.Summary || result.Profile.Summary != "SQL joins improved after later correct evidence." {
		t.Fatalf("summary was not revised: %#v", result.Profile)
	}
	if len(result.Profile.Notes) != 1 || result.Profile.Notes[0].ID != "note_sql_joins" || result.Profile.Notes[0].Action != "keep" {
		t.Fatalf("note identity/action = %#v", result.Profile.Notes)
	}
}

func TestProfileSynthesizerRunsCodingNotesFirstWithNotesOnlyProfileInput(t *testing.T) {
	ctx := scopedContext()
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	service := memory.NewService(memory.NewInMemoryStore(), func() time.Time { return now })
	_, err := service.ReplaceProfile(ctx, memory.Profile{
		Summary:      "Queue practice is beginning.",
		UpdatedAt:    now.Add(-time.Hour),
		NextReviewAt: now.Add(time.Hour),
		Strengths:    []string{},
		GrowthEdges:  []string{},
		Skills:       []memory.SkillProficiency{},
		Notes:        []memory.Note{},
	})
	if err != nil {
		t.Fatalf("seed profile: %v", err)
	}
	completion, err := service.RecordEvent(ctx, memory.RecordEventInput{
		Source: memory.SourceWorkspace, Type: memory.TypeAttemptSolved, Summary: "Solved a queue implementation problem.",
		Payload: json.RawMessage(`{"session_id":"coding_r1","problem_id":"queue-1","submission_id":"exec-1","graded":true,"concept":"Queues","passed":true,"total":1,"passed_count":1,"failed_count":0,"assistance_status":"used","used_help":true,"hint_count":1}`),
	})
	if err != nil {
		t.Fatalf("seed event: %v", err)
	}
	notePayload := json.RawMessage(`{"actions":[{"op":"create","note":{"id":"note_queue_1","problem_id":"queue-1","title":"Queue implementation","summary":"Practice queue boundary handling; the observed success used assistance.","tags":["queues"],"action":"review"}}]}`)
	profilePayload := json.RawMessage(`{"summary":"Queue implementation has been practiced with assistance, so independent fluency remains uncertain.","strengths":[],"growth_edges":["Queue implementation"],"skills":[],"notes":[]}`)
	generator := &scriptedGenerator{payloads: []json.RawMessage{notePayload, profilePayload}}
	synthesizer := NewProfileSynthesizer(NewOrchestrator(service, generator), service).WithClock(func() time.Time { return now })

	result, err := synthesizer.RefreshProfile(ctx, ProfileRefreshInput{
		SessionID: "coding_r1", ProblemID: "queue-1", SubmissionID: "exec-1", Trigger: "set-completion",
	})
	if err != nil {
		t.Fatalf("RefreshProfile: %v", err)
	}
	if result.Skipped != "" {
		t.Fatalf("unexpected skipped result: %s", result.Skipped)
	}
	if len(generator.requests) != 2 || generator.requests[0].Kind != KindNotes || generator.requests[1].Kind != KindProfile {
		t.Fatalf("requests = %#v", generator.requests)
	}
	var noteInput profileEvidence
	if err := json.Unmarshal(generator.requests[0].Spec, &noteInput); err != nil {
		t.Fatalf("decode note input: %v", err)
	}
	if noteInput.SessionID != "coding_r1" || noteInput.ProblemID != "queue-1" || noteInput.SubmissionID != "exec-1" || len(noteInput.Recent) != 1 {
		t.Fatalf("targeted coding note input = %#v", noteInput)
	}
	if noteInput.Recent[0].EventID != completion.ID || noteInput.Recent[0].Details["assistance_status"] != "used" || noteInput.Recent[0].Details["submission_id"] != "exec-1" {
		t.Fatalf("coding source/assistance input = %#v", noteInput.Recent[0])
	}
	var profileInput codingProfileInput
	if err := json.Unmarshal(generator.requests[1].Spec, &profileInput); err != nil {
		t.Fatalf("decode coding profile input: %v", err)
	}
	if profileInput.Version != 1 || len(profileInput.MaintainedNotes) != 1 || profileInput.MaintainedNotes[0].ID != "note_queue_1" {
		t.Fatalf("coding profile input = %#v", profileInput)
	}
	for _, forbidden := range []string{"recent_events", "event_counts", "submission_id", "attempt_solved", "assistance_status", "used_help"} {
		if strings.Contains(string(generator.requests[1].Spec), forbidden) {
			t.Fatalf("raw coding evidence %q leaked into profile input: %s", forbidden, generator.requests[1].Spec)
		}
	}
	if len(generator.requests[1].MemoryContext.Notes) != 1 || generator.requests[1].MemoryContext.Notes[0].Summary != profileInput.MaintainedNotes[0].Summary {
		t.Fatalf("profile memory context did not use maintained notes: %#v", generator.requests[1].MemoryContext)
	}
	if len(result.Profile.Notes) != 1 || len(result.MaintainedNotes) != 1 || result.Profile.Notes[0].ID != "note_queue_1" {
		t.Fatalf("maintained notes = %#v result snapshot=%#v", result.Profile.Notes, result.MaintainedNotes)
	}
	provenance := result.Profile.Provenance
	if provenance == nil || provenance.RefreshID == "" || provenance.RefreshAttempt != 1 ||
		provenance.InputVersion != 1 || provenance.OutputVersion != 2 || provenance.Completion == nil ||
		provenance.Completion.EventID != completion.ID || provenance.Completion.SubmissionID != "exec-1" ||
		provenance.NotesStage == nil || provenance.ProfileStage == nil ||
		provenance.NotesStage.Provider != "scripted" || provenance.ProfileStage.PromptRevision != codingProfilePromptRevision {
		t.Fatalf("coding provenance = %#v", provenance)
	}
	if !reflect.DeepEqual(profileInput.MaintainedNotes, provenance.MaintainedNotes) {
		t.Fatalf("profile input snapshot=%#v provenance snapshot=%#v", profileInput.MaintainedNotes, provenance.MaintainedNotes)
	}
	result.MaintainedNotes[0].Summary = "mutated result"
	result.MaintainedNotes[0].Tags[0] = "mutated"
	persisted, err := service.GetProfile(ctx)
	if err != nil || persisted.Provenance.MaintainedNotes[0].Summary == "mutated result" || persisted.Provenance.MaintainedNotes[0].Tags[0] == "mutated" {
		t.Fatalf("persisted snapshot was mutable: profile=%#v err=%v", persisted, err)
	}
}

func TestCodingRefreshFeedsNextGenerationMemoryContext(t *testing.T) {
	ctx := scopedContext()
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	service := memory.NewService(memory.NewInMemoryStore(), func() time.Time { return now })
	if _, err := service.RecordEvent(ctx, memory.RecordEventInput{
		Source: memory.SourceWorkspace, Type: memory.TypeAttemptSolved, Summary: "Solved a queue implementation problem.",
		Payload: json.RawMessage(`{"session_id":"coding_r1","problem_id":"queue-1","submission_id":"exec-1","graded":true,"concept":"Queues","passed":true,"total":1,"passed_count":1,"failed_count":0,"assistance_status":"used","used_help":true,"hint_count":1}`),
	}); err != nil {
		t.Fatalf("seed completion: %v", err)
	}

	refreshGenerator := &scriptedGenerator{payloads: []json.RawMessage{
		json.RawMessage(`{"actions":[{"op":"create","note":{"id":"note_queue_1","problem_id":"queue-1","title":"Queue implementation","summary":"Practice queue boundary handling; the observed success used assistance.","tags":["queues"],"action":"review"}}]}`),
		json.RawMessage(`{"summary":"Queue implementation has been practiced with assistance.","strengths":[],"growth_edges":["Queue implementation"],"skills":[],"notes":[]}`),
	}}
	synthesizer := NewProfileSynthesizer(NewOrchestrator(service, refreshGenerator), service).WithClock(func() time.Time { return now })
	if _, err := synthesizer.RefreshProfile(ctx, ProfileRefreshInput{
		SessionID: "coding_r1", ProblemID: "queue-1", SubmissionID: "exec-1", Trigger: "set-completion",
	}); err != nil {
		t.Fatalf("refresh coding profile: %v", err)
	}

	nextGenerator := &recordingGenerator{result: GenerateResult{
		Object: json.RawMessage(`{"title":"Queue practice"}`), Provider: "test", Model: "fake",
	}}
	orchestrator := NewOrchestrator(service, nextGenerator)
	if _, err := orchestrator.Generate(ctx, GenerateInput{
		Kind: KindProblem, Spec: json.RawMessage(`{"topic":"queues"}`),
		Schema: Schema{Name: "coding_problem", Version: "v1", JSONSchema: json.RawMessage(`{"type":"object"}`)},
	}); err != nil {
		t.Fatalf("generate after refresh: %v", err)
	}

	notes := nextGenerator.request.MemoryContext.Notes
	if len(notes) != 1 || notes[0].Title != "Queue implementation" || notes[0].ProblemID != "queue-1" {
		t.Fatalf("next generation memory notes = %#v", notes)
	}
	if notes[0].Summary != "Practice queue boundary handling; the observed success used assistance." {
		t.Fatalf("next generation note summary = %q", notes[0].Summary)
	}
	if nextGenerator.request.MemoryContext.Summary != "Queue implementation has been practiced with assistance." {
		t.Fatalf("next generation memory summary = %q", nextGenerator.request.MemoryContext.Summary)
	}
}

func TestCodingCompletionReplaySurvivesServiceReconstruction(t *testing.T) {
	ctx := scopedContext()
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	store := memory.NewInMemoryStore()
	service := memory.NewService(store, func() time.Time { return now })
	seedCodingCompletion(t, ctx, service, memory.TypeAttemptSolved, "coding-1", "queue-1", "exec-1", true, "not_used")
	firstGenerator := &scriptedGenerator{payloads: codingProfilePayloads("first profile")}
	first, err := NewProfileSynthesizer(NewOrchestrator(service, firstGenerator), service).WithClock(func() time.Time { return now }).RefreshProfile(ctx, ProfileRefreshInput{
		SessionID: "coding-1", ProblemID: "queue-1", SubmissionID: "exec-1", Trigger: "set-completion",
	})
	if err != nil || !first.Changed || first.Profile.Version != 1 {
		t.Fatalf("first refresh=%#v err=%v", first, err)
	}

	service = memory.NewService(store, func() time.Time { return now.Add(time.Minute) })
	replayGenerator := &scriptedGenerator{payloads: codingProfilePayloads("must not run")}
	replay, err := NewProfileSynthesizer(NewOrchestrator(service, replayGenerator), service).RefreshProfile(ctx, ProfileRefreshInput{
		SessionID: "coding-1", ProblemID: "queue-1", SubmissionID: "exec-1", Trigger: "set-completion",
	})
	if err != nil || replay.Changed || replay.Skipped != "coding completion already processed" || len(replayGenerator.requests) != 0 {
		t.Fatalf("replay=%#v err=%v requests=%d", replay, err, len(replayGenerator.requests))
	}
	if replay.Profile.Version != 1 || replay.Profile.Summary != "first profile" {
		t.Fatalf("replay profile=%#v", replay.Profile)
	}
}

func TestCodingCompletionReplayAfterLaterCompletionIsStillDeduped(t *testing.T) {
	ctx := scopedContext()
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	service := memory.NewService(memory.NewInMemoryStore(), func() time.Time { return now })
	seedCodingCompletion(t, ctx, service, memory.TypeAttemptSolved, "coding-1", "queue-1", "exec-a", true, "not_used")
	seedCodingCompletion(t, ctx, service, memory.TypeAttemptFailed, "coding-1", "queue-1", "exec-b", false, "unknown")
	generator := &scriptedGenerator{payloads: append(codingProfilePayloads("profile A"), codingProfilePayloads("profile B")...)}
	synthesizer := NewProfileSynthesizer(NewOrchestrator(service, generator), service).WithClock(func() time.Time { return now })
	for _, submissionID := range []string{"exec-a", "exec-b"} {
		if result, err := synthesizer.RefreshProfile(ctx, ProfileRefreshInput{SessionID: "coding-1", ProblemID: "queue-1", SubmissionID: submissionID}); err != nil || !result.Changed {
			t.Fatalf("refresh %s result=%#v err=%v", submissionID, result, err)
		}
	}
	replay, err := synthesizer.RefreshProfile(ctx, ProfileRefreshInput{SessionID: "coding-1", ProblemID: "queue-1", SubmissionID: "exec-a"})
	if err != nil || replay.Changed || len(generator.requests) != 4 || replay.Profile.Version != 2 || replay.Profile.Summary != "profile B" {
		t.Fatalf("A-B-A replay=%#v err=%v requests=%d", replay, err, len(generator.requests))
	}
}

func TestConcurrentCodingCompletionRefreshCommitsOnce(t *testing.T) {
	ctx := scopedContext()
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	service := memory.NewService(memory.NewInMemoryStore(), func() time.Time { return now })
	seedCodingCompletion(t, ctx, service, memory.TypeAttemptSolved, "coding-1", "queue-1", "exec-1", true, "not_used")
	generator := &synchronizedCodingGenerator{}
	synthesizer := NewProfileSynthesizer(NewOrchestrator(service, generator), service).WithClock(func() time.Time { return now })
	input := ProfileRefreshInput{SessionID: "coding-1", ProblemID: "queue-1", SubmissionID: "exec-1"}
	results := make(chan ProfileRefreshResult, 2)
	errs := make(chan error, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, err := synthesizer.RefreshProfile(ctx, input)
			results <- result
			errs <- err
		}()
	}
	wait.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent refresh: %v", err)
		}
	}
	changed := 0
	for result := range results {
		if result.Changed {
			changed++
		}
	}
	if changed != 1 {
		t.Fatalf("changed results=%d, want one", changed)
	}
	persisted, err := service.GetProfile(ctx)
	if err != nil || persisted.Version != 1 {
		t.Fatalf("persisted=%#v err=%v", persisted, err)
	}
}

func TestProfileSynthesizerRunsCodingNotesFirstForGradedFailure(t *testing.T) {
	ctx := scopedContext()
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	service := memory.NewService(memory.NewInMemoryStore(), func() time.Time { return now })
	if _, err := service.RecordEvent(ctx, memory.RecordEventInput{
		Source: memory.SourceWorkspace, Type: memory.TypeAttemptFailed, Summary: "Queue submission did not pass.",
		Payload: json.RawMessage(`{"session_id":"coding-fail","problem_id":"queue-1","submission_id":"exec-fail","graded":true,"concept":"Queues","passed":false,"total":1,"passed_count":0,"failed_count":1,"assistance_status":"unknown"}`),
	}); err != nil {
		t.Fatal(err)
	}
	generator := &scriptedGenerator{payloads: []json.RawMessage{
		json.RawMessage(`{"actions":[]}`),
		json.RawMessage(`{"summary":"Queue behavior remains uncertain after one failed graded attempt.","strengths":[],"growth_edges":["Queues"],"skills":[],"notes":[]}`),
	}}
	result, err := NewProfileSynthesizer(NewOrchestrator(service, generator), service).WithClock(func() time.Time { return now }).RefreshProfile(ctx, ProfileRefreshInput{
		SessionID: "coding-fail", ProblemID: "queue-1", SubmissionID: "exec-fail", Trigger: "set-completion",
	})
	if err != nil || result.Skipped != "" || len(generator.requests) != 2 || generator.requests[0].Kind != KindNotes {
		t.Fatalf("graded failure result=%#v err=%v requests=%#v", result, err, generator.requests)
	}
}

func TestCodingNotesFirstRequiresExactGradedCompletionIdentity(t *testing.T) {
	valid := memory.Event{
		ID: "event-valid", Source: memory.SourceWorkspace, Type: memory.TypeAttemptSolved,
		Payload: json.RawMessage(`{"session_id":"coding-1","problem_id":"queue-1","submission_id":"exec-1","graded":true,"passed":true,"total":1,"passed_count":1,"failed_count":0,"assistance_status":"not_used"}`),
	}
	tests := []struct {
		name   string
		input  ProfileRefreshInput
		events []memory.Event
	}{
		{name: "missing submission", input: ProfileRefreshInput{SessionID: "coding-1", ProblemID: "queue-1"}, events: []memory.Event{valid}},
		{name: "wrong submission", input: ProfileRefreshInput{SessionID: "coding-1", ProblemID: "queue-1", SubmissionID: "exec-2"}, events: []memory.Event{valid}},
		{name: "wrong session", input: ProfileRefreshInput{SessionID: "coding-2", ProblemID: "queue-1", SubmissionID: "exec-1"}, events: []memory.Event{valid}},
		{name: "wrong problem", input: ProfileRefreshInput{SessionID: "coding-1", ProblemID: "stack-1", SubmissionID: "exec-1"}, events: []memory.Event{valid}},
		{name: "run only", input: ProfileRefreshInput{SessionID: "coding-1", ProblemID: "queue-1", SubmissionID: "exec-1"}, events: []memory.Event{{ID: "run", Source: memory.SourceWorkspace, Type: memory.TypeTestsRun, Payload: json.RawMessage(`{"session_id":"coding-1","problem_id":"queue-1","submission_id":"exec-1"}`)}}},
		{name: "neutral skip", input: ProfileRefreshInput{SessionID: "coding-1", ProblemID: "queue-1", SubmissionID: "exec-1"}, events: []memory.Event{{ID: "skip", Source: memory.SourceMCQ, Type: memory.TypeQuestionSkipped, Payload: json.RawMessage(`{"session_id":"coding-1","question_id":"q1","skipped":true}`)}}},
		{name: "legacy outcome", input: ProfileRefreshInput{SessionID: "coding-1", ProblemID: "queue-1", SubmissionID: "exec-1"}, events: []memory.Event{{ID: "legacy", Source: memory.SourceWorkspace, Type: memory.TypeAttemptSolved, Payload: json.RawMessage(`{"session_id":"coding-1","problem_id":"queue-1","submission_id":"exec-1","passed":true,"assistance_status":"unknown"}`)}}},
		{name: "malformed graded result", input: ProfileRefreshInput{SessionID: "coding-1", ProblemID: "queue-1", SubmissionID: "exec-1"}, events: []memory.Event{{ID: "malformed", Source: memory.SourceWorkspace, Type: memory.TypeAttemptSolved, Payload: json.RawMessage(`{"session_id":"coding-1","problem_id":"queue-1","submission_id":"exec-1","graded":true,"passed":true,"assistance_status":"unknown"}`)}}},
		{name: "platform failure", input: ProfileRefreshInput{SessionID: "coding-1", ProblemID: "queue-1", SubmissionID: "exec-1"}, events: []memory.Event{{ID: "platform", Source: memory.SourceWorkspace, Type: memory.TypeAttemptFailed, Payload: json.RawMessage(`{"session_id":"coding-1","problem_id":"queue-1","submission_id":"exec-1","graded":false,"passed":false,"assistance_status":"unknown"}`)}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := codingCompletionEvidence(test.input, profileEvidenceEvents(test.events))
			if !errors.Is(err, ErrCodingCompletionEvidenceNotFound) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestTargetedCodingRefreshFailsExplicitlyWhenEvidenceIsUnavailable(t *testing.T) {
	ctx := scopedContext()
	service := memory.NewService(memory.NewInMemoryStore(), nil)
	generator := &scriptedGenerator{payloads: []json.RawMessage{json.RawMessage(`{"actions":[]}`)}}
	_, err := NewProfileSynthesizer(NewOrchestrator(service, generator), service).RefreshProfile(ctx, ProfileRefreshInput{
		SessionID: "coding-1", ProblemID: "queue-1", SubmissionID: "exec-missing", Trigger: "set-completion",
	})
	if !errors.Is(err, ErrCodingCompletionEvidenceNotFound) || len(generator.requests) != 0 {
		t.Fatalf("error=%v requests=%#v", err, generator.requests)
	}
}

func TestReplayedCodingEvidenceCannotDowngradeAssistance(t *testing.T) {
	events := []memory.Event{
		{ID: "first", Source: memory.SourceWorkspace, Type: memory.TypeAttemptSolved, Payload: json.RawMessage(`{"session_id":"coding-1","problem_id":"queue-1","submission_id":"exec-1","graded":true,"passed":true,"assistance_status":"used","used_help":true,"hint_count":1}`)},
		{ID: "replay", Source: memory.SourceWorkspace, Type: memory.TypeAttemptSolved, Payload: json.RawMessage(`{"session_id":"coding-1","problem_id":"queue-1","submission_id":"exec-1","graded":true,"passed":true,"assistance_status":"not_used","used_help":false,"hint_count":0,"coaching_used":false}`)},
	}
	canonical := profileEvidenceEvents(events)
	if len(canonical) != 1 || canonical[0].ID != "first" {
		t.Fatalf("canonical=%#v", canonical)
	}
	details := compactEvidencePayload(canonical[0].Payload)
	if details["assistance_status"] != "used" || details["used_help"] != true || details["hint_count"] != float64(1) {
		t.Fatalf("assistance snapshot was downgraded: %#v", details)
	}
}

func TestCodingInvalidMaintainedNotesStopBeforeProfileAndSave(t *testing.T) {
	ctx := scopedContext()
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	service := memory.NewService(memory.NewInMemoryStore(), func() time.Time { return now })
	previous, err := service.ReplaceProfile(ctx, memory.Profile{
		Summary: "Known-good coding profile.", UpdatedAt: now.Add(-time.Hour), NextReviewAt: now.Add(time.Hour),
		Strengths: []string{}, GrowthEdges: []string{}, Skills: []memory.SkillProficiency{}, Notes: []memory.Note{},
	})
	if err != nil {
		t.Fatal(err)
	}
	seedCodingCompletion(t, ctx, service, memory.TypeAttemptSolved, "coding-1", "queue-1", "exec-1", true, "unknown")
	generator := &scriptedGenerator{payloads: []json.RawMessage{
		json.RawMessage(`{"actions":[{"op":"create","note":{"id":"note_queue","title":"Queues","summary":"Review queue boundaries.","tags":["queues"],"action":"review"}}]}`),
		json.RawMessage(`{"summary":"must not run","strengths":[],"growth_edges":[],"skills":[],"notes":[]}`),
	}}
	result, err := NewProfileSynthesizer(NewOrchestrator(service, generator), service).WithClock(func() time.Time { return now }).RefreshProfile(ctx, ProfileRefreshInput{
		SessionID: "coding-1", ProblemID: "queue-1", SubmissionID: "exec-1", Trigger: "set-completion",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Skipped, "invalid maintained notes") || len(generator.requests) != 1 {
		t.Fatalf("result=%#v requests=%#v", result, generator.requests)
	}
	persisted, err := service.GetProfile(ctx)
	if err != nil || persisted.Version != previous.Version || persisted.Summary != previous.Summary || len(persisted.Notes) != 0 {
		t.Fatalf("persisted=%#v err=%v", persisted, err)
	}
	if processed, err := service.IsCompletionProcessed(ctx, "coding-1", "queue-1", "exec-1"); err != nil || processed {
		t.Fatalf("invalid notes marked completion: processed=%t err=%v", processed, err)
	}
}

func TestCodingProfileFailurePreservesProfileAndMaintainedNotes(t *testing.T) {
	ctx := scopedContext()
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	createdAt := now.Add(-time.Hour)
	service := memory.NewService(memory.NewInMemoryStore(), func() time.Time { return now })
	previous, err := service.ReplaceProfile(ctx, memory.Profile{
		Summary: "Known-good coding profile.", UpdatedAt: createdAt, NextReviewAt: now.Add(time.Hour),
		Strengths: []string{}, GrowthEdges: []string{}, Skills: []memory.SkillProficiency{},
		Notes: []memory.Note{{ID: "note_queue", ProblemID: "queue-1", Title: "Queues", Summary: "Review queue boundaries.", CreatedAt: createdAt, Tags: []string{"queues"}, Action: "review"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	seedCodingCompletion(t, ctx, service, memory.TypeAttemptSolved, "coding-1", "queue-1", "exec-1", true, "not_used")
	generator := &scriptedGenerator{
		payloads: []json.RawMessage{
			json.RawMessage(`{"actions":[{"op":"update","note":{"id":"note_queue","summary":"Queue boundaries improved, but repeat independently.","action":"keep"}}]}`),
			json.RawMessage(`{}`),
		},
		errs: []error{nil, errors.New("profile provider unavailable")},
	}
	result, err := NewProfileSynthesizer(NewOrchestrator(service, generator), service).WithClock(func() time.Time { return now }).RefreshProfile(ctx, ProfileRefreshInput{
		SessionID: "coding-1", ProblemID: "queue-1", SubmissionID: "exec-1", Trigger: "set-completion",
	})
	if err != nil || !strings.Contains(result.Skipped, "profile generation failed") || len(generator.requests) != 2 {
		t.Fatalf("result=%#v err=%v requests=%#v", result, err, generator.requests)
	}
	persisted, err := service.GetProfile(ctx)
	if err != nil || persisted.Version != previous.Version || !reflect.DeepEqual(persisted.Notes, previous.Notes) {
		t.Fatalf("persisted=%#v err=%v", persisted, err)
	}
	if processed, err := service.IsCompletionProcessed(ctx, "coding-1", "queue-1", "exec-1"); err != nil || processed {
		t.Fatalf("profile failure marked completion: processed=%t err=%v", processed, err)
	}
}

func TestCodingMaintainedNotesReuseCarryForwardAndPruneIdentities(t *testing.T) {
	ctx := scopedContext()
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	createdAt := now.Add(-2 * time.Hour)
	service := memory.NewService(memory.NewInMemoryStore(), func() time.Time { return now })
	_, err := service.ReplaceProfile(ctx, memory.Profile{
		Summary: "Queues and stacks have both been practiced.", UpdatedAt: createdAt, NextReviewAt: now.Add(time.Hour),
		Strengths: []string{}, GrowthEdges: []string{}, Skills: []memory.SkillProficiency{},
		Notes: []memory.Note{
			{ID: "note_queue_original", ProblemID: "queue-1", Title: "Queues", Summary: "Review queue boundaries.", CreatedAt: createdAt, Tags: []string{"queues"}, Action: "review"},
			{ID: "note_stack", ProblemID: "stack-1", Title: "Stacks", Summary: "Carry forward stack ordering reminders.", CreatedAt: createdAt.Add(-time.Hour), Tags: []string{"stacks"}, Action: "keep"},
			{ID: "note_old", ProblemID: "old-1", Title: "Old topic", Summary: "Stale reminder.", CreatedAt: createdAt.Add(-2 * time.Hour), Tags: []string{"old"}, Action: "review"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	seedCodingCompletion(t, ctx, service, memory.TypeAttemptSolved, "coding-1", "queue-1", "exec-1", true, "not_used")
	generator := &scriptedGenerator{payloads: []json.RawMessage{
		json.RawMessage(`{"actions":[{"op":"create","note":{"id":"note_queue_duplicate","problem_id":"queue-1","title":"Queues","summary":"Queue boundary handling improved independently.","tags":["queues"],"action":"keep"}},{"op":"prune","note":{"id":"note_old"}}]}`),
		json.RawMessage(`{"summary":"Queue handling improved while stack knowledge remains useful.","strengths":[],"growth_edges":[],"skills":[],"notes":[]}`),
	}}
	result, err := NewProfileSynthesizer(NewOrchestrator(service, generator), service).WithClock(func() time.Time { return now }).RefreshProfile(ctx, ProfileRefreshInput{
		SessionID: "coding-1", ProblemID: "queue-1", SubmissionID: "exec-1", Trigger: "set-completion",
	})
	if err != nil || result.Skipped != "" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if len(result.Profile.Notes) != 2 || result.Profile.Notes[0].ID != "note_queue_original" || result.Profile.Notes[1].ID != "note_stack" {
		t.Fatalf("notes=%#v", result.Profile.Notes)
	}
	if !result.Profile.Notes[0].CreatedAt.Equal(createdAt) || result.Profile.Notes[0].Summary != "Queue boundary handling improved independently." {
		t.Fatalf("updated queue note=%#v", result.Profile.Notes[0])
	}
}

func seedCodingCompletion(t *testing.T, ctx context.Context, service *memory.Service, eventType, sessionID, problemID, submissionID string, passed bool, assistance string) memory.Event {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"session_id": sessionID, "problem_id": problemID, "submission_id": submissionID,
		"graded": true, "passed": passed, "assistance_status": assistance, "concept": "Queues",
		"total": 1, "passed_count": map[bool]int{true: 1, false: 0}[passed], "failed_count": map[bool]int{true: 0, false: 1}[passed],
	})
	if err != nil {
		t.Fatal(err)
	}
	event, err := service.RecordEvent(ctx, memory.RecordEventInput{
		Source: memory.SourceWorkspace, Type: eventType, Summary: "Finalized graded coding submission.",
		Payload: payload,
	})
	if err != nil {
		t.Fatal(err)
	}
	return event
}

func codingProfilePayloads(summary string) []json.RawMessage {
	profile, _ := json.Marshal(map[string]any{
		"summary": summary, "strengths": []string{}, "growth_edges": []string{},
		"skills": []memory.SkillProficiency{}, "notes": []memory.Note{},
	})
	return []json.RawMessage{json.RawMessage(`{"actions":[]}`), profile}
}

type synchronizedCodingGenerator struct {
	mu       sync.Mutex
	requests []GenerateRequest
}

func (g *synchronizedCodingGenerator) Generate(_ context.Context, request GenerateRequest) (GenerateResult, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.requests = append(g.requests, request)
	payload := json.RawMessage(`{"actions":[]}`)
	if request.Kind == KindProfile {
		payload = json.RawMessage(`{"summary":"concurrent profile","strengths":[],"growth_edges":[],"skills":[],"notes":[]}`)
	}
	return GenerateResult{Object: payload, Provider: "synchronized", Model: "fake"}, nil
}

func TestProfileSynthesizerRequiresMatchingMCQCompletionSessionForNotesFirst(t *testing.T) {
	ctx := scopedContext()
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	service := memory.NewService(memory.NewInMemoryStore(), func() time.Time { return now })
	for _, input := range []memory.RecordEventInput{
		{Source: "mcq", Type: "answer_incorrect", Summary: "Missed SQL joins in the completed round.", Payload: json.RawMessage(`{"session_id":"mcq_r1","question_id":"q1","topic":"SQL Joins","correct":false}`)},
		{Source: "mcq", Type: "session_completed", Summary: "Finished round 1.", Payload: json.RawMessage(`{"session_id":"mcq_r1","round":1,"question_count":1,"answered_count":1,"correct_count":0}`)},
		{Source: "mcq", Type: "answer_incorrect", Summary: "Missed indexes in a different pending round.", Payload: json.RawMessage(`{"session_id":"mcq_r2","question_id":"q2","topic":"Indexes","correct":false}`)},
	} {
		if _, err := service.RecordEvent(ctx, input); err != nil {
			t.Fatalf("seed event: %v", err)
		}
	}
	generator := &scriptedGenerator{payloads: []json.RawMessage{json.RawMessage(`{
		"summary":"Indexes need more practice.","strengths":[],"growth_edges":["Indexes"],
		"skills":[{"id":"indexes","label":"Indexes","area":"Data Systems","level":2,"confidence":45,"trend":"down"}],
		"notes":[]
	}`)}}
	synthesizer := NewProfileSynthesizer(NewOrchestrator(service, generator), service).WithClock(func() time.Time { return now })

	result, err := synthesizer.RefreshProfile(ctx, ProfileRefreshInput{SessionID: "mcq_r2", Trigger: "set-completion"})
	if err != nil {
		t.Fatalf("RefreshProfile: %v", err)
	}
	if result.Skipped != "" {
		t.Fatalf("unexpected skipped result: %s", result.Skipped)
	}
	if len(generator.requests) != 1 || generator.requests[0].Kind != KindProfile {
		t.Fatalf("notes-first ran without a matching completion session: %#v", generator.requests)
	}
	var evidence profileEvidence
	if err := json.Unmarshal(generator.requests[0].Spec, &evidence); err != nil {
		t.Fatalf("decode evidence: %v", err)
	}
	if evidence.SessionID != "mcq_r2" {
		t.Fatalf("focused session id = %q", evidence.SessionID)
	}
}

type staleOnceProfileStore struct {
	*memory.InMemoryStore
	replaceAttempts int
}

type staleOnceCompletionStore struct {
	*memory.InMemoryStore
	replaceAttempts int
	refreshIDs      []string
	refreshAttempts []int
}

func (s *staleOnceCompletionStore) ReplaceProfileAndMarkCompletionIfVersion(ctx context.Context, workspaceID, userID string, profile memory.Profile, expectedVersion int64, completion memory.ProcessedCompletion) error {
	s.replaceAttempts++
	s.refreshIDs = append(s.refreshIDs, profile.Provenance.RefreshID)
	s.refreshAttempts = append(s.refreshAttempts, profile.Provenance.RefreshAttempt)
	if s.replaceAttempts == 1 {
		return memory.ErrStaleProfile
	}
	return s.InMemoryStore.ReplaceProfileAndMarkCompletionIfVersion(ctx, workspaceID, userID, profile, expectedVersion, completion)
}

func TestCodingRefreshRetriesStaleSaveWithSameRefreshID(t *testing.T) {
	ctx := scopedContext()
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	store := &staleOnceCompletionStore{InMemoryStore: memory.NewInMemoryStore()}
	service := memory.NewService(store, func() time.Time { return now })
	seedCodingCompletion(t, ctx, service, memory.TypeAttemptSolved, "coding-1", "queue-1", "exec-1", true, "not_used")
	generator := &scriptedGenerator{payloads: append(codingProfilePayloads("first attempt"), codingProfilePayloads("retried attempt")...)}
	result, err := NewProfileSynthesizer(NewOrchestrator(service, generator), service).WithClock(func() time.Time { return now }).RefreshProfile(ctx, ProfileRefreshInput{
		SessionID: "coding-1", ProblemID: "queue-1", SubmissionID: "exec-1",
	})
	if err != nil || result.Profile.Summary != "retried attempt" || len(generator.requests) != 4 {
		t.Fatalf("result=%#v err=%v requests=%d", result, err, len(generator.requests))
	}
	if store.replaceAttempts != 2 || store.refreshIDs[0] == "" || store.refreshIDs[0] != store.refreshIDs[1] ||
		!reflect.DeepEqual(store.refreshAttempts, []int{1, 2}) || result.Profile.Provenance.RefreshAttempt != 2 {
		t.Fatalf("refresh ids=%#v attempts=%#v provenance=%#v", store.refreshIDs, store.refreshAttempts, result.Profile.Provenance)
	}
}

type failedCompletionStore struct {
	*memory.InMemoryStore
}

func (s *failedCompletionStore) ReplaceProfileAndMarkCompletionIfVersion(context.Context, string, string, memory.Profile, int64, memory.ProcessedCompletion) error {
	return errors.New("persistence unavailable")
}

func TestCodingPersistenceFailureCommitsNeitherProfileNorMarker(t *testing.T) {
	ctx := scopedContext()
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	store := &failedCompletionStore{InMemoryStore: memory.NewInMemoryStore()}
	service := memory.NewService(store, func() time.Time { return now })
	seedCodingCompletion(t, ctx, service, memory.TypeAttemptSolved, "coding-1", "queue-1", "exec-1", true, "not_used")
	_, err := NewProfileSynthesizer(NewOrchestrator(service, &scriptedGenerator{payloads: codingProfilePayloads("must not persist")}), service).WithClock(func() time.Time { return now }).RefreshProfile(ctx, ProfileRefreshInput{
		SessionID: "coding-1", ProblemID: "queue-1", SubmissionID: "exec-1",
	})
	if err == nil || !strings.Contains(err.Error(), "persistence unavailable") {
		t.Fatalf("error=%v", err)
	}
	processed, markerErr := service.IsCompletionProcessed(ctx, "coding-1", "queue-1", "exec-1")
	if markerErr != nil || processed {
		t.Fatalf("processed=%t err=%v", processed, markerErr)
	}
	if _, getErr := store.GetProfile(ctx, "personal-kevin", "kevin"); !errors.Is(getErr, memory.ErrProfileNotFound) {
		t.Fatalf("profile persisted after failure: %v", getErr)
	}
}

type lostCompletionResponseStore struct {
	*memory.InMemoryStore
	lost bool
}

func (s *lostCompletionResponseStore) ReplaceProfileAndMarkCompletionIfVersion(ctx context.Context, workspaceID, userID string, profile memory.Profile, expectedVersion int64, completion memory.ProcessedCompletion) error {
	err := s.InMemoryStore.ReplaceProfileAndMarkCompletionIfVersion(ctx, workspaceID, userID, profile, expectedVersion, completion)
	if err == nil && !s.lost {
		s.lost = true
		return errors.New("connection lost after commit")
	}
	return err
}

func TestCodingRetryAfterUncertainCommitDoesNotRegenerate(t *testing.T) {
	ctx := scopedContext()
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	store := &lostCompletionResponseStore{InMemoryStore: memory.NewInMemoryStore()}
	service := memory.NewService(store, func() time.Time { return now })
	seedCodingCompletion(t, ctx, service, memory.TypeAttemptSolved, "coding-1", "queue-1", "exec-1", true, "not_used")
	firstGenerator := &scriptedGenerator{payloads: codingProfilePayloads("committed profile")}
	_, err := NewProfileSynthesizer(NewOrchestrator(service, firstGenerator), service).WithClock(func() time.Time { return now }).RefreshProfile(ctx, ProfileRefreshInput{
		SessionID: "coding-1", ProblemID: "queue-1", SubmissionID: "exec-1",
	})
	if err == nil || !strings.Contains(err.Error(), "connection lost after commit") {
		t.Fatalf("error=%v", err)
	}
	replayGenerator := &scriptedGenerator{payloads: codingProfilePayloads("must not run")}
	result, err := NewProfileSynthesizer(NewOrchestrator(memory.NewService(store, nil), replayGenerator), memory.NewService(store, nil)).RefreshProfile(ctx, ProfileRefreshInput{
		SessionID: "coding-1", ProblemID: "queue-1", SubmissionID: "exec-1",
	})
	if err != nil || result.Changed || len(replayGenerator.requests) != 0 || result.Profile.Summary != "committed profile" {
		t.Fatalf("retry=%#v err=%v requests=%d", result, err, len(replayGenerator.requests))
	}
}

func (s *staleOnceProfileStore) ReplaceProfileIfVersion(ctx context.Context, workspaceID, userID string, profile memory.Profile, expectedVersion int64) error {
	s.replaceAttempts++
	if s.replaceAttempts == 1 {
		return memory.ErrStaleProfile
	}
	return s.InMemoryStore.ReplaceProfileIfVersion(ctx, workspaceID, userID, profile, expectedVersion)
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
	profilePayload := json.RawMessage(`{
		"summary":"Queue ordering needs reinforcement.","strengths":[],"growth_edges":["Queues"],
		"skills":[{"id":"queues","label":"Queues","area":"DSA","level":2,"confidence":50,"trend":"down"}],
		"notes":[]
	}`)
	generator := &scriptedGenerator{payloads: []json.RawMessage{profilePayload, profilePayload}}
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
