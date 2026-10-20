package envintegration

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/daytona/clients/sdk-go/pkg/types"
	"github.com/kvn8888/codegym/backend/internal/agentruntime"
	"github.com/kvn8888/codegym/backend/internal/environmentregistry"
	"github.com/kvn8888/codegym/backend/internal/environmentverify"
)

// stubRuntime stands in for the pinned agent: it reports a completed run
// without touching the sandbox, since the tree is set up by the factory.
type stubRuntime struct {
	termination agentruntime.TerminationReason
	err         error
	tasks       []agentruntime.TaskSpec
}

func (s *stubRuntime) Name() string { return "stub-opencode-sandbox" }

func (s *stubRuntime) Run(_ context.Context, task agentruntime.TaskSpec) (agentruntime.RunResult, error) {
	s.tasks = append(s.tasks, task)
	termination := s.termination
	if termination == "" {
		termination = agentruntime.TerminationCompleted
	}
	return agentruntime.RunResult{Telemetry: agentruntime.Telemetry{
		Runtime:     s.Name(),
		Termination: termination,
	}}, s.err
}

// buildSandbox is a leased build sandbox with a real in-memory tree plus the
// promotion surface the builder uses.
type buildSandbox struct {
	*fakeTreeSandbox
	id        string
	snapshots []string
	deletes   int
}

func (s *buildSandbox) ID() string { return s.id }

func (s *buildSandbox) CreateFolder(_ context.Context, path string) error {
	s.dirs[path] = struct{}{}
	return nil
}

func (s *buildSandbox) Delete(context.Context) error {
	s.deletes++
	return nil
}

func (s *buildSandbox) CreateSnapshot(_ context.Context, name string, _ time.Duration) error {
	s.snapshots = append(s.snapshots, name)
	return nil
}

// buildClient hands out buildSandboxes and records snapshot deletions.
type buildClient struct {
	sandboxes  []*buildSandbox
	deleted    []string
	lastParams types.SnapshotParams
	createErr  error
}

func (c *buildClient) Create(_ context.Context, params types.SnapshotParams) (SnapshottingSandbox, error) {
	if c.createErr != nil {
		return nil, c.createErr
	}
	sandbox := &buildSandbox{fakeTreeSandbox: newFakeTreeSandbox(), id: "build-sandbox-1"}
	c.sandboxes = append(c.sandboxes, sandbox)
	c.lastParams = params
	return sandbox, nil
}

func (c *buildClient) DeleteSnapshot(_ context.Context, name string) error {
	c.deleted = append(c.deleted, name)
	return nil
}

// seedBuildTree writes the artifact the agent would have produced into the
// sandbox, so the materializer, manifest drafting, and verification all have
// real content to work with.
func seedBuildTree(t *testing.T, sandbox *buildSandbox, root string) {
	t.Helper()
	files := map[string]string{
		"README.md":                      "# exercise",
		"src/main.go":                    "package main\nfunc Add(a, b int) int { return a + b }",
		"src/main_test.go":               "package main\n// hidden-shaped test file",
		".codegym/commands.json":         `{"base":"golang:1.22","setup":"go mod download","build":"go build ./...","test":"go test ./...","run":"go run ."}`,
		".codegym/starter/src/main.go":   "package main\nfunc Add(a, b int) int { return 0 }",
		".codegym/hidden/hidden_test.go": "package main\n// hidden assertion file",
	}
	for rel, content := range files {
		if err := sandbox.UploadFile(context.Background(), []byte(content), root+"/"+rel); err != nil {
			t.Fatalf("seed %s: %v", rel, err)
		}
	}
}

func newDaytonaBuilderForTest(t *testing.T, client DaytonaClient, runtime agentruntime.AgentRuntime, builds *DaytonaBuilds) (*DaytonaBuilder, string) {
	t.Helper()
	root := DefaultSandboxWorkspaceDir
	builder := &DaytonaBuilder{
		Client:       client,
		BaseSnapshot: "codegym-base",
		WorkspaceDir: root,
		Builds:       builds,
		NewRuntime: func(_ context.Context, _ environmentregistry.BuildRequest, sandbox agentruntime.SandboxExecutor) (agentruntime.AgentRuntime, func(), error) {
			if build, ok := sandbox.(*buildSandbox); ok {
				seedBuildTree(t, build, root)
			}
			return runtime, nil, nil
		},
		TurnCeiling: 8,
		Timeout:     time.Minute,
	}
	return builder, root
}

var errBoom = errors.New("boom")

