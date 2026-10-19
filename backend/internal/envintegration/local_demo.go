package envintegration

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/kvn8888/codegym/backend/internal/agentruntime"
	"github.com/kvn8888/codegym/backend/internal/envbuild"
	"github.com/kvn8888/codegym/backend/internal/environmentregistry"
	"github.com/kvn8888/codegym/backend/internal/practice"
)

// This file assembles the request pipeline for a single development host:
// the agent builds on the host, snapshots are host directories, and
// promotion and practice commands run on the host. It exists so the full
// request → build → verify → promote → practice corridor can be
// demonstrated before sandbox snapshots back the registry. Host execution
// cannot enforce network policy; it is never a production configuration.

type stageReporterKey struct{}

// withStageReporter carries an operation's progress callback to the
// registry adapters running inside Resolve.
func withStageReporter(ctx context.Context, report func(stage string)) context.Context {
	if report == nil {
		return ctx
	}
	return context.WithValue(ctx, stageReporterKey{}, report)
}

func reportStage(ctx context.Context, stage string) {
	if report, ok := ctx.Value(stageReporterKey{}).(func(string)); ok {
		report(stage)
	}
}

// RuntimeFactory returns an agent runtime for one build plus a release
// callback. Relay-backed runtimes need a fresh scoped token per build.
type RuntimeFactory func(ctx context.Context) (agentruntime.AgentRuntime, func(), error)

// AgentBuilder implements the registry Builder with a fresh agent runtime
// per build, delegating preparation and handoff to RegistryBuilder.
type AgentBuilder struct {
	NewRuntime  RuntimeFactory
	RuntimeName string
	Snapshots   *SnapshotDirectories
	TurnCeiling int
	Timeout     time.Duration
}

// Build prepares one environment with a freshly issued runtime.
func (b *AgentBuilder) Build(ctx context.Context, req environmentregistry.BuildRequest) (environmentregistry.BuildOutput, error) {
	if b == nil || b.NewRuntime == nil {
		return environmentregistry.BuildOutput{}, fmt.Errorf("envintegration: runtime factory is required")
	}
	runtime, release, err := b.NewRuntime(ctx)
	if err != nil {
		return environmentregistry.BuildOutput{}, fmt.Errorf("envintegration: start agent runtime: %w", err)
	}
	if release != nil {
		defer release()
	}
	reportStage(ctx, "agent building project and resolving dependencies")
	inner := &RegistryBuilder{
		Preparer: &envbuild.Preparer{
			Runtime:     runtime,
			Provisioner: envbuild.HostProvisioner{},
			TurnCeiling: b.TurnCeiling,
			Timeout:     b.Timeout,
		},
		RuntimeName: b.RuntimeName,
		Snapshots:   b.Snapshots,
	}
	output, err := inner.Build(ctx, req)
	if err == nil {
		reportStage(ctx, "verifying build, tests, and starter")
	}
	return output, err
}

// LocalSnapshotProvider boots promoted snapshots as fresh host copies.
type LocalSnapshotProvider struct {
	Snapshots *SnapshotDirectories
}

// BootFresh copies the snapshot into a temporary directory.
func (p *LocalSnapshotProvider) BootFresh(ctx context.Context, reference string, _ environmentregistry.NetworkPolicy) (environmentregistry.Boot, error) {
	if p == nil || p.Snapshots == nil {
		return nil, fmt.Errorf("envintegration: snapshot directories are required")
	}
	reportStage(ctx, "promotion checks on a fresh copy")
	root, err := p.Snapshots.Resolve(reference)
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "codegym-boot-")
	if err != nil {
		return nil, err
	}
	if err := copyTree(root, dir); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	return &localBoot{dir: dir}, nil
}

type localBoot struct {
	dir string
}

func (b *localBoot) Run(ctx context.Context, command string) (environmentregistry.BootResult, error) {
	exitCode, output, timedOut, err := runLocalStageCommand(ctx, command, b.dir, DefaultLocalOutputCap)
	if err != nil {
		return environmentregistry.BootResult{}, err
	}
	if timedOut {
		return environmentregistry.BootResult{}, context.DeadlineExceeded
	}
	return environmentregistry.BootResult{ExitCode: exitCode, Output: output}, nil
}

