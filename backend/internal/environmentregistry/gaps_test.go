package environmentregistry

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/environmentverify"
)

// Regression tests for the review-round gaps: pinned versions through
// fallback, objective/tags output validation, fail-closed quota and owner
// validation, store mutation isolation, workspace-from-checked-boot
// lifetime, cancellation and cleanup handling, and receipt reconciliation.

// Gap 1: exact version pins survive builder fallback; mismatched output is
// rejected before promotion and before any workspace is issued.

func TestResolveMissPinnedVersionMismatch(t *testing.T) {
	snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}}
	service, builder, _, verifier := resolveService(snapshots)
	manifest := loadGreetManifest(t)
	manifest.Artifact.Version = "0.2.0"
	builder.output = greetBuildOutput(t, "buildrun_pin_1", "snap_pin_1", manifest)
	verifier.result = verifiedGreetResult(t, "buildrun_pin_1")
	resolution, err := service.Resolve(context.Background(), ResolveRequest{Technology: "python", Version: "0.1.0", Owner: "owner-1"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolution.Status != ResolutionOutputMismatch {
		t.Fatalf("status = %q, want output-mismatch", resolution.Status)
	}
	if !strings.Contains(resolution.Reason, `"0.2.0"`) || !strings.Contains(resolution.Reason, `"0.1.0"`) {
		t.Fatalf("reason = %q, want both versions named", resolution.Reason)
	}
	if resolution.Workspace.ID != "" {
		t.Fatalf("workspace issued for mismatched output")
	}
	if _, err := service.Store.GetArtifact(context.Background(), "greet-basics-2026-10-01", "0.2.0"); err != ErrArtifactNotFound {
		t.Fatalf("mismatched output was promoted")
	}
	// The pin survives the miss into the builder request itself.
	requests := builder.requests()
	if len(requests) != 1 {
		t.Fatalf("builder calls = %d, want 1", len(requests))
	}
	if requests[0].Version != "0.1.0" || requests[0].Technology != "python" {
		t.Fatalf("builder request = %+v, want the pinned version preserved", requests[0])
	}
}

// Gap 2: explicit objective compatibility on hits, and builder output
// validated against technology, objective, version, and tags.

func TestResolveHitObjectiveMatching(t *testing.T) {
	snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}}
	service, _, _, _ := resolveService(snapshots)
	promoteGreet(t, service, "owner-1")
	matched, err := service.Resolve(context.Background(), ResolveRequest{
		Technology: "python", Objective: "Practice string handling with functions", Owner: "owner-1",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if matched.Status != ResolutionHit {
		t.Fatalf("status = %q, want hit for the recorded objective", matched.Status)
	}
	mismatched, err := service.Resolve(context.Background(), ResolveRequest{
		Technology: "python", Objective: "Practice concurrency", Owner: "owner-1",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if mismatched.Status == ResolutionHit {
		t.Fatalf("status = hit for a different objective")
	}
	if mismatched.Workspace.ID != "" {
		t.Fatalf("workspace issued without an exact objective match")
	}
}

func TestResolveMissOutputValidation(t *testing.T) {
	objective := "Practice string handling with functions"
	cases := []struct {
		name    string
		mutate  func(manifest *environmentverify.Manifest)
		request ResolveRequest
		want    string
	}{
		{"technology", func(m *environmentverify.Manifest) { m.Artifact.Technology = "ruby" },
			ResolveRequest{Technology: "python", Owner: "owner-1"}, `"ruby"`},
		{"objective", func(m *environmentverify.Manifest) { m.Compatibility.RequestedObjective = "Practice concurrency" },
			ResolveRequest{Technology: "python", Objective: objective, Owner: "owner-1"}, "Practice concurrency"},
		{"tags", func(m *environmentverify.Manifest) {},
			ResolveRequest{Technology: "python", ReuseTags: []string{"strings"}, Owner: "owner-1"}, "reuse tags"},
		{"sections", func(m *environmentverify.Manifest) { m.Artifact = nil },
			ResolveRequest{Technology: "python", Owner: "owner-1"}, "required sections"},
	}
	for _, tc := range cases {
		snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}}
		service, builder, _, verifier := resolveService(snapshots)
		manifest := loadGreetManifest(t)
		tc.mutate(&manifest)
		builder.output = greetBuildOutput(t, "buildrun_out_1", "snap_out_1", manifest)
		verifier.result = verifiedGreetResult(t, "buildrun_out_1")
		resolution, err := service.Resolve(context.Background(), tc.request)
		if err != nil {
			t.Fatalf("%s: Resolve: %v", tc.name, err)
		}
		if resolution.Status != ResolutionOutputMismatch {
			t.Fatalf("%s: status = %q, want output-mismatch", tc.name, resolution.Status)
		}
		if !strings.Contains(resolution.Reason, tc.want) {
			t.Fatalf("%s: reason = %q, want it to name %q", tc.name, resolution.Reason, tc.want)
		}
		if resolution.Workspace.ID != "" {
			t.Fatalf("%s: workspace issued for mismatched output", tc.name)
		}
		if _, err := service.Store.GetArtifact(context.Background(), "greet-basics-2026-10-01", "0.1.0"); err != ErrArtifactNotFound {
			t.Fatalf("%s: mismatched output was promoted", tc.name)
		}
	}
}

// Gap 4: missing quota policy fails closed with zero builder calls; blank
// owners never reach the builder.

func TestResolveMissNilQuotaFailsClosed(t *testing.T) {
	snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}}
	builder := &fakeBuilder{}
	service := &Service{
		Store:        NewInMemoryStore(),
		Snapshots:    snapshots,
		Builder:      builder,
		Capabilities: &fakeCapabilities{supported: map[string]bool{"python": true}},
		Quota:        nil,
		Verifier:     &fakeVerifier{},
	}
	resolution, err := service.Resolve(context.Background(), ResolveRequest{Technology: "python", Owner: "owner-1"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolution.Status != ResolutionQuotaExhausted {
		t.Fatalf("status = %q, want quota-exhausted", resolution.Status)
	}
	if !strings.Contains(resolution.Reason, "No quota policy") {
		t.Fatalf("reason = %q, want the missing policy named", resolution.Reason)
	}
	if builder.callCount() != 0 {
		t.Fatalf("builder calls = %d, want 0", builder.callCount())
	}
}

func TestResolveMissBlankOwnerRejected(t *testing.T) {
	snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}}
	service, builder, _, _ := resolveService(snapshots)
	_, err := service.Resolve(context.Background(), ResolveRequest{Technology: "python", Owner: "  "})
	if err == nil {
		t.Fatalf("err = nil, want owner validation")
	}
	if !strings.Contains(err.Error(), "owner") {
		t.Fatalf("err = %q, want owner named", err.Error())
	}
	if builder.callCount() != 0 {
		t.Fatalf("builder calls = %d, want 0", builder.callCount())
	}
}

