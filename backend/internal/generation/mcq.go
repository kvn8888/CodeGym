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

type MCQQuestionType string

const (
	MCQSingleSelect MCQQuestionType = "single_select"
	MCQMultiSelect  MCQQuestionType = "multi_select"
	MCQFreeResponse MCQQuestionType = "free_response"
)

// MCQQuestion matches the frontend MarathonQuestion shape
// (frontend/src/features/marathon/MarathonPage.tsx) so generated sets drop
// straight into the marathon UI.
type MCQQuestion struct {
	ID             string          `json:"id"`
	Type           MCQQuestionType `json:"type,omitempty"`
	Text           string          `json:"text"`
	Options        []string        `json:"options,omitempty"`
	CorrectIndex   *int            `json:"correctIndex,omitempty"`
	CorrectIndices []int           `json:"correctIndices,omitempty"`
	ExpectedAnswer string          `json:"expectedAnswer,omitempty"`
	Rubric         string          `json:"rubric,omitempty"`
	Concept        string          `json:"concept"`
	HelpContent    string          `json:"helpContent"`
}

const (
	mcqMinCount     = 1
	mcqMaxCount     = 20
	mcqDefaultCount = 5
	mcqOptionCount  = 4
	// mcqMaxAttempts bounds the validate/repair loop: one generation plus one
	// retry that feeds the validation error back to the model.
	mcqMaxAttempts = 2
)

