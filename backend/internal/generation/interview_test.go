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

type interviewScript struct {
	outputs []json.RawMessage
	calls   int
}

func contextWithMemoryScope() context.Context {
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{
		UserID: "user-a", DefaultWorkspaceID: "workspace-a", WorkspaceIDs: []string{"workspace-a"},
	})
	return workspace.WithScope(ctx, workspace.Scope{WorkspaceID: "workspace-a"})
}

func (s *interviewScript) Generate(_ context.Context, _ GenerateRequest) (GenerateResult, error) {
	index := s.calls
	s.calls++
	if index >= len(s.outputs) {
		index = len(s.outputs) - 1
	}
	return GenerateResult{Object: s.outputs[index]}, nil
}

func TestAssessInterviewRepairsInvalidOutputAndKeepsServerOwnedMetrics(t *testing.T) {
	script := &interviewScript{outputs: []json.RawMessage{
		json.RawMessage(`{"strengths":["Clear"],"growth_edges":["Depth"],"topic":""}`),
		json.RawMessage(`{"strengths":["Clear","Clear"],"growth_edges":["Depth"],"topic":"Graph traversal"}`),
	}}
	service := memory.NewService(memory.NewInMemoryStore(), time.Now)
	orchestrator := NewOrchestrator(service, script)
	assessment, err := orchestrator.AssessInterview(contextWithMemoryScope(), InterviewAssessmentSpec{
		Mode: "coding", TurnCount: 4, DurationSeconds: 90,
		Messages: []InterviewTranscriptMessage{{Role: "user", Content: "raw answer"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if script.calls != 2 {
		t.Fatalf("calls = %d", script.calls)
	}
	if assessment.Mode != "coding" || assessment.TurnCount != 4 || assessment.DurationSeconds != 90 {
		t.Fatalf("server-owned fields changed: %#v", assessment)
	}
	if len(assessment.Strengths) != 1 || assessment.Topic != "Graph traversal" {
		t.Fatalf("assessment = %#v", assessment)
	}
}
