package envintegration

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/agentruntime"
	"github.com/kvn8888/codegym/backend/internal/environmentverify"
)

// fakeCommandResult scripts one sandbox command's outcome.
type fakeCommandResult struct {
	exitCode int
	output   string
	err      error
}

// fakeTreeSandbox is an in-memory sandbox with a real directory tree, so
// materialization and assessment uploads can be tested without a provider.
type fakeTreeSandbox struct {
	tree    map[string][]byte
	dirs    map[string]struct{}
	uploads map[string][]byte
	cmds    []string
	results map[string]fakeCommandResult
}

func newFakeTreeSandbox() *fakeTreeSandbox {
	return &fakeTreeSandbox{
		tree:    map[string][]byte{},
		dirs:    map[string]struct{}{},
		uploads: map[string][]byte{},
		results: map[string]fakeCommandResult{},
	}
}

func (s *fakeTreeSandbox) UploadFile(_ context.Context, content []byte, path string) error {
	s.uploads[path] = append([]byte(nil), content...)
	s.tree[path] = append([]byte(nil), content...)
	parent := path
	for {
		idx := strings.LastIndex(parent, "/")
		if idx <= 0 {
			break
		}
		parent = parent[:idx]
		s.dirs[parent] = struct{}{}
	}
	return nil
}

func (s *fakeTreeSandbox) ExecuteCommand(_ context.Context, command string, _ time.Duration) (int, string, error) {
	s.cmds = append(s.cmds, command)
	for needle, result := range s.results {
		if strings.Contains(command, needle) {
			return result.exitCode, result.output, result.err
		}
	}
	return 0, "", nil
}

func (s *fakeTreeSandbox) DownloadFile(_ context.Context, path string) ([]byte, error) {
	content, ok := s.tree[path]
	if !ok {
		return nil, errors.New("no such file: " + path)
	}
	return content, nil
}

func (s *fakeTreeSandbox) ListFiles(_ context.Context, dir string) ([]agentruntime.SandboxEntry, error) {
	dir = strings.TrimRight(dir, "/")
	prefix := dir + "/"
	seen := map[string]struct{}{}
	var entries []agentruntime.SandboxEntry
	for path := range s.tree {
		if !strings.HasPrefix(path, prefix) {
			continue
		}
		rest := strings.TrimPrefix(path, prefix)
		name := rest
		isDir := false
		if idx := strings.Index(rest, "/"); idx >= 0 {
			name = rest[:idx]
			isDir = true
		}
		child := prefix + name
		if _, ok := seen[child]; ok {
			continue
		}
		seen[child] = struct{}{}
		entries = append(entries, agentruntime.SandboxEntry{Path: child, IsDirectory: isDir})
	}
	for extra := range s.dirs {
		if strings.HasPrefix(extra, prefix) {
			rest := strings.TrimPrefix(extra, prefix)
			if rest == "" || strings.Contains(rest, "/") {
				continue
			}
			if _, ok := seen[extra]; ok {
				continue
			}
			seen[extra] = struct{}{}
			entries = append(entries, agentruntime.SandboxEntry{Path: extra, IsDirectory: true})
		}
	}
	return entries, nil
}

