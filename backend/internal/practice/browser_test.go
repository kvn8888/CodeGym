package practice

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func browserService(t *testing.T) (*Service, *fakeArtifactSource, *fakeContentStore, *fakeRunner) {
	t.Helper()
	service, source, store, runner := practiceService(t)
	service.Saved = NewInMemorySavedExerciseStore()
	service.Simulated = true
	return service, source, store, runner
}

func openBrowser(t *testing.T, service *Service, owner, workspace string) BrowserSession {
	t.Helper()
	view, err := service.OpenBrowserSession(context.Background(), owner, workspace, ArtifactRef{
		ArtifactID: "greet-basics-2026-10-01",
		Technology: "python",
		Version:    "0.1.0",
	})
	if err != nil {
		t.Fatalf("OpenBrowserSession: %v", err)
	}
	return view
}

func TestBrowserOpenRequiresArtifactPin(t *testing.T) {
	service, _, _, _ := browserService(t)
	_, err := service.OpenBrowserSession(context.Background(), "owner-1", "ws-1", ArtifactRef{Technology: "python"})
	if err == nil || !strings.Contains(err.Error(), "artifact_id pin") {
		t.Fatalf("err = %v, want explicit artifact-ID pin requirement", err)
	}
}

func TestBrowserOpenServesSafeInstructions(t *testing.T) {
	service, _, _, _ := browserService(t)
	view := openBrowser(t, service, "owner-1", "ws-1")
	if len(view.Files) != 1 || view.Files[0].Path != "solution.py" {
		t.Fatalf("files = %+v, want exactly the editable set", view.Files)
	}
	if !strings.Contains(view.Instructions, "greet") {
		t.Fatalf("instructions missing exercise description")
	}
	for _, leak := range []string{"test_greet", "correct.py", "wrong.py", "SAMPLE_ASSERT", "STRIP_WHITESPACE"} {
		if strings.Contains(view.Instructions, leak) {
			t.Fatalf("instructions leak %q", leak)
		}
	}
	if view.ArtifactID != "greet-basics-2026-10-01" || view.ArtifactVersion != "0.1.0" {
		t.Fatalf("identity = %s %s, want exact served version", view.ArtifactID, view.ArtifactVersion)
	}
	if view.WorkspaceID != "ws-1" || view.Owner != "owner-1" {
		t.Fatalf("ownership = %+v, want backend identity", view)
	}
	if !view.Simulated {
		t.Fatalf("simulated = false, want explicit dev-only label")
	}
}

func TestBrowserWriteValidatesPathsAndSizes(t *testing.T) {
	service, _, _, _ := browserService(t)
	service.MaxFileBytes = 10
	view := openBrowser(t, service, "owner-1", "ws-1")
	if err := service.WriteBrowserFiles(view.SessionID, "owner-1", "ws-1", map[string]string{"solution.py": "short"}); err != nil {
		t.Fatalf("WriteBrowserFiles: %v", err)
	}
	if err := service.WriteBrowserFiles(view.SessionID, "owner-1", "ws-1", map[string]string{"solution.py": "this content is too long"}); err == nil {
		t.Fatalf("oversize write = nil, want refusal")
	}
	if err := service.WriteBrowserFiles(view.SessionID, "owner-1", "ws-1", map[string]string{"protected/test_greet.py": "x"}); err == nil {
		t.Fatalf("protected write = nil, want refusal")
	}
	if err := service.WriteBrowserFiles(view.SessionID, "intruder", "ws-1", map[string]string{"solution.py": "x"}); err == nil {
		t.Fatalf("cross-owner write = nil, want refusal")
	}
	if err := service.WriteBrowserFiles(view.SessionID, "owner-1", "ws-other", map[string]string{"solution.py": "x"}); err == nil {
		t.Fatalf("cross-workspace write = nil, want refusal")
	}
}

func TestBrowserSaveResumeRestoresEditsAndIdentity(t *testing.T) {
	service, _, _, _ := browserService(t)
	view := openBrowser(t, service, "owner-1", "ws-1")
	if err := service.WriteBrowserFiles(view.SessionID, "owner-1", "ws-1", map[string]string{"solution.py": "learner edit"}); err != nil {
		t.Fatalf("WriteBrowserFiles: %v", err)
	}
	saved, err := service.SaveBrowserExercise(context.Background(), view.SessionID, "owner-1", "ws-1")
	if err != nil {
		t.Fatalf("SaveBrowserExercise: %v", err)
	}
	if saved.ArtifactID != "greet-basics-2026-10-01" || saved.SnapshotRef == "" || saved.BuilderRunID == "" {
		t.Fatalf("saved identity incomplete: %+v", saved)
	}
	if saved.Files["solution.py"] != "learner edit" {
		t.Fatalf("saved files = %+v, want learner edits", saved.Files)
	}
	resumed, err := service.ResumeBrowserSaved(context.Background(), "owner-1", "ws-1", saved.ID)
	if err != nil {
		t.Fatalf("ResumeBrowserSaved: %v", err)
	}
	if resumed.ArtifactID != saved.ArtifactID || resumed.ArtifactVersion != saved.ArtifactVersion {
		t.Fatalf("resumed identity = %s %s, want exact saved identity", resumed.ArtifactID, resumed.ArtifactVersion)
	}
	if len(resumed.Files) != 1 || resumed.Files[0].Content != "learner edit" {
		t.Fatalf("resumed files = %+v, want saved edits restored", resumed.Files)
	}
	if resumed.SessionID == view.SessionID {
		t.Fatalf("resume reused session ID; want a fresh session")
	}
}

