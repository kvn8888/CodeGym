package envintegration

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/execution"
	"github.com/kvn8888/codegym/backend/internal/practice"
)

type fakeDaytonaSandbox struct {
	folders    []string
	uploads    map[string]string
	commands   []string
	deleted    bool
	onExec     func(command string) (int, string, error)
	onDownload func(path string) ([]byte, error)
}

func (f *fakeDaytonaSandbox) CreateFolder(_ context.Context, path string) error {
	f.folders = append(f.folders, path)
	return nil
}

func (f *fakeDaytonaSandbox) UploadFile(_ context.Context, content []byte, path string) error {
	if f.uploads == nil {
		f.uploads = map[string]string{}
	}
	f.uploads[path] = string(content)
	return nil
}

func (f *fakeDaytonaSandbox) DownloadFile(_ context.Context, path string) ([]byte, error) {
	if content, ok := f.uploads[path]; ok {
		return []byte(content), nil
	}
	if f.onDownload != nil {
		return f.onDownload(path)
	}
	return nil, errors.New("no such file: " + path)
}

func (f *fakeDaytonaSandbox) ExecuteCommand(_ context.Context, command string, _ time.Duration) (int, string, error) {
	f.commands = append(f.commands, command)
	if f.onExec != nil {
		return f.onExec(command)
	}
	return 0, "", nil
}

func (f *fakeDaytonaSandbox) Delete(context.Context) error {
	f.deleted = true
	return nil
}

type fakeSandboxFactory struct {
	sandboxes    []*fakeDaytonaSandbox
	networkFlags []bool
	failCreate   error
	onDownload   func(path string) ([]byte, error)
}

func (f *fakeSandboxFactory) Create(_ context.Context, _ string, networkBlockAll bool, _ map[string]string) (DaytonaSandbox, error) {
	if f.failCreate != nil {
		return nil, f.failCreate
	}
	sandbox := &fakeDaytonaSandbox{onDownload: f.onDownload}
	f.sandboxes = append(f.sandboxes, sandbox)
	f.networkFlags = append(f.networkFlags, networkBlockAll)
	return sandbox, nil
}

func practiceRunSpec(executionID, command string, files map[string]string) practice.RunSpec {
	specFiles := make([]execution.File, 0, len(files))
	for path, content := range files {
		specFiles = append(specFiles, execution.File{Path: path, Content: content})
	}
	return practice.RunSpec{
		ExecutionID:   executionID,
		SnapshotRef:   "local-test",
		Command:       command,
		Files:         specFiles,
		NetworkPolicy: "block-all",
	}
}

func TestDaytonaRunnerUploadsAndRunsInWorkspace(t *testing.T) {
	factory := &fakeSandboxFactory{}
	runner := &DaytonaPracticeRunner{Factory: factory}
	outcome, err := runner.RunCommand(t.Context(), practiceRunSpec("exec-1", "true", map[string]string{"solution.py": "VALUE = 1\n"}))
	if err != nil {
		t.Fatal(err)
	}
	if outcome.ExitCode != 0 {
		t.Fatalf("outcome = %#v", outcome)
	}
	sandbox := factory.sandboxes[0]
	if sandbox.uploads["work/solution.py"] != "VALUE = 1\n" {
		t.Fatalf("uploads = %#v", sandbox.uploads)
	}
	if len(sandbox.commands) != 1 || !strings.HasPrefix(sandbox.commands[0], "cd work && ") {
		t.Fatalf("commands = %#v", sandbox.commands)
	}
	if !factory.networkFlags[0] {
		t.Fatal("block-all policy was not forwarded to sandbox creation")
	}
}

func TestDaytonaRunnerReusesSpaceAcrossSteps(t *testing.T) {
	factory := &fakeSandboxFactory{}
	runner := &DaytonaPracticeRunner{Factory: factory}
	if _, err := runner.RunCommand(t.Context(), practiceRunSpec("exec-1", "touch built.txt", nil)); err != nil {
		t.Fatal(err)
	}
	if _, err := runner.RunCommand(t.Context(), practiceRunSpec("exec-1", "test -f built.txt", nil)); err != nil {
		t.Fatal(err)
	}
	if len(factory.sandboxes) != 1 {
		t.Fatalf("sandboxes = %d, want 1 shared across steps", len(factory.sandboxes))
	}
	if _, err := runner.RunCommand(t.Context(), practiceRunSpec("exec-2", "true", nil)); err != nil {
		t.Fatal(err)
	}
	if len(factory.sandboxes) != 2 {
		t.Fatalf("sandboxes = %d, want isolation between executions", len(factory.sandboxes))
	}
}

