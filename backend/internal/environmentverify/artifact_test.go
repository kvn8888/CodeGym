package environmentverify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// manifestPaths collects every declared file path across the six manifest
// file lists.
func manifestPaths(manifest Manifest) []string {
	var paths []string
	paths = append(paths, manifest.Environment.DependencyFiles...)
	paths = append(paths, manifest.Environment.Lockfiles...)
	paths = append(paths, manifest.Workspace.LearnerEditable...)
	paths = append(paths, manifest.Workspace.Protected...)
	paths = append(paths, manifest.Assessment.VisibleTests...)
	paths = append(paths, manifest.Assessment.HiddenTests...)
	return paths
}

// materializeArtifact writes an empty regular file for every declared
// manifest path under root/workspace-root, then returns the workspace
// directory. Callers mutate the manifest first so only the intended
// violation remains.
func materializeArtifact(t *testing.T, root string, manifest Manifest) {
	t.Helper()
	workspaceDir := filepath.Join(root, filepath.FromSlash(strings.TrimSpace(manifest.Workspace.Root)))
	for _, declared := range manifestPaths(manifest) {
		full := filepath.Join(workspaceDir, filepath.FromSlash(strings.TrimSpace(declared)))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte("test"), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}
}

func requireArtifactError(t *testing.T, root string, manifest Manifest, wants ...string) error {
	t.Helper()
	err := ValidateArtifact(root, manifest)
	if err == nil {
		t.Fatalf("ValidateArtifact = nil, want error mentioning %q", strings.Join(wants, ", "))
	}
	for _, want := range wants {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("ValidateArtifact = %q, want it to mention %q", err.Error(), want)
		}
	}
	return err
}

func TestValidateArtifactValid(t *testing.T) {
	// workspace.root "." resolves to the artifact root itself: valid, not
	// a placeholder. The checked-in example also ships lockfiles: [].
	manifest := mustValidManifest(t)
	root := t.TempDir()
	materializeArtifact(t, root, manifest)
	if err := ValidateArtifact(root, manifest); err != nil {
		t.Fatalf("ValidateArtifact(valid): %v", err)
	}
}

func TestValidateArtifactWorkspaceSubdirectory(t *testing.T) {
	// A non-"." workspace root resolves to that subdirectory of the
	// artifact root; declared paths stay relative to the workspace dir.
	manifest := mustValidManifest(t)
	manifest.Workspace.Root = "exercise"
	root := t.TempDir()
	materializeArtifact(t, root, manifest)
	if err := ValidateArtifact(root, manifest); err != nil {
		t.Fatalf("ValidateArtifact(subdirectory root): %v", err)
	}
}

func TestValidateArtifactMissingFiles(t *testing.T) {
	// Each victim is unique to its list so the missing-file error names
	// the list under test (lists resolve in a fixed order, and shared
	// paths would blame whichever list resolves first).
	cases := []struct {
		field  string
		victim func(*Manifest) string
	}{
		{"environment.dependencyFiles", func(m *Manifest) string { return m.Environment.DependencyFiles[0] }},
		{"environment.lockfiles", func(m *Manifest) string {
			m.Environment.Lockfiles = append(m.Environment.Lockfiles, "extra.lock")
			return "extra.lock"
		}},
		{"workspace.learnerEditable", func(m *Manifest) string { return m.Workspace.LearnerEditable[0] }},
		{"workspace.protected", func(m *Manifest) string {
			return ".codegym/reference/GreetingServiceReference.java"
		}},
		{"assessment.visibleTests", func(m *Manifest) string {
			m.Assessment.VisibleTests = append(m.Assessment.VisibleTests, "extra-visible.txt")
			return "extra-visible.txt"
		}},
		{"assessment.hiddenTests", func(m *Manifest) string {
			m.Assessment.HiddenTests = append(m.Assessment.HiddenTests, "extra-hidden.txt")
			return "extra-hidden.txt"
		}},
	}
	for _, tc := range cases {
		manifest := mustValidManifest(t)
		victim := tc.victim(&manifest)
		root := t.TempDir()
		materializeArtifact(t, root, manifest)
		full := filepath.Join(root, filepath.FromSlash(victim))
		if err := os.Remove(full); err != nil {
			t.Fatalf("remove %s: %v", full, err)
		}
		requireArtifactError(t, root, manifest, tc.field, "no such file")
	}
}

