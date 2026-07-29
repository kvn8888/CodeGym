package generation

import (
	"context"
	"encoding/json"
)

type Kind string

const (
	KindProblem   Kind = "problem"
	KindMCQ       Kind = "mcq"
	KindInterview Kind = "interview"
	// KindPracticeIntake is the internal typed baseline-question generation pass.
	// It is exposed through /practice-intakes rather than /generate.
	KindPracticeIntake Kind = "practice_intake"
	// KindMCQEvaluation is the internal AI grading pass for free-response items.
	KindMCQEvaluation Kind = "mcq_evaluation"
	// KindProfile is the internal full-profile memory synthesis pass.
	KindProfile Kind = "memory_profile"
	// KindNotes is the internal memory note-maintenance pass; it is not
	// exposed as a client-requestable kind on POST /api/v1/generate.
	KindNotes Kind = "notes"
)

// Generator is the provider-neutral seam for all model-backed generation.
// Provider adapters implement this interface; orchestration code depends on it.
type Generator interface {
	Generate(ctx context.Context, request GenerateRequest) (GenerateResult, error)
}

type GenerateRequest struct {
	Kind          Kind            `json:"kind"`
	Spec          json.RawMessage `json:"spec"`
	MemoryContext MemoryContext   `json:"memory_context"`
	Schema        Schema          `json:"schema"`
	ModelPolicy   ModelPolicy     `json:"model_policy"`
	// Instructions carries the kind-specific system prompt built by
	// orchestration. Provider adapters include it verbatim; they never invent
	// product prompt text themselves.
	Instructions string `json:"instructions,omitempty"`
}

type Schema struct {
	Name       string          `json:"name"`
	Version    string          `json:"version"`
	JSONSchema json.RawMessage `json:"json_schema"`
}

type ModelPolicy struct {
	PreferredProvider string   `json:"preferred_provider,omitempty"`
	PreferredModel    string   `json:"preferred_model,omitempty"`
	AllowedProviders  []string `json:"allowed_providers,omitempty"`
	MaxTokens         int      `json:"max_tokens,omitempty"`
	Temperature       *float64 `json:"temperature,omitempty"`
}

type GenerateResult struct {
	Object    json.RawMessage `json:"object"`
	Provider  string          `json:"provider"`
	Model     string          `json:"model"`
	TokensIn  int             `json:"tokens_in"`
	TokensOut int             `json:"tokens_out"`
	CostUnits int             `json:"cost_units"`
}

type MemoryContext struct {
	Summary     string               `json:"summary"`
	Strengths   []string             `json:"strengths"`
	GrowthEdges []string             `json:"growth_edges"`
	Skills      []MemorySkillContext `json:"skills"`
	Notes       []MemoryNoteContext  `json:"notes"`
	// SelfReportedBaseline is deliberately separate from demonstrated memory.
	// Generators must prefer the profile fields above when the two disagree.
	SelfReportedBaseline *PracticeIntakeContext `json:"self_reported_baseline,omitempty"`
}

type PracticeIntakeContext struct {
	IntakeID        string                 `json:"intake_id"`
	NormalizedTopic string                 `json:"normalized_topic"`
	PracticeSeed    string                 `json:"practice_seed"`
	Answers         []PracticeIntakeAnswer `json:"answers"`
}

type PracticeIntakeAnswer struct {
	Dimension   string `json:"dimension"`
	Question    string `json:"question"`
	OptionID    string `json:"option_id"`
	OptionLabel string `json:"option_label"`
}

type MemorySkillContext struct {
	Label      string `json:"label"`
	Area       string `json:"area"`
	Level      int    `json:"level"`
	Confidence int    `json:"confidence"`
	Trend      string `json:"trend"`
}

type MemoryNoteContext struct {
	ProblemID string   `json:"problem_id"`
	Title     string   `json:"title"`
	Summary   string   `json:"summary"`
	Tags      []string `json:"tags"`
	Action    string   `json:"action"`
}
