package generation

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEvaluateFreeResponse(t *testing.T) {
	generator := &scriptedGenerator{payloads: []json.RawMessage{json.RawMessage(`{"correct":true,"feedback":"The answer identifies FIFO ordering."}`)}}
	evaluation, err := EvaluateFreeResponse(scopedContext(), newTestOrchestrator(generator), FreeResponseEvaluationInput{
		QuestionID: "mq1", Question: "How does a queue remove items?", Concept: "Queues",
		ExpectedAnswer: "First in, first out.", Rubric: "Must identify FIFO ordering.", Answer: "It removes the oldest inserted item first.",
	})
	if err != nil {
		t.Fatalf("EvaluateFreeResponse: %v", err)
	}
	if !evaluation.Correct || evaluation.Provider != "scripted" || !strings.Contains(evaluation.Feedback, "FIFO") {
		t.Fatalf("evaluation = %#v", evaluation)
	}
	if len(generator.requests) != 1 || generator.requests[0].Kind != KindMCQEvaluation || generator.requests[0].Schema.Name != "mcq_free_response_evaluation" {
		t.Fatalf("request = %#v", generator.requests)
	}
}

func TestEvaluateFreeResponseRejectsInvalidInputAndOutput(t *testing.T) {
	generator := &scriptedGenerator{payloads: []json.RawMessage{json.RawMessage(`{"correct":false,"feedback":""}`)}}
	orchestrator := newTestOrchestrator(generator)
	if _, err := EvaluateFreeResponse(scopedContext(), orchestrator, FreeResponseEvaluationInput{}); err == nil {
		t.Fatal("expected input validation error")
	}
	if len(generator.requests) != 0 {
		t.Fatal("invalid input should not call the provider")
	}
	_, err := EvaluateFreeResponse(scopedContext(), orchestrator, FreeResponseEvaluationInput{
		Question: "Q", ExpectedAnswer: "A", Rubric: "R", Answer: "response",
	})
	if err == nil || DiagnosticClass(err) != "invalid_output" {
		t.Fatalf("err = %v", err)
	}
}