func TestValidateArtifactDirectoryFileMismatch(t *testing.T) {
	// A declared dependency file that is a directory is rejected.
	manifest := mustValidManifest(t)
	root := t.TempDir()
	materializeArtifact(t, root, manifest)
	dirAsFile := filepath.Join(root, filepath.FromSlash(manifest.Environment.DependencyFiles[0]))
	if err := os.Remove(dirAsFile); err != nil {
		t.Fatalf("remove %s: %v", dirAsFile, err)
	}
	if err := os.MkdirAll(dirAsFile, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dirAsFile, err)
	}
	requireArtifactError(t, root, manifest, "environment.dependencyFiles", "not a regular file")

	// A workspace root that is a regular file is rejected before any
	// declared files are consulted, so only the root file is materialized.
	fileRoot := mustValidManifest(t)
	fileRoot.Workspace.Root = "not-a-dir.txt"
	other := t.TempDir()
	if err := os.WriteFile(filepath.Join(other, "not-a-dir.txt"), []byte("test"), 0o644); err != nil {
		t.Fatalf("write root file: %v", err)
	}
	requireArtifactError(t, other, fileRoot, "workspace.root", "not a directory")
}

func TestValidateArtifactPathTraversal(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Manifest)
		wants  []string
	}{
		{
			name: "absolute protected path",
			mutate: func(m *Manifest) {
				m.Workspace.Protected = append(m.Workspace.Protected, "/etc/passwd")
			},
			wants: []string{"workspace.protected", "absolute paths are not allowed"},
		},
		{
			name: "parent escape in editable",
			mutate: func(m *Manifest) {
				m.Workspace.LearnerEditable = append(m.Workspace.LearnerEditable, "../outside.txt")
			},
			wants: []string{"workspace.learnerEditable", "escapes the artifact root"},
		},
		{
			name: "nested escape in hidden tests",
			mutate: func(m *Manifest) {
				m.Assessment.HiddenTests = append(m.Assessment.HiddenTests, "sub/../../outside.txt")
			},
			wants: []string{"assessment.hiddenTests", "escapes the artifact root"},
		},
		{
			name: "workspace root escapes",
			mutate: func(m *Manifest) {
				m.Workspace.Root = ".."
			},
			wants: []string{"workspace.root", "escapes the artifact root"},
		},
		{
			name: "workspace root absolute",
			mutate: func(m *Manifest) {
				m.Workspace.Root = "/opt/exercise"
			},
			wants: []string{"workspace.root", "absolute paths are not allowed"},
		},
	}
	for _, tc := range cases {
		manifest := mustValidManifest(t)
		tc.mutate(&manifest)
		root := t.TempDir()
		// Materialize only the legitimate declared files; traversal
		// targets must fail before any filesystem use.
		materializeArtifact(t, root, mustValidManifest(t))
		requireArtifactError(t, root, manifest, tc.wants...)
	}
}

