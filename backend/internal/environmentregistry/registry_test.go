package environmentregistry

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kvn8888/codegym/backend/internal/environmentverify"
)

// Acceptance criterion 1: rejected/unverified artifacts and invalid
// manifests are refused at sealing, with evidence bound to the exact
// preparation — all without booting anything or storing anything.

func promoteService(snapshots *fakeSnapshotProvider) *Service {
	return &Service{Store: NewInMemoryStore(), Snapshots: snapshots}
}

func TestPromoteRefusesRejectedResult(t *testing.T) {
	snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}}
	service := promoteService(snapshots)
	manifest := loadGreetManifest(t)
	failed := "build"
	service.Verifier = &fakeVerifier{result: environmentverify.VerifierResult{
		SchemaVersion: environmentverify.VerifierResultSchemaVersion,
		VerifierRunID: "verify_fail",
		BuilderRunID:  "buildrun_fail",
		Verdict:       environmentverify.VerdictRejected,
		FailedStage:   &failed,
		Diagnostics:   []environmentverify.Diagnostic{{Severity: "error", Stage: "build", Code: "BUILD_FAILED", Message: "build failed"}},
		Promotion:     environmentverify.Promotion{Allowed: false},
	}}
	build := greetBuildOutput(t, "buildrun_fail", "snap_fail", manifest)
	_, err := service.VerifyPreparation(context.Background(), build)
	refusal := requireRefusal(t, err, "not verified")
	if !refusal.Repairable {
		t.Fatalf("repairable = false, want true so the builder track can retry")
	}
	if snapshots.bootCount() != 0 {
		t.Fatalf("boots = %d, want 0: rejected results never reach boot checks", snapshots.bootCount())
	}
	if _, err := service.Store.GetArtifact(context.Background(), "greet-basics-2026-10-01", "0.1.0"); err != ErrArtifactNotFound {
		t.Fatalf("record stored despite refusal: %v", err)
	}
}

// Malformed or self-contradictory verifier results never seal: supported
// schema, nonblank verifier run ID, non-empty evidence, and zero check
// exits are all required before a verified verdict can authorize
// anything. Refusals precede boot and storage, and are backend faults —
// never builder repair signals.
func TestSealRefusesMalformedVerifierResult(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*environmentverify.VerifierResult)
		want   string
	}{
		{"schema", func(v *environmentverify.VerifierResult) { v.SchemaVersion = "" }, "schema"},
		{"run ID", func(v *environmentverify.VerifierResult) { v.VerifierRunID = "  " }, "verifier run ID"},
		{"evidence", func(v *environmentverify.VerifierResult) { v.Evidence = environmentverify.Evidence{} }, "check evidence"},
		{"exit", func(v *environmentverify.VerifierResult) { v.Evidence.Checks[2].ExitCode = 1 }, "exit code"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}}
			service := promoteService(snapshots)
			result := verifiedGreetResult(t, "buildrun_shape_1")
			tc.mutate(&result)
			service.Verifier = &fakeVerifier{result: result}
			manifest := loadGreetManifest(t)
			_, err := service.VerifyPreparation(context.Background(),
				greetBuildOutput(t, "buildrun_shape_1", "snap_shape_1", manifest))
			refusal := requireRefusal(t, err, tc.want)
			if refusal.Repairable {
				t.Fatalf("repairable = true, want false: a malformed verifier result is a backend fault")
			}
			if snapshots.bootCount() != 0 {
				t.Fatalf("boots = %d, want 0", snapshots.bootCount())
			}
			if _, err := service.Store.GetArtifact(context.Background(), "greet-basics-2026-10-01", "0.1.0"); err != ErrArtifactNotFound {
				t.Fatalf("%s: record stored from a malformed verifier result", tc.name)
			}
		})
	}
}

