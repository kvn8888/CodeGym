package envintegration

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/daytona/clients/sdk-go/pkg/types"
	"github.com/kvn8888/codegym/backend/internal/agentruntime"
	"github.com/kvn8888/codegym/backend/internal/environmentregistry"
	"github.com/kvn8888/codegym/backend/internal/environmentverify"
	"github.com/kvn8888/codegym/backend/internal/execution"
	"github.com/kvn8888/codegym/backend/internal/practice"
)

// The adapters must satisfy the seams the rest of the backend consumes.
var (
	_ SandboxFactory                     = (*DaytonaSandboxFactory)(nil)
	_ environmentregistry.Boot           = (*daytonaBoot)(nil)
	_ SnapshottingSandbox                = (*fakeSandbox)(nil)
	_ agentruntime.SandboxExecutor       = (*fakeTreeSandbox)(nil)
	_ environmentverify.CommandRunner    = (*SandboxCommandRunner)(nil)
	_ environmentverify.AssessmentRunner = (*SandboxAssessor)(nil)
)

// The real Daytona adapter is the one production uses, so it must satisfy the
// executor seam every off-host step is written against.
var _ agentruntime.SandboxExecutor = daytonaSandboxAdapter{}

// fakeClient records every Create and hands out recording sandboxes.
type fakeClient struct {
	created   []types.SnapshotParams
	sandboxes []*fakeSandbox
	deleted   []string
	createErr error
}

func (c *fakeClient) Create(_ context.Context, params types.SnapshotParams) (SnapshottingSandbox, error) {
	if c.createErr != nil {
		return nil, c.createErr
	}
	c.created = append(c.created, params)
	sandbox := &fakeSandbox{id: "sandbox-" + strconv.Itoa(len(c.sandboxes)+1)}
	c.sandboxes = append(c.sandboxes, sandbox)
	return sandbox, nil
}

func (c *fakeClient) DeleteSnapshot(_ context.Context, name string) error {
	c.deleted = append(c.deleted, name)
	return nil
}

// fakeSandbox records folder, upload, execute, snapshot, and delete calls.
type fakeSandbox struct {
	id           string
	folders      []string
	uploads      []string
	commands     []string
	deletedCount int
	snapshotName string
	snapshotTTL  time.Duration
	exitCode     int
	output       string
	execErr      error
	deleteErr    error
}

func (s *fakeSandbox) ID() string { return s.id }

func (s *fakeSandbox) CreateFolder(_ context.Context, path string) error {
	s.folders = append(s.folders, path)
	return nil
}

func (s *fakeSandbox) UploadFile(_ context.Context, _ []byte, path string) error {
	s.uploads = append(s.uploads, path)
	return nil
}

func (s *fakeSandbox) DownloadFile(context.Context, string) ([]byte, error) {
	return []byte("contents"), nil
}

func (s *fakeSandbox) ExecuteCommand(_ context.Context, command string, _ time.Duration) (int, string, error) {
	s.commands = append(s.commands, command)
	if s.execErr != nil {
		return 0, "", s.execErr
	}
	return s.exitCode, s.output, nil
}

func (s *fakeSandbox) CreateSnapshot(_ context.Context, name string, timeout time.Duration) error {
	s.snapshotName = name
	s.snapshotTTL = timeout
	return nil
}

func (s *fakeSandbox) Delete(context.Context) error {
	s.deletedCount++
	return s.deleteErr
}

// TestSandboxFactoryLeasesIsolatedSandbox proves the factory boots the
// requested snapshot with the network posture and labels it is given, so
// learner code runs off-host and blocked.
func TestSandboxFactoryLeasesIsolatedSandbox(t *testing.T) {
	client := &fakeClient{}
	factory := &DaytonaSandboxFactory{Client: client, Ephemeral: true}
	labels := map[string]string{"codegym": "practice-run", "execution-id": "exec-1"}

	sandbox, err := factory.Create(context.Background(), "snapshot-9", true, labels)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if sandbox == nil {
		t.Fatal("expected a sandbox")
	}
	if len(client.created) != 1 {
		t.Fatalf("expected one create call, got %d", len(client.created))
	}
	params := client.created[0]
	if params.Snapshot != "snapshot-9" {
		t.Fatalf("snapshot = %q", params.Snapshot)
	}
	if !params.NetworkBlockAll {
		t.Fatal("practice sandbox must block all network")
	}
	if !params.Ephemeral {
		t.Fatal("practice sandbox must be ephemeral")
	}
	if params.Labels["execution-id"] != "exec-1" || params.Labels["codegym"] != "practice-run" {
		t.Fatalf("labels not forwarded: %+v", params.Labels)
	}
}

