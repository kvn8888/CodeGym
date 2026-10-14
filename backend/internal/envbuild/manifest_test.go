package envbuild

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeManifestFixture(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func containsExactly(list []string, want ...string) bool {
	if len(list) != len(want) {
		return false
	}
	seen := map[string]bool{}
	for _, item := range list {
		seen[item] = true
	}
	for _, item := range want {
		if !seen[item] {
			return false
		}
	}
	return true
}

func TestDraftManifestMixedWorkspace(t *testing.T) {
	root := t.TempDir()
	writeManifestFixture(t, root, "src/app.js", "export const app = 1;")
	writeManifestFixture(t, root, "src/app.test.js", "test app")
	writeManifestFixture(t, root, ".codegym/hidden/secret.js", "hidden")
	writeManifestFixture(t, root, ".codegym/reference/solution.js", "reference")
	writeManifestFixture(t, root, "package.json", `{"name":"demo"}`)
	writeManifestFixture(t, root, ".codegym/commands.json", `{"setup":"npm install","build":"npm run build","test":"npm test","run":"npm start"}`)
	manifest, diagnostics, err := DraftManifest(BuildRequest{Technology: "TypeScript Express", Objective: "learn routing"}, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want declared commands without fallback", diagnostics)
	}
	if !containsExactly(manifest.Workspace.LearnerEditable, "src/app.js") {
		t.Fatalf("editable = %#v", manifest.Workspace.LearnerEditable)
	}
	if !containsExactly(manifest.Workspace.Protected, "src/app.test.js", ".codegym/hidden/secret.js", ".codegym/reference/solution.js") {
		t.Fatalf("protected = %#v", manifest.Workspace.Protected)
	}
	if !containsExactly(manifest.Environment.DependencyFiles, "package.json") {
		t.Fatalf("dependencyFiles = %#v", manifest.Environment.DependencyFiles)
	}
	if manifest.Commands.Setup != "npm install" || manifest.Commands.Test != "npm test" {
		t.Fatalf("commands = %#v", manifest.Commands)
	}
	if !containsExactly(manifest.Assessment.VisibleTests, "src/app.test.js") {
		t.Fatalf("visibleTests = %#v", manifest.Assessment.VisibleTests)
	}
	if !containsExactly(manifest.Assessment.HiddenTests, ".codegym/hidden/secret.js") {
		t.Fatalf("hiddenTests = %#v", manifest.Assessment.HiddenTests)
	}
	if manifest.SchemaVersion != "environment.manifest.v1" || manifest.Artifact.Version != "0.1.0" {
		t.Fatalf("identity = %#v", manifest.Artifact)
	}
	if !strings.HasPrefix(manifest.Artifact.ID, "typescript-express-") {
		t.Fatalf("artifact id = %q", manifest.Artifact.ID)
	}
	if payload, err := json.Marshal(manifest); err != nil {
		t.Fatal(err)
	} else {
		var decoded EnvironmentManifest
		if err := json.Unmarshal(payload, &decoded); err != nil {
			t.Fatalf("manifest does not round-trip: %v", err)
		}
	}
}

func TestDraftManifestFallsBackToDetectedCommands(t *testing.T) {
	root := t.TempDir()
	writeManifestFixture(t, root, "src/Main.java", "class Main {}")
	writeManifestFixture(t, root, "pom.xml", "<project/>")
	manifest, diagnostics, err := DraftManifest(BuildRequest{Technology: "Spring Boot", Objective: "learn DI"}, root)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Commands.Test != "mvn -q test" || manifest.Commands.Build != "mvn -q test-compile" {
		t.Fatalf("commands = %#v", manifest.Commands)
	}
	found := false
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "COMMANDS_DETECTED" && diagnostic.Severity == "warning" {
			found = true
		}
	}
	if !found {
		t.Fatalf("diagnostics = %#v, want fallback annotation", diagnostics)
	}
}

func TestDraftManifestRefusesEmptyWorkspace(t *testing.T) {
	manifest, diagnostics, err := DraftManifest(BuildRequest{Technology: "Widget Framework", Objective: "learn widgets"}, t.TempDir())
	if err == nil {
		t.Fatal("expected drafting error for an empty workspace")
	}
	if manifest != nil {
		t.Fatalf("manifest = %#v, want nil", manifest)
	}
	found := false
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "NO_PROJECT_FILES" {
			found = true
		}
	}
	if !found {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
}
