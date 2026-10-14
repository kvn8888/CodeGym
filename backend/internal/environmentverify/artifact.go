package environmentverify

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// Path-resolution convention (proposed — the shared contract does not define
// how manifest paths map onto artifact files, so both tracks should review
// this before it becomes shared policy):
//
//   - artifactRoot is a backend-owned absolute directory holding the prepared
//     artifact. The backend chooses it; the manifest never does.
//   - workspaceDir is artifactRoot joined with workspace.root.
//     workspace.root "." means the artifact root itself and is valid, not a
//     placeholder or an error.
//   - Every file entry in environment.dependencyFiles,
//     environment.lockfiles, workspace.learnerEditable, workspace.protected,
//     assessment.visibleTests, and assessment.hiddenTests is a
//     forward-slash-separated path relative to workspaceDir. Absolute paths
//     are rejected.
//   - Containment is enforced twice: lexically (cleaned join must stay
//     inside the artifact root) and after symlink resolution (the resolved
//     target must stay inside the artifact root). Internal symlinks are
//     allowed; only escapes are rejected.
//   - Identity for overlap and hidden-test rules is the filesystem identity
//     (device plus inode where the platform exposes it) falling back to the
//     symlink-resolved absolute path, so lexical variants ("a/./b",
//     "x/../a/b"), symlink aliases, and hard-link aliases of the same file
//     compare equal and cannot bypass editable/protected separation.
//
// Alternatives considered: resolving dependency files against the artifact
// root instead of workspaceDir. The single-base rule above is simpler to
// audit and identical whenever workspace.root is "."; if Track A needs
// split bases, the contract should say so explicitly.
//
// ValidateArtifact checks a structurally validated manifest against the
// files actually present under a backend-provided artifact root. It is
// read-only: it stats and resolves paths, never writes, moves, or deletes
// files, and never executes manifest commands. Protected files stay in the
// verification artifact; they are only required to exist as regular files
// and to be disjoint from the learner-editable set.
//
// A nil error means the artifact matches its manifest, not that the
// environment is ready or promotable.
func ValidateArtifact(artifactRoot string, manifest Manifest) error {
	if err := Validate(manifest); err != nil {
		return err
	}
	root, err := resolveArtifactRoot(artifactRoot)
	if err != nil {
		return err
	}
	_, err = validateArtifactContents(root, manifest)
	return err
}

// PlatformError reports an infrastructure fault encountered while checking
// the artifact (permissions, I/O, unresolvable links that are present).
// Unlike a missing file, which is a validation failure, a PlatformError
// tells the verifier to report a platform fault with the underlying error
// preserved instead of mislabeling it.
type PlatformError struct {
	Op    string
	Field string
	Path  string
	Err   error
}

func (e *PlatformError) Error() string {
	return fmt.Sprintf("environment artifact: %s %s %q: %v", e.Op, e.Field, e.Path, e.Err)
}

func (e *PlatformError) Unwrap() error { return e.Err }

// ArtifactError classifies a manifest/artifact mismatch with a stable code
// plus the offending field and entry, so the verifier can build actionable
// diagnostics without parsing message text.
type ArtifactError struct {
	Code  string
	Field string
	Entry string
	msg   string
}

func (e *ArtifactError) Error() string { return e.msg }

// resolveArtifactRoot normalizes a backend-provided artifact root. Failures
// here are the backend's infrastructure, not the builder's manifest.
func resolveArtifactRoot(artifactRoot string) (string, error) {
	if strings.TrimSpace(artifactRoot) == "" {
		return "", &PlatformError{Op: "resolve", Field: "artifact root", Path: artifactRoot, Err: errors.New("artifact root is required")}
	}
	rootAbs, err := filepath.Abs(artifactRoot)
	if err != nil {
		return "", &PlatformError{Op: "resolve", Field: "artifact root", Path: artifactRoot, Err: err}
	}
	root, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		if os.IsNotExist(err) {
			return "", &PlatformError{Op: "resolve", Field: "artifact root", Path: artifactRoot, Err: err}
		}
		return "", &PlatformError{Op: "resolve", Field: "artifact root", Path: artifactRoot, Err: err}
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		if os.IsNotExist(err) {
			return "", &PlatformError{Op: "resolve", Field: "artifact root", Path: artifactRoot, Err: err}
		}
		return "", &PlatformError{Op: "resolve", Field: "artifact root", Path: artifactRoot, Err: fmt.Errorf("not a directory: %w", err)}
	}
	return root, nil
}

