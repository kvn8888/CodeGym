package envintegration

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/kvn8888/codegym/backend/internal/practice"
)

// DefaultPracticeSandboxTTL bounds how long an idle execution sandbox lives.
// The sweeper reaps expired spaces on every call, so no background goroutine
// and no explicit end-of-operation signal are needed.
const DefaultPracticeSandboxTTL = 10 * time.Minute

// DaytonaSandbox is one leased execution sandbox behind the practice runner.
// It is the minimal surface the runner needs; the production implementation
// wraps the Daytona client, tests use fakes.
type DaytonaSandbox interface {
	CreateFolder(ctx context.Context, path string) error
	UploadFile(ctx context.Context, content []byte, path string) error
	// DownloadFile reads one file back out of the sandbox, so sealed
	// snapshot contents can be read without a host copy.
	DownloadFile(ctx context.Context, path string) ([]byte, error)
	ExecuteCommand(ctx context.Context, command string, timeout time.Duration) (exitCode int, output string, err error)
	Delete(ctx context.Context) error
}

// SandboxFactory leases execution sandboxes for practice runs. Production
// points it at Daytona snapshots; tests point it at fakes. Snapshot
// contents arrive through the content store, so the factory only needs the
// reference, the network posture, and labels.
type SandboxFactory interface {
	Create(ctx context.Context, snapshotRef string, networkBlockAll bool, labels map[string]string) (DaytonaSandbox, error)
}

// DaytonaPracticeRunner implements the practice runner against leased
// sandboxes. All calls carrying one execution ID share one sandbox, which
// preserves build outputs across the operation's steps; idle sandboxes are
// reaped by TTL on every call. File scoping stays the service's job: only
// the files the service passes are uploaded, and commands run inside the
// workspace directory.
type DaytonaPracticeRunner struct {
	Factory SandboxFactory
	IdleTTL time.Duration

	mu     sync.Mutex
	spaces map[string]*practiceSandboxSpace
}

type practiceSandboxSpace struct {
	sandbox  DaytonaSandbox
	lastUsed time.Time
}

func (r *DaytonaPracticeRunner) idleTTL() time.Duration {
	if r.IdleTTL > 0 {
		return r.IdleTTL
	}
	return DefaultPracticeSandboxTTL
}

// RunCommand executes one stage command in the operation's sandbox,
// uploading the scoped files first. A nonzero exit is a normal stage
// result; only infrastructure faults return errors.
func (r *DaytonaPracticeRunner) RunCommand(ctx context.Context, spec practice.RunSpec) (practice.RunOutcome, error) {
	if r == nil || r.Factory == nil {
		return practice.RunOutcome{}, fmt.Errorf("envintegration: sandbox factory is required")
	}
	if strings.TrimSpace(spec.ExecutionID) == "" {
		return practice.RunOutcome{}, fmt.Errorf("envintegration: execution ID is required")
	}
	now := time.Now()
	r.mu.Lock()
	if r.spaces == nil {
		r.spaces = map[string]*practiceSandboxSpace{}
	}
	for id, space := range r.spaces {
		if now.Sub(space.lastUsed) > r.idleTTL() {
			_ = space.sandbox.Delete(ctx)
			delete(r.spaces, id)
		}
	}
	space, ok := r.spaces[spec.ExecutionID]
	if !ok {
		sandbox, err := r.Factory.Create(ctx, spec.SnapshotRef, spec.NetworkPolicy == "block-all", map[string]string{
			"codegym":      "practice-run",
			"execution-id": spec.ExecutionID,
		})
		if err != nil {
			r.mu.Unlock()
			return practice.RunOutcome{}, fmt.Errorf("envintegration: lease practice sandbox: %w", err)
		}
		space = &practiceSandboxSpace{sandbox: sandbox}
		r.spaces[spec.ExecutionID] = space
	}
	space.lastUsed = now
	sandbox := space.sandbox
	r.mu.Unlock()

	workDir := "work"
	if strings.TrimSpace(spec.WorkDir) != "" {
		workDir = "work/" + strings.Trim(strings.TrimSpace(spec.WorkDir), "/")
	}
	if err := sandbox.CreateFolder(ctx, workDir); err != nil {
		return practice.RunOutcome{}, fmt.Errorf("envintegration: create practice workspace: %w", err)
	}
	for _, file := range spec.Files {
		clean := strings.Trim(strings.ReplaceAll(file.Path, "\\", "/"), "/")
		if clean == "" || clean == "." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
			return practice.RunOutcome{}, fmt.Errorf("envintegration: run file escapes the workspace: %q", file.Path)
		}
		if err := sandbox.UploadFile(ctx, []byte(file.Content), workDir+"/"+clean); err != nil {
			return practice.RunOutcome{}, fmt.Errorf("envintegration: upload practice file: %w", err)
		}
	}
	command := "cd " + workDir + " && " + spec.Command
	exitCode, output, err := sandbox.ExecuteCommand(ctx, command, 0)
	if err != nil {
		if ctx.Err() != nil {
			return practice.RunOutcome{ExitCode: -1, TimedOut: true}, nil
		}
		return practice.RunOutcome{}, fmt.Errorf("envintegration: execute practice command: %w", err)
	}
	return practice.RunOutcome{ExitCode: exitCode, Output: output}, nil
}

