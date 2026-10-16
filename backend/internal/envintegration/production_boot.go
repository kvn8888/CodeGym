package envintegration

import (
	"context"
	"fmt"

	"github.com/kvn8888/codegym/backend/internal/environmentregistry"
	"github.com/kvn8888/codegym/backend/internal/environmentverify"
)

// LocalSnapshotProvider boots promotion checks against retained workspace
// directories for development and demonstration. Production boots real
// snapshots through the Daytona-backed provider; this provider exists so
// the promotion gate, boot checks, and reuse path execute for real without
// snapshot infrastructure. It must never serve production learners.
type LocalSnapshotProvider struct {
	Roots  func(reference string) (string, error)
	Runner *LocalCommandRunner
}

// BootFresh opens one boot rooted at the snapshot reference directory.
func (p *LocalSnapshotProvider) BootFresh(ctx context.Context, snapshotRef string, _ environmentregistry.NetworkPolicy) (environmentregistry.Boot, error) {
	if p == nil || p.Roots == nil {
		return nil, fmt.Errorf("envintegration: snapshot resolver is required")
	}
	root, err := p.Roots(snapshotRef)
	if err != nil {
		return nil, err
	}
	if p.Runner == nil {
		p.Runner = &LocalCommandRunner{}
	}
	return &localSnapshotBoot{root: root, runner: p.Runner}, nil
}

type localSnapshotBoot struct {
	root   string
	runner *LocalCommandRunner
}

// Run executes one representative check command in the snapshot root. Setup
// commands never reach a boot: promotion and reuse run build and test only.
func (b *localSnapshotBoot) Run(ctx context.Context, command string) (environmentregistry.BootResult, error) {
	result, err := b.runner.Run(ctx, environmentverify.CommandSpec{Stage: "boot", Command: command, WorkDir: b.root})
	if err != nil {
		return environmentregistry.BootResult{}, err
	}
	return environmentregistry.BootResult{ExitCode: result.ExitCode, Output: result.Output}, nil
}

// NewWorkspace separates one clean learner workspace identity for the boot.
// Demo-local workspaces share the sealed root read-only by convention; the
// practice layer copies editable files into its own session overlay, so the
// boot root itself is never written by learners.
func (b *localSnapshotBoot) NewWorkspace(context.Context) (environmentregistry.Workspace, error) {
	suffix := b.root
	if len(suffix) > 8 {
		suffix = suffix[len(suffix)-8:]
	}
	return environmentregistry.Workspace{ID: "ws-demo-" + suffix}, nil
}

// Close releases the boot. Local directories persist as the demo snapshot
// itself; nothing temporary is destroyed here.
func (b *localSnapshotBoot) Close(context.Context) error { return nil }