// TestPracticeRunnerIsolatesLearnerRun wires the real factory into the
// existing Daytona practice runner to prove the two halves meet: files are
// uploaded, the command runs under the sandbox, and a nonzero exit is a
// normal outcome rather than an error.
func TestPracticeRunnerIsolatesLearnerRun(t *testing.T) {
	client := &fakeClient{}
	runner := &DaytonaPracticeRunner{Factory: &DaytonaSandboxFactory{Client: client}}
	outcome, err := runner.RunCommand(context.Background(), practice.RunSpec{
		ExecutionID:   "exec-42",
		SnapshotRef:   "snapshot-42",
		Command:       "mvn -q test",
		NetworkPolicy: execution.NetworkModeBlockAll,
		Files:         []execution.File{{Path: "src/main/App.java", Content: "class App {}"}},
	})
	if err != nil {
		t.Fatalf("run command: %v", err)
	}
	if outcome.ExitCode != 0 {
		t.Fatalf("expected zero exit, got %d", outcome.ExitCode)
	}
	if len(client.created) != 1 || client.created[0].Snapshot != "snapshot-42" {
		t.Fatalf("expected a sandbox from snapshot-42, got %+v", client.created)
	}
	sandbox := client.sandboxes[0]
	if len(sandbox.uploads) != 1 || sandbox.uploads[0] != "work/src/main/App.java" {
		t.Fatalf("files not uploaded into the workspace: %+v", sandbox.uploads)
	}
	if len(sandbox.commands) != 1 || sandbox.commands[0] != "cd work && mvn -q test" {
		t.Fatalf("command not run in the workspace: %+v", sandbox.commands)
	}
}

// TestSnapshotProviderBootsBlockedFromSnapshot proves boot checks run
// off-host from the promoted snapshot under an isolated policy.
func TestSnapshotProviderBootsBlockedFromSnapshot(t *testing.T) {
	client := &fakeClient{}
	client.sandboxes = nil
	provider := &DaytonaSnapshotProvider{Client: client, Labels: map[string]string{"owner": "ws-1"}}

	boot, err := provider.BootFresh(context.Background(), "snapshot-promoted", environmentregistry.NetworkPolicyIsolated)
	if err != nil {
		t.Fatalf("boot fresh: %v", err)
	}
	params := client.created[0]
	if params.Snapshot != "snapshot-promoted" {
		t.Fatalf("snapshot = %q", params.Snapshot)
	}
	if !params.NetworkBlockAll {
		t.Fatal("isolated boot must block all network")
	}
	if params.Labels["codegym"] != "boot-check" || params.Labels["owner"] != "ws-1" {
		t.Fatalf("labels not forwarded: %+v", params.Labels)
	}

	client.sandboxes[0].exitCode = 1
	client.sandboxes[0].output = "build failed"
	result, err := boot.Run(context.Background(), "mvn -q verify")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.ExitCode != 1 || result.Output != "build failed" {
		t.Fatalf("unexpected boot result: %+v", result)
	}

	first, err := boot.NewWorkspace(context.Background())
	if err != nil {
		t.Fatalf("new workspace: %v", err)
	}
	second, err := boot.NewWorkspace(context.Background())
	if err != nil {
		t.Fatalf("new workspace: %v", err)
	}
	if first.ID == "" || first.ID == second.ID {
		t.Fatalf("workspaces must be unique and non-empty: %q %q", first.ID, second.ID)
	}

	if err := boot.Close(context.Background()); err != nil {
		t.Fatalf("close: %v", err)
	}
	if client.sandboxes[0].deletedCount != 1 {
		t.Fatalf("expected the boot sandbox to be deleted once, got %d", client.sandboxes[0].deletedCount)
	}
}

// TestSnapshotProviderRequiresClient proves a missing client is an
// infrastructure fault, not a silent success.
func TestSnapshotProviderRequiresClient(t *testing.T) {
	provider := &DaytonaSnapshotProvider{}
	if _, err := provider.BootFresh(context.Background(), "snapshot-1", environmentregistry.NetworkPolicyIsolated); err == nil {
		t.Fatal("expected an error without a client")
	}
}

// TestSnapshotProviderPropagatesCreateError proves boot failures surface.
func TestSnapshotProviderPropagatesCreateError(t *testing.T) {
	client := &fakeClient{createErr: errors.New("quota exceeded")}
	provider := &DaytonaSnapshotProvider{Client: client}
	_, err := provider.BootFresh(context.Background(), "snapshot-1", environmentregistry.NetworkPolicyIsolated)
	if err == nil || !strings.Contains(err.Error(), "quota exceeded") {
		t.Fatalf("expected the create error to surface, got %v", err)
	}
}

// TestPromoteSnapshotRecordsName proves promotion names the snapshot, applies
// a default timeout, and surfaces sandbox faults.
func TestPromoteSnapshotRecordsName(t *testing.T) {
	sandbox := &fakeSandbox{id: "sandbox-build"}
	name, err := PromoteSnapshot(context.Background(), sandbox, "codegym-spring-boot-abc", 0)
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if name != "codegym-spring-boot-abc" {
		t.Fatalf("name = %q", name)
	}
	if sandbox.snapshotName != "codegym-spring-boot-abc" {
		t.Fatalf("sandbox recorded %q", sandbox.snapshotName)
	}
	if sandbox.snapshotTTL != 10*time.Minute {
		t.Fatalf("expected the default promotion timeout, got %s", sandbox.snapshotTTL)
	}
	if _, err := PromoteSnapshot(context.Background(), nil, "name", time.Minute); err == nil {
		t.Fatal("expected an error without a sandbox")
	}
}