// Gap 5: store writes and reads never alias caller memory.

func TestStoreMutationIsolation(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore()
	record := ArtifactRecord{
		ArtifactID: "iso", ArtifactVersion: "1.0", Technology: "python",
		DependencyFiles: []string{"a.txt"}, Lockfiles: []string{"b.lock"},
		Compatibility: CompatibilityRules{Technology: "python", ReuseTags: []string{"x"}},
	}
	if err := store.SaveArtifact(ctx, record); err != nil {
		t.Fatalf("SaveArtifact: %v", err)
	}
	record.DependencyFiles[0] = "mutated"
	record.Compatibility.ReuseTags[0] = "mutated"
	got, err := store.GetArtifact(ctx, "iso", "1.0")
	if err != nil {
		t.Fatalf("GetArtifact: %v", err)
	}
	if got.DependencyFiles[0] != "a.txt" || got.Compatibility.ReuseTags[0] != "x" {
		t.Fatalf("stored record mutated through caller slices: %+v", got)
	}
	got.Lockfiles[0] = "mutated"
	got.Compatibility.ReuseTags = append(got.Compatibility.ReuseTags, "y")
	again, err := store.GetArtifact(ctx, "iso", "1.0")
	if err != nil {
		t.Fatalf("GetArtifact: %v", err)
	}
	if again.Lockfiles[0] != "b.lock" || len(again.Compatibility.ReuseTags) != 1 {
		t.Fatalf("stored record mutated through read result: %+v", again)
	}
	listed, err := store.ListByTechnology(ctx, "python")
	if err != nil {
		t.Fatalf("ListByTechnology: %v", err)
	}
	listed[0].DependencyFiles[0] = "mutated"
	third, err := store.GetArtifact(ctx, "iso", "1.0")
	if err != nil {
		t.Fatalf("GetArtifact: %v", err)
	}
	if third.DependencyFiles[0] != "a.txt" {
		t.Fatalf("stored record mutated through list result")
	}
}

