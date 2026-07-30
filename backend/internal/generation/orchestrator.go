package generation

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/kvn8888/codegym/backend/internal/memory"
	"github.com/kvn8888/codegym/backend/internal/usage"
	"github.com/kvn8888/codegym/backend/internal/workflow"
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

	profile, err := o.LoadProfile(ctx)
	if err != nil {
		return GenerateResult{}, err
	}
	return o.GenerateWithProfile(ctx, input, profile)
}

// LoadProfile resolves the authenticated workspace's persisted personalization
// context. Structured workflows call it once before bounded provider retries so
// progress remains ordered and every retry uses the same evidence snapshot.
func (o *Orchestrator) LoadProfile(ctx context.Context) (memory.Profile, error) {
	if o == nil || o.memory == nil {
		return memory.Profile{}, errors.New("generation orchestrator requires memory")
	}
	reportWorkflow(ctx, "load_context", workflow.StatusRunning, nil, false)
	profile, err := o.memory.GetProfile(ctx)
	if err != nil {
		reportWorkflow(ctx, "load_context", workflow.StatusFailed, map[string]any{
			"reason_code": "context_unavailable", "retryable": true,
		}, true)
		return memory.Profile{}, err
	}
	reportWorkflow(ctx, "load_context", workflow.StatusSucceeded, nil, false)
	return profile, nil
}

func reportWorkflow(
	ctx context.Context,
	stepID string,
	status workflow.Status,
	metadata map[string]any,
	terminal bool,
) {
	reporter := workflow.ReporterFromContext(ctx)
	if reporter == nil {
		return
	}
	if err := reporter.Report(ctx, stepID, status, metadata, terminal); err != nil && ctx.Err() != nil {
		_ = reporter.ReportDetached(stepID, status, metadata, terminal)
	}
}

// Stream delegates a conversational turn through the configured provider
// router and records usage at the same orchestration boundary as structured
// generation.
func (o *Orchestrator) Stream(
	ctx context.Context,
	request StreamRequest,
	emit func(StreamDelta) error,
) (StreamResult, error) {
	if o == nil || o.generator == nil {
		return StreamResult{}, errors.New("generation orchestrator requires a generator")
	}
	streamer, ok := o.generator.(Streamer)
	if !ok {
		return StreamResult{}, errors.New("configured generator does not support streaming")
	}
	result, err := streamer.Stream(ctx, request, emit)
	if err != nil {
		return StreamResult{}, err
	}
	if o.usage != nil && (result.TokensIn > 0 || result.TokensOut > 0) {
		o.usage.RecordBestEffort(ctx, usage.RecordInput{
			Provider: result.Provider, Model: result.Model, Kind: "chat",
			TokensIn: result.TokensIn, TokensOut: result.TokensOut,
		})
	}
	return result, nil
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
