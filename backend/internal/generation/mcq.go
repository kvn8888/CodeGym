package generation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
)

// MCQSpec is the caller-supplied request for an MCQ set. Topic may be empty,
// in which case the model spreads questions across the user's growth edges.
type MCQSpec struct {
	Topic string `json:"topic"`
	// Prompt is the user's free-text "what do you want to study?" ask. It is
	// inserted into the generation spec and steers concept selection together
	// with memory notes.
	Prompt     string `json:"prompt,omitempty"`
	Count      int    `json:"count"`
	Difficulty string `json:"difficulty,omitempty"`
	// Round distinguishes successive rounds of a continuous marathon so the
	// model can avoid repeating earlier questions verbatim.
	Round int `json:"round,omitempty"`
}

// MCQQuestion matches the frontend MarathonQuestion shape
// (frontend/src/features/marathon/MarathonPage.tsx) so generated sets drop
// straight into the marathon UI.
type MCQQuestion struct {
	ID           string   `json:"id"`
	Text         string   `json:"text"`
	Options      []string `json:"options"`
	CorrectIndex int      `json:"correctIndex"`
	Concept      string   `json:"concept"`
	HelpContent  string   `json:"helpContent"`
}

const (
	mcqMinCount     = 1
	mcqMaxCount     = 20
	mcqDefaultCount = 5
	mcqOptionCount  = 4
	mcqMaxTokens    = 4096
	// mcqMaxAttempts bounds the validate/repair loop: one generation plus one
	// retry that feeds the validation error back to the model.
	mcqMaxAttempts = 2
)

// mcqSystemPrompt is the kind-specific instruction block. It mirrors
// docs/ai-prompts/05-mcq-marathon.md; keep the two in sync when tuning.
const mcqSystemPrompt = `You are the MCQ marathon generator for CodeGym, an interview-practice tool. Produce a set of single-best-answer multiple-choice questions as ONE JSON array.

Each element:
{
  "id": "mq1",                 // "mq1".."mqN" in order
  "text": "the question",       // one concept, no trick wording
  "options": ["a","b","c","d"], // EXACTLY 4, plausible, mutually exclusive
  "correctIndex": 0,            // integer 0..3, the single correct option
  "concept": "Short Concept Label",
  "helpContent": "1-3 sentences explaining the concept so a learner who missed it understands why."
}

Rules:
- Generate exactly the requested count of questions. The spec's "prompt" field is the user's own ask ("what do you want to study?") — treat it as the primary topic directive when present; fall back to "topic", then to a spread across the user's growth edges.
- The personalization context includes memory NOTES — the user's living study journal. Notes with action "review" are known gaps: prioritize questions that probe those concepts. Notes with action "keep" are mastered techniques: avoid re-testing them unless the user's prompt asks for them.
- Exactly 4 options each; exactly one correct. Distractors must be plausible common misconceptions, not filler.
- Vary correctIndex across the set — do not always put the answer first.
- Calibrate difficulty to the user's level from the personalization context: bias toward growth edges, don't waste questions on demonstrated strengths.
- In later rounds (spec "round" > 1), do not repeat earlier questions verbatim — approach the same weak concepts from new angles.
- helpContent teaches the underlying idea; never just restate the answer.
- Keep each question standalone.`

// mcqJSONSchema is advisory documentation for providers with native
// structured-output support; validation below is the real gate.
var mcqJSONSchema = json.RawMessage(`{
  "type": "array",
  "items": {
    "type": "object",
    "required": ["id", "text", "options", "correctIndex", "concept", "helpContent"],
    "properties": {
      "id": {"type": "string"},
      "text": {"type": "string"},
      "options": {"type": "array", "items": {"type": "string"}, "minItems": 4, "maxItems": 4},
      "correctIndex": {"type": "integer", "minimum": 0, "maximum": 3},
      "concept": {"type": "string"},
      "helpContent": {"type": "string"}
    }
  }
}`)

// NormalizeMCQSpec applies defaults and bounds; it rejects nothing except an
// out-of-range explicit count so the endpoint stays forgiving.
func NormalizeMCQSpec(spec MCQSpec) (MCQSpec, error) {
	spec.Topic = strings.TrimSpace(spec.Topic)
	spec.Prompt = strings.TrimSpace(spec.Prompt)
	if len(spec.Prompt) > 500 {
		spec.Prompt = spec.Prompt[:500]
	}
	spec.Difficulty = strings.TrimSpace(strings.ToLower(spec.Difficulty))
	if spec.Round < 0 {
		spec.Round = 0
	}
	if spec.Count == 0 {
		spec.Count = mcqDefaultCount
	}
	if spec.Count < mcqMinCount || spec.Count > mcqMaxCount {
		return spec, fmt.Errorf("count must be between %d and %d", mcqMinCount, mcqMaxCount)
	}
	return spec, nil
}

