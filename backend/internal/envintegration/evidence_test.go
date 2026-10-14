package envintegration

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/kvn8888/codegym/backend/internal/agentruntime"
	"github.com/kvn8888/codegym/backend/internal/envbuild"
	"github.com/kvn8888/codegym/backend/internal/environmentregistry"
	"github.com/kvn8888/codegym/backend/internal/environmentverify"
)

func TestEvidenceBundleRecordsDemonstration(t *testing.T) {
	client := &scriptedAgentClient{turns: demoFixtureTurns()}
	dirs := &SnapshotDirectories{}
	preparer := &envbuild.Preparer{
		Runtime:     &agentruntime.PurposeBuiltRuntime{Client: client},
		Provisioner: envbuild.HostProvisioner{},
	}
	builder := &RegistryBuilder{Preparer: preparer, RuntimeName: "purpose-built", Snapshots: dirs}
	out, err := builder.Build(t.Context(), environmentregistry.BuildRequest{Technology: "demo", Objective: "learn demo", Owner: "owner-1"})
	if err != nil {
		t.Fatal(err)
	}
	failure := environmentverify.ExpectedFailure{
		TestID: "t1", Code: "WRONG_VALUE", Expected: "1", Actual: "2", Reason: "wrong solution returns 2",
	}
	spec := &environmentverify.AssessmentSpec{
		ArtifactID:        out.Manifest.Artifact.ID,
		ArtifactVersion:   out.Manifest.Artifact.Version,
		Objective:         "learn demo",
		CorrectSolution:   map[string]string{"solution.py": "VALUE = 1\n"},
		WrongSolution:     map[string]string{"solution.py": "VALUE = 2\n"},
		ExpectedFailure:   failure,
		ReviewedBy:        "test-review",
		ReviewedReference: "slice-5-evidence",
	}
	verifier := &RegistryVerifier{
		Roots:      dirs.Resolve,
		Runner:     &LocalCommandRunner{},
		Assessment: spec,
		Assessor:   &fakeAssessor{failure: failure},
	}
	res := verifier.Verify(t.Context(), out)
	if res.Verdict != environmentverify.VerdictVerified {
		t.Fatalf("verdict = %q, diagnostics = %#v", res.Verdict, res.Diagnostics)
	}
	service := &environmentregistry.Service{
		Store:          environmentregistry.NewInMemoryStore(),
		Snapshots:      &fakeSnapshotProvider{dirs: dirs},
		Builder:        builder,
		Capabilities:   &fakeCapabilities{allowed: map[string]bool{"demo": true}},
		Quota:          &fakeQuota{denied: map[string]environmentregistry.QuotaDenialReason{}},
		Verifier:       verifier,
		PracticePolicy: environmentregistry.NetworkPolicyIsolated,
	}
	verified, err := service.VerifyPreparation(t.Context(), out)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := service.Promote(t.Context(), environmentregistry.PromotionRequest{Verified: verified, Owner: "owner-1", BuiltBy: "test"})
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := AssembleEvidence("test-commit", "demo", "learn demo", out, res, receipt)
	if err != nil {
		t.Fatal(err)
	}
	path, err := WriteEvidenceBundle(t.TempDir(), "demo", bundle)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var decoded DemoEvidence
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.SchemaVersion != DemoEvidenceSchemaVersion {
		t.Fatalf("schema = %q", decoded.SchemaVersion)
	}
	if decoded.ApplicationCommit != "test-commit" || decoded.Technology != "demo" || decoded.Objective != "learn demo" {
		t.Fatalf("identity = %#v", decoded)
	}
	if decoded.Manifest.Artifact.ID != out.Manifest.Artifact.ID {
		t.Fatalf("manifest artifact = %#v", decoded.Manifest.Artifact)
	}
	if decoded.Verification.Verdict != environmentverify.VerdictVerified {
		t.Fatalf("verification verdict = %q", decoded.Verification.Verdict)
	}
	if len(decoded.Verification.Evidence.Checks) == 0 {
		t.Fatal("no checks recorded")
	}
	if !decoded.Promotion.Allowed || decoded.Promotion.Record.ArtifactID == "" {
		t.Fatalf("promotion = %#v", decoded.Promotion)
	}
	if decoded.Builder.BuilderRunID == "" || decoded.Verification.BuilderRunID != decoded.Builder.BuilderRunID {
		t.Fatalf("run identity broken across bundle: %#v", decoded.Builder)
	}
}
