package generation

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/memory"
	"github.com/kvn8888/codegym/backend/internal/workspace"
)

func validMCQJSON(count int) json.RawMessage {
	questions := make([]MCQQuestion, 0, count)
	for i := 0; i < count; i++ {
		correctIndex := i % 4
		questions = append(questions, MCQQuestion{
			ID:           fmt.Sprintf("mq%d", i+1),
			Type:         MCQSingleSelect,
			Text:         fmt.Sprintf("Question %d?", i+1),
			Options:      []string{"a", "b", "c", "d"},
			CorrectIndex: &correctIndex,
			Concept:      "Concept",
			HelpContent:  "Because reasons.",
		})
	}
	encoded, _ := json.Marshal(questions)
	return encoded
}

// scriptedGenerator returns queued payloads in order, recording requests.
type scriptedGenerator struct {
	payloads []json.RawMessage
	errs     []error
	requests []GenerateRequest
}

func (s *scriptedGenerator) Generate(_ context.Context, request GenerateRequest) (GenerateResult, error) {
	s.requests = append(s.requests, request)
	index := len(s.requests) - 1
	if index >= len(s.payloads) {
		index = len(s.payloads) - 1
	}
	if index < len(s.errs) && s.errs[index] != nil {
		return GenerateResult{}, s.errs[index]
	}
	return GenerateResult{
		Object:   s.payloads[index],
		Provider: "scripted",
		Model:    "fake",
	}, nil
}

func scopedContext() context.Context {
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{
		UserID:             "kevin",
		DefaultWorkspaceID: "personal-kevin",
		WorkspaceIDs:       []string{"personal-kevin"},
	})
	return workspace.WithScope(ctx, workspace.Scope{WorkspaceID: "personal-kevin"})
}

func newTestOrchestrator(generator Generator) *Orchestrator {
	store := memory.NewInMemoryStore()
	service := memory.NewService(store, func() time.Time {
		return time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	})
	return NewOrchestrator(service, generator)
}

func TestGenerateMCQSetHappyPath(t *testing.T) {
	generator := &scriptedGenerator{payloads: []json.RawMessage{validMCQJSON(3)}}
	orchestrator := newTestOrchestrator(generator)

	questions, result, err := GenerateMCQSet(scopedContext(), orchestrator, MCQSpec{Topic: "hash tables", Count: 3})
	if err != nil {
		t.Fatalf("GenerateMCQSet: %v", err)
	}
	if len(questions) != 3 {
		t.Fatalf("got %d questions, want 3", len(questions))
	}
	if result.Provider != "scripted" {
		t.Errorf("provider = %q", result.Provider)
	}
	if len(generator.requests) != 1 {
		t.Fatalf("generator called %d times, want 1", len(generator.requests))
	}

	request := generator.requests[0]
	if request.Kind != KindMCQ {
		t.Errorf("kind = %q", request.Kind)
	}
	if !strings.Contains(request.Instructions, "mixed-question marathon generator") {
		t.Error("instructions are missing the MCQ system prompt")
	}
	if request.Schema.Name != "mcq_set" {
		t.Errorf("schema name = %q", request.Schema.Name)
	}
}

func TestGenerateMCQSetRetriesOnceOnInvalidOutput(t *testing.T) {
	generator := &scriptedGenerator{payloads: []json.RawMessage{
		json.RawMessage(`[{"id":"mq1","text":"Q?","options":["a","b"],"correctIndex":0,"concept":"C","helpContent":"H"}]`),
		validMCQJSON(2),
	}}
	orchestrator := newTestOrchestrator(generator)

	questions, _, err := GenerateMCQSet(scopedContext(), orchestrator, MCQSpec{Count: 2})
	if err != nil {
		t.Fatalf("GenerateMCQSet: %v", err)
	}
	if len(questions) != 2 {
		t.Fatalf("got %d questions, want 2", len(questions))
	}
	if len(generator.requests) != 2 {
		t.Fatalf("generator called %d times, want 2", len(generator.requests))
	}
	if !strings.Contains(generator.requests[1].Instructions, "previous output was rejected") {
		t.Error("retry instructions do not carry the validation error back")
	}
}

func TestGenerateMCQSetFailsAfterRetryBudget(t *testing.T) {
	generator := &scriptedGenerator{payloads: []json.RawMessage{json.RawMessage(`{"nope":true}`)}}
	orchestrator := newTestOrchestrator(generator)

	_, _, err := GenerateMCQSet(scopedContext(), orchestrator, MCQSpec{Count: 2})
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if len(generator.requests) != 2 {
		t.Fatalf("generator called %d times, want 2 (initial + one retry)", len(generator.requests))
	}
	if DiagnosticClass(err) != "invalid_output" {
		t.Fatalf("DiagnosticClass = %q, want invalid_output; err=%v", DiagnosticClass(err), err)
	}
	if !strings.Contains(DiagnosticMessage(err), "raw_output=") {
		t.Fatalf("DiagnosticMessage missing raw output snapshot: %s", DiagnosticMessage(err))
	}
}