// validateArtifactContents runs the manifest's file checks against an
// already-resolved root and returns the resolved workspace directory for
// backend-chosen command execution.
func validateArtifactContents(root string, manifest Manifest) (string, error) {
	workspaceDir, err := resolveWorkspaceDir(root, strings.TrimSpace(manifest.Workspace.Root))
	if err != nil {
		return "", err
	}

	editable, err := resolveFileList(root, workspaceDir, "workspace.learnerEditable", manifest.Workspace.LearnerEditable)
	if err != nil {
		return "", err
	}
	protected, err := resolveFileList(root, workspaceDir, "workspace.protected", manifest.Workspace.Protected)
	if err != nil {
		return "", err
	}
	if _, err := resolveFileList(root, workspaceDir, "environment.dependencyFiles", manifest.Environment.DependencyFiles); err != nil {
		return "", err
	}
	if _, err := resolveFileList(root, workspaceDir, "environment.lockfiles", manifest.Environment.Lockfiles); err != nil {
		return "", err
	}
	if _, err := resolveFileList(root, workspaceDir, "assessment.visibleTests", manifest.Assessment.VisibleTests); err != nil {
		return "", err
	}
	hidden, err := resolveFileList(root, workspaceDir, "assessment.hiddenTests", manifest.Assessment.HiddenTests)
	if err != nil {
		return "", err
	}

	for _, file := range editable {
		if other, ok := findSameFile(protected, file); ok {
			return "", &ArtifactError{
				Code: CodeBoundaryViolation, Field: "workspace.learnerEditable", Entry: file.declared,
				msg: fmt.Sprintf("environment artifact: workspace file %q (workspace.learnerEditable) resolves to the same file as workspace.protected entry %q", file.declared, other.declared),
			}
		}
	}
	for _, file := range hidden {
		if _, ok := findSameFile(editable, file); ok {
			return "", &ArtifactError{
				Code: CodeHiddenTestEditable, Field: "assessment.hiddenTests", Entry: file.declared,
				msg: fmt.Sprintf("environment artifact: assessment.hiddenTests entry %q must not be learner-editable", file.declared),
			}
		}
		if _, ok := findSameFile(protected, file); !ok {
			return "", &ArtifactError{
				Code: CodeHiddenTestUnprotected, Field: "assessment.hiddenTests", Entry: file.declared,
				msg: fmt.Sprintf("environment artifact: assessment.hiddenTests entry %q must be listed in workspace.protected", file.declared),
			}
		}
	}
	return workspaceDir, nil
}

// resolvedFile pairs a manifest entry with its symlink-resolved absolute
// path and, where the platform exposes it, its filesystem identity. The
// declared form is kept so errors can quote what the builder wrote while
// comparisons use identity.
type resolvedFile struct {
	declared  string
	canonical string
	dev       uint64
	ino       uint64
	hasID     bool
}

// sameFile reports whether two resolved entries are the same file: equal
// resolved paths, or equal device-plus-inode identity (hard-link aliases).
func sameFile(a, b resolvedFile) bool {
	if a.canonical == b.canonical {
		return true
	}
	return a.hasID && b.hasID && a.dev == b.dev && a.ino == b.ino
}

func findSameFile(haystack []resolvedFile, needle resolvedFile) (resolvedFile, bool) {
	for _, candidate := range haystack {
		if sameFile(candidate, needle) {
			return candidate, true
		}
	}
	return resolvedFile{}, false
}

// fileIdentity extracts device-plus-inode identity where supported.
func fileIdentity(info os.FileInfo) (dev, ino uint64, ok bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, false
	}
	return uint64(stat.Dev), stat.Ino, true
}

