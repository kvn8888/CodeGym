package envintegration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/kvn8888/codegym/backend/internal/environmentregistry"
	"github.com/kvn8888/codegym/backend/internal/environmentverify"
	"github.com/kvn8888/codegym/backend/internal/practice"
)

// RegistryArtifactSource implements the practice artifact source against the
// registry store. The manifest travels with the snapshot contents, never
// from learner input: it is read from the sealed snapshot and parsed before
// the service checks it.
type RegistryArtifactSource struct {
	Store    environmentregistry.Store
	Contents practice.ContentStore
}

// LoadVerifiedArtifact loads the pinned record plus its sealed manifest.
func (s *RegistryArtifactSource) LoadVerifiedArtifact(ctx context.Context, owner string, ref practice.ArtifactRef) (practice.VerifiedArtifact, error) {
	if s == nil || s.Store == nil {
		return practice.VerifiedArtifact{}, fmt.Errorf("envintegration: artifact store is required")
	}
	version := strings.TrimSpace(ref.Version)
	technology := strings.TrimSpace(ref.Technology)
	var record environmentregistry.ArtifactRecord
	if version != "" {
		loaded, err := s.Store.GetArtifact(ctx, ref.ArtifactID, version)
		if err != nil {
			return practice.VerifiedArtifact{}, err
		}
		record = loaded
	} else {
		listed, err := s.Store.ListByTechnology(ctx, technology)
		if err != nil {
			return practice.VerifiedArtifact{}, err
		}
		if len(listed) == 0 {
			return practice.VerifiedArtifact{}, environmentregistry.ErrArtifactNotFound
		}
		record = listed[0]
	}
	raw, err := s.Contents.ReadArtifactFile(ctx, record.SnapshotRef, ManifestFileName)
	if err != nil {
		return practice.VerifiedArtifact{}, fmt.Errorf("envintegration: read sealed manifest: %w", err)
	}
	manifest, err := environmentverify.ParseManifest([]byte(raw))
	if err != nil {
		return practice.VerifiedArtifact{}, fmt.Errorf("envintegration: parse sealed manifest: %w", err)
	}
	return practice.VerifiedArtifact{Record: record, Manifest: manifest}, nil
}

// SnapshotContents implements the practice content store against snapshot
// directories. Paths are confined to the snapshot root.
type SnapshotContents struct {
	Roots func(reference string) (string, error)
}

// ReadArtifactFile reads one sealed snapshot file.
func (s *SnapshotContents) ReadArtifactFile(_ context.Context, snapshotRef, filePath string) (string, error) {
	if s == nil || s.Roots == nil {
		return "", fmt.Errorf("envintegration: snapshot resolver is required")
	}
	clean := filepath.Clean(filepath.FromSlash(strings.TrimSpace(filePath)))
	if clean == "" || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("envintegration: artifact path escapes the snapshot: %q", filePath)
	}
	root, err := s.Roots(snapshotRef)
	if err != nil {
		return "", err
	}
	payload, err := os.ReadFile(filepath.Join(root, clean))
	if err != nil {
		return "", fmt.Errorf("envintegration: read snapshot file: %w", err)
	}
	return string(payload), nil
}

// CopyTree materializes the sealed snapshot tree into dest, preserving
// relative paths. Only regular files are copied; symlinks and special
// files are skipped so a hostile artifact cannot escape the space through
// the local runner. Callers overlay learner files afterwards, matching
// the production runner where the snapshot is baked into the sandbox and
// scoped files arrive as the overlay.
func (s *SnapshotContents) CopyTree(_ context.Context, snapshotRef, dest string) error {
	if s == nil || s.Roots == nil {
		return fmt.Errorf("envintegration: snapshot resolver is required")
	}
	root, err := s.Roots(snapshotRef)
	if err != nil {
		return err
	}
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if entry.IsDir() {
			return os.MkdirAll(filepath.Join(dest, rel), 0o755)
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		full := filepath.Join(dest, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		return os.WriteFile(full, payload, 0o644)
	})
}

// PracticeLocalRunner implements the practice runner with local command
// execution for development and CI. Each execution ID gets an isolated
// directory under Root that persists across that operation's steps; the
// Root itself is test-owned so cleanup is automatic.
type PracticeLocalRunner struct {
	Root           string
	MaxOutputBytes int
	// Snapshots materializes the sealed artifact tree into each fresh
	// execution space before learner files overlay it. Nil keeps the
	// legacy single-file behavior (spaces hold scoped files only), which
	// suffices for exercises with no support files; multi-file projects
	// such as Spring Boot require it.
	Snapshots *SnapshotContents

	mu     sync.Mutex
	spaces map[string]string
}

// RunCommand executes one stage command in the operation's isolated space,
// writing the scoped files first. Filesystem state persists across steps of
// the same execution ID, matching the provider contract.
func (r *PracticeLocalRunner) RunCommand(ctx context.Context, spec practice.RunSpec) (practice.RunOutcome, error) {
	if strings.TrimSpace(spec.ExecutionID) == "" {
		return practice.RunOutcome{}, fmt.Errorf("envintegration: execution ID is required")
	}
	r.mu.Lock()
	if r.spaces == nil {
		r.spaces = map[string]string{}
	}
	space, ok := r.spaces[spec.ExecutionID]
	if !ok {
		dir, err := os.MkdirTemp(r.Root, "codegym-practice-exec-")
		if err != nil {
			r.mu.Unlock()
			return practice.RunOutcome{}, err
		}
		if r.Snapshots != nil && strings.TrimSpace(spec.SnapshotRef) != "" {
			// Copy under the same lock that publishes the space so a
			// concurrent step for this execution can never observe a
			// half-materialized workspace.
			if err := r.Snapshots.CopyTree(ctx, spec.SnapshotRef, dir); err != nil {
				_ = os.RemoveAll(dir)
				r.mu.Unlock()
				return practice.RunOutcome{}, fmt.Errorf("envintegration: materialize practice workspace: %w", err)
			}
		}
		space = dir
		r.spaces[spec.ExecutionID] = space
	}
	r.mu.Unlock()
	for _, file := range spec.Files {
		clean := filepath.Clean(filepath.FromSlash(file.Path))
		if clean == "" || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return practice.RunOutcome{}, fmt.Errorf("envintegration: run file escapes the workspace: %q", file.Path)
		}
		full := filepath.Join(space, clean)
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			return practice.RunOutcome{}, err
		}
		if err := os.WriteFile(full, []byte(file.Content), 0o600); err != nil {
			return practice.RunOutcome{}, err
		}
	}
	exitCode, output, timedOut, err := runLocalStageCommand(ctx, spec.Command, space, r.MaxOutputBytes)
	if err != nil {
		return practice.RunOutcome{}, err
	}
	return practice.RunOutcome{ExitCode: exitCode, Output: output, TimedOut: timedOut}, nil
}
