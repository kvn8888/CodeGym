package envintegration

import (
	"strings"
	"testing"

	"github.com/kvn8888/codegym/backend/internal/agentruntime"
	"github.com/kvn8888/codegym/backend/internal/envbuild"
	"github.com/kvn8888/codegym/backend/internal/environmentregistry"
	"github.com/kvn8888/codegym/backend/internal/environmentverify"
	"github.com/kvn8888/codegym/backend/internal/practice"
)

func reviewedDemoSource() *MapAssessmentSource {
	failure := environmentverify.ExpectedFailure{
		TestID: "t1", Code: "WRONG_VALUE", Expected: "1", Actual: "2", Reason: "wrong solution returns 2",
	}
	return &MapAssessmentSource{Entries: map[string]ReviewedSolutions{
		"demo": {
			CorrectSolution:   map[string]string{"solution.py": "VALUE = 1\n"},
			WrongSolution:     map[string]string{"solution.py": "VALUE = 2\n"},
			ExpectedFailure:   failure,
			ReviewedBy:        "test-review",
			ReviewedReference: "pipeline-fixture",
			Assessor:          &fakeAssessor{failure: failure},
		},
	}}
}

func pipelineHarness(client *scriptedAgentClient, source *MapAssessmentSource) (*ProductionPipeline, *SnapshotDirectories) {
	dirs := &SnapshotDirectories{}
	preparer := &envbuild.Preparer{
		Runtime:     &agentruntime.PurposeBuiltRuntime{Client: client},
		Provisioner: envbuild.HostProvisioner{},
	}
	registryService := &environmentregistry.Service{
		Store:        environmentregistry.NewInMemoryStore(),
		Snapshots:    &fakeSnapshotProvider{dirs: dirs},
		Builder:      &RegistryBuilder{Preparer: preparer, RuntimeName: "purpose-built", Snapshots: dirs},
		Capabilities: &fakeCapabilities{allowed: map[string]bool{"demo": true}},
		Quota:        &fakeQuota{denied: map[string]environmentregistry.QuotaDenialReason{}},
		Verifier: &AssessmentResolvingVerifier{
			Inner:       &RegistryVerifier{Roots: dirs.Resolve, Runner: &LocalCommandRunner{}},
			Assessments: source,
		},
		PracticePolicy: environmentregistry.NetworkPolicyIsolated,
	}
	contents := &SnapshotContents{Roots: dirs.Resolve}
	practiceService := &practice.Service{
		Artifacts: &RegistryArtifactSource{Store: registryService.Store, Contents: contents},
		Contents:  contents,
		Runner:    &PracticeLocalRunner{},
		Records:   practice.NewInMemoryRecordStore(),
		Saved:     &mapSavedStore{},
	}
	return &ProductionPipeline{Registry: registryService, Practice: practiceService}, dirs
}

func TestProductionPipelineOpensSession(t *testing.T) {
	client := &scriptedAgentClient{turns: demoFixtureTurns()}
	pipeline, _ := pipelineHarness(client, reviewedDemoSource())
	stages := []string{}
	sessionID, err := pipeline.Run(t.Context(), &EnvironmentOperation{
		Owner: "owner-1", WorkspaceID: "workspace-1", Technology: "demo", Objective: "learn demo",
	}, func(stage string) { stages = append(stages, stage) })
	if err != nil {
		t.Fatal(err)
	}
	if sessionID == "" {
		t.Fatal("no session ID returned")
	}
	if len(stages) == 0 {
		t.Fatal("no stages reported")
	}
}

func TestProductionPipelineRequiresReviewedInputs(t *testing.T) {
	client := &scriptedAgentClient{turns: demoFixtureTurns()}
	pipeline, _ := pipelineHarness(client, &MapAssessmentSource{Entries: map[string]ReviewedSolutions{}})
	_, err := pipeline.Run(t.Context(), &EnvironmentOperation{
		Owner: "owner-1", WorkspaceID: "workspace-1", Technology: "demo", Objective: "learn demo",
	}, func(string) {})
	if err == nil || !strings.Contains(err.Error(), "verification-failed") {
		t.Fatalf("err = %v, want failed verification without reviewed inputs", err)
	}
}

func TestAssessmentResolvingVerifierFailsClosed(t *testing.T) {
	verifier := &AssessmentResolvingVerifier{
		Inner:       &RegistryVerifier{Roots: func(string) (string, error) { return "", nil }},
		Assessments: &MapAssessmentSource{Entries: map[string]ReviewedSolutions{}},
	}
	build := environmentregistry.BuildOutput{}
	build.Manifest.Artifact = &environmentverify.Artifact{ID: "unreviewed-1", Technology: "unreviewed-tech", Version: "0.1.0"}
	result := verifier.Verify(t.Context(), build)
	if result.Verdict != environmentverify.VerdictRejected {
		t.Fatalf("verdict = %q, want rejected", result.Verdict)
	}
	found := false
	for _, diagnostic := range result.Diagnostics {
		if strings.Contains(diagnostic.Message, "not reviewed yet") {
			found = true
		}
	}
	if !found {
		t.Fatalf("diagnostics = %#v, want explicit review-pending message", result.Diagnostics)
	}
}