// The seal requires successful setup, build, and test evidence, each
// recording the sealed manifest's command: manifest-only evidence, a
// missing required stage, or a required check with no recorded command
// all refuse before boot or storage, as backend faults.
func TestSealRequiresStageEvidence(t *testing.T) {
	drop := func(checks []environmentverify.EvidenceCheck, stage string) []environmentverify.EvidenceCheck {
		kept := make([]environmentverify.EvidenceCheck, 0, len(checks))
		for _, check := range checks {
			if check.Stage != stage {
				kept = append(kept, check)
			}
		}
		return kept
	}
	cases := []struct {
		name   string
		mutate func(*environmentverify.VerifierResult)
		want   string
	}{
		{"manifest-only", func(v *environmentverify.VerifierResult) {
			v.Evidence.Checks = v.Evidence.Checks[:2]
		}, `omits required "setup" check`},
		{"missing setup", func(v *environmentverify.VerifierResult) {
			v.Evidence.Checks = drop(v.Evidence.Checks, environmentverify.StageSetup)
		}, `omits required "setup" check`},
		{"missing build", func(v *environmentverify.VerifierResult) {
			v.Evidence.Checks = drop(v.Evidence.Checks, environmentverify.StageBuild)
		}, `omits required "build" check`},
		{"missing test", func(v *environmentverify.VerifierResult) {
			v.Evidence.Checks = drop(v.Evidence.Checks, environmentverify.StageTest)
		}, `omits required "test" check`},
		{"nil command", func(v *environmentverify.VerifierResult) {
			v.Evidence.Checks[3].Command = nil
		}, "records no command"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}}
			service := promoteService(snapshots)
			result := verifiedGreetResult(t, "buildrun_stage_1")
			tc.mutate(&result)
			service.Verifier = &fakeVerifier{result: result}
			manifest := loadGreetManifest(t)
			_, err := service.VerifyPreparation(context.Background(),
				greetBuildOutput(t, "buildrun_stage_1", "snap_stage_1", manifest))
			refusal := requireRefusal(t, err, tc.want)
			if refusal.Repairable {
				t.Fatalf("repairable = true, want false: stage evidence is backend-produced")
			}
			if snapshots.bootCount() != 0 {
				t.Fatalf("boots = %d, want 0", snapshots.bootCount())
			}
			if _, err := service.Store.GetArtifact(context.Background(), "greet-basics-2026-10-01", "0.1.0"); err != ErrArtifactNotFound {
				t.Fatalf("%s: record stored without required stage evidence", tc.name)
			}
		})
	}
}

func TestSealRefusesBindingMismatch(t *testing.T) {
	snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}}
	service := promoteService(snapshots)
	// The handoff carries one builder run but the verifier result is bound
	// to another: preparation binding refuses sealing after verification,
	// before any boot or storage.
	service.Verifier = &fakeVerifier{result: verifiedGreetResult(t, "buildrun_other_run")}
	manifest := loadGreetManifest(t)
	build := greetBuildOutput(t, "buildrun_greet_1", "snap_greet_1", manifest)
	_, err := service.VerifyPreparation(context.Background(), build)
	requireRefusal(t, err, "does not match the preparation")
	if snapshots.bootCount() != 0 {
		t.Fatalf("boots = %d, want 0", snapshots.bootCount())
	}
}

// A handoff whose manifest differs from the verified evidence cannot be
// sealed: the run-ID check passes, but the evidence-command check refuses
// sealing before any boot runs and stores nothing.
func TestSealRefusesEvidenceForOtherPreparation(t *testing.T) {
	snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}}
	service := promoteService(snapshots)
	manifestB := loadGreetManifest(t)
	manifestB.Commands.Build = "python3 -m py_compile other.py"
	manifestB.Commands.Test = "python3 other_test.py"
	service.Verifier = &fakeVerifier{result: verifiedGreetResult(t, "buildrun_greet_1")}
	build := greetBuildOutput(t, "buildrun_greet_1", "snap_other_1", manifestB)
	_, err := service.VerifyPreparation(context.Background(), build)
	// The shape gate fires first: transferred evidence contradicts the
	// sealed manifest before binding is even reached.
	refusal := requireRefusal(t, err, "does not match the sealed manifest")
	if refusal.Repairable {
		t.Fatalf("repairable = true, want false: backend-produced evidence contradicting the manifest is a backend fault")
	}
	if snapshots.bootCount() != 0 {
		t.Fatalf("boots = %d, want 0: transferred evidence never reaches boot checks", snapshots.bootCount())
	}
	if _, err := service.Store.GetArtifact(context.Background(), "greet-basics-2026-10-01", "0.1.0"); err != ErrArtifactNotFound {
		t.Fatalf("record stored from another preparation's evidence: %v", err)
	}
}

