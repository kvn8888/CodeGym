package envintegration

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/kvn8888/codegym/backend/internal/agentruntime"
	"github.com/kvn8888/codegym/backend/internal/environmentverify"
)

// This file moves verification off the host. The verifier's contract is
// local by design: it walks an artifact root on disk and runs stage commands
// through a CommandRunner. A build sandbox has neither a host filesystem path
// nor host processes, so the corridor bridges the gap in two halves that keep
// the verifier unchanged:
//
//   - SandboxTreeMaterializer downloads the sandbox tree into a host temp
//     root, so the verifier's file reads (manifest, artifact validation) see
//     the real bytes.
//   - SandboxCommandRunner and SandboxAssessor execute stage and assessment
//     commands inside the sandbox, translating the verifier's host paths back
//     to the sandbox paths that mirror them.
//
// The consequence is the point of the whole corridor: no generated command
// ever runs on the host in production, and the host only ever reads copies.

// SandboxTreeMaterializer downloads a remote directory tree to a host
// directory so local file reads see the sandbox's contents.
type SandboxTreeMaterializer struct {
	Executor agentruntime.SandboxExecutor
	// MaxBytes bounds one download; zero means no extra bound beyond the
	// sandbox's own limits.
	MaxBytes int64
}

// Materialize copies remoteRoot into localRoot, preserving relative layout.
// Directories are created as they are discovered; file modes are not
// meaningful across the boundary, so files land with host defaults.
func (m SandboxTreeMaterializer) Materialize(ctx context.Context, remoteRoot, localRoot string) error {
	if m.Executor == nil {
		return errors.New("envintegration: sandbox executor is required to materialize a tree")
	}
	remoteRoot = strings.TrimRight(strings.TrimSpace(remoteRoot), "/")
	localRoot = strings.TrimSpace(localRoot)
	if remoteRoot == "" || localRoot == "" {
		return errors.New("envintegration: remote and local roots are required to materialize a tree")
	}
	if err := os.MkdirAll(localRoot, 0o750); err != nil {
		return fmt.Errorf("envintegration: create local root: %w", err)
	}
	return m.copyDir(ctx, remoteRoot, remoteRoot, localRoot)
}

// copyDir walks one remote directory level by level. The walk is explicit
// rather than recursive-through-concurrency: a build tree is small, and a
// deterministic depth-first copy is easier to reason about when a failure
// has to identify which file could not be read.
func (m SandboxTreeMaterializer) copyDir(ctx context.Context, remoteRoot, dir, localRoot string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	entries, err := m.Executor.ListFiles(ctx, dir)
	if err != nil {
		return fmt.Errorf("envintegration: list sandbox directory %q: %w", dir, err)
	}
	for _, entry := range entries {
		remotePath := strings.TrimSpace(entry.Path)
		if remotePath == "" {
			// Some providers report only a name; fall back to the parent path.
			remotePath = path.Join(dir, entry.Path)
		}
		rel, err := relativeTo(remoteRoot, remotePath)
		if err != nil {
			return err
		}
		if rel == "." || rel == "" {
			continue
		}
		localPath := filepath.Join(localRoot, filepath.FromSlash(rel))
		if entry.IsDirectory {
			if err := os.MkdirAll(localPath, 0o750); err != nil {
				return fmt.Errorf("envintegration: create local directory %q: %w", rel, err)
			}
			if err := m.copyDir(ctx, remoteRoot, remotePath, localRoot); err != nil {
				return err
			}
			continue
		}
		content, err := m.Executor.DownloadFile(ctx, remotePath)
		if err != nil {
			return fmt.Errorf("envintegration: download sandbox file %q: %w", remotePath, err)
		}
		if m.MaxBytes > 0 && int64(len(content)) > m.MaxBytes {
			return fmt.Errorf("envintegration: sandbox file %q exceeds the %d-byte materialization cap", remotePath, m.MaxBytes)
		}
		if err := os.MkdirAll(filepath.Dir(localPath), 0o750); err != nil {
			return fmt.Errorf("envintegration: create local directory for %q: %w", rel, err)
		}
		if err := os.WriteFile(localPath, content, 0o640); err != nil {
			return fmt.Errorf("envintegration: write local file %q: %w", rel, err)
		}
	}
	return nil
}