// mcqSystemPrompt is the kind-specific instruction block. It mirrors
// docs/ai-prompts/05-mcq-marathon.md; keep the two in sync when tuning.
const mcqSystemPrompt = `You are the mixed-question marathon generator for CodeGym, an interview-practice tool. Produce ONE JSON array of questions. For each question, choose the type that best tests that specific concept: single_select, multi_select, or free_response.

Single select:
{
  "id": "mq1", "type": "single_select", "text": "the question",
  "options": ["a","b","c","d"], "correctIndex": 0,
  "concept": "Short Concept Label",
  "helpContent": "1-3 sentence explanation"
}

Multi select:
{
  "id": "mq2", "type": "multi_select", "text": "Select every correct statement.",
  "options": ["a","b","c","d"], "correctIndices": [0,2],
  "concept": "Short Concept Label", "helpContent": "1-3 sentence explanation"
}

Free response:
{
  "id": "mq3", "type": "free_response", "text": "Short-answer prompt",
  "expectedAnswer": "concise reference answer",
  "rubric": "objective criteria for a correct answer",
  "concept": "Short Concept Label", "helpContent": "A useful hint that does not reveal the answer"
}

Rules:
- Return a top-level JSON **array** of questions only. Do not wrap the array in an object, schema document, or {"type":"array","items":...} envelope.
- Generate exactly the requested count of questions. The spec's "prompt" field is the user's own ask ("what do you want to study?") — treat it as the primary topic directive when present; fall back to "topic", then to a spread across the user's growth edges.
- Choose each question's type independently based on pedagogical fit. Use single_select for one clearly best option, multi_select when recognizing a complete set matters, and free_response when the learner should explain or recall an idea without answer cues.
- Do not force an even quota or a particular mix. A set may use one type repeatedly when that is genuinely the best fit, but vary formats when the concepts support it.
- The personalization context includes memory NOTES — the user's living study journal. Notes with action "review" are known gaps: prioritize questions that probe those concepts. Notes with action "keep" are mastered techniques: avoid re-testing them unless the user's prompt asks for them.
- Single-select and multi-select items have exactly 4 plausible options. Single-select has exactly one correctIndex. Multi-select has 1-3 unique correctIndices and must require selecting the complete set.
- Free-response items have no options or correct indices. expectedAnswer and rubric must be concise and objective. helpContent must not reveal expectedAnswer.
- Vary correct option positions across the set.
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
    "required": ["id", "type", "text", "concept", "helpContent"],
    "properties": {
      "id": {"type": "string"},
      "type": {"type": "string", "enum": ["single_select", "multi_select", "free_response"]},
      "text": {"type": "string"},
      "options": {"type": "array", "items": {"type": "string"}, "maxItems": 4},
      "correctIndex": {"type": "integer", "minimum": 0, "maximum": 3},
      "correctIndices": {"type": "array", "items": {"type": "integer", "minimum": 0, "maximum": 3}, "minItems": 1, "maxItems": 3, "uniqueItems": true},
      "expectedAnswer": {"type": "string"},
      "rubric": {"type": "string"},
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
	questions, err := parseMCQQuestions(raw)
	if err != nil {
		return nil, err
	}

	if len(questions) == 0 {
		return nil, errors.New("output contains no questions")
	}
	if expectedCount > 0 && len(questions) != expectedCount {
		return nil, fmt.Errorf("expected %d questions, got %d", expectedCount, len(questions))
	}

	for i := range questions {
		question := &questions[i]
		if question.Type == "" {
			question.Type = MCQSingleSelect
		}
		if strings.TrimSpace(question.ID) == "" {
			question.ID = fmt.Sprintf("mq%d", i+1)
		}
		if strings.TrimSpace(question.Text) == "" {
			return nil, fmt.Errorf("question %d has empty text", i+1)
		}
		if err := validateMCQQuestionAnswer(i+1, question); err != nil {
			return nil, err
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

func validateMCQQuestionAnswer(position int, question *MCQQuestion) error {
	switch question.Type {
	case MCQSingleSelect, MCQMultiSelect:
		if len(question.Options) != mcqOptionCount {
			return fmt.Errorf("question %d has %d options, want exactly %d", position, len(question.Options), mcqOptionCount)
		}
		for index, option := range question.Options {
			if strings.TrimSpace(option) == "" {
				return fmt.Errorf("question %d option %d is empty", position, index+1)
			}
		}
	case MCQFreeResponse:
		if len(question.Options) != 0 || question.CorrectIndex != nil || len(question.CorrectIndices) != 0 {
			return fmt.Errorf("question %d free response must not contain options or correct indices", position)
		}
		if strings.TrimSpace(question.ExpectedAnswer) == "" || strings.TrimSpace(question.Rubric) == "" {
			return fmt.Errorf("question %d free response requires expectedAnswer and rubric", position)
		}
		return nil
	default:
		return fmt.Errorf("question %d has unsupported type %q", position, question.Type)
	}

	if question.Type == MCQSingleSelect {
		if question.CorrectIndex == nil || *question.CorrectIndex < 0 || *question.CorrectIndex >= mcqOptionCount {
			return fmt.Errorf("question %d correctIndex is required in range 0..%d", position, mcqOptionCount-1)
		}
		if len(question.CorrectIndices) != 0 {
			return fmt.Errorf("question %d single select must not contain correctIndices", position)
		}
		return nil
	}

	if question.CorrectIndex != nil || len(question.CorrectIndices) == 0 || len(question.CorrectIndices) >= mcqOptionCount {
		return fmt.Errorf("question %d multi select requires 1..%d correctIndices and no correctIndex", position, mcqOptionCount-1)
	}
	seen := map[int]bool{}
	for _, index := range question.CorrectIndices {
		if index < 0 || index >= mcqOptionCount || seen[index] {
			return fmt.Errorf("question %d has invalid or duplicate correctIndices", position)
		}
		seen[index] = true
	}
	return nil
}

func parseMCQQuestions(raw json.RawMessage) ([]MCQQuestion, error) {
	var questions []MCQQuestion
	if err := json.Unmarshal(raw, &questions); err == nil {
		return questions, nil
	} else {
		// Tolerate models that wrap the array in {"questions": [...]}.
		var wrapped struct {
			Questions []MCQQuestion `json:"questions"`
		}
		if wrapErr := json.Unmarshal(raw, &wrapped); wrapErr == nil && len(wrapped.Questions) > 0 {
			return wrapped.Questions, nil
		}
		// Muse Spark sometimes emits a JSON-Schema-shaped envelope:
		// {"type":"array","items":[...questions...]}.
		var schemaWrap struct {
			Type  string        `json:"type"`
			Items []MCQQuestion `json:"items"`
		}
		if wrapErr := json.Unmarshal(raw, &schemaWrap); wrapErr == nil &&
			strings.EqualFold(schemaWrap.Type, "array") && len(schemaWrap.Items) > 0 {
			return schemaWrap.Items, nil
		}
		// Single question object (common when providers force json_object).
		var single MCQQuestion
		if wrapErr := json.Unmarshal(raw, &single); wrapErr == nil &&
			strings.TrimSpace(single.Text) != "" && (len(single.Options) > 0 || strings.TrimSpace(single.ExpectedAnswer) != "") {
			return []MCQQuestion{single}, nil
		}
		return nil, fmt.Errorf("output is not a JSON array of questions: %w", err)
	}
}

// mcqDefaultMaxTokens leaves headroom for reasoning models (Meta Muse Spark)
// while remaining reasonable for non-reasoning providers.
const mcqDefaultMaxTokens = 4096

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
			ModelPolicy: ModelPolicy{
				MaxTokens: mcqDefaultMaxTokens,
			},
			Instructions: instructions,
		})
		if err != nil {
			log.Printf("mcq generation attempt %d failed class=%s detail=%s", attempt, DiagnosticClass(err), DiagnosticMessage(err))
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
		instructions = mcqSystemPrompt + "\n\nYour previous output was rejected: " +
			validateErr.Error() + ". Regenerate the FULL set, fixing that problem."
	}

	return nil, GenerateResult{}, &InvalidOutputError{
		Reason:    fmt.Sprintf("mcq generation produced invalid output after %d attempts", mcqMaxAttempts),
		RawOutput: lastRawOutput,
		Err:       lastErr,
	}
}
