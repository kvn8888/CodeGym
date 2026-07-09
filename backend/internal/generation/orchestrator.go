package generation

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/kvn8888/codegym/backend/internal/memory"
)

type MemoryService interface {
	GetProfile(ctx context.Context) (memory.Profile, error)
}

// Orchestrator composes scoped memory with a provider-neutral Generator.
// Provider adapters stay behind Generator and must not fetch memory directly.
type Orchestrator struct {
	memory    MemoryService
	generator Generator
}

func NewOrchestrator(memoryService MemoryService, generator Generator) *Orchestrator {
	return &Orchestrator{memory: memoryService, generator: generator}
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

	return o.generator.Generate(ctx, GenerateRequest{
		Kind:          input.Kind,
		Spec:          input.Spec,
		MemoryContext: MemoryContextFromProfile(profile),
		Schema:        input.Schema,
		ModelPolicy:   input.ModelPolicy,
		Instructions:  input.Instructions,
	})
}

type GenerateInput struct {
	Kind         Kind
	Spec         json.RawMessage
	Schema       Schema
	ModelPolicy  ModelPolicy
	Instructions string
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
