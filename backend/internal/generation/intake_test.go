package generation

import (
	"encoding/json"
	"testing"
)

func TestValidateIntakeQuestionsRejectsDuplicateDimensionsAndOptions(t *testing.T) {
	_, err := ValidateIntakeQuestions(json.RawMessage(`[
		{"id":"q1","dimension":"experience","text":"One?","options":[{"id":"a","label":"A"},{"id":"a","label":"B"}]},
		{"id":"q2","dimension":"experience","text":"Two?","options":[{"id":"a","label":"A"},{"id":"b","label":"B"}]}
	]`))
	if err == nil {
		t.Fatal("expected invalid intake output to be rejected")
	}
}

func TestValidateIntakeQuestionsAcceptsThreeTypedQuestions(t *testing.T) {
	questions, err := ValidateIntakeQuestions(json.RawMessage(`[
		{"id":"q1","dimension":"exposure","text":"One?","options":[{"id":"a","label":"A"},{"id":"b","label":"B"}]},
		{"id":"q2","dimension":"application","text":"Two?","options":[{"id":"a","label":"A"},{"id":"b","label":"B"}]},
		{"id":"q3","dimension":"challenge","text":"Three?","options":[{"id":"a","label":"A"},{"id":"b","label":"B"}]}
	]`))
	if err != nil {
		t.Fatalf("ValidateIntakeQuestions: %v", err)
	}
	if len(questions) != 3 {
		t.Fatalf("got %d questions", len(questions))
	}
}
