package envintegration

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/agentruntime"
	"github.com/kvn8888/codegym/backend/internal/envbuild"
	"github.com/kvn8888/codegym/backend/internal/environmentregistry"
	"github.com/kvn8888/codegym/backend/internal/environmentverify"
)

type scriptedAgentClient struct {
	calls int
	turns []agentruntime.CompletionResponse
}

func (s *scriptedAgentClient) Complete(_ context.Context, _ agentruntime.CompletionRequest) (agentruntime.CompletionResponse, error) {
	s.calls++
	if s.calls > len(s.turns) {
		return agentruntime.CompletionResponse{
			Message: agentruntime.ChatMessage{Role: "assistant", Content: "DONE"},
		}, nil
	}
	return s.turns[s.calls-1], nil
}

func writeFilesTurn(files map[string]string) agentruntime.CompletionResponse {
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	calls := make([]agentruntime.ToolCall, 0, len(paths))
	for i, path := range paths {
		arguments, err := json.Marshal(map[string]string{"path": path, "content": files[path]})
		if err != nil {
			panic(err)
		}
		calls = append(calls, agentruntime.ToolCall{
			ID:   fmt.Sprintf("call-%d", i),
			Type: "function",
			Function: agentruntime.ToolFunction{
				Name:      "write_file",
				Arguments: string(arguments),
			},
		})
	}
	return agentruntime.CompletionResponse{
		Message: agentruntime.ChatMessage{Role: "assistant", ToolCalls: calls},
	}
}

func demoFixtureTurns() []agentruntime.CompletionResponse {
	return []agentruntime.CompletionResponse{
		writeFilesTurn(map[string]string{
			"solution.py":                    "VALUE = 1\n",
			"protected/test_demo.py":         "assert True\n",
			".codegym/hidden/test_hidden.py": "assert True\n",
			`.codegym/commands.json`:         `{"setup":"true","build":"true","test":"true","run":"true","base":"generic"}`,
		}),
		{Message: agentruntime.ChatMessage{Role: "assistant", Content: "DONE"}},
	}
}

type fakeBoot struct {
	root     string
	commands []string
}

func (b *fakeBoot) Run(ctx context.Context, command string) (environmentregistry.BootResult, error) {
	b.commands = append(b.commands, command)
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = b.root
	output, err := cmd.CombinedOutput()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			return environmentregistry.BootResult{ExitCode: exitError.ExitCode(), Output: string(output)}, nil
		}
		return environmentregistry.BootResult{}, err
	}
	return environmentregistry.BootResult{ExitCode: 0, Output: string(output)}, nil
}

func (b *fakeBoot) NewWorkspace(context.Context) (environmentregistry.Workspace, error) {
	return environmentregistry.Workspace{ID: "ws-test-1"}, nil
}

func (b *fakeBoot) Close(context.Context) error { return nil }

type fakeSnapshotProvider struct {
	dirs  *SnapshotDirectories
	boots []*fakeBoot
}

func (f *fakeSnapshotProvider) BootFresh(_ context.Context, reference string, _ environmentregistry.NetworkPolicy) (environmentregistry.Boot, error) {
	root, err := f.dirs.Resolve(reference)
	if err != nil {
		return nil, err
	}
	boot := &fakeBoot{root: root}
	f.boots = append(f.boots, boot)
	return boot, nil
}

type fakeCapabilities struct {
	allowed map[string]bool
}

func (f *fakeCapabilities) SupportsTechnology(technology string) bool {
	return f.allowed[technology]
}

type fakeQuota struct {
	denied map[string]environmentregistry.QuotaDenialReason
}

func (f *fakeQuota) CheckBuild(_ context.Context, owner string) error {
	if reason, ok := f.denied[owner]; ok {
		return &environmentregistry.QuotaDenial{Reason: reason, Owner: owner}
	}
	return nil
}

type fakeAssessor struct {
	failure environmentverify.ExpectedFailure
}

