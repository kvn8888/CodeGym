package envintegration

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/agentruntime"
	"github.com/kvn8888/codegym/backend/internal/envbuild"
	"github.com/kvn8888/codegym/backend/internal/environmentregistry"
	"github.com/kvn8888/codegym/backend/internal/environmentverify"
	"github.com/kvn8888/codegym/backend/internal/practice"
)

type mapSavedStore struct {
	records map[string]practice.SavedExercise
}

func (s *mapSavedStore) SaveExercise(_ context.Context, saved practice.SavedExercise) error {
	if s.records == nil {
		s.records = map[string]practice.SavedExercise{}
	}
	s.records[saved.ID] = saved
	return nil
}

func (s *mapSavedStore) GetSavedExercise(_ context.Context, id string) (practice.SavedExercise, error) {
	saved, ok := s.records[id]
	if !ok {
		return practice.SavedExercise{}, practice.ErrSavedNotFound
	}
	return saved, nil
}

func (s *mapSavedStore) ListSavedExercises(_ context.Context, owner, workspaceID string) ([]practice.SavedExercise, error) {
	listed := []practice.SavedExercise{}
	for _, saved := range s.records {
		if saved.Owner == owner && saved.WorkspaceID == workspaceID {
			listed = append(listed, saved)
		}
	}
	return listed, nil
}

type practiceHarness struct {
	registry *environmentregistry.Service
	dirs     *SnapshotDirectories
	saved    *mapSavedStore
}

func newPracticeHarness(client *scriptedAgentClient, technology, objective string, assessment *environmentverify.AssessmentSpec, assessor *fakeAssessor) (*practice.Service, *practiceHarness) {
	dirs := &SnapshotDirectories{}
	snapshots := &fakeSnapshotProvider{dirs: dirs}
	preparer := &envbuild.Preparer{
		Runtime:     &agentruntime.PurposeBuiltRuntime{Client: client},
		Provisioner: envbuild.HostProvisioner{},
	}
	registryService := &environmentregistry.Service{
		Store:          environmentregistry.NewInMemoryStore(),
		Snapshots:      snapshots,
		Builder:        &RegistryBuilder{Preparer: preparer, RuntimeName: "purpose-built", Snapshots: dirs},
		Capabilities:   &fakeCapabilities{allowed: map[string]bool{technology: true}},
		Quota:          &fakeQuota{denied: map[string]environmentregistry.QuotaDenialReason{}},
		Verifier:       &RegistryVerifier{Roots: dirs.Resolve, Runner: &LocalCommandRunner{}, Assessment: assessment, Assessor: assessor},
		PracticePolicy: environmentregistry.NetworkPolicyIsolated,
	}
	contents := &SnapshotContents{Roots: dirs.Resolve}
	saved := &mapSavedStore{}
	service := &practice.Service{
		Artifacts:      &RegistryArtifactSource{Store: registryService.Store, Contents: contents},
		Contents:       contents,
		Runner:         &PracticeLocalRunner{},
		Records:        practice.NewInMemoryRecordStore(),
		Saved:          saved,
		CommandTimeout: 30 * time.Second,
	}
	return service, &practiceHarness{registry: registryService, dirs: dirs, saved: saved}
}

func (h *practiceHarness) promote(t *testing.T, technology, objective, owner string) environmentregistry.ArtifactRecord {
	t.Helper()
	resolution, err := h.registry.Resolve(t.Context(), environmentregistry.ResolveRequest{Technology: technology, Objective: objective, Owner: owner})
	if err != nil {
		t.Fatal(err)
	}
	if resolution.Status != environmentregistry.ResolutionBuilt {
		t.Fatalf("status = %q, want built", resolution.Status)
	}
	return resolution.Record
}