// relativeTo expresses remotePath relative to root using slash semantics, so
// the walk never depends on the host's path rules.
func relativeTo(root, remotePath string) (string, error) {
	root = strings.TrimRight(root, "/")
	remotePath = strings.TrimRight(remotePath, "/")
	if remotePath == root {
		return ".", nil
	}
	prefix := root + "/"
	if !strings.HasPrefix(remotePath, prefix) {
		return "", fmt.Errorf("envintegration: sandbox path %q is outside %q", remotePath, root)
	}
	return strings.TrimPrefix(remotePath, prefix), nil
}

// SandboxPathMapper translates a host path under the materialized root to the
// sandbox path that mirrors it. The verifier only ever speaks host paths, so
// the runner and assessor translate at the boundary instead of teaching the
// verifier about remotes.
type SandboxPathMapper struct {
	LocalRoot  string
	RemoteRoot string
}

// Remote returns the sandbox path corresponding to one host path.
func (p SandboxPathMapper) Remote(hostPath string) (string, error) {
	localRoot := strings.TrimSpace(p.LocalRoot)
	remoteRoot := strings.TrimRight(strings.TrimSpace(p.RemoteRoot), "/")
	if localRoot == "" || remoteRoot == "" {
		return "", errors.New("envintegration: local and remote roots are required to map a path")
	}
	cleaned := filepath.Clean(hostPath)
	root := filepath.Clean(localRoot)
	if cleaned == root {
		return remoteRoot, nil
	}
	rel, err := filepath.Rel(root, cleaned)
	if err != nil {
		return "", fmt.Errorf("envintegration: map host path %q: %w", hostPath, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("envintegration: host path %q is outside the materialized root", hostPath)
	}
	return path.Join(remoteRoot, filepath.ToSlash(rel)), nil
}

// SandboxCommandRunner runs verifier stage commands inside a boot sandbox.
// It implements environmentverify.CommandRunner.
type SandboxCommandRunner struct {
	Executor agentruntime.SandboxExecutor
	Paths    SandboxPathMapper
	// Timeout bounds one stage command. The verifier also applies its own
	// per-stage deadline through the context; this is the sandbox-side guard.
	Timeout time.Duration
}

// Run executes one stage command in the sandbox directory mirroring the
// verifier's host working directory.
func (r SandboxCommandRunner) Run(ctx context.Context, spec environmentverify.CommandSpec) (environmentverify.CommandResult, error) {
	if r.Executor == nil {
		return environmentverify.CommandResult{}, errors.New("envintegration: sandbox executor is required to run a stage command")
	}
	command := strings.TrimSpace(spec.Command)
	if command == "" {
		return environmentverify.CommandResult{}, errors.New("envintegration: stage command is empty")
	}
	remoteDir, err := r.pathsForHost(spec.WorkDir)
	if err != nil {
		return environmentverify.CommandResult{}, err
	}
	// The verifier's host work dir does not exist in the sandbox by that
	// path, so cd first; a failed cd makes the command fail rather than run
	// somewhere unintended.
	wrapped := "cd " + shellSingleQuote(remoteDir) + " && " + command
	exitCode, output, err := r.Executor.ExecuteCommand(ctx, wrapped, r.timeoutFor(ctx))
	if err != nil {
		return environmentverify.CommandResult{}, err
	}
	return environmentverify.CommandResult{ExitCode: exitCode, Output: output, TimedOut: false}, nil
}

// SandboxAssessor runs one assessment in the sandbox. The verifier prepares
// each solution workspace as a fresh host directory, so the assessor uploads
// that workspace into a sandbox scratch directory, runs the test command
// there, and removes the scratch afterwards. It implements
// environmentverify.AssessmentRunner.
type SandboxAssessor struct {
	Executor   agentruntime.SandboxExecutor
	ScratchDir string
	// Timeout bounds one assessment command.
	Timeout time.Duration
}

// Assess uploads the prepared workspace, runs the manifest test command
// there, and reports the same single-test outcome the host assessor reports.
func (a SandboxAssessor) Assess(ctx context.Context, work environmentverify.AssessmentWork) (environmentverify.AssessmentReport, error) {
	if a.Executor == nil {
		return environmentverify.AssessmentReport{}, errors.New("envintegration: sandbox executor is required to assess a solution")
	}
	testCommand := strings.TrimSpace(work.TestCommand)
	if testCommand == "" {
		return environmentverify.AssessmentReport{}, errors.New("envintegration: manifest declares no test command")
	}
	workDir := strings.TrimSpace(work.WorkDir)
	if workDir == "" {
		return environmentverify.AssessmentReport{}, errors.New("envintegration: assessment workspace is required")
	}
	scratch := strings.TrimRight(strings.TrimSpace(a.ScratchDir), "/")
	if scratch == "" {
		scratch = defaultAssessScratchDir
	}
	// A fresh, role-specific directory per assessment keeps the correct and
	// wrong solutions from ever observing each other's files.
	remoteDir := path.Join(scratch, assessmentDirName(work.Role))
	if err := a.uploadTree(ctx, workDir, remoteDir); err != nil {
		return environmentverify.AssessmentReport{}, err
	}
	defer func() {
		_, _, _ = a.Executor.ExecuteCommand(context.WithoutCancel(ctx), "rm -rf "+shellSingleQuote(remoteDir), time.Minute)
	}()
	wrapped := "cd " + shellSingleQuote(remoteDir) + " && " + testCommand
	exitCode, output, err := a.Executor.ExecuteCommand(ctx, wrapped, a.timeoutFor(ctx))
	if err != nil {
		return environmentverify.AssessmentReport{}, err
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return environmentverify.AssessmentReport{}, ctxErr
	}
	return ReportFromCommandResult(exitCode, output), nil
}

const defaultAssessScratchDir = "/tmp/codegym-assess"

// assessmentDirName maps an assessment role to a directory name, defaulting
// to the role itself so an unexpected role still gets its own directory.
func assessmentDirName(role string) string {
	role = strings.TrimSpace(role)
	if role == "" {
		return "assessment"
	}
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, role)
	return safe
}

