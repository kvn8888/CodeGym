package generation

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/kvn8888/codegym/backend/internal/memory"
	"github.com/kvn8888/codegym/backend/internal/usage"
)

type MemoryService interface {
	GetProfile(ctx context.Context) (memory.Profile, error)
}

// Orchestrator composes scoped memory with a provider-neutral Generator.
// Provider adapters stay behind Generator and must not fetch memory directly.
type Orchestrator struct {
	memory    MemoryService
	generator Generator
	usage     *usage.Service
}

func NewOrchestrator(memoryService MemoryService, generator Generator) *Orchestrator {
	return &Orchestrator{memory: memoryService, generator: generator}
}

// WithUsage attaches optional GenAI usage recording (tokens + estimated cost).
func (o *Orchestrator) WithUsage(usageService *usage.Service) *Orchestrator {
	if o != nil {
		o.usage = usageService
	}
	return o
}

func (o *Orchestrator) Generate(ctx context.Context, input GenerateInput) (GenerateResult, error) {
	if o == nil || o.generator == nil {
		return GenerateResult{}, errors.New("generation orchestrator requires a generator")
	}
	if o.memory == nil {
		return GenerateResult{}, errors.New("generation orchestrator requires memory")
	}

	profile, err := o.memory.GetProfile(ctx)
	if err != nil {
		return GenerateResult{}, err
	}
	return o.GenerateWithProfile(ctx, input, profile)
}

// GenerateWithProfile runs a provider call with an explicitly supplied memory
// context. Background profile synthesis uses this to avoid recursively reading
// the profile it is currently replacing.
func (o *Orchestrator) GenerateWithProfile(ctx context.Context, input GenerateInput, profile memory.Profile) (GenerateResult, error) {
	if o == nil || o.generator == nil {
		return GenerateResult{}, errors.New("generation orchestrator requires a generator")
	}

	memoryContext := MemoryContextFromProfile(profile)
	memoryContext.SelfReportedBaseline = input.IntakeContext
	result, err := o.generator.Generate(ctx, GenerateRequest{
		Kind:          input.Kind,
		Spec:          input.Spec,
		MemoryContext: memoryContext,
		Schema:        input.Schema,
		ModelPolicy:   input.ModelPolicy,
		Instructions:  input.Instructions,
	})
	if err != nil {
		return GenerateResult{}, err
	}

	if o.usage != nil && (result.TokensIn > 0 || result.TokensOut > 0) {
		o.usage.RecordBestEffort(ctx, usage.RecordInput{
			Provider:  result.Provider,
			Model:     result.Model,
			Kind:      string(input.Kind),
			TokensIn:  result.TokensIn,
			TokensOut: result.TokensOut,
		})
	}
	return result, nil
}

type GenerateInput struct {
	Kind          Kind
	Spec          json.RawMessage
	Schema        Schema
	ModelPolicy   ModelPolicy
	Instructions  string
	IntakeContext *PracticeIntakeContext
}

func MemoryContextFromProfile(profile memory.Profile) MemoryContext {
	skills := make([]MemorySkillContext, 0, len(profile.Skills))
	for _, skill := range profile.Skills {
		skills = append(skills, MemorySkillContext{
			Label:      skill.Label,
			Area:       skill.Area,
			Level:      skill.Level,
			Confidence: skill.Confidence,
			Trend:      skill.Trend,
		})
	}

	notes := make([]MemoryNoteContext, 0, len(profile.Notes))
	for _, note := range profile.Notes {
		notes = append(notes, MemoryNoteContext{
			ProblemID: note.ProblemID,
			Title:     note.Title,
			Summary:   note.Summary,
			Tags:      append([]string(nil), note.Tags...),
			Action:    note.Action,
		})
	}

	return MemoryContext{
		Summary:     profile.Summary,
		Strengths:   append([]string(nil), profile.Strengths...),
		GrowthEdges: append([]string(nil), profile.GrowthEdges...),
		Skills:      skills,
		Notes:       notes,
	}
}