// Gap 6: the learner workspace comes from the boot that passed checks and
// stays usable after that boot is closed.

func TestResolveHitWorkspaceFromCheckedBoot(t *testing.T) {
	snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}}
	service, _, _, _ := resolveService(snapshots)
	promoteGreet(t, service, "owner-1")
	if snapshots.bootCount() != 1 {
		t.Fatalf("boots = %d, want 1 after promotion", snapshots.bootCount())
	}
	resolution, err := service.Resolve(context.Background(), ResolveRequest{Technology: "python", Owner: "owner-1"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolution.Status != ResolutionHit {
		t.Fatalf("status = %q, want hit", resolution.Status)
	}
	// Boots: 0 = promotion checks, 1 = hit checks. The workspace must come
	// from boot 1 — the boot that passed the reuse checks.
	if snapshots.bootCount() != 2 {
		t.Fatalf("boots = %d, want exactly promotion + hit-check boots", snapshots.bootCount())
	}
	if got := snapshots.workspaceBoot[resolution.Workspace.ID]; got != 1 {
		t.Fatalf("workspace boot = %d, want the checked boot (1)", got)
	}
	if snapshots.closes != 2 {
		t.Fatalf("closes = %d, want both boots closed", snapshots.closes)
	}
	// Lifetime: the workspace outlives its boot; learner state still lands
	// after close.
	snapshots.writeWorkspaceFile(resolution.Workspace.ID, "solution.py", "learner work")
	if files := snapshots.workspaceFiles(resolution.Workspace.ID); files["solution.py"] != "learner work" {
		t.Fatalf("workspace unusable after boot close")
	}
}

// Gap 7: cancellation fails work loudly; cleanup failures are reported and
// never silently dropped.

func TestPromoteCancelledContext(t *testing.T) {
	snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}}
	service := promoteService(snapshots)
	sealed := sealGreet(t, service, "buildrun_greet_1", "snap_greet_1")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := service.Promote(ctx, PromotionRequest{
		Verified: sealed,
		Owner:    "owner-1",
		BuiltBy:  "test-loop",
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want cancellation", err)
	}
	if snapshots.bootCount() != 0 {
		t.Fatalf("boots = %d, want 0", snapshots.bootCount())
	}
	if _, err := service.Store.GetArtifact(context.Background(), "greet-basics-2026-10-01", "0.1.0"); err != ErrArtifactNotFound {
		t.Fatalf("record stored on a cancelled promotion")
	}
}

func TestResolveCancelledContext(t *testing.T) {
	snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}}
	service, _, _, _ := resolveService(snapshots)
	promoteGreet(t, service, "owner-1")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	resolution, err := service.Resolve(ctx, ResolveRequest{Technology: "python", Owner: "owner-1"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want cancellation", err)
	}
	if resolution.Workspace.ID != "" {
		t.Fatalf("workspace issued on a cancelled resolve")
	}
}

func TestResolveMissBuilderCancelsContext(t *testing.T) {
	snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}}
	service, builder, _, verifier := resolveService(snapshots)
	ctx, cancel := context.WithCancel(context.Background())
	builder.cancel = cancel
	manifest := loadGreetManifest(t)
	builder.output = greetBuildOutput(t, "buildrun_cancel_1", "snap_cancel_1", manifest)
	verifier.result = verifiedGreetResult(t, "buildrun_cancel_1")
	_, err := service.Resolve(ctx, ResolveRequest{Technology: "python", Owner: "owner-1"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want cancellation to supersede the build", err)
	}
	if _, err := service.Store.GetArtifact(context.Background(), "greet-basics-2026-10-01", "0.1.0"); err != ErrArtifactNotFound {
		t.Fatalf("cancelled build was promoted")
	}
}

