package environmentregistry

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/environmentverify"
)

// This file holds test-only fakes: an in-memory snapshot provider that
// records boots, commands, policies, and workspaces; scriptable builder,
// verifier, capability, and quota doubles. Fakes prove orchestration and
// gating — never production sandbox or network enforcement.

// loadGreetManifest reuses the checked-in #188 sample manifest.
func loadGreetManifest(t *testing.T) environmentverify.Manifest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "environmentverify", "testdata", "greet", "manifest.json"))
	if err != nil {
		t.Fatalf("read greet manifest: %v", err)
	}
	manifest, err := environmentverify.ValidateBytes(data)
	if err != nil {
		t.Fatalf("parse greet manifest: %v", err)
	}
	return manifest
}

func strptr(value string) *string { return &value }

// verifiedGreetResult builds a verified verifier result bound to the given
// builder run. It mirrors real #188 evidence shape: setup/build/test
// checks record the exact manifest commands executed, and promotion
// carries #188's standing registry-pending stance (not a denial). It is
// test-only review evidence: no human approval.
func verifiedGreetResult(t *testing.T, builderRunID string) environmentverify.VerifierResult {
	t.Helper()
	manifest := loadGreetManifest(t)
	return environmentverify.VerifierResult{
		SchemaVersion: environmentverify.VerifierResultSchemaVersion,
		VerifierRunID: "verify_greet_1",
		BuilderRunID:  builderRunID,
		Verdict:       environmentverify.VerdictVerified,
		FailedStage:   nil,
		Repairable:    false,
		Diagnostics:   []environmentverify.Diagnostic{},
		Evidence: environmentverify.Evidence{Checks: []environmentverify.EvidenceCheck{
			{Stage: "manifest", Summary: "Manifest passed."},
			{Stage: "validation", Summary: "Artifact passed."},
			{Stage: "setup", Command: strptr(manifest.Commands.Setup), Summary: "Setup passed."},
			{Stage: "build", Command: strptr(manifest.Commands.Build), Summary: "Build passed."},
			{Stage: "test", Command: strptr(manifest.Commands.Test), Summary: "Tests passed."},
		}},
		Promotion: environmentverify.Promotion{Allowed: false, Reason: pendingRegistryPromotionReason},
	}
}

type fakeBootCall struct {
	SnapshotRef string
	Policy      NetworkPolicy
}

type fakeCommandResult struct {
	result BootResult
	err    error
}

type fakeSnapshotProvider struct {
	mu               sync.Mutex
	boots            []fakeBootCall
	commands         []string
	results          map[string]fakeCommandResult
	bootErr          error
	closeErr         error
	closeDelay       time.Duration
	workspaces       []Workspace
	workspaceBoot    map[string]int
	workspaceErr     error
	files            map[string]map[string]string
	closes           int
	closeSawLiveCtx  []bool
	closeHadDeadline []bool
	// onRun fires inside Boot.Run to simulate provider-side timing,
	// e.g. cancelling the caller context mid-check.
	onRun func(ctx context.Context, command string)
	// onWorkspace fires inside Boot.NewWorkspace, e.g. to cancel the
	// caller context between checks and separation.
	onWorkspace func(ctx context.Context)
	// onClose fires inside Boot.Close, e.g. to cancel the caller context
	// while cancellation-independent cleanup runs.
	onClose func(ctx context.Context)
}

func (f *fakeSnapshotProvider) BootFresh(_ context.Context, snapshotRef string, policy NetworkPolicy) (Boot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	index := len(f.boots)
	f.boots = append(f.boots, fakeBootCall{SnapshotRef: snapshotRef, Policy: policy})
	if f.bootErr != nil {
		return nil, f.bootErr
	}
	return &fakeBoot{provider: f, ref: snapshotRef, index: index}, nil
}

func (f *fakeSnapshotProvider) bootCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.boots)
}

func (f *fakeSnapshotProvider) commandLog() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.commands...)
}

type fakeBoot struct {
	provider *fakeSnapshotProvider
	ref      string
	index    int
}

