package envintegration

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/daytona/clients/sdk-go/pkg/daytona"
	"github.com/daytona/clients/sdk-go/pkg/options"
	"github.com/daytona/clients/sdk-go/pkg/types"
	"github.com/kvn8888/codegym/backend/internal/agentruntime"
	"github.com/kvn8888/codegym/backend/internal/environmentregistry"
)

// DefaultBootCommandTimeout bounds one boot check command. Dependency
// installation never runs on a boot, so this only has to cover a build or a
// test run.
const DefaultBootCommandTimeout = 10 * time.Minute

// SnapshottingSandbox is a leased sandbox that can additionally be promoted
// to a reusable snapshot. The practice runner only needs DaytonaSandbox;
// the build/promotion path needs this wider surface.
type SnapshottingSandbox interface {
	DaytonaSandbox
	// ID identifies the sandbox for labels and diagnostics.
	ID() string
	// CreateSnapshot promotes the sandbox's current filesystem to a named
	// snapshot other sandboxes can boot from.
	CreateSnapshot(ctx context.Context, name string, timeout time.Duration) error
	// DownloadFile reads one file back out of the sandbox.
	DownloadFile(ctx context.Context, path string) ([]byte, error)
	// ListFiles lists one sandbox directory so the build path can read a
	// whole tree back before verification.
	ListFiles(ctx context.Context, dir string) ([]agentruntime.SandboxEntry, error)
}

// sandboxParams builds the SDK create request for one sandbox. It is shared so
// practice, boot checks, and builds request the same shape.
func sandboxParams(snapshot string, labels map[string]string, networkBlockAll, ephemeral bool) types.SnapshotParams {
	return types.SnapshotParams{
		Snapshot: snapshot,
		SandboxBaseParams: types.SandboxBaseParams{
			Labels:          labels,
			NetworkBlockAll: networkBlockAll,
			Ephemeral:       ephemeral,
		},
	}
}

// DaytonaClient is the minimal Daytona surface these adapters need. The real
// implementation wraps *daytona.Client; tests supply fakes.
type DaytonaClient interface {
	Create(ctx context.Context, params types.SnapshotParams) (SnapshottingSandbox, error)
	DeleteSnapshot(ctx context.Context, name string) error
}

// DaytonaSandboxFactory leases practice-run sandboxes from a snapshot. It
// implements SandboxFactory, so DaytonaPracticeRunner can run learner code
// in isolation with no other wiring.
type DaytonaSandboxFactory struct {
	Client     DaytonaClient
	Ephemeral  bool
	CreateNote string
}

// Create boots one sandbox from the snapshot reference under the requested
// network posture.
func (f *DaytonaSandboxFactory) Create(ctx context.Context, snapshotRef string, networkBlockAll bool, labels map[string]string) (DaytonaSandbox, error) {
	if f == nil || f.Client == nil {
		return nil, fmt.Errorf("envintegration: Daytona client is required")
	}
	sandbox, err := f.Client.Create(ctx, sandboxParams(snapshotRef, labels, networkBlockAll, f.Ephemeral))
	if err != nil {
		return nil, fmt.Errorf("envintegration: create practice sandbox: %w", err)
	}
	return sandbox, nil
}

// DaytonaSnapshotProvider boots promoted snapshots as isolated sandboxes.
// It implements environmentregistry.SnapshotProvider, which makes the
// registry's promotion boot checks and reuse paths run off-host.
type DaytonaSnapshotProvider struct {
	Client         DaytonaClient
	CommandTimeout time.Duration
	Labels         map[string]string
}

func (p *DaytonaSnapshotProvider) commandTimeout() time.Duration {
	if p.CommandTimeout > 0 {
		return p.CommandTimeout
	}
	return DefaultBootCommandTimeout
}

// BootFresh creates a network-blocked sandbox from the promoted snapshot.
func (p *DaytonaSnapshotProvider) BootFresh(ctx context.Context, snapshotRef string, policy environmentregistry.NetworkPolicy) (environmentregistry.Boot, error) {
	if p == nil || p.Client == nil {
		return nil, fmt.Errorf("envintegration: Daytona client is required")
	}
	labels := map[string]string{"codegym": "boot-check"}
	for key, value := range p.Labels {
		labels[key] = value
	}
	sandbox, err := p.Client.Create(ctx, sandboxParams(snapshotRef, labels, policy == environmentregistry.NetworkPolicyIsolated, true))
	if err != nil {
		return nil, fmt.Errorf("envintegration: boot snapshot %q: %w", snapshotRef, err)
	}
	return &daytonaBoot{sandbox: sandbox, timeout: p.commandTimeout()}, nil
}

// daytonaBoot is one fresh boot of a promoted snapshot.
type daytonaBoot struct {
	sandbox SnapshottingSandbox
	timeout time.Duration
}

// Run executes one representative check command on the fresh boot. A nonzero
// exit is a normal result; only infrastructure faults return errors.
func (b *daytonaBoot) Run(ctx context.Context, command string) (environmentregistry.BootResult, error) {
	exitCode, output, err := b.sandbox.ExecuteCommand(ctx, command, b.timeout)
	if err != nil {
		return environmentregistry.BootResult{}, fmt.Errorf("envintegration: run boot command: %w", err)
	}
	return environmentregistry.BootResult{ExitCode: exitCode, Output: output}, nil
}

