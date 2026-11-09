package practice

import (
	"context"
	"strings"
	"testing"

	"github.com/kvn8888/codegym/backend/internal/environmentverify"
)

func TestSampleArtifactIdentity(t *testing.T) {
	artifact, err := SampleArtifact()
	if err != nil {
		t.Fatalf("SampleArtifact: %v", err)
	}
	if artifact.Record.ArtifactID != SampleArtifactID || artifact.Record.ArtifactVersion != SampleArtifactVersion {
		t.Fatalf("sample identity = %s %s", artifact.Record.ArtifactID, artifact.Record.ArtifactVersion)
	}
}

func TestSampleSourcePinsExactID(t *testing.T) {
	var source SampleArtifactSource
	if _, err := source.LoadVerifiedArtifact(context.Background(), "owner-1", ArtifactRef{ArtifactID: SampleArtifactID, Technology: "python"}); err != nil {
		t.Fatalf("exact pin: %v", err)
	}
	if _, err := source.LoadVerifiedArtifact(context.Background(), "owner-1", ArtifactRef{ArtifactID: "other-exercise", Technology: "python"}); err == nil {
		t.Fatalf("other ID = nil, want explicit not-found, never substitution")
	}
}

// The fake sample provider is explicitly simulated: static scripted
// outcomes from the learner overlay alone. It never executes learner
// code, never reads hidden/reference files, and never touches the host.
func TestFakeSampleRunnerSimulation(t *testing.T) {
	runner := FakeSampleRunner{}
	starter := RunSpec{
		Stage:   environmentverify.StageTest,
		Command: "python3 protected/test_greet.py",
		Files:   overlayFiles(map[string]string{"solution.py": "def greet(name):\n    raise NotImplementedError\n"}),
		// Even full visibility must not expose hidden files: the fake
		// never reads them.
		Visibility: FileScopeFull,
		Protected:  []string{"protected/test_greet.py"},
	}
	outcome, err := runner.RunCommand(context.Background(), starter)
	if err != nil {
		t.Fatalf("RunCommand starter: %v", err)
	}
	if outcome.ExitCode == 0 {
		t.Fatalf("starter exit = 0, want simulated failure")
	}
	if strings.Contains(outcome.Output, "test_greet") || strings.Contains(outcome.Output, "SAMPLE_ASSERT") {
		t.Fatalf("fake output leaks hidden content: %q", outcome.Output)
	}

	passing := starter
	passing.Files = overlayFiles(map[string]string{"solution.py": "def greet(name):\n    cleaned = name.strip()\n    if not cleaned:\n        return 'Hello, stranger!'\n    return 'Hello, ' + cleaned + '!'\n"})
	outcome, err = runner.RunCommand(context.Background(), passing)
	if err != nil {
		t.Fatalf("RunCommand passing: %v", err)
	}
	if outcome.ExitCode != 0 {
		t.Fatalf("passing exit = %d, want simulated pass", outcome.ExitCode)
	}

	build := RunSpec{
		Stage:   environmentverify.StageBuild,
		Command: "python3 -m py_compile solution.py",
		Files:   overlayFiles(map[string]string{"solution.py": "not python at all"}),
	}
	outcome, err = runner.RunCommand(context.Background(), build)
	if err != nil {
		t.Fatalf("RunCommand build: %v", err)
	}
	if outcome.ExitCode == 0 {
		t.Fatalf("build exit = 0 without greet, want simulated build-error")
	}
}

func TestFakeSampleRunnerEndToEnd(t *testing.T) {
	service := &Service{
		Artifacts: SampleArtifactSource{},
		Contents:  SampleContentStore{},
		Runner:    FakeSampleRunner{},
		Saved:     NewInMemorySavedExerciseStore(),
		Simulated: true,
	}
	view, err := service.OpenBrowserSession(context.Background(), "owner-1", "ws-1", ArtifactRef{
		ArtifactID: SampleArtifactID, Technology: "python", Version: SampleArtifactVersion,
	})
	if err != nil {
		t.Fatalf("OpenBrowserSession: %v", err)
	}
	// Starter fails without leaking hidden content.
	failed, err := service.Submit(context.Background(), view.SessionID, "owner-1")
	if err != nil {
		t.Fatalf("Submit starter: %v", err)
	}
	if failed.Category != CategoryTestFailure && failed.Category != CategoryBuildError {
		t.Fatalf("starter category = %q, want a simulated failure", failed.Category)
	}
	if !failed.Simulated {
		t.Fatalf("simulated = false, want explicit label")
	}
	public := failed.Summary + "\x00" + failed.Output + "\x00" + string(failed.Category)
	for _, secret := range []string{"SAMPLE_ASSERT", "STRIP_WHITESPACE", "test_greet", SampleSnapshotRef, SampleBuilderRunID} {
		if strings.Contains(public, secret) {
			t.Fatalf("simulated result leaks %q: %+v", secret, failed)
		}
	}
	// Simulation-passing content passes, still labelled simulated.
	if err := service.WriteBrowserFiles(view.SessionID, "owner-1", "ws-1", map[string]string{
		"solution.py": "def greet(name):\n    cleaned = name.strip()\n    if not cleaned:\n        return 'Hello, stranger!'\n    return 'Hello, ' + cleaned + '!'\n",
	}); err != nil {
		t.Fatalf("WriteBrowserFiles: %v", err)
	}
	passed, err := service.Submit(context.Background(), view.SessionID, "owner-1")
	if err != nil {
		t.Fatalf("Submit passing: %v", err)
	}
	if passed.Category != CategoryPassed || !passed.Simulated {
		t.Fatalf("passing category = %q simulated=%v, want passed/simulated", passed.Category, passed.Simulated)
	}
}