func TestGenerateMCQSetSurfacesProviderErrors(t *testing.T) {
	generator := &scriptedGenerator{
		payloads: []json.RawMessage{validMCQJSON(1)},
		errs: []error{&ProviderError{
			StatusCode: http.StatusBadRequest,
			Message:    "model not found",
		}},
	}
	orchestrator := newTestOrchestrator(generator)

	_, _, err := GenerateMCQSet(scopedContext(), orchestrator, MCQSpec{Count: 1})
	if err == nil {
		t.Fatal("expected provider error")
	}
	if DiagnosticClass(err) != "provider_error" {
		t.Fatalf("DiagnosticClass = %q, want provider_error; err=%v", DiagnosticClass(err), err)
	}
}

func TestGenerateMCQSetRejectsOutOfRangeCount(t *testing.T) {
	generator := &scriptedGenerator{payloads: []json.RawMessage{validMCQJSON(1)}}
	orchestrator := newTestOrchestrator(generator)

	if _, _, err := GenerateMCQSet(scopedContext(), orchestrator, MCQSpec{Count: 99}); err == nil {
		t.Fatal("expected error for count out of range")
	}
	if len(generator.requests) != 0 {
		t.Fatal("generator should not be called for invalid specs")
	}
}

func TestValidateMCQSet(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		count   int
		wantErr bool
	}{
		{"valid", string(validMCQJSON(2)), 2, false},
		{"multi select", `[{"id":"mq1","type":"multi_select","text":"Select all","options":["a","b","c","d"],"correctIndices":[0,2],"concept":"C","helpContent":"H"}]`, 1, false},
		{"free response", `[{"id":"mq1","type":"free_response","text":"Explain FIFO","expectedAnswer":"First in, first out","rubric":"Identifies removal order","concept":"Queues","helpContent":"Think about arrival order."}]`, 1, false},
		{"wrapped object", `{"questions":` + string(validMCQJSON(2)) + `}`, 2, false},
		{"schema items envelope", `{"type":"array","items":` + string(validMCQJSON(2)) + `}`, 2, false},
		{"single question object", `{"id":"mq1","text":"Q?","options":["a","b","c","d"],"correctIndex":0,"concept":"C","helpContent":"H"}`, 1, false},
		{"wrong count", string(validMCQJSON(3)), 2, true},
		{"bad correctIndex", `[{"id":"mq1","text":"Q?","options":["a","b","c","d"],"correctIndex":4,"concept":"C","helpContent":"H"}]`, 1, true},
		{"duplicate multi indices", `[{"id":"mq1","type":"multi_select","text":"Q?","options":["a","b","c","d"],"correctIndices":[1,1],"concept":"C","helpContent":"H"}]`, 1, true},
		{"free response missing rubric", `[{"id":"mq1","type":"free_response","text":"Q?","expectedAnswer":"A","concept":"C","helpContent":"H"}]`, 1, true},
		{"three options", `[{"id":"mq1","text":"Q?","options":["a","b","c"],"correctIndex":0,"concept":"C","helpContent":"H"}]`, 1, true},
		{"empty text", `[{"id":"mq1","text":" ","options":["a","b","c","d"],"correctIndex":0,"concept":"C","helpContent":"H"}]`, 1, true},
		{"empty help", `[{"id":"mq1","text":"Q?","options":["a","b","c","d"],"correctIndex":0,"concept":"C","helpContent":""}]`, 1, true},
		{"not json array", `"hello"`, 1, true},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := ValidateMCQSet(json.RawMessage(testCase.raw), testCase.count)
			if (err != nil) != testCase.wantErr {
				t.Errorf("err = %v, wantErr = %v", err, testCase.wantErr)
			}
		})
	}
}

func TestNormalizeMCQSpecQuestionTypes(t *testing.T) {
	defaulted, err := NormalizeMCQSpec(MCQSpec{Count: 2})
	if err != nil || len(defaulted.QuestionTypes) != 1 || defaulted.QuestionTypes[0] != MCQSingleSelect {
		t.Fatalf("defaulted spec = %#v, err=%v", defaulted, err)
	}
	mixed, err := NormalizeMCQSpec(MCQSpec{Count: 2, QuestionTypes: []MCQQuestionType{MCQMultiSelect, MCQFreeResponse, MCQMultiSelect}})
	if err != nil || len(mixed.QuestionTypes) != 2 {
		t.Fatalf("mixed spec = %#v, err=%v", mixed, err)
	}
	if _, err := NormalizeMCQSpec(MCQSpec{Count: 2, QuestionTypes: []MCQQuestionType{"essay"}}); err == nil {
		t.Fatal("expected unsupported type error")
	}
}

func TestValidateMCQSetFillsMissingIDs(t *testing.T) {
	raw := `[{"text":"Q?","options":["a","b","c","d"],"correctIndex":1,"concept":"C","helpContent":"H"}]`
	questions, err := ValidateMCQSet(json.RawMessage(raw), 1)
	if err != nil {
		t.Fatalf("ValidateMCQSet: %v", err)
	}
	if questions[0].ID != "mq1" {
		t.Errorf("id = %q, want mq1", questions[0].ID)
	}
}