// NewWorkspace separates one clean learner workspace from the boot. The
// workspace is an identity: contents come from the snapshot, and learner runs
// lease their own sandbox from the same reference.
func (b *daytonaBoot) NewWorkspace(context.Context) (environmentregistry.Workspace, error) {
	raw := make([]byte, 6)
	if _, err := rand.Read(raw); err != nil {
		return environmentregistry.Workspace{}, err
	}
	return environmentregistry.Workspace{ID: "daytona-ws-" + hex.EncodeToString(raw)}, nil
}

// Close releases the boot sandbox.
func (b *daytonaBoot) Close(ctx context.Context) error {
	if b == nil || b.sandbox == nil {
		return nil
	}
	return b.sandbox.Delete(ctx)
}

// PromoteSnapshot creates a reusable snapshot from a built sandbox and
// returns the snapshot name to record as the artifact's SnapshotRef. It
// implements the promotion step the registry records after verification.
func PromoteSnapshot(ctx context.Context, sandbox SnapshottingSandbox, name string, timeout time.Duration) (string, error) {
	if sandbox == nil {
		return "", fmt.Errorf("envintegration: sandbox is required to promote a snapshot")
	}
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	if err := sandbox.CreateSnapshot(ctx, name, timeout); err != nil {
		return "", fmt.Errorf("envintegration: promote snapshot %q: %w", name, err)
	}
	return name, nil
}

// daytonaClientAdapter wraps the SDK client in the narrow interfaces above.
type daytonaClientAdapter struct {
	client *daytona.Client
}

// NewDaytonaClient builds the narrow client from explicit credentials, loaded
// from config rather than raw environment so callers stay explicit.
func NewDaytonaClient(apiKey, apiURL string) (DaytonaClient, error) {
	client, err := daytona.NewClientWithConfig(&types.DaytonaConfig{
		APIKey: apiKey,
		APIUrl: apiURL,
	})
	if err != nil {
		return nil, fmt.Errorf("create Daytona client: %w", err)
	}
	return daytonaClientAdapter{client: client}, nil
}

func (a daytonaClientAdapter) Create(ctx context.Context, params types.SnapshotParams) (SnapshottingSandbox, error) {
	sandbox, err := a.client.Create(ctx, params)
	if err != nil {
		return nil, err
	}
	return daytonaSandboxAdapter{sandbox: sandbox}, nil
}

func (a daytonaClientAdapter) DeleteSnapshot(ctx context.Context, name string) error {
	snapshot, err := a.client.Snapshot.Get(ctx, name)
	if err != nil {
		return err
	}
	return a.client.Snapshot.Delete(ctx, snapshot)
}

// daytonaSandboxAdapter adapts one SDK sandbox to both sandbox interfaces.
type daytonaSandboxAdapter struct {
	sandbox *daytona.Sandbox
}

func (a daytonaSandboxAdapter) ID() string {
	if a.sandbox == nil {
		return ""
	}
	return a.sandbox.ID
}

func (a daytonaSandboxAdapter) CreateFolder(ctx context.Context, path string) error {
	return a.sandbox.FileSystem.CreateFolder(ctx, path)
}

func (a daytonaSandboxAdapter) UploadFile(ctx context.Context, content []byte, path string) error {
	return a.sandbox.FileSystem.UploadFile(ctx, content, path)
}

func (a daytonaSandboxAdapter) DownloadFile(ctx context.Context, path string) ([]byte, error) {
	return a.sandbox.FileSystem.DownloadFile(ctx, path, nil)
}

func (a daytonaSandboxAdapter) ListFiles(ctx context.Context, dir string) ([]agentruntime.SandboxEntry, error) {
	entries, err := a.sandbox.FileSystem.ListFiles(ctx, dir)
	if err != nil {
		return nil, err
	}
	neutral := make([]agentruntime.SandboxEntry, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		neutral = append(neutral, agentruntime.SandboxEntry{
			Path:        entry.Path,
			IsDirectory: entry.IsDirectory,
			Size:        entry.Size,
		})
	}
	return neutral, nil
}

func (a daytonaSandboxAdapter) ExecuteCommand(ctx context.Context, command string, timeout time.Duration) (int, string, error) {
	// A zero timeout means "no explicit deadline": the SDK applies its own
	// default, which is what the practice runner's 0 asks for.
	var opts []func(*options.ExecuteCommand)
	if timeout > 0 {
		opts = append(opts, options.WithExecuteTimeout(timeout))
	}
	response, err := a.sandbox.Process.ExecuteCommand(ctx, command, opts...)
	if err != nil {
		return 0, "", err
	}
	return response.ExitCode, response.Result, nil
}

func (a daytonaSandboxAdapter) CreateSnapshot(ctx context.Context, name string, timeout time.Duration) error {
	return a.sandbox.ExperimentalCreateSnapshotWithTimeout(ctx, name, timeout)
}

func (a daytonaSandboxAdapter) Delete(ctx context.Context) error {
	return a.sandbox.Delete(ctx)
}
