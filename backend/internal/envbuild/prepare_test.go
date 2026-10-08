package envbuild

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/kvn8888/codegym/backend/internal/agentruntime"
)

type fakeCompletionClient struct {
	calls   int
	respond func() (agentruntime.CompletionResponse, error)
}

func (f *fakeCompletionClient) Complete(context.Context, agentruntime.CompletionRequest) (agentruntime.CompletionResponse, error) {
	f.calls++
	return f.respond()
}

func doneClient() *fakeCompletionClient {
	return &fakeCompletionClient{
		respond: func() (agentruntime.CompletionResponse, error) {
			return agentruntime.CompletionResponse{
				Message: agentruntime.ChatMessage{Role: "assistant", Content: "DONE"},
				Usage:   agentruntime.TokenUsage{Total: 15, Input: 10, Output: 5},
				CostUSD: 2_000_000,
			}, nil
		},
	}
}

func TestPrepareCompletesFixture(t *testing.T) {
	client := &fakeCompletionClient{}
	client.respond = func() (agentruntime.CompletionResponse, error) {
		if client.calls == 1 {
			return agentruntime.CompletionResponse{
				Message: agentruntime.ChatMessage{
					Role: "assistant",
					ToolCalls: []agentruntime.ToolCall{{
						ID:   "call-1",
						Type: "function",
						Function: agentruntime.ToolFunction{
							Name:      "write_file",
							Arguments: `{"path":"app.txt","content":"hello"}`,
						},
					}},
				},
			}, nil
		}
		return agentruntime.CompletionResponse{
			Message: agentruntime.ChatMessage{Role: "assistant", Content: "DONE"},
			Usage:   agentruntime.TokenUsage{Total: 15, Input: 10, Output: 5},
			CostUSD: 2_000_000,
		}, nil
	}
	preparer := &Preparer{
		Runtime:     &agentruntime.PurposeBuiltRuntime{Client: client},
		Provisioner: HostProvisioner{},
	}
	result, err := preparer.Prepare(t.Context(), BuildRequest{Technology: "Widget Framework", Objective: "learn widgets"})
	if err != nil {
		t.Fatal(err)
	}
	if client.calls == 0 {
		t.Fatal("agent runtime was never called")
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
	if result.ProposedManifest == nil {
		t.Fatal("manifest was not drafted for a completed run")
	}
	found := false
	for _, file := range result.ProposedManifest.Workspace.LearnerEditable {
		if file == "app.txt" {
			found = true
		}
	}
	if !found {
		t.Fatalf("editable = %#v", result.ProposedManifest.Workspace.LearnerEditable)
	}
	if result.Telemetry.TokensIn != 10 || result.Telemetry.TokensOut != 5 || result.Telemetry.EstimatedCostUsd != 2 {
		t.Fatalf("telemetry = %#v", result.Telemetry)
	}
	if err := preparer.Provisioner.Destroy(t.Context(), result.Workspace); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareRefusesEmptyWorkspace(t *testing.T) {
	client := doneClient()
	preparer := &Preparer{
		Runtime:     &agentruntime.PurposeBuiltRuntime{Client: client},
		Provisioner: HostProvisioner{},
	}
	root := ""
	wrapper := provisionerFunc{
		provision: func(ctx context.Context, labels map[string]string) (Workspace, error) {
			workspace, err := HostProvisioner{}.Provision(ctx, labels)
			root = workspace.Root
			return workspace, err
		},
		destroy: HostProvisioner{}.Destroy,
	}
	preparer.Provisioner = wrapper
	result, err := preparer.Prepare(t.Context(), BuildRequest{Technology: "Widget Framework", Objective: "learn widgets"})
	if err == nil {
		t.Fatal("expected drafting error for an empty workspace")
	}
	if result.ProposedManifest != nil {
		t.Fatalf("manifest = %#v, want nil", result.ProposedManifest)
	}
	found := false
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == "NO_PROJECT_FILES" {
			found = true
		}
	}
	if !found {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
	if _, statErr := os.Stat(root); !os.IsNotExist(statErr) {
		t.Fatalf("workspace survived drafting failure: %q", root)
	}
}

func TestPrepareFailsFastOnMissingPrerequisite(t *testing.T) {
	t.Setenv("CODEGYM_MAVEN_REPO", t.TempDir())
	client := doneClient()
	preparer := &Preparer{
		Runtime:     &agentruntime.PurposeBuiltRuntime{Client: client},
		Provisioner: HostProvisioner{},
	}
	result, err := preparer.Prepare(t.Context(), BuildRequest{Technology: "Spring Boot", Objective: "practice dependency injection"})
	if err != nil {
		t.Fatal(err)
	}
	if client.calls != 0 {
		t.Fatalf("agent runtime called %d times on a fail-fast path", client.calls)
	}
	if result.Workspace.Root != "" {
		t.Fatalf("workspace provisioned on a fail-fast path: %#v", result.Workspace)
	}
	if result.ProposedManifest != nil {
		t.Fatalf("manifest drafted on a fail-fast path")
	}
	found := false
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == "PREREQUISITE_MISSING" {
			found = true
		}
	}
	if !found {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
}

func TestPrepareCleansUpOnAgentFailure(t *testing.T) {
	client := &fakeCompletionClient{
		respond: func() (agentruntime.CompletionResponse, error) {
			return agentruntime.CompletionResponse{}, errors.New("relay unavailable")
		},
	}
	preparer := &Preparer{
		Runtime:     &agentruntime.PurposeBuiltRuntime{Client: client},
		Provisioner: HostProvisioner{},
	}
	// Capture the workspace root via a provisioner wrapper to assert cleanup.
	root := ""
	wrapper := provisionerFunc{
		provision: func(ctx context.Context, labels map[string]string) (Workspace, error) {
			workspace, err := HostProvisioner{}.Provision(ctx, labels)
			root = workspace.Root
			return workspace, err
		},
		destroy: HostProvisioner{}.Destroy,
	}
	preparer.Provisioner = wrapper
	result, err := preparer.Prepare(t.Context(), BuildRequest{Technology: "Widget Framework", Objective: "learn widgets"})
	if err == nil {
		t.Fatal("expected agent failure error")
	}
	if len(result.Diagnostics) == 0 {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
	if _, statErr := os.Stat(root); !os.IsNotExist(statErr) {
		t.Fatalf("workspace survived agent failure: %q", root)
	}
}

type provisionerFunc struct {
	provision func(ctx context.Context, labels map[string]string) (Workspace, error)
	destroy   func(ctx context.Context, workspace Workspace) error
}

func (f provisionerFunc) Provision(ctx context.Context, labels map[string]string) (Workspace, error) {
	return f.provision(ctx, labels)
}

func (f provisionerFunc) Destroy(ctx context.Context, workspace Workspace) error {
	return f.destroy(ctx, workspace)
}