func (f *fakeAssessor) Assess(_ context.Context, work environmentverify.AssessmentWork) (environmentverify.AssessmentReport, error) {
	if work.Role == environmentverify.RoleCorrectSolution {
		return environmentverify.AssessmentReport{
			Tests: []environmentverify.TestOutcome{{TestID: f.failure.TestID, Passed: true}},
		}, nil
	}
	return environmentverify.AssessmentReport{
		Tests: []environmentverify.TestOutcome{{
			TestID:       f.failure.TestID,
			Passed:       false,
			FailureClass: environmentverify.ClassAssertion,
			Assertion: &environmentverify.ObservedAssertion{
				Code:     f.failure.Code,
				Expected: f.failure.Expected,
				Actual:   f.failure.Actual,
			},
		}},
	}, nil
}

type registryHarness struct {
	builder    *RegistryBuilder
	dirs       *SnapshotDirectories
	snapshots  *fakeSnapshotProvider
	quota      *fakeQuota
	buildCalls *int
}

func newRegistryHarness(client *scriptedAgentClient, quota *fakeQuota) (*environmentregistry.Service, *registryHarness) {
	dirs := &SnapshotDirectories{}
	snapshots := &fakeSnapshotProvider{dirs: dirs}
	preparer := &envbuild.Preparer{
		Runtime:     &agentruntime.PurposeBuiltRuntime{Client: client},
		Provisioner: envbuild.HostProvisioner{},
	}
	harness := &registryHarness{dirs: dirs, snapshots: snapshots, quota: quota, buildCalls: new(int)}
	builder := &RegistryBuilder{Preparer: preparer, RuntimeName: "purpose-built", Snapshots: dirs}
	harness.builder = builder
	service := &environmentregistry.Service{
		Store:          environmentregistry.NewInMemoryStore(),
		Snapshots:      snapshots,
		Builder:        countingBuilder{builder: builder, calls: harness.buildCalls},
		Capabilities:   &fakeCapabilities{allowed: map[string]bool{"demo": true}},
		Quota:          quota,
		Verifier:       nil,
		PracticePolicy: environmentregistry.NetworkPolicyIsolated,
	}
	return service, harness
}

type countingBuilder struct {
	builder *RegistryBuilder
	calls   *int
}

func (c countingBuilder) Build(ctx context.Context, req environmentregistry.BuildRequest) (environmentregistry.BuildOutput, error) {
	*c.calls++
	return c.builder.Build(ctx, req)
}

func demoAssessment(artifactID string) (*environmentverify.AssessmentSpec, *fakeAssessor) {
	failure := environmentverify.ExpectedFailure{
		TestID: "t1", Code: "WRONG_VALUE", Expected: "1", Actual: "2", Reason: "wrong solution returns 2",
	}
	spec := &environmentverify.AssessmentSpec{
		ArtifactID:        artifactID,
		ArtifactVersion:   "0.1.0",
		Objective:         "learn demo",
		CorrectSolution:   map[string]string{"solution.py": "VALUE = 1\n"},
		WrongSolution:     map[string]string{"solution.py": "VALUE = 2\n"},
		ExpectedFailure:   failure,
		ReviewedBy:        "test-review",
		ReviewedReference: "slice-2-fixture",
	}
	return spec, &fakeAssessor{failure: failure}
}

func expectedDemoArtifactID() string {
	return "demo-" + time.Now().UTC().Format("20060102")
}