func TestPromoteCloseFailureFailsClosed(t *testing.T) {
	snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}, closeErr: errors.New("delete failed")}
	service := promoteService(snapshots)
	sealed := sealGreet(t, service, "buildrun_greet_1", "snap_greet_1")
	_, err := service.Promote(context.Background(), PromotionRequest{
		Verified: sealed,
		Owner:    "owner-1",
		BuiltBy:  "test-loop",
	})
	if err == nil || !strings.Contains(err.Error(), "cleanup") {
		t.Fatalf("err = %v, want explicit cleanup failure", err)
	}
	if _, err := service.Store.GetArtifact(context.Background(), "greet-basics-2026-10-01", "0.1.0"); err != ErrArtifactNotFound {
		t.Fatalf("record stored despite cleanup failure")
	}
}

func TestResolveHitCloseFailureIssuesNothing(t *testing.T) {
	snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}}
	service, _, _, _ := resolveService(snapshots)
	promoteGreet(t, service, "owner-1")
	snapshots.closeErr = errors.New("delete failed")
	resolution, err := service.Resolve(context.Background(), ResolveRequest{Technology: "python", Owner: "owner-1"})
	if err == nil || !strings.Contains(err.Error(), "cleanup") {
		t.Fatalf("err = %v, want explicit cleanup failure", err)
	}
	if resolution.Workspace.ID != "" {
		t.Fatalf("workspace issued from a boot that failed cleanup")
	}
}

// Gap 2 (continued): reuse tags are applied on hits — every requested
// tag must be present on the record.

