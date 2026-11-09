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

// PracticeLocalRunner implements the practice runner with local command
// execution for development and CI. Each execution ID gets an isolated
// directory under Root that persists across that operation's steps; the
// Root itself is test-owned so cleanup is automatic.
type PracticeLocalRunner struct {
	Root           string
	MaxOutputBytes int
	// Snapshots, when set, seeds each new execution space with the sealed
	// snapshot so build files and hidden tests are present, matching the
	// Daytona runner that boots from the snapshot. Learner files overlay it.
	Snapshots func(reference string) (string, error)

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
		if r.Snapshots != nil {
			snapshotRoot, err := r.Snapshots(spec.SnapshotRef)
			if err == nil {
				err = copyTree(snapshotRoot, dir)
			}
			if err != nil {
				r.mu.Unlock()
				os.RemoveAll(dir)
				return practice.RunOutcome{}, fmt.Errorf("envintegration: seed execution space: %w", err)
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