func (b *localBoot) NewWorkspace(context.Context) (environmentregistry.Workspace, error) {
	raw := make([]byte, 6)
	if _, err := rand.Read(raw); err != nil {
		return environmentregistry.Workspace{}, err
	}
	return environmentregistry.Workspace{ID: "local-ws-" + hex.EncodeToString(raw)}, nil
}

func (b *localBoot) Close(context.Context) error {
	return os.RemoveAll(b.dir)
}

// AllTechnologies admits every technology: the registry is populated by
// successful builds, not a curated list.
type AllTechnologies struct{}

func (AllTechnologies) SupportsTechnology(string) bool { return true }

// NoBuildQuota never refuses a build. Development only.
type NoBuildQuota struct{}

func (NoBuildQuota) CheckBuild(context.Context, string) error { return nil }

// LocalDemoConfig configures NewLocalDemo.
type LocalDemoConfig struct {
	NewRuntime  RuntimeFactory
	RuntimeName string
	TurnCeiling int
	Timeout     time.Duration
	Saved       practice.SavedExerciseStore
	// Store persists promoted artifacts. Nil keeps the in-memory store, so
	// tests and demos need no database; production passes the durable one.
	Store environmentregistry.Store
}

// LocalDemo is the assembled host-only corridor.
type LocalDemo struct {
	Operations *OperationStore
	Practice   *practice.Service
	Registry   *environmentregistry.Service
}

// NewLocalDemo assembles registry, demo-mode verification, practice, and
// the operation store around one host snapshot directory set.
func NewLocalDemo(cfg LocalDemoConfig) (*LocalDemo, error) {
	if cfg.NewRuntime == nil {
		return nil, fmt.Errorf("envintegration: runtime factory is required")
	}
	dirs := &SnapshotDirectories{}
	store := cfg.Store
	if store == nil {
		store = environmentregistry.NewInMemoryStore()
	}
	registryService := &environmentregistry.Service{
		Store:     store,
		Snapshots: &LocalSnapshotProvider{Snapshots: dirs},
		Builder: &AgentBuilder{
			NewRuntime: cfg.NewRuntime, RuntimeName: cfg.RuntimeName, Snapshots: dirs,
			TurnCeiling: cfg.TurnCeiling, Timeout: cfg.Timeout,
		},
		Capabilities: AllTechnologies{},
		Quota:        NoBuildQuota{},
		Verifier: &AssessmentResolvingVerifier{
			Inner:       &RegistryVerifier{Roots: dirs.Resolve, Runner: &LocalCommandRunner{}},
			Assessments: &StarterAssessmentSource{Roots: dirs.Resolve},
		},
		PracticePolicy: environmentregistry.NetworkPolicyIsolated,
	}
	contents := &StarterContents{Inner: &SnapshotContents{Roots: dirs.Resolve}}
	practiceService := &practice.Service{
		Artifacts: &RegistryArtifactSource{Store: registryService.Store, Contents: &SnapshotContents{Roots: dirs.Resolve}},
		Contents:  contents,
		Runner:    &PracticeLocalRunner{Snapshots: dirs.Resolve},
		Records:   practice.NewInMemoryRecordStore(),
		Saved:     cfg.Saved,
	}
	pipeline := &ProductionPipeline{Registry: registryService, Practice: practiceService}
	return &LocalDemo{
		Operations: &OperationStore{Pipeline: pipeline.Pipeline},
		Practice:   practiceService,
		Registry:   registryService,
	}, nil
}

// copyTree copies regular files and directories from src into dst,
// skipping symlinks and build output directories.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		switch {
		case entry.IsDir():
			if rel != "." && (entry.Name() == "target" || entry.Name() == "node_modules") {
				return filepath.SkipDir
			}
			return os.MkdirAll(target, 0o700)
		case entry.Type().IsRegular():
			return copyFile(path, target)
		default:
			return nil
		}
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
