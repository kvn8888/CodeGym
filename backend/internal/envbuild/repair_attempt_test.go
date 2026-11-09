package envbuild

import (
	"context"
	"strings"
	"testing"

	"github.com/kvn8888/codegym/backend/internal/agentruntime"
)

// scriptedRepairClient serves scripted model turns and records every request
// so tests can inspect what the repair round was told.
type scriptedRepairClient struct {
	calls    int
	requests []agentruntime.CompletionRequest
	turns    []agentruntime.CompletionResponse
}

func (s *scriptedRepairClient) Complete(_ context.Context, request agentruntime.CompletionRequest) (agentruntime.CompletionResponse, error) {
	s.calls++
	s.requests = append(s.requests, request)
	if s.calls > len(s.turns) {
		return agentruntime.CompletionResponse{
			Message: agentruntime.ChatMessage{Role: "assistant", Content: "DONE"},
		}, nil
	}
	return s.turns[s.calls-1], nil
}

func writeTurn(files map[string]string) agentruntime.CompletionResponse {
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	calls := make([]agentruntime.ToolCall, 0, len(paths))
	for i, path := range paths {
		calls = append(calls, agentruntime.ToolCall{
			ID:   string(rune('a' + i)),
			Type: "function",
			Function: agentruntime.ToolFunction{
				Name:      "write_file",
				Arguments: `{"path":"` + path + `","content":"` + files[path] + `"}`,
			},
		})
	}
	return agentruntime.CompletionResponse{
		Message: agentruntime.ChatMessage{Role: "assistant", ToolCalls: calls},
	}
}

func lastUserText(request agentruntime.CompletionRequest) string {
	for i := len(request.Messages) - 1; i >= 0; i-- {
		if request.Messages[i].Role == "user" {
			return request.Messages[i].Content
		}
	}
	return ""
}

func TestRepairAttemptReusesWorkspaceAndFeedsBack(t *testing.T) {
	client := &scriptedRepairClient{turns: []agentruntime.CompletionResponse{
		writeTurn(map[string]string{"app.txt": "broken"}),
		{Message: agentruntime.ChatMessage{Role: "assistant", Content: "DONE"}},
		writeTurn(map[string]string{"app.txt": "fixed"}),
		{Message: agentruntime.ChatMessage{Role: "assistant", Content: "DONE"}},
	}}
	attempt := &RepairAttempt{
		Request:     BuildRequest{Technology: "Widget Framework", Objective: "learn widgets"},
		Runtime:     &agentruntime.PurposeBuiltRuntime{Client: client},
		Provisioner: HostProvisioner{},
	}
	roundZero, err := attempt.Attempt(t.Context(), 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	roundOne, err := attempt.Attempt(t.Context(), 1, []Diagnostic{{
		Severity: "error",
		Stage:    "build",
		Code:     "DEPENDENCY_MISSING",
		Message:  "test dependency is missing",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if roundZero.Workspace.Root != roundOne.Workspace.Root {
		t.Fatalf("workspace changed across rounds: %q vs %q", roundZero.Workspace.Root, roundOne.Workspace.Root)
	}
	repairTold := false
	for _, request := range client.requests {
		if strings.Contains(lastUserText(request), "DEPENDENCY_MISSING") {
			repairTold = true
		}
	}
	if !repairTold {
		t.Fatal("round 2 instructions do not contain the round 1 diagnostic")
	}
	if roundOne.ProposedManifest == nil {
		t.Fatal("no manifest re-drafted after repair")
	}
	seen := map[string]bool{}
	for _, diagnostic := range roundOne.Diagnostics {
		seen[diagnostic.Code] = true
	}
	if !seen["DEPENDENCY_MISSING"] {
		t.Fatalf("round history lost the verifier diagnostic: %#v", roundOne.Diagnostics)
	}
	if err := attempt.Provisioner.Destroy(t.Context(), roundOne.Workspace); err != nil {
		t.Fatal(err)
	}
}