// A handoff with no snapshot reference cannot be sealed: there is nothing
// the seal could name for boot checks. Refusal precedes the verifier call.
func TestSealRefusesSnapshotlessHandoff(t *testing.T) {
	snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}}
	service := promoteService(snapshots)
	manifest := loadGreetManifest(t)
	verifier := &fakeVerifier{result: verifiedGreetResult(t, "buildrun_greet_1")}
	service.Verifier = verifier
	build := greetBuildOutput(t, "buildrun_greet_1", "", manifest)
	_, err := service.VerifyPreparation(context.Background(), build)
	refusal := requireRefusal(t, err, "no snapshot reference")
	if !refusal.Repairable {
		t.Fatalf("repairable = false, want true so the builder track can retry with a complete handoff")
	}
	if verifier.verifyCount() != 0 {
		t.Fatalf("verifier ran for a snapshotless handoff")
	}
	if snapshots.bootCount() != 0 {
		t.Fatalf("boots = %d, want 0", snapshots.bootCount())
	}
}

// Promotion derives exclusively from the seal: the request carries no
// caller-supplied preparation, so snapshot substitution is
// inexpressible — the booted snapshot and recorded metadata can only be
// the sealed ones.
func TestPromotionDerivesFromSeal(t *testing.T) {
	snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}}
	service := promoteService(snapshots)
	sealed := sealGreet(t, service, "buildrun_greet_1", "snap_greet_1")
	receipt, err := service.Promote(context.Background(), PromotionRequest{
		Verified: sealed,
		Owner:    "owner-1",
		BuiltBy:  "test-loop",
	})
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if !receipt.Allowed {
		t.Fatalf("receipt allowed = false, want true")
	}
	if receipt.Record.SnapshotRef != "snap_greet_1" {
		t.Fatalf("record snapshot = %q, want the sealed snapshot", receipt.Record.SnapshotRef)
	}
	if len(snapshots.boots) != 1 || snapshots.boots[0].SnapshotRef != "snap_greet_1" {
		t.Fatalf("boots = %+v, want exactly one boot of the sealed snapshot", snapshots.boots)
	}
	if receipt.Record.ArtifactID != "greet-basics-2026-10-01" || receipt.Record.ArtifactVersion != "0.1.0" {
		t.Fatalf("record identity = %s %s, want the sealed manifest identity",
			receipt.Record.ArtifactID, receipt.Record.ArtifactVersion)
	}
}