func TestResolveHitTagMatching(t *testing.T) {
	snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}}
	service, _, _, _ := resolveService(snapshots)
	promoteGreet(t, service, "owner-1")
	manifest := loadGreetManifest(t)
	manifest.Artifact.Version = "0.2.0"
	manifest.Compatibility.ReuseTags = []string{"strings", "functions"}
	service.Verifier = &fakeVerifier{result: verifiedGreetResult(t, "buildrun_greet_2")}
	sealed := mustSeal(t, service, greetBuildOutput(t, "buildrun_greet_2", "snap_greet_2", manifest))
	if _, err := service.Promote(context.Background(), PromotionRequest{
		Verified: sealed,
		Owner:    "owner-1",
		BuiltBy:  "test-loop",
	}); err != nil {
		t.Fatalf("Promote: %v", err)
	}
	hit, err := service.Resolve(context.Background(), ResolveRequest{
		Technology: "python", Version: "0.2.0", ReuseTags: []string{"strings"}, Owner: "owner-1",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if hit.Status != ResolutionHit || hit.Record.ArtifactVersion != "0.2.0" {
		t.Fatalf("resolution = %+v, want a hit on the tag-covered version", hit)
	}
	missing, err := service.Resolve(context.Background(), ResolveRequest{
		Technology: "python", Version: "0.2.0", ReuseTags: []string{"concurrency"}, Owner: "owner-1",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if missing.Status == ResolutionHit {
		t.Fatalf("status = hit for tags the record does not cover")
	}
	if missing.Workspace.ID != "" {
		t.Fatalf("workspace issued without tag coverage")
	}
}

// Builder/verifier seam: the verifier receives the builder track's real
// handoff untouched — real run identity, runtime, and manifest path —
// and provenance records the builder's own runtime name. No
// registry-fabricated metadata may flow to the verifier.

func TestResolveMissVerifierReceivesRealHandoff(t *testing.T) {
	snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}}
	service, builder, _, verifier := resolveService(snapshots)
	manifest := loadGreetManifest(t)
	builder.output = greetBuildOutput(t, "buildrun_hand_1", "snap_hand_1", manifest)
	verified := verifiedGreetResult(t, "buildrun_hand_1")
	verified.VerifierRunID = "verify_hand_1"
	verifier.result = verified
	resolution, err := service.Resolve(context.Background(), ResolveRequest{Technology: "python", Owner: "owner-1"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolution.Status != ResolutionBuilt {
		t.Fatalf("status = %q, want built", resolution.Status)
	}
	received := verifier.received()
	if len(received) != 1 {
		t.Fatalf("verifier calls = %d, want 1", len(received))
	}
	handoff := received[0]
	if handoff.Builder.BuilderRunID != "buildrun_hand_1" {
		t.Fatalf("verifier handoff run = %q, want the builder's own run ID", handoff.Builder.BuilderRunID)
	}
	if handoff.Builder.Runtime == nil || handoff.Builder.Runtime.Name != "test-builder" {
		t.Fatalf("verifier handoff runtime = %+v, want the builder's own runtime", handoff.Builder.Runtime)
	}
	if handoff.Builder.ProposedManifestPath == nil || *handoff.Builder.ProposedManifestPath == "" {
		t.Fatalf("verifier handoff has no manifest path; a production adapter could not use #188")
	}
	if handoff.SnapshotRef != "snap_hand_1" {
		t.Fatalf("verifier handoff snapshot = %q", handoff.SnapshotRef)
	}
	if resolution.Record.Provenance.BuiltBy != "test-builder" {
		t.Fatalf("provenance builtBy = %q, want the builder's own runtime name", resolution.Record.Provenance.BuiltBy)
	}
}

// Fresh output without builder run identity is rejected before
// verification, promotion, and workspace issuance alike.

func TestResolveMissOutputMissingRunID(t *testing.T) {
	snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}}
	service, builder, _, verifier := resolveService(snapshots)
	builder.output = BuildOutput{SnapshotRef: "snap_norun_1", Manifest: loadGreetManifest(t)}
	resolution, err := service.Resolve(context.Background(), ResolveRequest{Technology: "python", Owner: "owner-1"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolution.Status != ResolutionOutputMismatch {
		t.Fatalf("status = %q, want output-mismatch", resolution.Status)
	}
	if !strings.Contains(resolution.Reason, "builder run ID") {
		t.Fatalf("reason = %q, want the missing run identity named", resolution.Reason)
	}
	if verifier.verifyCount() != 0 {
		t.Fatalf("verifier calls = %d, want 0: run-less output never verifies", verifier.verifyCount())
	}
	if resolution.Workspace.ID != "" {
		t.Fatalf("workspace issued for run-less output")
	}
}

// A miss that cannot be verified must never invoke the builder.

func TestResolveMissNilVerifierFailsBeforeBuild(t *testing.T) {
	snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}}
	builder := &fakeBuilder{}
	service := &Service{
		Store:        NewInMemoryStore(),
		Snapshots:    snapshots,
		Builder:      builder,
		Capabilities: &fakeCapabilities{supported: map[string]bool{"python": true}},
		Quota:        &fakeQuota{},
		Verifier:     nil,
	}
	_, err := service.Resolve(context.Background(), ResolveRequest{Technology: "python", Owner: "owner-1"})
	if err == nil || !strings.Contains(err.Error(), "verifier is not configured") {
		t.Fatalf("err = %v, want explicit verifier configuration failure", err)
	}
	if builder.callCount() != 0 {
		t.Fatalf("builder calls = %d, want 0", builder.callCount())
	}
}

// Gap 7 (continued): success reported on a dead context never advances.
// The last check cancels the context, then reports exit 0; promotion
// must fail loudly, store nothing, and still run teardown on a cleanup
// context decoupled from the cancellation.

func TestPromoteCheckSuccessAfterCancelFailsClosed(t *testing.T) {
	snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}}
	ctx, cancel := context.WithCancel(context.Background())
	snapshots.onRun = func(_ context.Context, command string) {
		if command == "python3 protected/test_greet.py" {
			cancel()
		}
	}
	service := promoteService(snapshots)
	sealed := sealGreet(t, service, "buildrun_greet_1", "snap_greet_1")
	_, err := service.Promote(ctx, PromotionRequest{
		Verified: sealed,
		Owner:    "owner-1",
		BuiltBy:  "test-loop",
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want cancellation to supersede the reported success", err)
	}
	if _, err := service.Store.GetArtifact(context.Background(), "greet-basics-2026-10-01", "0.1.0"); err != ErrArtifactNotFound {
		t.Fatalf("record stored from checks that completed on a dead context")
	}
	if len(snapshots.closeSawLiveCtx) != 1 {
		t.Fatalf("cleanups = %d, want exactly 1 teardown", len(snapshots.closeSawLiveCtx))
	}
	if !snapshots.closeSawLiveCtx[0] {
		t.Fatalf("teardown ran on the cancelled context instead of a decoupled cleanup context")
	}
}

// A run-less handoff cannot be sealed: sealing refuses before the
// verifier runs, boots, or stores anything.

