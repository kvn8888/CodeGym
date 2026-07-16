package generation

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/memory"
	"github.com/kvn8888/codegym/backend/internal/workspace"
)

func TestOrchestratorInjectsMemoryContext(t *testing.T) {
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{
		UserID:             "kevin",
		DefaultWorkspaceID: "personal-kevin",
		WorkspaceIDs:       []string{"personal-kevin"},
	})
	ctx = workspace.WithScope(ctx, workspace.Scope{WorkspaceID: "personal-kevin"})

	store := memory.NewInMemoryStore()
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	err := store.UpsertProfile(ctx, "personal-kevin", "kevin", memory.Profile{
		Summary:      "Prefers Go and needs graph practice.",
		UpdatedAt:    now,
		NextReviewAt: now.Add(24 * time.Hour),
		Strengths:    []string{"Go syntax"},
		GrowthEdges:  []string{"DFS"},
		Skills: []memory.SkillProficiency{
			{Label: "Graphs", Area: "DSA", Level: 2, Confidence: 60, Trend: "up"},
		},
		Notes: []memory.Note{
			{ProblemID: "prob_graph", Title: "DFS", Summary: "Missed visited set", Tags: []string{"graphs"}, Action: "review"},
		},
	})
	if err != nil {
		t.Fatalf("seed profile: %v", err)
	}

	fake := &recordingGenerator{
		result: GenerateResult{
			Object:    json.RawMessage(`{"title":"Graph paths"}`),
			Provider:  "test",
			Model:     "fake",
			TokensIn:  10,
			TokensOut: 20,
		},
	}
	orchestrator := NewOrchestrator(memory.NewService(store, func() time.Time { return now }), fake)

	result, err := orchestrator.Generate(ctx, GenerateInput{
		Kind: KindProblem,
		Spec: json.RawMessage(`{"topic":"graphs"}`),
		Schema: Schema{
			Name:       "coding_problem",
			Version:    "v1",
			JSONSchema: json.RawMessage(`{"type":"object"}`),
		},
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if result.Provider != "test" {
		t.Fatalf("Provider = %q", result.Provider)
	}
	if fake.request.MemoryContext.Summary != "Prefers Go and needs graph practice." {
		t.Fatalf("memory summary = %q", fake.request.MemoryContext.Summary)
	}
	if len(fake.request.MemoryContext.Skills) != 1 || fake.request.MemoryContext.Skills[0].Label != "Graphs" {
		t.Fatalf("memory skills = %#v", fake.request.MemoryContext.Skills)
	}
	if len(fake.request.MemoryContext.Notes) != 1 || fake.request.MemoryContext.Notes[0].ProblemID != "prob_graph" {
		t.Fatalf("memory notes = %#v", fake.request.MemoryContext.Notes)
	}
	if fake.request.MemoryContext.Notes[0].Action != "review" {
		t.Fatalf("memory note action = %q", fake.request.MemoryContext.Notes[0].Action)
	}
}

type recordingGenerator struct {
	request GenerateRequest
	result  GenerateResult
}

func (g *recordingGenerator) Generate(_ context.Context, request GenerateRequest) (GenerateResult, error) {
	g.request = request
	return g.result, nil
}