func TestSandboxTreeMaterializerCopiesNestedTree(t *testing.T) {
	sandbox := newFakeTreeSandbox()
	for path, content := range map[string]string{
		"/home/agent/work/pom.xml":                    "<project/>",
		"/home/agent/work/src/main/App.java":          "class App {}",
		"/home/agent/work/.codegym/agent-result.json": `{"version":1,"completed":true}`,
	} {
		if _, _, err := sandbox.ExecuteCommand(context.Background(), "mkdir -p "+filepath.ToSlash(filepath.Dir(path)), time.Minute); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := sandbox.UploadFile(context.Background(), []byte(content), path); err != nil {
			t.Fatalf("upload: %v", err)
		}
	}
	localRoot := t.TempDir()
	materializer := SandboxTreeMaterializer{Executor: sandbox}
	if err := materializer.Materialize(context.Background(), "/home/agent/work", localRoot); err != nil {
		t.Fatalf("materialize: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(localRoot, "src", "main", "App.java"))
	if err != nil {
		t.Fatalf("read materialized file: %v", err)
	}
	if string(got) != "class App {}" {
		t.Fatalf("materialized content = %q", got)
	}
	if _, err := os.Stat(filepath.Join(localRoot, ".codegym", "agent-result.json")); err != nil {
		t.Fatalf("hidden directory not materialized: %v", err)
	}
}

func TestSandboxPathMapperMapsHostPathsToSandbox(t *testing.T) {
	mapper := SandboxPathMapper{LocalRoot: filepath.Join("tmp", "materialized"), RemoteRoot: "/home/agent/work"}
	cases := map[string]string{
		filepath.Join("tmp", "materialized"):                    "/home/agent/work",
		filepath.Join("tmp", "materialized", "workspace"):       "/home/agent/work/workspace",
		filepath.Join("tmp", "materialized", "a", "b", "c.txt"): "/home/agent/work/a/b/c.txt",
	}
	for host, want := range cases {
		got, err := mapper.Remote(host)
		if err != nil {
			t.Fatalf("map %q: %v", host, err)
		}
		if got != want {
			t.Fatalf("map %q = %q, want %q", host, got, want)
		}
	}
	if _, err := mapper.Remote(filepath.Join("elsewhere", "file.txt")); err == nil {
		t.Fatal("expected an error for a path outside the materialized root")
	}
}

func TestSandboxCommandRunnerRunsStageInSandboxDirectory(t *testing.T) {
	sandbox := newFakeTreeSandbox()
	sandbox.results["mvn -q test"] = fakeCommandResult{exitCode: 1, output: "tests failed"}
	runner := SandboxCommandRunner{
		Executor: sandbox,
		Paths:    SandboxPathMapper{LocalRoot: filepath.Join("tmp", "root"), RemoteRoot: "/home/agent/work"},
		Timeout:  time.Minute,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := runner.Run(ctx, environmentverify.CommandSpec{
		Stage:   environmentverify.StageTest,
		Command: "mvn -q test",
		WorkDir: filepath.Join("tmp", "root", "workspace"),
	})
	if err != nil {
		t.Fatalf("run stage: %v", err)
	}
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode)
	}
	if len(sandbox.cmds) != 1 {
		t.Fatalf("expected exactly one sandbox command, got %d: %v", len(sandbox.cmds), sandbox.cmds)
	}
	if !strings.Contains(sandbox.cmds[0], "cd '/home/agent/work/workspace' && mvn -q test") {
		t.Fatalf("unexpected sandbox command: %s", sandbox.cmds[0])
	}
}

func TestSandboxCommandRunnerRejectsEmptyCommand(t *testing.T) {
	runner := SandboxCommandRunner{Executor: newFakeTreeSandbox()}
	if _, err := runner.Run(context.Background(), environmentverify.CommandSpec{WorkDir: "/work"}); err == nil {
		t.Fatal("expected an error for an empty command")
	}
}

func TestSandboxAssessorUploadsWorkspaceRunsTestAndCleansUp(t *testing.T) {
	sandbox := newFakeTreeSandbox()
	sandbox.results["go test ./..."] = fakeCommandResult{output: "ok  example"}
	workDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workDir, "src"), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "src", "main.go"), []byte("package main"), 0o640); err != nil {
		t.Fatalf("write: %v", err)
	}
	assessor := SandboxAssessor{Executor: sandbox, ScratchDir: "/tmp/codegym-assess", Timeout: time.Minute}
	report, err := assessor.Assess(context.Background(), environmentverify.AssessmentWork{
		Role:        environmentverify.RoleCorrectSolution,
		WorkDir:     workDir,
		TestCommand: "go test ./...",
	})
	if err != nil {
		t.Fatalf("assess: %v", err)
	}
	if len(report.Tests) != 1 || !report.Tests[0].Passed {
		t.Fatalf("unexpected report: %+v", report)
	}
	uploaded := "/tmp/codegym-assess/" + environmentverify.RoleCorrectSolution + "/src/main.go"
	if string(sandbox.uploads[uploaded]) != "package main" {
		t.Fatalf("workspace not uploaded: %v", sandbox.uploads)
	}
	joined := strings.Join(sandbox.cmds, "\n")
	if !strings.Contains(joined, "cd '/tmp/codegym-assess/"+environmentverify.RoleCorrectSolution+"' && go test ./...") {
		t.Fatalf("test command did not run in the uploaded workspace:\n%s", joined)
	}
	if !strings.Contains(joined, "rm -rf '/tmp/codegym-assess/"+environmentverify.RoleCorrectSolution+"'") {
		t.Fatalf("scratch directory was not cleaned up:\n%s", joined)
	}
}

func TestSandboxAssessorReportsFailedTest(t *testing.T) {
	sandbox := newFakeTreeSandbox()
	sandbox.results["mvn test"] = fakeCommandResult{exitCode: 1, output: "AssertionError: expected ok"}
	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "pom.xml"), []byte("<project/>"), 0o640); err != nil {
		t.Fatalf("write: %v", err)
	}
	assessor := SandboxAssessor{Executor: sandbox, ScratchDir: "/tmp/codegym-assess"}
	report, err := assessor.Assess(context.Background(), environmentverify.AssessmentWork{
		Role:        environmentverify.RoleWrongSolution,
		WorkDir:     workDir,
		TestCommand: "mvn test",
	})
	if err != nil {
		t.Fatalf("assess: %v", err)
	}
	if len(report.Tests) != 1 || report.Tests[0].Passed {
		t.Fatalf("expected one failed test, got %+v", report)
	}
	if report.Tests[0].TestID != DemoSuiteTestID {
		t.Fatalf("test id = %q, want %q", report.Tests[0].TestID, DemoSuiteTestID)
	}
}

func TestSandboxAssessorRequiresTestCommand(t *testing.T) {
	assessor := SandboxAssessor{Executor: newFakeTreeSandbox()}
	if _, err := assessor.Assess(context.Background(), environmentverify.AssessmentWork{WorkDir: t.TempDir()}); err == nil {
		t.Fatal("expected an error when the manifest declares no test command")
	}
}