// SandboxContents serves sealed snapshot files by reading them out of a
// sandbox booted from the snapshot. It is the production replacement for
// SnapshotContents, which needs a host directory: a promoted artifact lives
// in a Daytona snapshot and has no host path.
//
// One sandbox is reused per snapshot and reaped by TTL, so reading the
// manifest and the starter costs one boot rather than one per file.
type SandboxContents struct {
	Factory SandboxFactory
	// WorkspaceDir is the sandbox directory the snapshot's tree lives in.
	WorkspaceDir string
	IdleTTL      time.Duration

	mu     sync.Mutex
	spaces map[string]*contentsSpace
}

type contentsSpace struct {
	sandbox  DaytonaSandbox
	lastUsed time.Time
}

func (s *SandboxContents) idleTTL() time.Duration {
	if s.IdleTTL > 0 {
		return s.IdleTTL
	}
	return DefaultPracticeSandboxTTL
}

func (s *SandboxContents) workspaceDir() string {
	if dir := strings.TrimRight(strings.TrimSpace(s.WorkspaceDir), "/"); dir != "" {
		return dir
	}
	return DefaultSandboxWorkspaceDir
}

func (s *SandboxContents) sandboxFor(ctx context.Context, snapshotRef string) (DaytonaSandbox, error) {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.spaces == nil {
		s.spaces = map[string]*contentsSpace{}
	}
	for ref, space := range s.spaces {
		if now.Sub(space.lastUsed) > s.idleTTL() {
			_ = space.sandbox.Delete(ctx)
			delete(s.spaces, ref)
		}
	}
	if space, ok := s.spaces[snapshotRef]; ok {
		space.lastUsed = now
		return space.sandbox, nil
	}
	// Reads boot the artifact exactly as a learner's practice session does:
	// isolated, so sealed contents cannot depend on network access.
	sandbox, err := s.Factory.Create(ctx, snapshotRef, true, map[string]string{"codegym": "artifact-read"})
	if err != nil {
		return nil, err
	}
	s.spaces[snapshotRef] = &contentsSpace{sandbox: sandbox, lastUsed: now}
	return sandbox, nil
}

// ReadArtifactFile reads one sealed snapshot file. Paths are confined to the
// snapshot workspace, matching SnapshotContents.
func (s *SandboxContents) ReadArtifactFile(ctx context.Context, snapshotRef, filePath string) (string, error) {
	if s == nil || s.Factory == nil {
		return "", fmt.Errorf("envintegration: sandbox factory is required")
	}
	slashed := strings.ReplaceAll(strings.TrimSpace(filePath), "\\", "/")
	if strings.HasPrefix(slashed, "/") {
		return "", fmt.Errorf("envintegration: artifact path escapes the snapshot: %q", filePath)
	}
	clean := strings.Trim(slashed, "/")
	if clean == "" || clean == "." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
		return "", fmt.Errorf("envintegration: artifact path escapes the snapshot: %q", filePath)
	}
	sandbox, err := s.sandboxFor(ctx, snapshotRef)
	if err != nil {
		return "", fmt.Errorf("envintegration: boot snapshot for read: %w", err)
	}
	payload, err := sandbox.DownloadFile(ctx, s.workspaceDir()+"/"+clean)
	if err != nil {
		return "", fmt.Errorf("envintegration: read snapshot file: %w", err)
	}
	return string(payload), nil
}

// ProductionPracticeService assembles the browser practice service from
// production seams: registry-backed artifacts, snapshot contents, a sandbox
// runner, and the caller's record, save, and limit configuration. It wires
// nothing itself; deploy configuration supplies each seam, which keeps this
// package testable and the wiring to one call site.
func ProductionPracticeService(
	artifacts *RegistryArtifactSource,
	contents practice.ContentStore,
	runner *DaytonaPracticeRunner,
	records practice.RecordStore,
	saved practice.SavedExerciseStore,
	commandTimeout time.Duration,
) *practice.Service {
	return &practice.Service{
		Artifacts:      artifacts,
		Contents:       contents,
		Runner:         runner,
		Records:        records,
		Saved:          saved,
		CommandTimeout: commandTimeout,
	}
}