func TestSealRefusesEmptyRunBinding(t *testing.T) {
	snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}}
	service := promoteService(snapshots)
	manifest := loadGreetManifest(t)
	verifier := &fakeVerifier{result: verifiedGreetResult(t, "")}
	service.Verifier = verifier
	build := greetBuildOutput(t, "", "snap_greet_1", manifest)
	_, err := service.VerifyPreparation(context.Background(), build)
	requireRefusal(t, err, "no builder run ID")
	if verifier.verifyCount() != 0 {
		t.Fatalf("verifier ran for a run-less handoff")
	}
	if snapshots.bootCount() != 0 {
		t.Fatalf("boots = %d, want 0: run-less evidence never reaches boot checks", snapshots.bootCount())
	}
}

// An explicit backend denial cannot be overridden. The verdict here is
// verified and the evidence matches, but promotion is disallowed for a
// reason other than #188's standing registry-pending stance: promotion
// is refused terminally (the builder cannot repair a backend decision),
// with zero boots and nothing stored.

func TestPromoteRefusesExplicitDenial(t *testing.T) {
	snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}}
	service := promoteService(snapshots)
	denied := verifiedGreetResult(t, "buildrun_greet_1")
	denied.Promotion = environmentverify.Promotion{
		Allowed: false,
		Reason:  "Backend denies promotion: artifact quarantined pending manual review.",
	}
	service.Verifier = &fakeVerifier{result: denied}
	manifest := loadGreetManifest(t)
	build := greetBuildOutput(t, "buildrun_greet_1", "snap_greet_1", manifest)
	_, err := service.VerifyPreparation(context.Background(), build)
	refusal := requireRefusal(t, err, "explicitly denies")
	if refusal.Repairable {
		t.Fatalf("repairable = true, want false: a backend denial is not a builder-repairable signal")
	}
	if snapshots.bootCount() != 0 {
		t.Fatalf("boots = %d, want 0: denied promotion never reaches boot checks", snapshots.bootCount())
	}
	if _, err := service.Store.GetArtifact(context.Background(), "greet-basics-2026-10-01", "0.1.0"); err != ErrArtifactNotFound {
		t.Fatalf("record stored despite an explicit backend denial")
	}
}

// Boot cleanup arrives live and bounded even when the caller context has
// neither life nor deadline: teardown is decoupled from cancellation and
// capped by the backend-owned timeout.

func TestPromoteCleanupBoundedByBackendTimeout(t *testing.T) {
	snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}}
	service := promoteService(snapshots)
	promoteGreet(t, service, "owner-1")
	if len(snapshots.closeSawLiveCtx) != 1 || !snapshots.closeSawLiveCtx[0] {
		t.Fatalf("cleanup liveness = %v, want exactly one live teardown", snapshots.closeSawLiveCtx)
	}
	if len(snapshots.closeHadDeadline) != 1 || !snapshots.closeHadDeadline[0] {
		t.Fatalf("cleanup deadlines = %v, want exactly one backend-bounded teardown", snapshots.closeHadDeadline)
	}
}

// A slow teardown hits the backend-owned timeout and fails the operation
// loudly; nothing is stored or issued.

func TestPromoteCleanupTimeoutFailsClosed(t *testing.T) {
	snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}, closeDelay: 5 * time.Second}
	service := promoteService(snapshots)
	service.CleanupTimeout = 50 * time.Millisecond
	sealed := sealGreet(t, service, "buildrun_greet_1", "snap_greet_1")
	_, err := service.Promote(context.Background(), PromotionRequest{
		Verified: sealed,
		Owner:    "owner-1",
		BuiltBy:  "test-loop",
	})
	if err == nil || !strings.Contains(err.Error(), "cleanup") {
		t.Fatalf("err = %v, want an explicit cleanup timeout failure", err)
	}
	if _, err := service.Store.GetArtifact(context.Background(), "greet-basics-2026-10-01", "0.1.0"); err != ErrArtifactNotFound {
		t.Fatalf("record stored despite a cleanup timeout")
	}
}

// Failed workspace issuance still tears down the boot: the registry owns
// boot lifetime on every issuance-failure path, and no workspace is
// issued from a separation failure.