// ValidateMCQSet parses and structurally validates a generated MCQ payload.
func ValidateMCQSet(raw json.RawMessage, expectedCount int) ([]MCQQuestion, error) {
	var questions []MCQQuestion
	if err := json.Unmarshal(raw, &questions); err != nil {
		// Tolerate models that wrap the array in {"questions": [...]}.
		var wrapped struct {
			Questions []MCQQuestion `json:"questions"`
		}
		if wrapErr := json.Unmarshal(raw, &wrapped); wrapErr != nil || len(wrapped.Questions) == 0 {
			return nil, fmt.Errorf("output is not a JSON array of questions: %w", err)
		}
		questions = wrapped.Questions
	}

	if len(questions) == 0 {
		return nil, errors.New("output contains no questions")
	}
	if expectedCount > 0 && len(questions) != expectedCount {
		return nil, fmt.Errorf("expected %d questions, got %d", expectedCount, len(questions))
	}

	for i := range questions {
		question := &questions[i]
		if strings.TrimSpace(question.ID) == "" {
			question.ID = fmt.Sprintf("mq%d", i+1)
		}
		if strings.TrimSpace(question.Text) == "" {
			return nil, fmt.Errorf("question %d has empty text", i+1)
		}
		if len(question.Options) != mcqOptionCount {
			return nil, fmt.Errorf("question %d has %d options, want exactly %d", i+1, len(question.Options), mcqOptionCount)
		}
		for j, option := range question.Options {
			if strings.TrimSpace(option) == "" {
				return nil, fmt.Errorf("question %d option %d is empty", i+1, j+1)
			}
		}
		if question.CorrectIndex < 0 || question.CorrectIndex >= mcqOptionCount {
			return nil, fmt.Errorf("question %d correctIndex %d is out of range 0..%d", i+1, question.CorrectIndex, mcqOptionCount-1)
		}
		if strings.TrimSpace(question.Concept) == "" {
			return nil, fmt.Errorf("question %d has empty concept", i+1)
		}
		if strings.TrimSpace(question.HelpContent) == "" {
			return nil, fmt.Errorf("question %d has empty helpContent", i+1)
		}
	}

	return questions, nil
}

// GenerateMCQSet runs the memory-aware orchestration for an MCQ set with a
// bounded validate/repair loop: an invalid payload gets one retry carrying the
// validation error back to the model; a second failure surfaces an error
// rather than a malformed set.
func GenerateMCQSet(ctx context.Context, orchestrator *Orchestrator, spec MCQSpec) ([]MCQQuestion, GenerateResult, error) {
	spec, err := NormalizeMCQSpec(spec)
	if err != nil {
		return nil, GenerateResult{}, err
	}

	specJSON, err := json.Marshal(spec)
	if err != nil {
		return nil, GenerateResult{}, fmt.Errorf("encode mcq spec: %w", err)
	}

	instructions := mcqSystemPrompt
	var lastErr error
	var lastRawOutput string
	for attempt := 1; attempt <= mcqMaxAttempts; attempt++ {
		result, err := orchestrator.Generate(ctx, GenerateInput{
			Kind: KindMCQ,
			Spec: specJSON,
			Schema: Schema{
				Name:       "mcq_set",
				Version:    "1",
				JSONSchema: mcqJSONSchema,
			},
			ModelPolicy:  ModelPolicy{MaxTokens: mcqMaxTokens},
			Instructions: instructions,
		})
		if err != nil {
			log.Printf("mcq generation attempt %d failed class=%s detail=%s", attempt, DiagnosticClass(err), DiagnosticMessage(err))
			var invalidErr *InvalidOutputError
			if errors.As(err, &invalidErr) {
				lastErr = err
				lastRawOutput = invalidErr.RawOutput
				if attempt == mcqMaxAttempts {
					break
				}
				instructions = mcqRepairInstructions(invalidOutputReason(err))
				continue
			}
			return nil, GenerateResult{}, err
		}

		questions, validateErr := ValidateMCQSet(result.Object, spec.Count)
		if validateErr == nil {
			return questions, result, nil
		}

		lastErr = validateErr
		lastRawOutput = string(result.Object)
		log.Printf("mcq generation attempt %d failed class=invalid_output detail=%s",
			attempt,
			DiagnosticMessage(&InvalidOutputError{
				Reason:    validateErr.Error(),
				RawOutput: lastRawOutput,
			}),
		)
		if attempt == mcqMaxAttempts {
			break
		}
		instructions = mcqRepairInstructions(validateErr.Error())
	}

	return nil, GenerateResult{}, &InvalidOutputError{
		Reason:    fmt.Sprintf("mcq generation produced invalid output after %d attempts", mcqMaxAttempts),
		RawOutput: lastRawOutput,
		Err:       lastErr,
	}
}

func mcqRepairInstructions(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "the output was not valid JSON matching the MCQ schema"
	}
	return mcqSystemPrompt + "\n\nYour previous output was rejected: " +
		reason + ". Regenerate the FULL set as a single valid JSON array. Do not include prose, markdown fences, or trailing commentary."
}

func invalidOutputReason(err error) string {
	var invalidErr *InvalidOutputError
	if errors.As(err, &invalidErr) {
		if strings.TrimSpace(invalidErr.Reason) != "" {
			return invalidErr.Reason
		}
		if invalidErr.Err != nil {
			return invalidErr.Err.Error()
		}
	}
	if err != nil {
		return err.Error()
	}
	return ""
}
