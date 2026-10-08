package envbuild

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckPrerequisitesUnknownTechnology(t *testing.T) {
	prerequisites, err := CheckPrerequisites(t.Context(), "Some Unseen Framework")
	if err != nil {
		t.Fatal(err)
	}
	if len(prerequisites) != 0 {
		t.Fatalf("prerequisites = %#v, want none", prerequisites)
	}
	if missing := MissingRequired(prerequisites); missing.ID != "" {
		t.Fatalf("missing = %#v, want none", missing)
	}
}

func TestCheckPrerequisitesSpringBootMissingCache(t *testing.T) {
	t.Setenv("CODEGYM_MAVEN_REPO", t.TempDir())
	prerequisites, err := CheckPrerequisites(t.Context(), "Spring Boot")
	if err != nil {
		t.Fatal(err)
	}
	if len(prerequisites) != 1 {
		t.Fatalf("prerequisites = %#v, want one row", prerequisites)
	}
	missing := MissingRequired(prerequisites)
	if missing.ID != "spring-boot-dependency-cache" {
		t.Fatalf("missing = %#v", missing)
	}
	diagnostic := MissingPrerequisiteDiagnostic(missing)
	if diagnostic.Severity != "error" || diagnostic.Stage != "prepare" || diagnostic.Code != "PREREQUISITE_MISSING" {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
	if !strings.Contains(diagnostic.Message, "spring-boot-starter-parent:3.3.5") {
		t.Fatalf("diagnostic message = %q", diagnostic.Message)
	}
}

func TestCheckPrerequisitesSpringBootSatisfiedCache(t *testing.T) {
	repository := t.TempDir()
	pom := filepath.Join(repository, "org", "springframework", "boot", "spring-boot-starter-parent", "3.3.5", "spring-boot-starter-parent-3.3.5.pom")
	if err := os.MkdirAll(filepath.Dir(pom), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pom, []byte("<project/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEGYM_MAVEN_REPO", repository)
	prerequisites, err := CheckPrerequisites(t.Context(), "spring-boot")
	if err != nil {
		t.Fatal(err)
	}
	if missing := MissingRequired(prerequisites); missing.ID != "" {
		t.Fatalf("missing = %#v, want none", missing)
	}
}

func TestEnvironmentManifestMatchesContractExample(t *testing.T) {
	payload, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "examples", "environment-contract", "valid-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest EnvironmentManifest
	if err := json.Unmarshal(payload, &manifest); err != nil {
		t.Fatalf("contract example does not decode into EnvironmentManifest: %v", err)
	}
	if manifest.SchemaVersion != "environment.manifest.v1" {
		t.Fatalf("schemaVersion = %q", manifest.SchemaVersion)
	}
	if manifest.Artifact.ID == "" || manifest.Artifact.Version == "" {
		t.Fatalf("artifact identity = %#v", manifest.Artifact)
	}
	if len(manifest.Workspace.LearnerEditable) == 0 || len(manifest.Workspace.Protected) == 0 {
		t.Fatalf("workspace split = %#v", manifest.Workspace)
	}
	if manifest.Commands.Setup == "" || manifest.Commands.Build == "" || manifest.Commands.Test == "" {
		t.Fatalf("commands = %#v", manifest.Commands)
	}
	if len(manifest.Environment.DependencyFiles) == 0 {
		t.Fatalf("environment = %#v", manifest.Environment)
	}
}
