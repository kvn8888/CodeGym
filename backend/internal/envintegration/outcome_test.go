package envintegration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/agentruntime"
	"github.com/kvn8888/codegym/backend/internal/envbuild"
	"github.com/kvn8888/codegym/backend/internal/environmentregistry"
	"github.com/kvn8888/codegym/backend/internal/environmentverify"
)

func roundFixtureTurns(buildCommand string) []agentruntime.CompletionResponse {
	return []agentruntime.CompletionResponse{
		writeFilesTurn(map[string]string{
			"solution.py":                    "VALUE = 1\n",
			"protected/test_demo.py":         "assert True\n",
			".codegym/hidden/test_hidden.py": "assert True\n",
			`.codegym/commands.json`:         `{"setup":"true","build":"` + buildCommand + `","test":"true","run":"true","base":"generic"}`,
		}),
		{Message: agentruntime.ChatMessage{Role: "assistant", Content: "DONE"}},
	}
}

func outcomeService(dirs *SnapshotDirectories) (*environmentregistry.Service, *fakeSnapshotProvider) {
	snapshots := &fakeSnapshotProvider{dirs: dirs}
	return &environmentregistry.Service{
		Store:          environmentregistry.NewInMemoryStore(),
		Snapshots:      snapshots,
		Builder:        nil,
		Capabilities:   &fakeCapabilities{allowed: map[string]bool{"demo": true}},
		Quota:          &fakeQuota{denied: map[string]environmentregistry.QuotaDenialReason{}},
		Verifier:       nil,
		PracticePolicy: environmentregistry.NetworkPolicyIsolated,
	}, snapshots
}