func (b *fakeBoot) Run(ctx context.Context, command string) (BootResult, error) {
	b.provider.mu.Lock()
	hook := b.provider.onRun
	b.provider.commands = append(b.provider.commands, command)
	canned, ok := b.provider.results[command]
	b.provider.mu.Unlock()
	// Fire outside the lock: the hook may cancel the caller context to
	// simulate a check that completes after its context died.
	if hook != nil {
		hook(ctx, command)
	}
	if ok {
		return canned.result, canned.err
	}
	return BootResult{ExitCode: 0}, nil
}

func (b *fakeBoot) NewWorkspace(ctx context.Context) (Workspace, error) {
	b.provider.mu.Lock()
	hook := b.provider.onWorkspace
	workspaceErr := b.provider.workspaceErr
	b.provider.mu.Unlock()
	// Fire outside the lock: the hook may cancel the caller context to
	// simulate separation completing after its context died.
	if hook != nil {
		hook(ctx)
	}
	if workspaceErr != nil {
		return Workspace{}, workspaceErr
	}
	b.provider.mu.Lock()
	defer b.provider.mu.Unlock()
	id := fmt.Sprintf("ws-%d", len(b.provider.workspaces)+1)
	workspace := Workspace{ID: id}
	b.provider.workspaces = append(b.provider.workspaces, workspace)
	if b.provider.workspaceBoot == nil {
		b.provider.workspaceBoot = map[string]int{}
	}
	b.provider.workspaceBoot[id] = b.index
	if b.provider.files == nil {
		b.provider.files = map[string]map[string]string{}
	}
	b.provider.files[id] = map[string]string{}
	return workspace, nil
}

func (b *fakeBoot) Close(ctx context.Context) error {
	b.provider.mu.Lock()
	delay := b.provider.closeDelay
	hook := b.provider.onClose
	b.provider.mu.Unlock()
	// Fire outside the lock: the hook may cancel the caller context while
	// cleanup — which must not depend on that context — still returns nil.
	if hook != nil {
		hook(ctx)
	}
	// Honor the cleanup context: a backend-owned timeout must bound
	// teardown even when the fake is slow.
	if delay > 0 {
		select {
		case <-ctx.Done():
			b.provider.mu.Lock()
			defer b.provider.mu.Unlock()
			b.provider.closeSawLiveCtx = append(b.provider.closeSawLiveCtx, false)
			_, hasDeadline := ctx.Deadline()
			b.provider.closeHadDeadline = append(b.provider.closeHadDeadline, hasDeadline)
			return fmt.Errorf("fake boot close: %w", ctx.Err())
		case <-time.After(delay):
		}
	}
	b.provider.mu.Lock()
	defer b.provider.mu.Unlock()
	// Record whether cleanup arrived live and bounded: the registry must
	// decouple teardown from caller cancellation and always bound it.
	b.provider.closeSawLiveCtx = append(b.provider.closeSawLiveCtx, ctx.Err() == nil)
	_, hasDeadline := ctx.Deadline()
	b.provider.closeHadDeadline = append(b.provider.closeHadDeadline, hasDeadline)
	if b.provider.closeErr != nil {
		return b.provider.closeErr
	}
	b.provider.closes++
	return nil
}

// writeWorkspaceFile simulates learner state inside one fake workspace.
func (f *fakeSnapshotProvider) writeWorkspaceFile(id, path, content string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.files == nil {
		f.files = map[string]map[string]string{}
	}
	if f.files[id] == nil {
		f.files[id] = map[string]string{}
	}
	f.files[id][path] = content
}

func (f *fakeSnapshotProvider) workspaceFiles(id string) map[string]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]string{}
	for path, content := range f.files[id] {
		out[path] = content
	}
	return out
}

type fakeCapabilities struct {
	supported map[string]bool
}

func (f *fakeCapabilities) SupportsTechnology(technology string) bool {
	return f.supported[technology]
}

type fakeQuota struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (f *fakeQuota) CheckBuild(_ context.Context, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.err
}

func (f *fakeQuota) checkCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type fakeBuilder struct {
	mu     sync.Mutex
	calls  []BuildRequest
	output BuildOutput
	err    error
	cancel context.CancelFunc
}

