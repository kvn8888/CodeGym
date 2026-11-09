package environmentregistry

import (
	"context"
	"strings"
	"testing"
)

// Acceptance criterion 3 (reuse half): compatible hits re-run boot checks
// under the practice policy; check failure marks the record and refuses
// to offer the artifact.

func resolveService(snapshots *fakeSnapshotProvider) (*Service, *fakeBuilder, *fakeQuota, *fakeVerifier) {
	builder := &fakeBuilder{}
	quota := &fakeQuota{}
	verifier := &fakeVerifier{}
	service := &Service{
		Store:        NewInMemoryStore(),
		Snapshots:    snapshots,
		Builder:      builder,
		Capabilities: &fakeCapabilities{supported: map[string]bool{"python": true}},
		Quota:        quota,
		Verifier:     verifier,
	}
	return service, builder, quota, verifier
}

func TestResolveHitBootCheckFailureMarksRecord(t *testing.T) {
	snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}}
	service, builder, _, _ := resolveService(snapshots)
	promoteGreet(t, service, "owner-1")
	manifest := loadGreetManifest(t)
	snapshots.results[manifest.Commands.Build] = fakeCommandResult{result: BootResult{ExitCode: 1, Output: "build broke"}}
	resolution, err := service.Resolve(context.Background(), ResolveRequest{Technology: "python", Owner: "owner-1"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolution.Status != ResolutionBootCheckFailed {
		t.Fatalf("status = %q, want boot-check-failed", resolution.Status)
	}
	if resolution.Workspace.ID != "" {
		t.Fatalf("workspace issued despite failed checks")
	}
	record, err := service.Store.GetArtifact(context.Background(), "greet-basics-2026-10-01", "0.1.0")
	if err != nil {
		t.Fatalf("GetArtifact: %v", err)
	}
	if record.ValidationState != ValidationBootCheckFailed {
		t.Fatalf("validation = %q, want boot-check-failed recorded", record.ValidationState)
	}
	// A failed record is no longer compatible: the next resolve misses and
	// must not serve it.
	resolution, err = service.Resolve(context.Background(), ResolveRequest{Technology: "python", Owner: "owner-1"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolution.Status == ResolutionHit {
		t.Fatalf("failed record served as a hit")
	}
	if builder.callCount() != 1 {
		t.Fatalf("builder calls = %d, want exactly 1 fallback build", builder.callCount())
	}
}

// Acceptance criterion 4: compatible hits boot clean, separate fresh
// learner workspaces with dependencies available, rerun no setup, keep no
// previous learner state, and never substitute versions.

func TestResolveHitReusesWithoutSetupOrPriorState(t *testing.T) {
	snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}}
	service, builder, _, _ := resolveService(snapshots)
	promoteGreet(t, service, "owner-1")
	commandsBefore := len(snapshots.commandLog())

	first, err := service.Resolve(context.Background(), ResolveRequest{Technology: "python", Owner: "owner-1"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if first.Status != ResolutionHit {
		t.Fatalf("status = %q, want hit", first.Status)
	}
	if first.Record.ArtifactVersion != "0.1.0" || first.Workspace.ArtifactVersion != "0.1.0" {
		t.Fatalf("resolution = %+v, want served version reported explicitly", first)
	}
	// Learner work lands in the first workspace only.
	snapshots.writeWorkspaceFile(first.Workspace.ID, "solution.py", "learner work")
	second, err := service.Resolve(context.Background(), ResolveRequest{Technology: "python", Owner: "owner-1"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if second.Status != ResolutionHit {
		t.Fatalf("status = %q, want hit", second.Status)
	}
	if second.Workspace.ID == first.Workspace.ID {
		t.Fatalf("workspace reused: learner workspaces must be separate")
	}
	if files := snapshots.workspaceFiles(second.Workspace.ID); len(files) != 0 {
		t.Fatalf("second workspace carries previous state: %v", files)
	}
	for _, commands := range []string{strings.Join(snapshots.commandLog()[commandsBefore:], "\n")} {
		if strings.Contains(commands, "python3 --version") {
			t.Fatalf("dependency installation rerun on the reuse path")
		}
		if !strings.Contains(commands, "py_compile") || !strings.Contains(commands, "test_greet") {
			t.Fatalf("reuse path skipped representative checks: %v", snapshots.commandLog())
		}
	}
	if builder.callCount() != 0 {
		t.Fatalf("builder calls = %d, want 0 on compatible hits", builder.callCount())
	}
}

func TestResolveVersionMismatchNeverSubstitutes(t *testing.T) {
	snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}}
	service, builder, _, _ := resolveService(snapshots)
	promoteGreet(t, service, "owner-1")
	resolution, err := service.Resolve(context.Background(), ResolveRequest{Technology: "python", Version: "9.9.9", Owner: "owner-1"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolution.Status != ResolutionIncompatibleVersion {
		t.Fatalf("status = %q, want incompatible-version", resolution.Status)
	}
	if resolution.Workspace.ID != "" {
		t.Fatalf("workspace issued for an incompatible version")
	}
	if builder.callCount() != 0 {
		t.Fatalf("builder calls = %d, want 0: mismatches never trigger builds", builder.callCount())
	}
	if snapshots.bootCount() != 1 {
		t.Fatalf("boots = %d, want only the promotion boot", snapshots.bootCount())
	}
}

func TestResolveVersionPinServesExact(t *testing.T) {
	snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}}
	service, _, _, _ := resolveService(snapshots)
	promoteGreet(t, service, "owner-1")
	manifest := loadGreetManifest(t)
	manifest.Artifact.Version = "0.2.0"
	service.Verifier = &fakeVerifier{result: verifiedGreetResult(t, "buildrun_greet_2")}
	sealed := mustSeal(t, service, greetBuildOutput(t, "buildrun_greet_2", "snap_greet_2", manifest))
	receipt, err := service.Promote(context.Background(), PromotionRequest{
		Verified: sealed,
		Owner:    "owner-1",
		BuiltBy:  "test-loop",
	})
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if receipt.Record.ArtifactVersion != "0.2.0" {
		t.Fatalf("promoted version = %q", receipt.Record.ArtifactVersion)
	}
	resolution, err := service.Resolve(context.Background(), ResolveRequest{Technology: "python", Version: "0.1.0", Owner: "owner-1"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolution.Status != ResolutionHit || resolution.Record.ArtifactVersion != "0.1.0" {
		t.Fatalf("resolution = %+v, want the exact pinned version", resolution)
	}
}

// Same version, new objective: the version exists but no compatible
// exercise matches, so this is an ordinary miss — capability/quota-gated
// builder fallback runs instead of an incompatible-version refusal, and
// the pin is re-enforced on the fresh output.
func TestResolveSameVersionNewObjectiveFallsBackToBuild(t *testing.T) {
	snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}}
	service, builder, _, verifier := resolveService(snapshots)
	promoteGreet(t, service, "owner-1")
	const objective = "Practice concurrency with goroutines"
	manifest := loadGreetManifest(t)
	manifest.Compatibility.RequestedObjective = objective
	builder.output = greetBuildOutput(t, "buildrun_obj_1", "snap_obj_1", manifest)
	verifier.result = verifiedGreetResult(t, "buildrun_obj_1")
	resolution, err := service.Resolve(context.Background(), ResolveRequest{
		Technology: "python", Version: "0.1.0", Objective: objective, Owner: "owner-1",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolution.Status != ResolutionBuilt {
		t.Fatalf("status = %q, want built via fallback, not incompatible-version", resolution.Status)
	}
	if builder.callCount() != 1 {
		t.Fatalf("builder calls = %d, want 1 fallback build", builder.callCount())
	}
	if resolution.Workspace.ID == "" {
		t.Fatalf("no workspace issued for the built environment")
	}
	if resolution.Record.ArtifactVersion != "0.1.0" || resolution.Record.Compatibility.Objective != objective {
		t.Fatalf("record = %+v, want the pinned version serving the requested objective", resolution.Record)
	}
}

// Same version, missing tag: fallback is attempted rather than refused,
// and the pin still binds the fresh output.
func TestResolveSameVersionMissingTagFallsBackToBuild(t *testing.T) {
	snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}}
	service, builder, _, verifier := resolveService(snapshots)
	promoteGreet(t, service, "owner-1")
	manifest := loadGreetManifest(t)
	manifest.Compatibility.ReuseTags = []string{"strings"}
	builder.output = greetBuildOutput(t, "buildrun_tag_1", "snap_tag_1", manifest)
	verifier.result = verifiedGreetResult(t, "buildrun_tag_1")
	resolution, err := service.Resolve(context.Background(), ResolveRequest{
		Technology: "python", Version: "0.1.0", ReuseTags: []string{"strings"}, Owner: "owner-1",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolution.Status != ResolutionBuilt {
		t.Fatalf("status = %q, want built via fallback, not incompatible-version", resolution.Status)
	}
	if builder.callCount() != 1 {
		t.Fatalf("builder calls = %d, want 1 fallback build", builder.callCount())
	}
	if resolution.Workspace.ID == "" {
		t.Fatalf("no workspace issued for the built environment")
	}
}

// Acceptance criterion 5: misses invoke the builder only when capability
// and quota/rate checks permit, with explicit reporting and zero builder
// calls on denial; fresh output verifies and promotes before reuse.

func TestResolveMissDeniedWithoutBuilderCalls(t *testing.T) {
	snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}}
	service, builder, quota, _ := resolveService(snapshots)

	unsupported, err := service.Resolve(context.Background(), ResolveRequest{Technology: "cobol", Owner: "owner-1"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if unsupported.Status != ResolutionUnsupportedCapability {
		t.Fatalf("status = %q, want unsupported-capability", unsupported.Status)
	}

	for _, denial := range []struct {
		reason QuotaDenialReason
		status ResolutionStatus
	}{
		{QuotaExhausted, ResolutionQuotaExhausted},
		{RateLimited, ResolutionRateLimited},
	} {
		quota.err = &QuotaDenial{Reason: denial.reason, Owner: "owner-1"}
		resolution, err := service.Resolve(context.Background(), ResolveRequest{Technology: "python", Owner: "owner-1"})
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if resolution.Status != denial.status {
			t.Fatalf("status = %q, want %q", resolution.Status, denial.status)
		}
		if resolution.Workspace.ID != "" {
			t.Fatalf("workspace issued on denial")
		}
	}
	if builder.callCount() != 0 {
		t.Fatalf("builder calls = %d, want 0 across all denials", builder.callCount())
	}
	if quota.checkCount() != 2 {
		t.Fatalf("quota checks = %d, want one per miss", quota.checkCount())
	}
}

func TestResolveMissBuildsVerifiesPromotesThenReuses(t *testing.T) {
	snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}}
	service, builder, _, verifier := resolveService(snapshots)
	manifest := loadGreetManifest(t)
	builder.output = greetBuildOutput(t, "buildrun_fresh_1", "snap_fresh_1", manifest)
	verified := verifiedGreetResult(t, "buildrun_fresh_1")
	verified.VerifierRunID = "verify_fresh_1"
	verifier.result = verified

	built, err := service.Resolve(context.Background(), ResolveRequest{Technology: "python", Owner: "owner-1", Objective: "Practice string handling with functions"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if built.Status != ResolutionBuilt {
		t.Fatalf("status = %q, want built", built.Status)
	}
	if built.Workspace.ID == "" {
		t.Fatalf("no workspace separated for fresh output")
	}
	if builder.callCount() != 1 {
		t.Fatalf("builder calls = %d, want 1", builder.callCount())
	}
	if _, err := service.Store.GetArtifact(context.Background(), "greet-basics-2026-10-01", "0.1.0"); err != nil {
		t.Fatalf("fresh output not promoted before reuse: %v", err)
	}

	hit, err := service.Resolve(context.Background(), ResolveRequest{Technology: "python", Owner: "owner-1"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if hit.Status != ResolutionHit || hit.Workspace.ID == built.Workspace.ID {
		t.Fatalf("resolution = %+v, want a fresh-workspace hit", hit)
	}
	if builder.callCount() != 1 {
		t.Fatalf("builder calls = %d, want no rebuild on compatible reuse", builder.callCount())
	}
}

func TestResolveMissUnverifiedOutputNeverReused(t *testing.T) {
	snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}}
	service, builder, _, verifier := resolveService(snapshots)
	manifest := loadGreetManifest(t)
	builder.output = greetBuildOutput(t, "buildrun_bad_1", "snap_bad_1", manifest)
	failed := "test"
	verifier.result = verifiedGreetResult(t, "buildrun_bad_1")
	verifier.result.Verdict = "rejected"
	verifier.result.FailedStage = &failed

	resolution, err := service.Resolve(context.Background(), ResolveRequest{Technology: "python", Owner: "owner-1"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolution.Status != ResolutionVerificationFailed {
		t.Fatalf("status = %q, want verification-failed", resolution.Status)
	}
	if _, err := service.Store.GetArtifact(context.Background(), "greet-basics-2026-10-01", "0.1.0"); err == nil {
		t.Fatalf("unverified output was promoted")
	}
}
