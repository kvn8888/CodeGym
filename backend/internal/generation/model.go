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
}