// Later mutations of the caller's originals cannot alter promotion: the
// seal captured independent deep copies of the manifest and the verdict
// evidence. After sealing, this test corrupts the original artifact
// identity, commands, editable/protected lists, compatibility tags, and
// even the verifier's own scripted evidence — including invalidating the
// original manifest outright — and promotion must still succeed on the
// sealed values, recording the originals.
func TestSealedCopiesImmuneToCallerMutation(t *testing.T) {
	snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}}
	service := promoteService(snapshots)
	manifest := loadGreetManifest(t)
	manifest.Compatibility.ReuseTags = []string{"strings"}
	verifier := &fakeVerifier{result: verifiedGreetResult(t, "buildrun_greet_1")}
	service.Verifier = verifier
	sealed := mustSeal(t, service, greetBuildOutput(t, "buildrun_greet_1", "snap_greet_1", manifest))
	manifest.Artifact.ID = "mutated-id"
	manifest.Artifact.Version = "9.9.9"
	manifest.Artifact.Technology = "ruby"
	manifest.Commands.Build = ""
	manifest.Commands.Test = ""
	manifest.Workspace.LearnerEditable[0] = "mutated.py"
	manifest.Workspace.Protected[0] = "mutated-protected"
	manifest.Workspace = nil
	manifest.Compatibility.RequestedObjective = "mutated objective"
	manifest.Compatibility.ReuseTags[0] = "mutated-tag"
	manifest.Compatibility.ReuseTags = append(manifest.Compatibility.ReuseTags, "injected")
	verifier.result.Evidence.Checks[2].Command = strptr("mutated command")
	verifier.result.Evidence.Checks = append(verifier.result.Evidence.Checks,
		environmentverify.EvidenceCheck{Stage: "test", Command: strptr("mutated command"), Summary: "injected"})
	receipt, err := service.Promote(context.Background(), PromotionRequest{
		Verified: sealed,
		Owner:    "owner-1",
		BuiltBy:  "test-loop",
	})
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	if !receipt.Allowed {
		t.Fatalf("receipt allowed = false, want true on the sealed values")
	}
	record := receipt.Record
	if record.ArtifactID != "greet-basics-2026-10-01" || record.ArtifactVersion != "0.1.0" || record.Technology != "python" {
		t.Fatalf("record identity = %s %s %s, want the sealed originals",
			record.ArtifactID, record.ArtifactVersion, record.Technology)
	}
	if record.BuildCommand != "python3 -m py_compile solution.py protected/test_greet.py" {
		t.Fatalf("record build = %q, want the sealed original", record.BuildCommand)
	}
	if record.TestCommand != "python3 protected/test_greet.py" {
		t.Fatalf("record test = %q, want the sealed original", record.TestCommand)
	}
	if record.Compatibility.Objective != "Practice string handling with functions" {
		t.Fatalf("record objective = %q, want the sealed original", record.Compatibility.Objective)
	}
	if len(record.Compatibility.ReuseTags) != 1 || record.Compatibility.ReuseTags[0] != "strings" {
		t.Fatalf("record tags = %q, want the sealed originals", record.Compatibility.ReuseTags)
	}
	if record.Evidence.CheckCount != 5 {
		t.Fatalf("evidence checks = %d, want the 5 sealed checks without the injected one", record.Evidence.CheckCount)
	}
}

// An unsealed (zero-value) association carries no verification authority
// and fails closed before boot or storage.
func TestPromoteRefusesUnsealedPreparation(t *testing.T) {
	snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}}
	service := promoteService(snapshots)
	_, err := service.Promote(context.Background(), PromotionRequest{
		Verified: VerifiedPreparation{},
		Owner:    "owner-1",
		BuiltBy:  "test-loop",
	})
	requireRefusal(t, err, "not verified")
	if snapshots.bootCount() != 0 {
		t.Fatalf("boots = %d, want 0", snapshots.bootCount())
	}
	if _, err := service.Store.GetArtifact(context.Background(), "greet-basics-2026-10-01", "0.1.0"); err != ErrArtifactNotFound {
		t.Fatalf("record stored for an unsealed preparation")
	}
}

func TestPromoteRefusesInvalidManifest(t *testing.T) {
	snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}}
	service := promoteService(snapshots)
	manifest := loadGreetManifest(t)
	manifest.Workspace = nil
	service.Verifier = &fakeVerifier{result: verifiedGreetResult(t, "buildrun_greet_1")}
	build := greetBuildOutput(t, "buildrun_greet_1", "snap_greet_1", manifest)
	_, err := service.VerifyPreparation(context.Background(), build)
	requireRefusal(t, err, "invalid manifest")
	if snapshots.bootCount() != 0 {
		t.Fatalf("boots = %d, want 0", snapshots.bootCount())
	}
}

// Acceptance criterion 2: promotion records full metadata with evidence
// bound to the exact preparation.