// uploadTree mirrors a host workspace into the sandbox. Symlinks are not
// followed: the verifier rejects artifact symlinks before assessment, so a
// symlink here would already be a fault worth failing on.
func (a SandboxAssessor) uploadTree(ctx context.Context, hostRoot, remoteRoot string) error {
	if _, _, err := a.Executor.ExecuteCommand(ctx, "mkdir -p "+shellSingleQuote(remoteRoot), time.Minute); err != nil {
		return fmt.Errorf("envintegration: create assessment scratch directory: %w", err)
	}
	return filepath.WalkDir(hostRoot, func(hostPath string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if walkErr := ctx.Err(); walkErr != nil {
			return walkErr
		}
		rel, relErr := filepath.Rel(hostRoot, hostPath)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return nil
		}
		remotePath := path.Join(remoteRoot, filepath.ToSlash(rel))
		if entry.IsDir() {
			if _, _, mkErr := a.Executor.ExecuteCommand(ctx, "mkdir -p "+shellSingleQuote(remotePath), time.Minute); mkErr != nil {
				return fmt.Errorf("envintegration: create sandbox directory %q: %w", remotePath, mkErr)
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("envintegration: assessment workspace contains a non-regular file %q", rel)
		}
		content, readErr := os.ReadFile(hostPath)
		if readErr != nil {
			return fmt.Errorf("envintegration: read workspace file %q: %w", rel, readErr)
		}
		if upErr := a.Executor.UploadFile(ctx, content, remotePath); upErr != nil {
			return fmt.Errorf("envintegration: upload workspace file %q: %w", rel, upErr)
		}
		return nil
	})
}

func (r SandboxCommandRunner) pathsForHost(hostPath string) (string, error) {
	if strings.TrimSpace(hostPath) == "" {
		return "", errors.New("envintegration: stage command requires a working directory")
	}
	return r.Paths.Remote(hostPath)
}

func (r SandboxCommandRunner) timeoutFor(ctx context.Context) time.Duration {
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining > 0 {
			return remaining
		}
		return time.Nanosecond
	}
	return r.Timeout
}

func (a SandboxAssessor) timeoutFor(ctx context.Context) time.Duration {
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining > 0 {
			return remaining
		}
		return time.Nanosecond
	}
	return a.Timeout
}

// shellSingleQuote quotes one argument for a POSIX shell.
func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