func TestRegistryPromotesVerifiedBuild(t *testing.T) {
	client := &scriptedAgentClient{turns: demoFixtureTurns()}
	service, harness := newRegistryHarness(client, &fakeQuota{denied: map[string]environmentregistry.QuotaDenialReason{}})
	spec, assessor := demoAssessment(expectedDemoArtifactID())
	service.Verifier = &RegistryVerifier{
		Roots:      harness.dirs.Resolve,
		Runner:     &LocalCommandRunner{},
		Assessment: spec,
		Assessor:   assessor,
	}
	first, err := service.Resolve(t.Context(), environmentregistry.ResolveRequest{Technology: "demo", Objective: "learn demo", Owner: "owner-1"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != environmentregistry.ResolutionBuilt {
		t.Fatalf("status = %q, want built", first.Status)
	}
	if first.Record.ArtifactID == "" || first.Record.SnapshotRef == "" {
		t.Fatalf("record = %#v", first.Record)
	}
	if first.Record.Provenance.BuilderRunID == "" || first.Record.Provenance.VerifierRunID == "" {
		t.Fatalf("provenance = %#v", first.Record.Provenance)
	}
	if *harness.buildCalls != 1 {
		t.Fatalf("builder calls = %d, want exactly 1", *harness.buildCalls)
	}
	second, err := service.Resolve(t.Context(), environmentregistry.ResolveRequest{Technology: "demo", Objective: "learn demo", Owner: "owner-1"})
	if err != nil {
		t.Fatal(err)
	}
	if second.Status != environmentregistry.ResolutionHit {
		t.Fatalf("status = %q, want hit", second.Status)
	}
	if *harness.buildCalls != 1 {
		t.Fatalf("builder calls = %d, reuse must not rebuild", *harness.buildCalls)
	}
	if second.Record.ArtifactID != first.Record.ArtifactID || second.Workspace.ID == "" {
		t.Fatalf("reuse = %#v", second)
	}
	for _, boot := range harness.snapshots.boots {
		for _, command := range boot.commands {
			if strings.Contains(command, "install") || strings.Contains(command, "download") {
				t.Fatalf("reuse ran installation: %q", command)
			}
		}
	}
}

func TestRegistryRefusesUnverified(t *testing.T) {
	turns := demoFixtureTurns()
	turns[0] = writeFilesTurn(map[string]string{
		"solution.py":                    "VALUE = 1\n",
		"protected/test_demo.py":         "assert True\n",
		".codegym/hidden/test_hidden.py": "assert True\n",
		`.codegym/commands.json`:         `{"setup":"true","build":"false","test":"true","run":"true","base":"generic"}`,
	})
	client := &scriptedAgentClient{turns: turns}
	service, harness := newRegistryHarness(client, &fakeQuota{denied: map[string]environmentregistry.QuotaDenialReason{}})
	spec, assessor := demoAssessment(expectedDemoArtifactID())
	service.Verifier = &RegistryVerifier{
		Roots:      harness.dirs.Resolve,
		Runner:     &LocalCommandRunner{},
		Assessment: spec,
		Assessor:   assessor,
	}
	output, err := harness.builder.Build(t.Context(), environmentregistry.BuildRequest{Technology: "demo", Objective: "learn demo", Owner: "owner-1"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.VerifyPreparation(t.Context(), output)
	if err == nil {
		t.Fatal("expected verification of a failing build to refuse preparation")
	}
	stored, storeErr := service.Store.GetArtifact(t.Context(), output.Manifest.Artifact.ID, output.Manifest.Artifact.Version)
	if storeErr == nil {
		t.Fatalf("rejected artifact stored: %#v", stored)
	}
}

func TestRegistryQuotaRefusal(t *testing.T) {
	client := &scriptedAgentClient{turns: demoFixtureTurns()}
	service, harness := newRegistryHarness(client, &fakeQuota{denied: map[string]environmentregistry.QuotaDenialReason{"owner-9": environmentregistry.QuotaExhausted}})
	spec, assessor := demoAssessment(expectedDemoArtifactID())
	service.Verifier = &RegistryVerifier{
		Roots:      harness.dirs.Resolve,
		Runner:     &LocalCommandRunner{},
		Assessment: spec,
		Assessor:   assessor,
	}
	resolution, err := service.Resolve(t.Context(), environmentregistry.ResolveRequest{Technology: "demo", Objective: "learn demo", Owner: "owner-9"})
	if err != nil {
		t.Fatal(err)
	}
	if resolution.Status != environmentregistry.ResolutionQuotaExhausted {
		t.Fatalf("status = %q, want quota-exhausted", resolution.Status)
	}
	if *harness.buildCalls != 0 {
		t.Fatalf("builder calls = %d, quota refusal must build nothing", *harness.buildCalls)
	}
}
