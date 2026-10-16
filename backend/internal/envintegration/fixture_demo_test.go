package envintegration

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/kvn8888/codegym/backend/internal/envbuild"
	"github.com/kvn8888/codegym/backend/internal/environmentregistry"
	"github.com/kvn8888/codegym/backend/internal/practice"
)

func fixtureDemoSource(t *testing.T) *MapAssessmentSource {
	t.Helper()
	path := filepath.Join("..", "..", "config", "spring-boot.assessment.json")
	assessments, err := LoadAssessmentFile(path)
	if err != nil {
		t.Fatalf("load assessment file: %v", err)
	}
	return assessments
}

func fixturePipelineHarness(source *MapAssessmentSource) (*ProductionPipeline, *SnapshotDirectories) {
	dirs := &SnapshotDirectories{}
	preparer := &envbuild.Preparer{
		Runtime:     NewSpringBootFixtureRuntime(),
		Provisioner: envbuild.HostProvisioner{},
	}
	registryService := &environmentregistry.Service{
		Store:        environmentregistry.NewInMemoryStore(),
		Snapshots:    &fakeSnapshotProvider{dirs: dirs},
		Builder:      &RegistryBuilder{Preparer: preparer, RuntimeName: "fixture", Snapshots: dirs},
		Capabilities: &fakeCapabilities{allowed: map[string]bool{"spring-boot": true, "Spring Boot": true}},
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

func TestFixtureDemoPipelineOpensSession(t *testing.T) {
	if _, err := exec.LookPath("mvn"); err != nil {
		t.Skipf("mvn not found on PATH: %v", err)
	}
	if _, err := exec.LookPath("java"); err != nil {
		t.Skipf("java not found on PATH: %v", err)
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skipf("sh not found on PATH: %v", err)
	}
	// The fixture pins the locally cached Spring Boot 2.6.2 parent, but the
	// prerequisite table probes for 3.3.5. The documented waiver accepts the
	// uncached row for demonstrations; the fixture itself needs no download.
	t.Setenv("CODEGYM_ALLOW_UNCACHED_SETUP", "1")

	pipeline, _ := fixturePipelineHarness(fixtureDemoSource(t))
	stages := []string{}
	sessionID, err := pipeline.Run(t.Context(), &EnvironmentOperation{
		Owner: "owner-fixture", WorkspaceID: "workspace-fixture", Technology: "spring-boot", Objective: "demo health endpoint",
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