func resolveWorkspaceDir(root, raw string) (string, error) {
	field := "workspace.root"
	rel := filepath.FromSlash(raw)
	if filepath.IsAbs(rel) {
		return "", &ArtifactError{Code: CodeArtifactPathUnsafe, Field: field, Entry: raw,
			msg: fmt.Sprintf("environment artifact: %s %q: absolute paths are not allowed", field, raw)}
	}
	joined := filepath.Join(root, rel)
	if escapesRoot(root, joined) {
		return "", &ArtifactError{Code: CodeArtifactPathUnsafe, Field: field, Entry: raw,
			msg: fmt.Sprintf("environment artifact: %s %q escapes the artifact root", field, raw)}
	}
	canonical, err := filepath.EvalSymlinks(joined)
	if err != nil {
		if pathMissing(joined) {
			return "", &ArtifactError{Code: CodeArtifactFileMissing, Field: field, Entry: raw,
				msg: fmt.Sprintf("environment artifact: %s %q: no such directory", field, raw)}
		}
		return "", &PlatformError{Op: "resolve", Field: field, Path: raw, Err: err}
	}
	if escapesRoot(root, canonical) {
		return "", &ArtifactError{Code: CodeArtifactPathUnsafe, Field: field, Entry: raw,
			msg: fmt.Sprintf("environment artifact: %s %q: symlink escapes the artifact root", field, raw)}
	}
	info, err := os.Stat(canonical)
	if err != nil {
		if os.IsNotExist(err) {
			return "", &ArtifactError{Code: CodeArtifactFileMissing, Field: field, Entry: raw,
				msg: fmt.Sprintf("environment artifact: %s %q: no such directory", field, raw)}
		}
		return "", &PlatformError{Op: "stat", Field: field, Path: raw, Err: err}
	}
	if !info.IsDir() {
		return "", &ArtifactError{Code: CodeArtifactTypeMismatch, Field: field, Entry: raw,
			msg: fmt.Sprintf("environment artifact: %s %q is not a directory", field, raw)}
	}
	return canonical, nil
}

// resolveFileList resolves every entry of one manifest file list against
// the workspace directory. Each entry must exist as a regular file inside
// the artifact root, reachable without absolute paths or escapes.
// Infrastructure faults surface as PlatformError with the underlying error
// preserved; they must not be mislabeled as missing files.
func resolveFileList(root, workspaceDir, field string, entries []string) ([]resolvedFile, error) {
	resolved := make([]resolvedFile, 0, len(entries))
	for _, entry := range entries {
		raw := strings.TrimSpace(entry)
		rel := filepath.FromSlash(raw)
		if filepath.IsAbs(rel) {
			return nil, &ArtifactError{Code: CodeArtifactPathUnsafe, Field: field, Entry: entry,
				msg: fmt.Sprintf("environment artifact: %s entry %q: absolute paths are not allowed", field, entry)}
		}
		joined := filepath.Join(workspaceDir, rel)
		if escapesRoot(root, joined) {
			return nil, &ArtifactError{Code: CodeArtifactPathUnsafe, Field: field, Entry: entry,
				msg: fmt.Sprintf("environment artifact: %s entry %q escapes the artifact root", field, entry)}
		}
		canonical, err := filepath.EvalSymlinks(joined)
		if err != nil {
			if pathMissing(joined) {
				return nil, &ArtifactError{Code: CodeArtifactFileMissing, Field: field, Entry: entry,
					msg: fmt.Sprintf("environment artifact: %s entry %q: no such file", field, entry)}
			}
			return nil, &PlatformError{Op: "resolve", Field: field, Path: entry, Err: err}
		}
		if escapesRoot(root, canonical) {
			return nil, &ArtifactError{Code: CodeArtifactPathUnsafe, Field: field, Entry: entry,
				msg: fmt.Sprintf("environment artifact: %s entry %q: symlink escapes the artifact root", field, entry)}
		}
		info, err := os.Stat(canonical)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, &ArtifactError{Code: CodeArtifactFileMissing, Field: field, Entry: entry,
					msg: fmt.Sprintf("environment artifact: %s entry %q: no such file", field, entry)}
			}
			return nil, &PlatformError{Op: "stat", Field: field, Path: entry, Err: err}
		}
		if !info.Mode().IsRegular() {
			return nil, &ArtifactError{Code: CodeArtifactTypeMismatch, Field: field, Entry: entry,
				msg: fmt.Sprintf("environment artifact: %s entry %q is not a regular file", field, entry)}
		}
		dev, ino, hasID := fileIdentity(info)
		resolved = append(resolved, resolvedFile{declared: entry, canonical: canonical, dev: dev, ino: ino, hasID: hasID})
	}
	return resolved, nil
}

// escapesRoot reports whether joined lies outside root. Both must already
// be absolute and cleaned; callers compare like with like (lexical joins
// against the lexical root, resolved targets against the resolved root).
func escapesRoot(root, joined string) bool {
	rel, err := filepath.Rel(root, joined)
	if err != nil {
		return true
	}
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// pathMissing reports whether a path is absent, including dangling symlinks
// whose own link is present but whose target is gone. Present-but-broken
// infrastructure (permissions, loops) is not missing.
func pathMissing(path string) bool {
	if _, err := os.Lstat(path); err != nil {
		return os.IsNotExist(err)
	}
	_, err := os.Stat(path)
	return os.IsNotExist(err)
}