func TestDaytonaRunnerSweepsIdleSandboxes(t *testing.T) {
	factory := &fakeSandboxFactory{}
	runner := &DaytonaPracticeRunner{Factory: factory, IdleTTL: time.Millisecond}
	if _, err := runner.RunCommand(t.Context(), practiceRunSpec("exec-old", "true", nil)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, err := runner.RunCommand(t.Context(), practiceRunSpec("exec-new", "true", nil)); err != nil {
		t.Fatal(err)
	}
	if !factory.sandboxes[0].deleted {
		t.Fatal("idle sandbox was not reaped")
	}
	if len(factory.sandboxes) != 2 {
		t.Fatalf("sandboxes = %d, want a fresh lease after the sweep", len(factory.sandboxes))
	}
}

func TestDaytonaRunnerCreateFailure(t *testing.T) {
	factory := &fakeSandboxFactory{failCreate: errors.New("no capacity")}
	runner := &DaytonaPracticeRunner{Factory: factory}
	if _, err := runner.RunCommand(t.Context(), practiceRunSpec("exec-1", "true", nil)); err == nil {
		t.Fatal("expected lease failure")
	}
}

func TestSandboxContentsReadsSealedFilesFromOneSandbox(t *testing.T) {
	files := map[string]string{
		"work/EXERCISE.md":         "# exercise",
		"work/starter/solution.py": "VALUE = 0\n",
	}
	factory := &fakeSandboxFactory{}
	contents := &SandboxContents{
		Factory:      factory,
		WorkspaceDir: "work",
	}
	factory.onDownload = func(path string) ([]byte, error) {
		if content, ok := files[path]; ok {
			return []byte(content), nil
		}
		return nil, errors.New("no such file: " + path)
	}

	manifest, err := contents.ReadArtifactFile(t.Context(), "snap-1", "EXERCISE.md")
	if err != nil {
		t.Fatal(err)
	}
	if manifest != "# exercise" {
		t.Fatalf("manifest = %q", manifest)
	}
	starter, err := contents.ReadArtifactFile(t.Context(), "snap-1", "starter/solution.py")
	if err != nil {
		t.Fatal(err)
	}
	if starter != "VALUE = 0\n" {
		t.Fatalf("starter = %q", starter)
	}
	if len(factory.sandboxes) != 1 {
		t.Fatalf("sandboxes = %d, want one reused across reads", len(factory.sandboxes))
	}
	if !factory.networkFlags[0] {
		t.Fatal("read sandbox must boot isolated")
	}
}

func TestSandboxContentsRejectsEscapingPath(t *testing.T) {
	contents := &SandboxContents{Factory: &fakeSandboxFactory{}, WorkspaceDir: "work"}
	for _, path := range []string{"", "../secret", "a/../../b", "/etc/passwd"} {
		if _, err := contents.ReadArtifactFile(t.Context(), "snap-1", path); err == nil {
			t.Fatalf("path %q was not rejected", path)
		}
	}
	if len((contents.Factory).(*fakeSandboxFactory).sandboxes) != 0 {
		t.Fatal("an invalid path must not boot a sandbox")
	}
}

func TestSandboxContentsReapsIdleSandboxes(t *testing.T) {
	factory := &fakeSandboxFactory{}
	factory.onDownload = func(string) ([]byte, error) { return []byte("x"), nil }
	contents := &SandboxContents{Factory: factory, WorkspaceDir: "work", IdleTTL: time.Millisecond}
	if _, err := contents.ReadArtifactFile(t.Context(), "snap-old", "EXERCISE.md"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, err := contents.ReadArtifactFile(t.Context(), "snap-new", "EXERCISE.md"); err != nil {
		t.Fatal(err)
	}
	if !factory.sandboxes[0].deleted {
		t.Fatal("idle read sandbox was not reaped")
	}
	if len(factory.sandboxes) != 2 {
		t.Fatalf("sandboxes = %d, want a fresh lease after the sweep", len(factory.sandboxes))
	}
}