func TestOutcomeRepairedPathPromotes(t *testing.T) {
	turns := append(roundFixtureTurns("false"), roundFixtureTurns("true")...)
	client := &scriptedAgentClient{turns: turns}
	dirs := &SnapshotDirectories{}
	spec, assessor := demoAssessment(expectedDemoArtifactID())
	verifier := &LoopVerifier{
		Snapshots:   dirs,
		Runner:      &LocalCommandRunner{},
		RuntimeName: "purpose-built",
		Assessment:  spec,
		Assessor:    assessor,
	}
	attempt := &envbuild.RepairAttempt{
		Request:     envbuild.BuildRequest{Technology: "demo", Objective: "learn demo"},
		Runtime:     &agentruntime.PurposeBuiltRuntime{Client: client},
		Provisioner: envbuild.HostProvisioner{},
	}
	loop := &envbuild.RepairLoop{Budget: envbuild.DefaultRepairBudget()}
	result, err := loop.Run(t.Context(), verifier, attempt.Attempt)
	if err != nil {
		t.Fatal(err)
	}
	if client.calls != 4 {
		t.Fatalf("model calls = %d, want 2 rounds of 2 turns", client.calls)
	}
	seenBuildFailed := false
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == environmentverify.CodeBuildFailed {
			seenBuildFailed = true
		}
	}
	if !seenBuildFailed {
		t.Fatalf("round history lost the round 1 rejection: %#v", result.Diagnostics)
	}
	if result.ProposedManifest == nil || result.ProposedManifest.Commands.Build != "true" {
		t.Fatalf("final manifest = %#v", result.ProposedManifest)
	}
	raw, err := os.ReadFile(filepath.Join(result.Workspace.Root, ManifestFileName))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := environmentverify.ParseManifest(raw)
	if err != nil {
		t.Fatal(err)
	}
	out := environmentregistry.BuildOutput{
		Builder:     ToBuilderResult(result.BuilderRunID, "purpose-built", "", nil, ManifestFileName, result.Diagnostics, result.Telemetry),
		SnapshotRef: dirs.Register(result.Workspace.Root),
		Manifest:    parsed,
	}
	store := environmentregistry.NewInMemoryStore()
	service, _ := outcomeService(dirs)
	service.Store = store
	service.Verifier = &RegistryVerifier{
		Roots:      dirs.Resolve,
		Runner:     &LocalCommandRunner{},
		Assessment: spec,
		Assessor:   assessor,
	}
	verified, err := service.VerifyPreparation(t.Context(), out)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := service.Promote(t.Context(), environmentregistry.PromotionRequest{Verified: verified, Owner: "owner-1", BuiltBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.Allowed {
		t.Fatalf("promotion refused: %#v", receipt)
	}
	if err := (envbuild.HostProvisioner{}.Destroy(t.Context(), result.Workspace)); err != nil {
		t.Fatal(err)
	}
}

func TestOutcomeTimedOutStage(t *testing.T) {
	turns := []agentruntime.CompletionResponse{
		writeFilesTurn(map[string]string{
			"solution.py":                    "VALUE = 1\n",
			"protected/test_demo.py":         "assert True\n",
			".codegym/hidden/test_hidden.py": "assert True\n",
			`.codegym/commands.json`:         `{"setup":"true","build":"sleep 30","test":"true","run":"true","base":"generic"}`,
		}),
		{Message: agentruntime.ChatMessage{Role: "assistant", Content: "DONE"}},
	}
	client := &scriptedAgentClient{turns: turns}
	dirs := &SnapshotDirectories{}
	attempt := &envbuild.RepairAttempt{
		Request:     envbuild.BuildRequest{Technology: "demo", Objective: "learn demo"},
		Runtime:     &agentruntime.PurposeBuiltRuntime{Client: client},
		Provisioner: envbuild.HostProvisioner{},
	}
	round, err := attempt.Attempt(t.Context(), 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	verifier := &LoopVerifier{
		Snapshots:   dirs,
		Runner:      &LocalCommandRunner{},
		RuntimeName: "purpose-built",
		Timeouts:    environmentverify.StageTimeouts{Build: 3 * time.Second},
	}
	verification, err := verifier.Verify(t.Context(), round)
	if err != nil {
		t.Fatal(err)
	}
	if verification.Verdict != envbuild.VerdictRejected {
		t.Fatalf("verdict = %q, want rejected", verification.Verdict)
	}
	if verification.FailedStage != environmentverify.StageTimeout {
		t.Fatalf("failed stage = %q, want timeout", verification.FailedStage)
	}
	if err := attempt.Provisioner.Destroy(t.Context(), round.Workspace); err != nil {
		t.Fatal(err)
	}
}

func TestOutcomeOverBudgetDestroysWorkspace(t *testing.T) {
	turns := roundFixtureTurns("false")
	client := &scriptedAgentClient{turns: turns}
	dirs := &SnapshotDirectories{}
	verifier := &LoopVerifier{
		Snapshots:   dirs,
		Runner:      &LocalCommandRunner{},
		RuntimeName: "purpose-built",
	}
	attempt := &envbuild.RepairAttempt{
		Request:     envbuild.BuildRequest{Technology: "demo", Objective: "learn demo"},
		Runtime:     &agentruntime.PurposeBuiltRuntime{Client: client},
		Provisioner: envbuild.HostProvisioner{},
	}
	loop := &envbuild.RepairLoop{
		Budget: envbuild.RepairBudget{MaxRepairs: 0, MaxWallTime: time.Hour, MaxTokens: 10_000_000},
		Teardown: func(ctx context.Context, result envbuild.BuildResult) error {
			return envbuild.HostProvisioner{}.Destroy(ctx, result.Workspace)
		},
	}
	result, err := loop.Run(t.Context(), verifier, attempt.Attempt)
	if err == nil {
		t.Fatal("expected budget exhaustion error")
	}
	if client.calls != 2 {
		t.Fatalf("model calls = %d, want exactly 1 round", client.calls)
	}
	found := false
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == "REPAIR_BUDGET_EXHAUSTED" {
			found = true
		}
	}
	if !found {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
	if result.Telemetry.RepairIterations != 0 {
		t.Fatalf("repair iterations = %d, want 0", result.Telemetry.RepairIterations)
	}
	if _, statErr := os.Stat(result.Workspace.Root); !os.IsNotExist(statErr) {
		t.Fatalf("workspace survived terminal failure: %q", result.Workspace.Root)
	}
	store := environmentregistry.NewInMemoryStore()
	records, err := store.ListByTechnology(t.Context(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Fatalf("rejected artifact present in registry: %#v", records)
	}
}