func TestPromoteRecordsMetadata(t *testing.T) {
	snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}}
	service := promoteService(snapshots)
	receipt := promoteGreet(t, service, "owner-1")
	if receipt.Policy != NetworkPolicyIsolated {
		t.Fatalf("policy = %q, want isolated practice policy", receipt.Policy)
	}
	if receipt.CheckedAt.IsZero() {
		t.Fatalf("checkedAt is zero")
	}
	record, err := service.Store.GetArtifact(context.Background(), "greet-basics-2026-10-01", "0.1.0")
	if err != nil {
		t.Fatalf("GetArtifact: %v", err)
	}
	if record.Technology != "python" || record.EnvironmentBase != "python-3.13" {
		t.Fatalf("identity = %+v, want python on python-3.13", record)
	}
	if len(record.DependencyFiles) != 0 || len(record.Lockfiles) != 0 {
		t.Fatalf("dependency refs = %+v / %+v, want recorded empty sets", record.DependencyFiles, record.Lockfiles)
	}
	if !strings.Contains(record.BuildCommand, "py_compile") || !strings.Contains(record.TestCommand, "test_greet") {
		t.Fatalf("commands = %+v, want recorded setup/build/test", record)
	}
	if record.SetupCommand == "" {
		t.Fatalf("setup command not recorded")
	}
	if record.Provenance.BuilderRunID != "buildrun_greet_1" || record.Provenance.VerifierRunID != "verify_greet_1" {
		t.Fatalf("provenance = %+v, want exact run binding", record.Provenance)
	}
	if record.Provenance.BuiltBy != "test-loop" || record.Provenance.Owner != "owner-1" || record.Owner != "owner-1" {
		t.Fatalf("provenance/owner = %+v, want builtBy/owner recorded", record.Provenance)
	}
	if record.CreatedAt.IsZero() || record.LastBootCheckAt.IsZero() {
		t.Fatalf("timestamps not recorded: %+v", record)
	}
	if record.ValidationState != ValidationPassed {
		t.Fatalf("validation = %q, want passed", record.ValidationState)
	}
	if record.Compatibility.Technology != "python" || record.Compatibility.Objective == "" {
		t.Fatalf("compatibility = %+v, want explicit rules", record.Compatibility)
	}
	if record.Evidence.BuilderRunID != "buildrun_greet_1" || record.Evidence.VerifierRunID != "verify_greet_1" ||
		record.Evidence.Verdict != "verified" || record.Evidence.CheckCount != 5 {
		t.Fatalf("evidence = %+v, want exact preparation association", record.Evidence)
	}
	if record.SnapshotRef != "snap_greet_1" {
		t.Fatalf("snapshot = %q", record.SnapshotRef)
	}
}

// Acceptance criterion 3 (promotion half): fresh boot plus representative
// build/test checks under the practice policy gate promotion; failures
// never offer the artifact as ready.

func TestPromoteRefusesFailedBootChecks(t *testing.T) {
	for _, step := range []struct {
		name    string
		command string
	}{
		{"boot", ""},
		{"build", "python3 -m py_compile solution.py protected/test_greet.py"},
		{"test", "python3 protected/test_greet.py"},
	} {
		snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}}
		if step.name == "boot" {
			snapshots.bootErr = errors.New("snapshot unavailable")
		} else {
			snapshots.results[step.command] = fakeCommandResult{result: BootResult{ExitCode: 1, Output: step.name + " broke"}}
		}
		service := promoteService(snapshots)
		sealed := sealGreet(t, service, "buildrun_greet_1", "snap_greet_1")
		_, err := service.Promote(context.Background(), PromotionRequest{
			Verified: sealed,
			Owner:    "owner-1",
			BuiltBy:  "test-loop",
		})
		requireRefusal(t, err, step.name)
		if _, err := service.Store.GetArtifact(context.Background(), "greet-basics-2026-10-01", "0.1.0"); err != ErrArtifactNotFound {
			t.Fatalf("%s: record stored despite failed checks", step.name)
		}
	}
}

func TestPromoteRunsIsolatedBootChecksWithoutSetup(t *testing.T) {
	snapshots := &fakeSnapshotProvider{results: map[string]fakeCommandResult{}}
	service := promoteService(snapshots)
	promoteGreet(t, service, "owner-1")
	if snapshots.bootCount() != 1 {
		t.Fatalf("boots = %d, want exactly 1 fresh boot", snapshots.bootCount())
	}
	for _, boot := range snapshots.boots {
		if boot.Policy != NetworkPolicyIsolated || boot.SnapshotRef != "snap_greet_1" {
			t.Fatalf("boot = %+v, want isolated boot of the prepared snapshot", boot)
		}
	}
	commands := snapshots.commandLog()
	joined := strings.Join(commands, "\n")
	if strings.Contains(joined, "python3 --version") {
		t.Fatalf("commands = %v, want no dependency installation (setup) on the promotion path", commands)
	}
	if len(commands) != 2 {
		t.Fatalf("commands = %v, want representative build+test only", commands)
	}
}