func TestDaytonaBuilderPreparesInSandboxAndPromotes(t *testing.T) {
	client := &buildClient{}
	builds := &DaytonaBuilds{}
	runtime := &stubRuntime{}
	builder, root := newDaytonaBuilderForTest(t, client, runtime, builds)

	output, err := builder.Build(context.Background(), environmentregistry.BuildRequest{
		Technology: "go",
		Objective:  "sum two integers",
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(client.sandboxes) != 1 {
		t.Fatalf("expected one leased sandbox, got %d", len(client.sandboxes))
	}
	sandbox := client.sandboxes[0]
	if len(sandbox.snapshots) != 1 {
		t.Fatalf("expected one promoted snapshot, got %v", sandbox.snapshots)
	}
	if output.SnapshotRef != sandbox.snapshots[0] {
		t.Fatalf("snapshot ref = %q, want the promoted snapshot %q", output.SnapshotRef, sandbox.snapshots[0])
	}
	if !strings.HasPrefix(output.SnapshotRef, "codegym-go-") {
		t.Fatalf("snapshot ref = %q, want it derived from the request", output.SnapshotRef)
	}
	// The build sandbox is promoted, so it must be non-ephemeral and must be
	// allowed network access for dependency installation.
	if client.lastParams.Snapshot != "codegym-base" {
		t.Fatalf("base snapshot = %q", client.lastParams.Snapshot)
	}
	if client.lastParams.Ephemeral || client.lastParams.NetworkBlockAll {
		t.Fatalf("build sandbox params = %+v, want networked and non-ephemeral", client.lastParams.SandboxBaseParams)
	}
	// The agent ran in the sandbox workspace, not on the host.
	if len(runtime.tasks) != 1 || runtime.tasks[0].WorkingDirectory != root {
		t.Fatalf("agent tasks = %+v, want one task in %q", runtime.tasks, root)
	}
	if !strings.Contains(runtime.tasks[0].Goal, "Prepare a coding practice exercise for go") {
		t.Fatalf("agent goal = %q, want the generic prep instructions", runtime.tasks[0].Goal)
	}
	if output.Manifest.Artifact == nil || output.Manifest.Artifact.Technology != "go" {
		t.Fatalf("manifest artifact = %+v, want technology go", output.Manifest.Artifact)
	}
	if output.Builder.ProposedManifestPath == nil || *output.Builder.ProposedManifestPath == "" {
		t.Fatal("builder output names no manifest path")
	}
	// The build is live for verification, so the sandbox is still leased.
	if sandbox.deletes != 0 {
		t.Fatalf("sandbox deleted %d times during build, want 0", sandbox.deletes)
	}
}

func TestDaytonaBuilderReleasesSandboxOnAgentFailure(t *testing.T) {
	client := &buildClient{}
	builds := &DaytonaBuilds{}
	runtime := &stubRuntime{err: errBoom}
	builder, _ := newDaytonaBuilderForTest(t, client, runtime, builds)

	if _, err := builder.Build(context.Background(), environmentregistry.BuildRequest{Technology: "go"}); err == nil {
		t.Fatal("expected a build error")
	}
	if client.sandboxes[0].deletes == 0 {
		t.Fatal("failed build did not release its sandbox")
	}
}

func TestDaytonaBuilderRejectsIncompleteAgentRun(t *testing.T) {
	client := &buildClient{}
	builds := &DaytonaBuilds{}
	runtime := &stubRuntime{termination: agentruntime.TerminationTurnCeiling}
	builder, _ := newDaytonaBuilderForTest(t, client, runtime, builds)

	if _, err := builder.Build(context.Background(), environmentregistry.BuildRequest{Technology: "go"}); err == nil {
		t.Fatal("expected a build error for an incomplete agent run")
	}
	if client.sandboxes[0].snapshots != nil {
		t.Fatal("incomplete run must not be promoted")
	}
}

// verifyAfterBuild runs Build then Verify, returning the verdict and the
// sandbox so tests can inspect what ran where.
func verifyAfterBuild(t *testing.T, mutate func(*buildSandbox)) (environmentverify.VerifierResult, *DaytonaBuilds, *buildSandbox, *buildClient) {
	t.Helper()
	client := &buildClient{}
	builds := &DaytonaBuilds{}
	builder, _ := newDaytonaBuilderForTest(t, client, &stubRuntime{}, builds)

	output, err := builder.Build(context.Background(), environmentregistry.BuildRequest{Technology: "go", Objective: "sum"})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	sandbox := client.sandboxes[0]
	if mutate != nil {
		mutate(sandbox)
	}
	verifier := &DaytonaVerifier{Client: client, Builds: builds}
	return verifier.Verify(context.Background(), output), builds, sandbox, client
}

func TestDaytonaVerifierRunsStagesAndAssessmentsInSandbox(t *testing.T) {
	// The wrong-solution assessment must fail; the correct one must pass.
	result, builds, sandbox, _ := verifyAfterBuild(t, func(sandbox *buildSandbox) {
		sandbox.results["/wrong-solution"] = fakeCommandResult{exitCode: 1, output: "AssertionError"}
	})

	if result.Verdict != environmentverify.VerdictVerified {
		t.Fatalf("verdict = %q (%s), want verified", result.Verdict, firstDiagnostic(result))
	}
	joined := strings.Join(sandbox.cmds, "\n")
	// Build and test ran in the sandbox; setup did not (recorded as satisfied).
	if !strings.Contains(joined, "go build ./...") || !strings.Contains(joined, "go test ./...") {
		t.Fatalf("stage commands did not run in the sandbox:\n%s", joined)
	}
	if strings.Contains(joined, "go mod download") {
		t.Fatalf("setup must not be re-run:\n%s", joined)
	}
	// Both assessments ran in the sandbox against uploaded workspaces.
	if !strings.Contains(joined, "/correct-solution") || !strings.Contains(joined, "/wrong-solution") {
		t.Fatalf("assessments did not run in the sandbox:\n%s", joined)
	}
	// The verification sandbox is released after judging.
	if sandbox.deletes != 1 {
		t.Fatalf("sandbox deleted %d times, want 1", sandbox.deletes)
	}
	if len(builds.entries) != 0 {
		t.Fatalf("build registry still holds %d entries", len(builds.entries))
	}
}

func TestDaytonaVerifierRejectionDeletesSnapshot(t *testing.T) {
	result, _, _, client := verifyAfterBuild(t, func(sandbox *buildSandbox) {
		// A failing build stage rejects the artifact.
		sandbox.results["go build ./..."] = fakeCommandResult{exitCode: 1, output: "compile error"}
	})
	if result.Verdict == environmentverify.VerdictVerified {
		t.Fatal("verdict = verified, want rejected when the build stage fails")
	}
	if got := firstDiagnostic(result); got != environmentverify.CodeBuildFailed {
		t.Fatalf("diagnostic code = %q, want %q", got, environmentverify.CodeBuildFailed)
	}
	// A rejected artifact must not leave a bootable snapshot behind.
	if len(client.deleted) != 1 {
		t.Fatalf("deleted snapshots = %v, want the rejected snapshot deleted", client.deleted)
	}
}

func TestDaytonaVerifierWithoutLiveBuildIsPlatformFault(t *testing.T) {
	verifier := &DaytonaVerifier{Builds: &DaytonaBuilds{}}
	result := verifier.Verify(context.Background(), environmentregistry.BuildOutput{
		Builder: environmentverify.BuilderResult{
			SchemaVersion: environmentverify.BuilderResultSchemaVersion,
			BuilderRunID:  "build_x",
		},
		SnapshotRef: "missing-snapshot",
	})
	if result.Verdict != environmentverify.VerdictRejected {
		t.Fatalf("verdict = %q, want rejected", result.Verdict)
	}
	if result.FailedStage == nil || *result.FailedStage != environmentverify.StagePlatform {
		t.Fatalf("failed stage = %v, want platform", result.FailedStage)
	}
}

func firstDiagnostic(result environmentverify.VerifierResult) string {
	if len(result.Diagnostics) == 0 {
		return ""
	}
	return result.Diagnostics[0].Code
}

// TestDaytonaBuilderMaterializationSkipsBuildOutput proves the host mirror
// stays small: generated directories are not downloaded.
func TestDaytonaBuilderMaterializationSkipsBuildOutput(t *testing.T) {
	localRoot := t.TempDir()
	sandbox := newFakeTreeSandbox()
	for _, path := range []string{
		"/home/agent/work/src/main.go",
		"/home/agent/work/target/classes/App.class",
		"/home/agent/work/node_modules/pkg/index.js",
	} {
		if err := sandbox.UploadFile(context.Background(), []byte("x"), path); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	materializer := SandboxTreeMaterializer{Executor: sandbox, Skip: skipBuildOutput}
	if err := materializer.Materialize(context.Background(), "/home/agent/work", localRoot); err != nil {
		t.Fatalf("materialize: %v", err)
	}
	if _, err := os.Stat(filepath.Join(localRoot, "src", "main.go")); err != nil {
		t.Fatalf("source file missing from mirror: %v", err)
	}
	for _, skipped := range []string{"target", "node_modules"} {
		if _, err := os.Stat(filepath.Join(localRoot, skipped)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s should not be mirrored: %v", skipped, err)
		}
	}
}