func TestResolveHitWorkspaceFailureClosesBoot(t *testing.T) {
	snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}}
	service, _, _, _ := resolveService(snapshots)
	promoteGreet(t, service, "owner-1")
	closesBefore := snapshots.closes
	snapshots.workspaceErr = errors.New("separation unavailable")
	resolution, err := service.Resolve(context.Background(), ResolveRequest{Technology: "python", Owner: "owner-1"})
	if err == nil || !strings.Contains(err.Error(), "workspace separation") {
		t.Fatalf("err = %v, want an explicit separation failure", err)
	}
	if resolution.Workspace.ID != "" {
		t.Fatalf("workspace issued from a failed separation")
	}
	if snapshots.closes != closesBefore+1 {
		t.Fatalf("closes = %d, want the failed-issuance boot torn down by its owner", snapshots.closes)
	}
}

// Cancelled workspace issuance issues nothing and still closes the boot
// on a live, bounded cleanup context: cancellation supersedes even a
// successful separation.

func TestResolveHitWorkspaceCancelledAfterSeparation(t *testing.T) {
	snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}}
	service, _, _, _ := resolveService(snapshots)
	promoteGreet(t, service, "owner-1")
	closesBefore := snapshots.closes
	ctx, cancel := context.WithCancel(context.Background())
	snapshots.onWorkspace = func(context.Context) { cancel() }
	resolution, err := service.Resolve(ctx, ResolveRequest{Technology: "python", Owner: "owner-1"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want cancellation to supersede the separation", err)
	}
	if resolution.Workspace.ID != "" {
		t.Fatalf("workspace issued on a dead context")
	}
	if snapshots.closes != closesBefore+1 {
		t.Fatalf("closes = %d, want the cancelled-issuance boot torn down by its owner", snapshots.closes)
	}
	last := len(snapshots.closeSawLiveCtx) - 1
	if !snapshots.closeSawLiveCtx[last] || !snapshots.closeHadDeadline[last] {
		t.Fatalf("cancelled-issuance teardown was not live and bounded: live=%v deadline=%v",
			snapshots.closeSawLiveCtx, snapshots.closeHadDeadline)
	}
}

// Cancellation inside Close — with cleanup itself returning nil — must
// not yield a workspace: the post-cleanup issuance recheck fails the
// resolve loudly while teardown stays live and bounded.
func TestResolveHitCancelledDuringCloseIssuesNothing(t *testing.T) {
	snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}}
	service, _, _, _ := resolveService(snapshots)
	promoteGreet(t, service, "owner-1")
	closesBefore := snapshots.closes
	ctx, cancel := context.WithCancel(context.Background())
	snapshots.onClose = func(context.Context) { cancel() }
	resolution, err := service.Resolve(ctx, ResolveRequest{Technology: "python", Owner: "owner-1"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want cancellation to supersede successful cleanup", err)
	}
	if resolution.Workspace.ID != "" {
		t.Fatalf("workspace issued after cancellation during cleanup")
	}
	if snapshots.closes != closesBefore+1 {
		t.Fatalf("closes = %d, want bounded cleanup to have run", snapshots.closes)
	}
	last := len(snapshots.closeSawLiveCtx) - 1
	if !snapshots.closeSawLiveCtx[last] || !snapshots.closeHadDeadline[last] {
		t.Fatalf("teardown was not live and bounded: live=%v deadline=%v",
			snapshots.closeSawLiveCtx, snapshots.closeHadDeadline)
	}
}

// Gap 8: the receipt reconciles with the shared promotion contract instead
// of silently bypassing the verifier's promotion field.

func TestPromoteReceiptReconcilesVerifierPromotion(t *testing.T) {
	snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}}
	service := promoteService(snapshots)
	receipt := promoteGreet(t, service, "owner-1")
	if receipt.VerifierPromotion.Allowed {
		t.Fatalf("verifier promotion echo = true, want the #188 disallowed stance preserved")
	}
	if receipt.VerifierPromotion.Reason != pendingRegistryPromotionReason {
		t.Fatalf("verifier promotion reason = %q, want the #188 registry-pending stance preserved verbatim", receipt.VerifierPromotion.Reason)
	}
	if !receipt.Allowed {
		t.Fatalf("receipt allowed = false, want registry-decided true")
	}
	if !strings.Contains(receipt.Reason, "registry-pending") || !strings.Contains(receipt.Reason, "denial") {
		t.Fatalf("reason = %q, want the pending-vs-denial reconciliation stated", receipt.Reason)
	}
}