func TestValidateArtifactSymlinkEscape(t *testing.T) {
	manifest := mustValidManifest(t)
	root := t.TempDir()
	materializeArtifact(t, root, manifest)

	outsideDir := t.TempDir()
	secret := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(secret, []byte("outside"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	link := filepath.Join(root, "evil.txt")
	if err := os.Symlink(secret, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	manifest.Workspace.LearnerEditable = append(manifest.Workspace.LearnerEditable, "evil.txt")
	requireArtifactError(t, root, manifest, "workspace.learnerEditable", "symlink escapes the artifact root")
}

func TestValidateArtifactNormalizedOverlap(t *testing.T) {
	// Lexically different entries resolving to the same file overlap, even
	// though structural validation (exact-string match) lets them through.
	manifest := mustValidManifest(t)
	manifest.Workspace.LearnerEditable = []string{"shared.txt"}
	manifest.Workspace.Protected = []string{"sub/../shared.txt"}
	root := t.TempDir()
	materializeArtifact(t, root, manifest)
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	err := ValidateArtifact(root, manifest)
	if err == nil {
		t.Fatalf("ValidateArtifact(normalized overlap) = nil, want error")
	}
	if !strings.Contains(err.Error(), "resolves to the same file") {
		t.Fatalf("ValidateArtifact(normalized overlap) = %q, want same-file overlap", err.Error())
	}
}

func TestValidateArtifactSymlinkAliasOverlap(t *testing.T) {
	// A symlink alias of a protected file in the editable set overlaps.
	manifest := mustValidManifest(t)
	manifest.Workspace.LearnerEditable = []string{"alias.txt"}
	manifest.Workspace.Protected = []string{"real.txt"}
	root := t.TempDir()
	materializeArtifact(t, root, manifest)
	if err := os.Remove(filepath.Join(root, "alias.txt")); err != nil {
		t.Fatalf("remove alias placeholder: %v", err)
	}
	if err := os.Symlink("real.txt", filepath.Join(root, "alias.txt")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	err := ValidateArtifact(root, manifest)
	if err == nil {
		t.Fatalf("ValidateArtifact(symlink alias overlap) = nil, want error")
	}
	if !strings.Contains(err.Error(), "resolves to the same file") {
		t.Fatalf("ValidateArtifact(symlink alias overlap) = %q, want same-file overlap", err.Error())
	}
}

func TestValidateArtifactHiddenMustBeProtected(t *testing.T) {
	manifest := mustValidManifest(t)
	hidden := manifest.Assessment.HiddenTests[0]
	kept := manifest.Workspace.Protected[:0:0]
	for _, p := range manifest.Workspace.Protected {
		if p != hidden {
			kept = append(kept, p)
		}
	}
	if len(kept) == len(manifest.Workspace.Protected) {
		t.Fatalf("example hidden test %q is not protected, cannot build fixture", hidden)
	}
	manifest.Workspace.Protected = kept
	root := t.TempDir()
	materializeArtifact(t, root, manifest)
	requireArtifactError(t, root, manifest, "assessment.hiddenTests", "must be listed in workspace.protected")
}

func TestValidateArtifactHiddenMustNotBeEditable(t *testing.T) {
	// A hidden test listed verbatim in both sets is rejected. The
	// structural exact-string overlap fires first here; the filesystem
	// alias case below isolates the artifact-level rules.
	direct := mustValidManifest(t)
	hidden := direct.Assessment.HiddenTests[0]
	direct.Workspace.LearnerEditable = append(direct.Workspace.LearnerEditable, hidden)
	directRoot := t.TempDir()
	materializeArtifact(t, directRoot, direct)
	err := ValidateArtifact(directRoot, direct)
	if err == nil {
		t.Fatalf("ValidateArtifact(editable hidden test) = nil, want error")
	}
	if !strings.Contains(err.Error(), hidden) || !strings.Contains(err.Error(), "protected") {
		t.Fatalf("ValidateArtifact(editable hidden test) = %q, want file and boundary", err.Error())
	}

	// Isolated hidden-editable rule: the hidden test stays out of
	// workspace.protected but is reachable through a learner-editable
	// symlink alias, so the "must not be learner-editable" rule fires.
	isolated := mustValidManifest(t)
	isolatedHidden := isolated.Assessment.HiddenTests[0]
	kept := make([]string, 0, len(isolated.Workspace.Protected))
	for _, p := range isolated.Workspace.Protected {
		if p != isolatedHidden {
			kept = append(kept, p)
		}
	}
	isolated.Workspace.Protected = kept
	isolated.Workspace.LearnerEditable = append(isolated.Workspace.LearnerEditable, "editable-link.txt")
	isolatedRoot := t.TempDir()
	materializeArtifact(t, isolatedRoot, isolated)
	if err := os.Remove(filepath.Join(isolatedRoot, "editable-link.txt")); err != nil {
		t.Fatalf("remove alias placeholder: %v", err)
	}
	if err := os.Symlink(
		filepath.FromSlash(isolatedHidden),
		filepath.Join(isolatedRoot, "editable-link.txt"),
	); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	err = ValidateArtifact(isolatedRoot, isolated)
	if err == nil {
		t.Fatalf("ValidateArtifact(editable hidden alias) = nil, want error")
	}
	if !strings.Contains(err.Error(), "must not be learner-editable") {
		t.Fatalf("ValidateArtifact(editable hidden alias) = %q, want learner-editable rule", err.Error())
	}
}

func TestValidateArtifactRejectsStructurallyInvalid(t *testing.T) {
	parsed, err := ParseManifest(loadExample(t, "malformed-manifest.json"))
	if err != nil {
		t.Fatalf("ParseManifest(malformed-manifest): %v", err)
	}
	if err := ValidateArtifact(t.TempDir(), parsed); err == nil {
		t.Fatalf("ValidateArtifact(malformed) = nil, want structural error")
	}
}

// Passing artifact checks must not establish readiness, authorize
// promotion, or execute manifest commands. ValidateArtifact returns only
// an error, leaves protected files in place, and never runs setup/build/
// test/run strings (even a command that would create a file if shelled
// out must have no effect).
func TestValidateArtifactGrantsNoReadiness(t *testing.T) {
	manifest := mustValidManifest(t)
	manifest.Commands.Setup = "touch PWNED-by-setup.txt"
	manifest.Commands.Build = "touch PWNED-by-build.txt"
	manifest.Commands.Test = "touch PWNED-by-test.txt"
	manifest.Commands.Run = "touch PWNED-by-run.txt"
	root := t.TempDir()
	materializeArtifact(t, root, manifest)
	if err := ValidateArtifact(root, manifest); err != nil {
		t.Fatalf("ValidateArtifact(valid): %v", err)
	}
	for _, planted := range []string{
		"PWNED-by-setup.txt", "PWNED-by-build.txt", "PWNED-by-test.txt", "PWNED-by-run.txt",
	} {
		if _, err := os.Stat(filepath.Join(root, planted)); !os.IsNotExist(err) {
			t.Fatalf("manifest command had an effect: %s exists", planted)
		}
	}
	// Protected files stay available in the verification artifact; they
	// are only excluded from the learner-editable set, never removed.
	for _, protected := range manifest.Workspace.Protected {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(protected))); err != nil {
			t.Fatalf("protected file %q missing after validation: %v", protected, err)
		}
	}
}

func TestValidateArtifactHardLinkOverlap(t *testing.T) {
	// A hard link shares device-plus-inode identity with its target under a
	// different path. Filesystem identity comparison must catch it even
	// though no path string or symlink matches.
	manifest := mustValidManifest(t)
	manifest.Workspace.LearnerEditable = []string{"real.txt"}
	manifest.Workspace.Protected = []string{"hardalias.txt"}
	root := t.TempDir()
	materializeArtifact(t, root, manifest)
	if err := os.Remove(filepath.Join(root, "hardalias.txt")); err != nil {
		t.Fatalf("remove alias placeholder: %v", err)
	}
	if err := os.Link(filepath.Join(root, "real.txt"), filepath.Join(root, "hardalias.txt")); err != nil {
		t.Fatalf("hard link: %v", err)
	}
	err := ValidateArtifact(root, manifest)
	if err == nil {
		t.Fatalf("ValidateArtifact(hard-link overlap) = nil, want error")
	}
	if !strings.Contains(err.Error(), "resolves to the same file") {
		t.Fatalf("ValidateArtifact(hard-link overlap) = %q, want same-file overlap", err.Error())
	}
}

func TestValidateArtifactHardLinkHiddenEditable(t *testing.T) {
	// A hidden test reachable through a learner-editable hard-link alias
	// must not bypass the editable exclusion, even while unprotected.
	manifest := mustValidManifest(t)
	hidden := manifest.Assessment.HiddenTests[0]
	kept := make([]string, 0, len(manifest.Workspace.Protected))
	for _, p := range manifest.Workspace.Protected {
		if p != hidden {
			kept = append(kept, p)
		}
	}
	manifest.Workspace.Protected = kept
	manifest.Workspace.LearnerEditable = append(manifest.Workspace.LearnerEditable, "hard-editable.txt")
	root := t.TempDir()
	materializeArtifact(t, root, manifest)
	if err := os.Remove(filepath.Join(root, "hard-editable.txt")); err != nil {
		t.Fatalf("remove alias placeholder: %v", err)
	}
	if err := os.Link(
		filepath.Join(root, filepath.FromSlash(hidden)),
		filepath.Join(root, "hard-editable.txt"),
	); err != nil {
		t.Fatalf("hard link: %v", err)
	}
	err := ValidateArtifact(root, manifest)
	if err == nil {
		t.Fatalf("ValidateArtifact(hard-link hidden alias) = nil, want error")
	}
	if !strings.Contains(err.Error(), "must not be learner-editable") {
		t.Fatalf("ValidateArtifact(hard-link hidden alias) = %q, want learner-editable rule", err.Error())
	}
}
