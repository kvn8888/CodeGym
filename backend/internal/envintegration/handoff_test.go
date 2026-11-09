package envintegration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kvn8888/codegym/backend/internal/envbuild"
	"github.com/kvn8888/codegym/backend/internal/environmentverify"
)

func writeHandoffFixture(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func passingManifest() *envbuild.EnvironmentManifest {
	return &envbuild.EnvironmentManifest{
		SchemaVersion: "environment.manifest.v1",
		Artifact:      envbuild.ManifestArtifact{ID: "demo-20261014", Technology: "demo", Version: "0.1.0"},
		Environment:   envbuild.ManifestEnvironment{Base: "generic", Network: "setup-only", DependencyFiles: []string{}, Lockfiles: []string{}},
		Workspace: envbuild.ManifestWorkspace{
			Root:            ".",
			LearnerEditable: []string{"solution.py"},
			Protected:       []string{"protected/test_demo.py", ".codegym/hidden/test_hidden.py"},
		},
		Commands: envbuild.ManifestCommands{Setup: "true", Build: "true", Test: "true", Run: "true"},
		Assessment: envbuild.ManifestAssessment{
			Type:         "unit-tests",
			VisibleTests: []string{"protected/test_demo.py"},
			HiddenTests:  []string{".codegym/hidden/test_hidden.py"},
		},
		Compatibility: envbuild.ManifestCompatibility{RequestedTechnology: "Demo", RequestedObjective: "learn demo", ReuseTags: []string{"demo"}},
	}
}

func TestHandoffVerifiesValidWorkspace(t *testing.T) {
	root := t.TempDir()
	writeHandoffFixture(t, root, "solution.py", "VALUE = 1\n")
	writeHandoffFixture(t, root, "protected/test_demo.py", "assert True\n")
	writeHandoffFixture(t, root, ".codegym/hidden/test_hidden.py", "assert True\n")
	manifestPath, err := WriteManifest(root, passingManifest())
	if err != nil {
		t.Fatal(err)
	}
	builder := ToBuilderResult("build_handoff_valid", "purpose-built", "test", nil, manifestPath, nil, envbuild.BuilderTelemetry{})
	runner := &LocalCommandRunner{}
	result := VerifyWorkspace(t.Context(), runner, root, manifestPath, builder.BuilderRunID, "verify_handoff_valid", builder)
	if len(runner.Calls) != 3 {
		t.Fatalf("executed stages = %d, want setup/build/test", len(runner.Calls))
	}
	for _, check := range result.Evidence.Checks {
		if (check.Stage == "setup" || check.Stage == "build" || check.Stage == "test") && check.ExitCode != 0 {
			t.Fatalf("evidence = %#v", result.Evidence)
		}
	}
	if result.Verdict != environmentverify.VerdictRejected {
		t.Fatalf("verdict = %q, want rejected on the assessment-evidence gate", result.Verdict)
	}
	found := false
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == environmentverify.CodeAssessmentEvidencePending {
			found = true
		}
	}
	if !found {
		t.Fatalf("diagnostics = %#v, want the assessment-evidence gate", result.Diagnostics)
	}
	if result.Promotion.Allowed {
		t.Fatal("promotion allowed without assessment evidence")
	}
}

func TestHandoffRejectsMalformedManifest(t *testing.T) {
	root := t.TempDir()
	writeHandoffFixture(t, root, ManifestFileName, `{"schemaVersion":`)
	builder := ToBuilderResult("build_handoff_malformed", "purpose-built", "test", nil, ManifestFileName, nil, envbuild.BuilderTelemetry{})
	runner := &LocalCommandRunner{}
	result := VerifyWorkspace(t.Context(), runner, root, ManifestFileName, builder.BuilderRunID, "verify_handoff_malformed", builder)
	if result.Verdict != environmentverify.VerdictRejected {
		t.Fatalf("verdict = %q, want rejected", result.Verdict)
	}
	if result.FailedStage == nil || *result.FailedStage != environmentverify.StageManifest {
		t.Fatalf("failed stage = %#v, want manifest", result.FailedStage)
	}
	if len(runner.Calls) != 0 {
		t.Fatalf("executed %d stages for a malformed manifest", len(runner.Calls))
	}
	if result.Promotion.Allowed {
		t.Fatal("promotion allowed for a malformed manifest")
	}
}

func TestHandoffReportsFailingBuild(t *testing.T) {
	root := t.TempDir()
	writeHandoffFixture(t, root, "solution.py", "VALUE = 1\n")
	writeHandoffFixture(t, root, "protected/test_demo.py", "assert True\n")
	writeHandoffFixture(t, root, ".codegym/hidden/test_hidden.py", "assert True\n")
	manifest := passingManifest()
	manifest.Commands.Build = "false"
	manifestPath, err := WriteManifest(root, manifest)
	if err != nil {
		t.Fatal(err)
	}
	builder := ToBuilderResult("build_handoff_failbuild", "purpose-built", "test", nil, manifestPath, nil, envbuild.BuilderTelemetry{})
	runner := &LocalCommandRunner{}
	result := VerifyWorkspace(t.Context(), runner, root, manifestPath, builder.BuilderRunID, "verify_handoff_failbuild", builder)
	if result.Verdict != environmentverify.VerdictRejected {
		t.Fatalf("verdict = %q, want rejected", result.Verdict)
	}
	if result.FailedStage == nil || *result.FailedStage != environmentverify.StageBuild {
		t.Fatalf("failed stage = %#v, want build", result.FailedStage)
	}
	seen := map[string]int{}
	for _, check := range result.Evidence.Checks {
		seen[check.Stage] = check.ExitCode
	}
	if seen["setup"] != 0 || seen["build"] == 0 {
		t.Fatalf("evidence = %#v", result.Evidence)
	}
	if result.Promotion.Allowed {
		t.Fatal("promotion allowed for a failed build")
	}
}

func TestBuilderDraftPassesVerifierValidation(t *testing.T) {
	root := t.TempDir()
	writeHandoffFixture(t, root, "src/app.js", "export const app = 1;")
	writeHandoffFixture(t, root, "src/app.test.js", "test app")
	writeHandoffFixture(t, root, ".codegym/hidden/secret.js", "hidden")
	writeHandoffFixture(t, root, ".codegym/commands.json", `{"setup":"npm install","build":"npm run build","test":"npm test","base":"node-25"}`)
	manifest, _, err := envbuild.DraftManifest(envbuild.BuildRequest{Technology: "Demo", Objective: "learn demo"}, root)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := WriteManifest(root, manifest)
	_ = payload
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, ManifestFileName))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := environmentverify.ParseManifest(raw)
	if err != nil {
		t.Fatalf("builder output rejected by verifier parser: %v", err)
	}
	if err := environmentverify.Validate(parsed); err != nil {
		t.Fatalf("builder output rejected by verifier validation: %v", err)
	}
	if !strings.Contains(manifest.Artifact.ID, "demo-") {
		t.Fatalf("artifact id = %q", manifest.Artifact.ID)
	}
}
