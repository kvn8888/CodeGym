package envbuild

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/agentruntime"
)

func alwaysFailingVerifier() *scriptedVerifier {
	verifier := &scriptedVerifier{}
	verifier.respond = func(int, BuildResult) (Verification, error) {
		return repairableRejection(), nil
	}
	return verifier
}

func TestRepairTerminalExhaustionEvidence(t *testing.T) {
	client := &scriptedRepairClient{turns: []agentruntime.CompletionResponse{
		writeTurn(map[string]string{"app.txt": "broken"}),
		{Message: agentruntime.ChatMessage{Role: "assistant", Content: "DONE"}},
		writeTurn(map[string]string{"app.txt": "fixed"}),
		{Message: agentruntime.ChatMessage{Role: "assistant", Content: "DONE"}},
	}}
	root := ""
	wrapper := provisionerFunc{
		provision: func(ctx context.Context, labels map[string]string) (Workspace, error) {
			workspace, err := HostProvisioner{}.Provision(ctx, labels)
			root = workspace.Root
			return workspace, err
		},
		destroy: HostProvisioner{}.Destroy,
	}
	attempt := &RepairAttempt{
		Request:     BuildRequest{Technology: "Widget Framework", Objective: "learn widgets"},
		Runtime:     &agentruntime.PurposeBuiltRuntime{Client: client},
		Provisioner: wrapper,
	}
	loop := &RepairLoop{
		Budget: RepairBudget{MaxRepairs: 1, MaxWallTime: time.Hour, MaxTokens: 10_000_000},
		Teardown: func(ctx context.Context, result BuildResult) error {
			return wrapper.Destroy(ctx, result.Workspace)
		},
	}
	result, err := loop.Run(t.Context(), alwaysFailingVerifier(), attempt.Attempt)
	if err == nil {
		t.Fatal("expected budget exhaustion error")
	}
	if client.calls != 4 {
		t.Fatalf("model calls = %d, want 2 rounds of 2 turns", client.calls)
	}
	found := false
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == "REPAIR_BUDGET_EXHAUSTED" && diagnostic.Stage == "build" {
			found = true
		}
	}
	if !found {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
	if result.Telemetry.RepairIterations != 1 {
		t.Fatalf("repair iterations = %d, want 1", result.Telemetry.RepairIterations)
	}
	if _, statErr := os.Stat(root); !os.IsNotExist(statErr) {
		t.Fatalf("workspace survived terminal failure: %q", root)
	}
	if result.ProposedManifest == nil {
		t.Fatal("last round drafted no manifest; terminal evidence is incomplete")
	}
}

func TestRepairClaimNeverOverridesRejection(t *testing.T) {
	claimArgs, err := json.Marshal(map[string]string{
		"path":    ".codegym/agent-result.json",
		"content": `{"version":1,"completed":true,"summary":"fixed"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	client := &scriptedRepairClient{turns: []agentruntime.CompletionResponse{
		writeTurn(map[string]string{"app.txt": "broken"}),
		{Message: agentruntime.ChatMessage{Role: "assistant", Content: "DONE"}},
		{Message: agentruntime.ChatMessage{
			Role: "assistant",
			ToolCalls: []agentruntime.ToolCall{{
				ID:   "claim",
				Type: "function",
				Function: agentruntime.ToolFunction{
					Name:      "write_file",
					Arguments: string(claimArgs),
				},
			}},
		}},
		{Message: agentruntime.ChatMessage{Role: "assistant", Content: "fixed"}},
	}}
	attempt := &RepairAttempt{
		Request:     BuildRequest{Technology: "Widget Framework", Objective: "learn widgets"},
		Runtime:     &agentruntime.PurposeBuiltRuntime{Client: client},
		Provisioner: HostProvisioner{},
	}
	loop := &RepairLoop{Budget: RepairBudget{MaxRepairs: 1, MaxWallTime: time.Hour, MaxTokens: 10_000_000}}
	result, err := loop.Run(t.Context(), alwaysFailingVerifier(), attempt.Attempt)
	if err == nil {
		t.Fatal("expected rejection to survive the agent success claim")
	}
	claim, readErr := os.ReadFile(filepath.Join(result.Workspace.Root, ".codegym", "agent-result.json"))
	if readErr != nil {
		t.Fatalf("agent claim file missing: %v", readErr)
	}
	if !strings.Contains(string(claim), `"completed":true`) {
		t.Fatalf("test setup broken: agent claim not recorded: %s", claim)
	}
	if client.calls != 4 {
		t.Fatalf("model calls = %d, want the claim round fully attempted", client.calls)
	}
	if err := attempt.Provisioner.Destroy(t.Context(), result.Workspace); err != nil {
		t.Fatal(err)
	}
}
