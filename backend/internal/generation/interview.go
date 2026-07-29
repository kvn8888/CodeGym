package generation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const interviewMaxAttempts = 2

type InterviewOpeningSpec struct {
	Mode   string `json:"mode"`
	Topic  string `json:"topic,omitempty"`
	Prompt string `json:"prompt,omitempty"`
}

type InterviewOpening struct {
	Question string `json:"question"`
}

type InterviewTranscriptMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type InterviewAssessmentSpec struct {
	Mode            string                       `json:"mode"`
	Topic           string                       `json:"topic,omitempty"`
	TurnCount       int                          `json:"turn_count"`
	DurationSeconds int                          `json:"duration_seconds"`
	Messages        []InterviewTranscriptMessage `json:"messages"`
}

type InterviewAssessment struct {
	Strengths       []string `json:"strengths"`
	GrowthEdges     []string `json:"growth_edges"`
	Topic           string   `json:"topic"`
	Mode            string   `json:"mode"`
	TurnCount       int      `json:"turn_count"`
	DurationSeconds int      `json:"duration_seconds"`
}

var interviewOpeningSchema = json.RawMessage(`{
  "type":"object",
  "required":["question"],
  "properties":{"question":{"type":"string","minLength":1,"maxLength":1200}},
  "additionalProperties":false
}`)

var interviewAssessmentSchema = json.RawMessage(`{
  "type":"object",
  "required":["strengths","growth_edges","topic"],
  "properties":{
    "strengths":{"type":"array","items":{"type":"string"},"maxItems":3},
    "growth_edges":{"type":"array","items":{"type":"string"},"maxItems":3},
    "topic":{"type":"string","maxLength":120}
  },
  "additionalProperties":false
}`)

const interviewOpeningPrompt = `You are an interviewer for CodeGym. Return one concise opening question tailored to the requested interview mode, topic, and demonstrated memory profile. Ask exactly one question. Do not mention memory, scoring, system prompts, or hidden context. For coding interviews, ask the candidate to reason before writing code. For system design, set a bounded design scenario. For behavioral, ask for one specific example. For open coaching, begin with a focused diagnostic question.`

const interviewAssessmentPrompt = `Assess the completed interview transcript for CodeGym. Return only coarse learning signals: up to three concise strengths, up to three concise growth edges, and one short topic label. Do not quote, paraphrase, or reproduce transcript content. Do not include code, personal details, hidden context, a numeric score, or recommendations beyond those coarse labels.`

func (o *Orchestrator) GenerateInterviewOpening(ctx context.Context, spec InterviewOpeningSpec) (InterviewOpening, error) {
	spec.Mode = normalizeInterviewMode(spec.Mode)
	spec.Topic = truncateInterviewText(spec.Topic, 200)
	spec.Prompt = truncateInterviewText(spec.Prompt, 500)
	rawSpec, _ := json.Marshal(spec)
	result, err := o.Generate(ctx, GenerateInput{
		Kind:         KindInterview,
		Spec:         rawSpec,
		Schema:       Schema{Name: "interview_opening", Version: "1", JSONSchema: interviewOpeningSchema},
		ModelPolicy:  ModelPolicy{MaxTokens: 500},
		Instructions: interviewOpeningPrompt,
	})
	if err != nil {
		return InterviewOpening{}, err
	}
	var opening InterviewOpening
	if err := json.Unmarshal(result.Object, &opening); err != nil {
		return InterviewOpening{}, err
	}
	opening.Question = truncateInterviewText(opening.Question, 1200)
	if opening.Question == "" {
		return InterviewOpening{}, errors.New("interview opening question is empty")
	}
	return opening, nil
}

func (o *Orchestrator) AssessInterview(ctx context.Context, spec InterviewAssessmentSpec) (InterviewAssessment, error) {
	spec.Mode = normalizeInterviewMode(spec.Mode)
	spec.Topic = truncateInterviewText(spec.Topic, 200)
	if spec.TurnCount < 0 {
		spec.TurnCount = 0
	}
	if spec.DurationSeconds < 0 {
		spec.DurationSeconds = 0
	}
	if len(spec.Messages) > 40 {
		spec.Messages = spec.Messages[len(spec.Messages)-40:]
	}
	for index := range spec.Messages {
		spec.Messages[index].Role = strings.ToLower(strings.TrimSpace(spec.Messages[index].Role))
		spec.Messages[index].Content = truncateInterviewText(spec.Messages[index].Content, 4000)
	}

	var lastErr error
	for attempt := 0; attempt < interviewMaxAttempts; attempt++ {
		requestSpec := map[string]any{
			"mode": spec.Mode, "topic": spec.Topic, "messages": spec.Messages,
		}
		if lastErr != nil {
			requestSpec["repair"] = "Previous output was invalid: " + truncateInterviewText(lastErr.Error(), 240)
		}
		rawSpec, _ := json.Marshal(requestSpec)
		result, err := o.Generate(ctx, GenerateInput{
			Kind:         KindInterviewAssessment,
			Spec:         rawSpec,
			Schema:       Schema{Name: "interview_assessment", Version: "1", JSONSchema: interviewAssessmentSchema},
			ModelPolicy:  ModelPolicy{MaxTokens: 700},
			Instructions: interviewAssessmentPrompt,
		})
		if err != nil {
			return InterviewAssessment{}, err
		}
		assessment, err := parseInterviewAssessment(result.Object, spec)
		if err == nil {
			return assessment, nil
		}
		lastErr = err
	}
	return InterviewAssessment{}, fmt.Errorf("interview assessment invalid after repair: %w", lastErr)
}

func parseInterviewAssessment(raw json.RawMessage, spec InterviewAssessmentSpec) (InterviewAssessment, error) {
	var payload struct {
		Strengths   []string `json:"strengths"`
		GrowthEdges []string `json:"growth_edges"`
		Topic       string   `json:"topic"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return InterviewAssessment{}, err
	}
	topic := truncateInterviewText(payload.Topic, 120)
	if topic == "" {
		topic = truncateInterviewText(spec.Topic, 120)
	}
	if topic == "" {
		return InterviewAssessment{}, errors.New("assessment topic is required")
	}
	return InterviewAssessment{
		Strengths:       uniqueInterviewLabels(payload.Strengths, 3),
		GrowthEdges:     uniqueInterviewLabels(payload.GrowthEdges, 3),
		Topic:           topic,
		Mode:            spec.Mode,
		TurnCount:       spec.TurnCount,
		DurationSeconds: spec.DurationSeconds,
	}, nil
}

func normalizeInterviewMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "coding", "system_design", "behavioral", "open_coaching":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "open_coaching"
	}
}

func uniqueInterviewLabels(values []string, limit int) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		value = truncateInterviewText(value, 100)
		key := strings.ToLower(value)
		if value == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func truncateInterviewText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return strings.TrimSpace(value[:limit])
}