func TestPracticeEndToEnd(t *testing.T) {
	client := &scriptedAgentClient{turns: demoFixtureTurns()}
	spec, assessor := demoAssessment(expectedDemoArtifactID())
	service, harness := newPracticeHarness(client, "demo", "learn demo", spec, assessor)
	record := harness.promote(t, "demo", "learn demo", "owner-1")
	service.Runner.(*PracticeLocalRunner).Root = t.TempDir()

	ref := practice.ArtifactRef{ArtifactID: record.ArtifactID, Technology: "demo", Version: record.ArtifactVersion, Objective: "learn demo"}
	session, err := service.OpenBrowserSession(t.Context(), "owner-1", "workspace-1", ref)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range session.Files {
		if strings.HasPrefix(file.Path, ".codegym/") {
			t.Fatalf("hidden file served to browser: %q", file.Path)
		}
	}
	if err := service.WriteBrowserFiles(session.SessionID, "owner-1", "workspace-1", map[string]string{"solution.py": "VALUE = 2\n"}); err != nil {
		t.Fatal(err)
	}
	if err := service.WriteBrowserFiles(session.SessionID, "owner-1", "workspace-1", map[string]string{"protected/test_demo.py": "tampered"}); err == nil {
		t.Fatal("expected protected write to be refused")
	}
	run, err := service.Run(t.Context(), session.SessionID, "owner-1")
	if err != nil {
		t.Fatal(err)
	}
	if run.Category != practice.CategoryPassed {
		t.Fatalf("run category = %q, want passed", run.Category)
	}
	submission, err := service.Submit(t.Context(), session.SessionID, "owner-1")
	if err != nil {
		t.Fatal(err)
	}
	if submission.Category != practice.CategoryPassed {
		t.Fatalf("submit category = %q, want passed", submission.Category)
	}
	saved, err := service.SaveBrowserExercise(t.Context(), session.SessionID, "owner-1", "workspace-1")
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := service.ResumeBrowserSaved(t.Context(), "owner-1", "workspace-1", saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	restored := ""
	for _, file := range resumed.Files {
		if file.Path == "solution.py" {
			restored = file.Content
		}
	}
	if restored != "VALUE = 2\n" {
		t.Fatalf("resumed solution = %q, want saved draft", restored)
	}
	tampered := saved
	tampered.BuilderRunID = "build_tampered"
	harness.saved.records[saved.ID] = tampered
	if _, err := service.ResumeBrowserSaved(t.Context(), "owner-1", "workspace-1", saved.ID); err == nil {
		t.Fatal("expected tampered build identity to be refused")
	}
}

func supportFileFixtureTurns() []agentruntime.CompletionResponse {
	return []agentruntime.CompletionResponse{
		writeFilesTurn(map[string]string{
			"solution.py":       "print('hi')\n",
			"requirements.txt":  "requests==2.31.0\n",
			"protected/test_demo.py":         "assert True\n",
			".codegym/hidden/test_hidden.py": "assert True\n",
			`.codegym/commands.json`: `{"setup":"true","build":"test -f requirements.txt","test":"true","run":"true","base":"generic"}`,
		}),
		{Message: agentruntime.ChatMessage{Role: "assistant", Content: "DONE"}},
	}
}

// TestPracticeMaterializesSnapshotSupportFiles proves the local runner
// serves multi-file projects: the build requires support/data.txt, which
// is not learner-overlay content but sealed snapshot content. The legacy
// runner without snapshot materialization fails the build; the wired
// runner passes it. This is the Spring Boot shape (pom.xml and friends
// must reach the execution space, not just the learner overlay).
func TestPracticeMaterializesSnapshotSupportFiles(t *testing.T) {
	client := &scriptedAgentClient{turns: supportFileFixtureTurns()}
	spec, assessor := demoAssessment(expectedDemoArtifactID())
	service, harness := newPracticeHarness(client, "demo", "learn demo", spec, assessor)
	record := harness.promote(t, "demo", "learn demo", "owner-1")
	contents := &SnapshotContents{Roots: harness.dirs.Resolve}
	ref := practice.ArtifactRef{ArtifactID: record.ArtifactID, Technology: "demo", Version: record.ArtifactVersion, Objective: "learn demo"}

	newScopedService := func(runner practice.Runner) *practice.Service {
		return &practice.Service{
			Artifacts:      service.Artifacts,
			Contents:       service.Contents,
			Runner:         runner,
			Records:        practice.NewInMemoryRecordStore(),
			Saved:          harness.saved,
			CommandTimeout: 30 * time.Second,
		}
	}

	// Control: the legacy runner without snapshot materialization fails
	// the build because the support file never reaches the execution
	// space.
	legacyScoped := newScopedService(&PracticeLocalRunner{Root: t.TempDir()})
	legacySession, err := legacyScoped.OpenBrowserSession(t.Context(), "owner-legacy", "workspace-1", ref)
	if err != nil {
		t.Fatal(err)
	}
	legacySubmission, err := legacyScoped.Submit(t.Context(), legacySession.SessionID, "owner-legacy")
	if err != nil {
		t.Fatal(err)
	}
	if legacySubmission.Category != practice.CategoryBuildError {
		t.Fatalf("legacy submit category = %q, want build-error", legacySubmission.Category)
	}

	// Wired runner: the sealed snapshot tree is materialized first, so
	// the support file is present and the build passes. Support files
	// must still never be served to the browser.
	wiredScoped := newScopedService(&PracticeLocalRunner{Root: t.TempDir(), Snapshots: contents})
	wiredSession, err := wiredScoped.OpenBrowserSession(t.Context(), "owner-wired", "workspace-1", ref)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range wiredSession.Files {
		if file.Path == "requirements.txt" {
			t.Fatalf("dependency file served to browser: %q", file.Path)
		}
	}
	submission, err := wiredScoped.Submit(t.Context(), wiredSession.SessionID, "owner-wired")
	if err != nil {
		t.Fatal(err)
	}
	if submission.Category != practice.CategoryPassed {
		t.Fatalf("wired submit category = %q, want passed (output %q)", submission.Category, submission.Output)
	}
}

func protocolFixtureTurns(testCommand string) []agentruntime.CompletionResponse {
	return []agentruntime.CompletionResponse{
		writeFilesTurn(map[string]string{
			"solution.py":                    "VALUE = 1\n",
			"protected/test_demo.py":         "assert True\n",
			".codegym/hidden/test_hidden.py": "assert True\n",
			`.codegym/commands.json`:         `{"setup":"true","build":"true","test":` + strconv.Quote(testCommand) + `,"run":"true","base":"generic"}`,
		}),
		{Message: agentruntime.ChatMessage{Role: "assistant", Content: "DONE"}},
	}
}

// TestPracticeSubmitEvaluatesTestProtocol proves browser practice Submit
// evaluates the same CODEGYM_TEST protocol the verifier speaks: a test
// command that exits 0 but prints FAIL yields test-failure naming the
// intended reason, while all-PASS output still passes. Hidden-test source
// never reaches the learner; only parsed verdict tokens do.
func TestPracticeSubmitEvaluatesTestProtocol(t *testing.T) {
	for _, tt := range []struct {
		name         string
		testCommand  string
		wantCategory practice.Category
		wantSummary  string
	}{
		{
			name:         "failing assertion fails submit for the intended reason",
			testCommand:  `echo "CODEGYM_TEST t1 FAIL code=WRONG_VALUE expected=1 actual=2"`,
			wantCategory: practice.CategoryTestFailure,
			wantSummary:  `t1 (WRONG_VALUE: expected "1", got "2")`,
		},
		{
			name:         "passing assertions still pass",
			testCommand:  `echo "CODEGYM_TEST t1 PASS"`,
			wantCategory: practice.CategoryPassed,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := &scriptedAgentClient{turns: protocolFixtureTurns(tt.testCommand)}
			spec, assessor := demoAssessment(expectedDemoArtifactID())
			service, harness := newPracticeHarness(client, "demo", "learn demo", spec, assessor)
			record := harness.promote(t, "demo", "learn demo", "owner-1")
			contents := &SnapshotContents{Roots: harness.dirs.Resolve}
			scoped := &practice.Service{
				Artifacts:      service.Artifacts,
				Contents:       service.Contents,
				Runner:         &PracticeLocalRunner{Root: t.TempDir(), Snapshots: contents},
				Records:        practice.NewInMemoryRecordStore(),
				Saved:          harness.saved,
				CommandTimeout: 30 * time.Second,
			}
			ref := practice.ArtifactRef{ArtifactID: record.ArtifactID, Technology: "demo", Version: record.ArtifactVersion, Objective: "learn demo"}
			session, err := scoped.OpenBrowserSession(t.Context(), "owner-1", "workspace-1", ref)
			if err != nil {
				t.Fatal(err)
			}
			submission, err := scoped.Submit(t.Context(), session.SessionID, "owner-1")
			if err != nil {
				t.Fatal(err)
			}
			if submission.Category != tt.wantCategory {
				t.Fatalf("submit category = %q, want %q (output %q)", submission.Category, tt.wantCategory, submission.Output)
			}
			if tt.wantSummary != "" && !strings.Contains(submission.Summary, tt.wantSummary) {
				t.Fatalf("submit summary = %q, want it to contain %q", submission.Summary, tt.wantSummary)
			}
		})
	}
}
