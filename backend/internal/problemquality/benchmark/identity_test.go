package benchmark

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kvn8888/codegym/backend/internal/problemquality"
)

func TestComputeContentIdentityPinsRelativeInputsAndDeclaredArtifactScope(t *testing.T) {
	c, config := loadFixture(t, fixtureDir)
	identity, err := computeContentIdentity(c, config)
	if err != nil {
		t.Fatal(err)
	}
	if identity.Algorithm != ContentIdentityAlgorithm {
		t.Fatalf("algorithm = %q", identity.Algorithm)
	}
	if identity.CaseDefinition.Path != "case.yaml" ||
		identity.Specification.Path != "spec.json" ||
		identity.RunConfig.Path == "" {
		t.Fatalf("identity paths = %+v/%+v/%+v", identity.CaseDefinition, identity.Specification, identity.RunConfig)
	}
	for name, digest := range map[string]string{
		"case":     identity.CaseDefinition.SHA256,
		"spec":     identity.Specification.SHA256,
		"config":   identity.RunConfig.SHA256,
		"artifact": identity.Artifact.SHA256,
	} {
		if len(digest) != 64 {
			t.Fatalf("%s digest = %q, want sha256 hex", name, digest)
		}
	}
	if identity.Reference.ID != "development-reference" || identity.Reference.SHA256 == "" {
		t.Fatalf("reference = %+v", identity.Reference)
	}
	if len(identity.CorrectAlternatives) != 2 || len(identity.WrongSolutions) != 3 {
		t.Fatalf("alternatives = %d wrong = %d", len(identity.CorrectAlternatives), len(identity.WrongSolutions))
	}
	artifactPaths := contentPaths(identity.Artifact.Files)
	for _, want := range []string{"manifest.json", "solution.py", "protected/test_order_tools.py"} {
		if _, ok := artifactPaths[want]; !ok {
			t.Fatalf("artifact inputs = %v, want %q", artifactPaths, want)
		}
	}
	for path := range artifactPaths {
		if strings.Contains(path, "__pycache__") {
			t.Fatalf("artifact identity included generated cache %q", path)
		}
		if filepath.IsAbs(path) || strings.HasPrefix(path, "/") {
			t.Fatalf("artifact path %q is not relative", path)
		}
	}
	for _, solution := range append(identity.CorrectAlternatives, identity.WrongSolutions...) {
		for _, file := range solution.Files {
			if filepath.IsAbs(file.Path) || strings.HasPrefix(file.Path, "/") {
				t.Fatalf("solution path %q is not relative", file.Path)
			}
		}
	}
}

func TestComputeContentIdentityChangesWhenDeclaredContentChanges(t *testing.T) {
	dir := copyFixture(t)
	identity := loadIdentity(t, dir)
	originalArtifact := identity.Artifact.SHA256
	originalAlt01 := identity.CorrectAlternatives[0].SHA256
	originalAlt02 := identity.CorrectAlternatives[1].SHA256
	originalSpec := identity.Specification.SHA256

	appendFile(t, filepath.Join(dir, "correct", "alt-01", "solution.py"), "\n# changed\n")
	identity = loadIdentity(t, dir)
	if identity.CorrectAlternatives[0].SHA256 == originalAlt01 {
		t.Fatal("changed correct alternative digest did not change")
	}
	if identity.CorrectAlternatives[1].SHA256 != originalAlt02 {
		t.Fatal("unchanged correct alternative digest changed")
	}
	if identity.Artifact.SHA256 != originalArtifact {
		t.Fatal("solution change altered the artifact input digest")
	}

	appendFile(t, filepath.Join(dir, "spec.json"), "\n")
	identity = loadIdentity(t, dir)
	if identity.Specification.SHA256 == originalSpec {
		t.Fatal("changed specification digest did not change")
	}

	originalArtifact = identity.Artifact.SHA256
	appendFile(t, filepath.Join(dir, "artifact", "protected", "test_order_tools.py"), "\n# changed\n")
	identity = loadIdentity(t, dir)
	if identity.Artifact.SHA256 == originalArtifact {
		t.Fatal("changed protected assessment file did not change the artifact digest")
	}

	originalConfig := identity.RunConfig.SHA256
	appendFile(t, filepath.Join(dir, RunConfigFileName), "\n")
	identity = loadIdentity(t, dir)
	if identity.RunConfig.SHA256 == originalConfig {
		t.Fatal("changed run config digest did not change")
	}
}

func TestReferenceRelativePathsNeverReportAbsoluteMachinePaths(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "reference", "solution.py")
	config := &RunConfig{
		Dir:       dir,
		Reference: ReferenceConfig{Files: map[string]string{"solution.py": source}},
	}
	paths := referenceRelativePaths(config)
	if paths["solution.py"] != "reference/solution.py" {
		t.Fatalf("reference path = %q, want a config-relative path", paths["solution.py"])
	}
}

func TestComputeContentIdentityIgnoresUndeclaredGeneratedCache(t *testing.T) {
	dir := copyFixture(t)
	before := loadIdentity(t, dir)
	cacheDir := filepath.Join(dir, "artifact", "__pycache__")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "generated.pyc"), []byte("cache"), 0o644); err != nil {
		t.Fatal(err)
	}
	after := loadIdentity(t, dir)
	if after.Artifact.SHA256 != before.Artifact.SHA256 {
		t.Fatal("undeclared generated cache changed the artifact digest")
	}
}

func loadFixture(t *testing.T, dir string) (*problemquality.Case, *RunConfig) {
	t.Helper()
	c, err := problemquality.LoadCase(dir)
	if err != nil {
		t.Fatal(err)
	}
	config, err := LoadRunConfig(filepath.Join(c.Dir, RunConfigFileName), c)
	if err != nil {
		t.Fatal(err)
	}
	return c, config
}

func loadIdentity(t *testing.T, dir string) *ContentIdentity {
	t.Helper()
	c, config := loadFixture(t, dir)
	identity, err := computeContentIdentity(c, config)
	if err != nil {
		t.Fatal(err)
	}
	return identity
}

func contentPaths(files []ContentFile) map[string]struct{} {
	paths := make(map[string]struct{}, len(files))
	for _, file := range files {
		paths[file.Path] = struct{}{}
	}
	return paths
}

func copyFixture(t *testing.T) string {
	t.Helper()
	source := fixtureDir
	target := filepath.Join(t.TempDir(), "case")
	err := filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, rel)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return err
		}
		return os.WriteFile(destination, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return target
}

func appendFile(t *testing.T, path, content string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	if _, err := file.WriteString(content); err != nil {
		t.Fatal(err)
	}
}