func TestBrowserResumeRejectsCrossOwnerAndWorkspace(t *testing.T) {
	service, _, _, _ := browserService(t)
	view := openBrowser(t, service, "owner-1", "ws-1")
	saved, err := service.SaveBrowserExercise(context.Background(), view.SessionID, "owner-1", "ws-1")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := service.ResumeBrowserSaved(context.Background(), "intruder", "ws-1", saved.ID); !errors.Is(err, ErrSavedOwnerMismatch) {
		t.Fatalf("cross-owner resume err = %v, want owner mismatch", err)
	}
	if _, err := service.ResumeBrowserSaved(context.Background(), "owner-1", "ws-other", saved.ID); err == nil {
		t.Fatalf("cross-workspace resume = nil, want refusal")
	}
	if _, err := service.ResumeBrowserSaved(context.Background(), "owner-1", "ws-1", "no-such-saved"); !errors.Is(err, ErrSavedNotFound) {
		t.Fatalf("missing resume err = %v, want not-found", err)
	}
}

func TestBrowserResumeDetectsReplacedBuild(t *testing.T) {
	service, source, _, _ := browserService(t)
	view := openBrowser(t, service, "owner-1", "ws-1")
	saved, err := service.SaveBrowserExercise(context.Background(), view.SessionID, "owner-1", "ws-1")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	replaced := sampleArtifact(t)
	replaced.Record.SnapshotRef = "snap_greet_2"
	source.artifact = replaced
	if _, err := service.ResumeBrowserSaved(context.Background(), "owner-1", "ws-1", saved.ID); !errors.Is(err, ErrBuildChanged) {
		t.Fatalf("replaced-build resume err = %v, want explicit build-changed mismatch", err)
	}
}

func TestBrowserResumeNeverSubstitutesAnotherArtifact(t *testing.T) {
	service, source, _, _ := browserService(t)
	view := openBrowser(t, service, "owner-1", "ws-1")
	saved, err := service.SaveBrowserExercise(context.Background(), view.SessionID, "owner-1", "ws-1")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	other := sampleArtifact(t)
	other.Record.ArtifactID = "other-exercise-1"
	other.Manifest.Artifact.ID = "other-exercise-1"
	source.artifact = other
	if _, err := service.ResumeBrowserSaved(context.Background(), "owner-1", "ws-1", saved.ID); !errors.Is(err, ErrArtifactMismatch) {
		t.Fatalf("other-artifact resume err = %v, want explicit artifact mismatch, never substitution", err)
	}
}

func TestBrowserSaveRequiresStore(t *testing.T) {
	service, _, _, _ := practiceService(t)
	view, err := service.OpenBrowserSession(context.Background(), "owner-1", "ws-1", ArtifactRef{
		ArtifactID: "greet-basics-2026-10-01",
		Technology: "python",
	})
	if err != nil {
		t.Fatalf("OpenBrowserSession: %v", err)
	}
	if _, err := service.SaveBrowserExercise(context.Background(), view.SessionID, "owner-1", "ws-1"); !errors.Is(err, ErrSavedStoreMissing) {
		t.Fatalf("save without store err = %v, want explicit store error", err)
	}
}

func TestBrowserResultCarriesSimulatedAndIdentity(t *testing.T) {
	service, _, _, _ := browserService(t)
	view := openBrowser(t, service, "owner-1", "ws-1")
	result, err := service.Submit(context.Background(), view.SessionID, "owner-1")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if !result.Simulated {
		t.Fatalf("simulated = false, want explicit dev-only label on results")
	}
	if result.ArtifactID != "greet-basics-2026-10-01" {
		t.Fatalf("result artifact = %q, want exact identity", result.ArtifactID)
	}
}

func TestBrowserSavedSurvivesServiceRestart(t *testing.T) {
	savedStore := NewInMemorySavedExerciseStore()
	first, source, _, _ := practiceService(t)
	first.Saved = savedStore
	first.Simulated = true
	view, err := first.OpenBrowserSession(context.Background(), "owner-1", "ws-1", ArtifactRef{
		ArtifactID: "greet-basics-2026-10-01",
		Technology: "python",
		Version:    "0.1.0",
	})
	if err != nil {
		t.Fatalf("OpenBrowserSession: %v", err)
	}
	if err := first.WriteBrowserFiles(view.SessionID, "owner-1", "ws-1", map[string]string{"solution.py": "restart edit"}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	saved, err := first.SaveBrowserExercise(context.Background(), view.SessionID, "owner-1", "ws-1")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	// Restart: a fresh service sharing only the saved store re-resolves the
	// exact artifact and restores edits.
	second, source2, _, _ := practiceService(t)
	second.Saved = savedStore
	second.Simulated = true
	_ = source
	_ = source2
	resumed, err := second.ResumeBrowserSaved(context.Background(), "owner-1", "ws-1", saved.ID)
	if err != nil {
		t.Fatalf("Resume after restart: %v", err)
	}
	if len(resumed.Files) != 1 || resumed.Files[0].Content != "restart edit" {
		t.Fatalf("resumed files = %+v, want edits surviving restart", resumed.Files)
	}
	if resumed.ArtifactID != saved.ArtifactID || resumed.ArtifactVersion != saved.ArtifactVersion {
		t.Fatalf("resumed identity mismatch after restart")
	}
}