func (f *fakeBuilder) Build(_ context.Context, req BuildRequest) (BuildOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, req)
	if f.cancel != nil {
		f.cancel()
	}
	return f.output, f.err
}

func (f *fakeBuilder) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// requests returns a copy of the build requests the builder received.
func (f *fakeBuilder) requests() []BuildRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]BuildRequest(nil), f.calls...)
}

type fakeVerifier struct {
	mu     sync.Mutex
	calls  int
	result environmentverify.VerifierResult
	builds []BuildOutput
}

func (f *fakeVerifier) Verify(_ context.Context, build BuildOutput) environmentverify.VerifierResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.builds = append(f.builds, build)
	return f.result
}

// received returns a copy of the build handoffs the verifier received.
func (f *fakeVerifier) received() []BuildOutput {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]BuildOutput(nil), f.builds...)
}

func (f *fakeVerifier) verifyCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// greetBuildOutput wraps the checked-in sample manifest in a realistic
// builder-track handoff: real schema version, run identity, runtime, and
// proposed manifest path — never registry-fabricated metadata.
func greetBuildOutput(t *testing.T, builderRunID, snapshotRef string, manifest environmentverify.Manifest) BuildOutput {
	t.Helper()
	manifestPath := "manifest.json"
	return BuildOutput{
		Builder: environmentverify.BuilderResult{
			SchemaVersion:        environmentverify.BuilderResultSchemaVersion,
			BuilderRunID:         builderRunID,
			Runtime:              &environmentverify.RuntimeInfo{Name: "test-builder", Version: "0.0.0-test"},
			ProposedManifestPath: &manifestPath,
			Diagnostics:          []environmentverify.Diagnostic{},
		},
		SnapshotRef: snapshotRef,
		Manifest:    manifest,
	}
}

// mustSeal seals one builder handoff through the service's verifier and
// returns the backend-owned association, failing the test on refusal.
func mustSeal(t *testing.T, service *Service, build BuildOutput) VerifiedPreparation {
	t.Helper()
	sealed, err := service.VerifyPreparation(context.Background(), build)
	if err != nil {
		t.Fatalf("VerifyPreparation: %v", err)
	}
	return sealed
}

// sealGreet seals the checked-in sample handoff through a scripted greet
// verification result. It reuses the service's installed fake verifier
// when present (so resolve tests keep their handle) and installs one
// otherwise. Test-only evidence throughout.
func sealGreet(t *testing.T, service *Service, builderRunID, snapshotRef string) VerifiedPreparation {
	t.Helper()
	manifest := loadGreetManifest(t)
	build := greetBuildOutput(t, builderRunID, snapshotRef, manifest)
	fv, ok := service.Verifier.(*fakeVerifier)
	if !ok || fv == nil {
		fv = &fakeVerifier{}
		service.Verifier = fv
	}
	fv.result = verifiedGreetResult(t, builderRunID)
	return mustSeal(t, service, build)
}

// promoteGreet seals and promotes the checked-in sample through a service
// wired with passing fakes. Test-only evidence throughout.
func promoteGreet(t *testing.T, service *Service, owner string) PromotionReceipt {
	t.Helper()
	sealed := sealGreet(t, service, "buildrun_greet_1", "snap_greet_1")
	receipt, err := service.Promote(context.Background(), PromotionRequest{
		Verified: sealed,
		Owner:    owner,
		BuiltBy:  "test-loop",
	})
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if !receipt.Allowed {
		t.Fatalf("receipt allowed = false, want true")
	}
	return receipt
}

func requireRefusal(t *testing.T, err error, want string) *PromotionRefusal {
	t.Helper()
	if err == nil {
		t.Fatalf("err = nil, want refusal containing %q", want)
	}
	refusal, ok := err.(*PromotionRefusal)
	if !ok {
		t.Fatalf("err type = %T, want *PromotionRefusal", err)
	}
	if !strings.Contains(refusal.Reason, want) {
		t.Fatalf("refusal reason = %q, want it to contain %q", refusal.Reason, want)
	}
	return refusal
}
