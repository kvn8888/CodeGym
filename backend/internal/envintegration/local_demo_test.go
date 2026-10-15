package envintegration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/agentruntime"
	"github.com/kvn8888/codegym/backend/internal/environmentregistry"
	"github.com/kvn8888/codegym/backend/internal/environmentverify"
	"github.com/kvn8888/codegym/backend/internal/practice"
)

const (
	starterGreet   = "echo TODO\n"
	referenceGreet = "echo hello\n"
)

// starterFixtureTurns scripts a builder that writes a complete reference
// project plus a learner starter that fails the hidden test.
func starterFixtureTurns() []agentruntime.CompletionResponse {
	return []agentruntime.CompletionResponse{
		writeFilesTurn(map[string]string{
			"EXERCISE.md":                   "# Greet\n\nMake greet.sh print hello.\n",
			"greet.sh":                      referenceGreet,
			".codegym/starter/greet.sh":     starterGreet,
			".codegym/hidden/test_greet.sh": `[ "$(sh greet.sh)" = hello ]` + "\n",
			`.codegym/commands.json`:        `{"setup":"true","build":"sh -n greet.sh","test":"sh .codegym/hidden/test_greet.sh","run":"sh greet.sh","base":"generic"}`,
		}),
		{Message: agentruntime.ChatMessage{Role: "assistant", Content: "DONE"}},
	}
}

func newTestLocalDemo(t *testing.T, turns []agentruntime.CompletionResponse) *LocalDemo {
	t.Helper()
	client := &scriptedAgentClient{turns: turns}
	demo, err := NewLocalDemo(LocalDemoConfig{
		NewRuntime: func(context.Context) (agentruntime.AgentRuntime, func(), error) {
			return &agentruntime.PurposeBuiltRuntime{Client: client}, nil, nil
		},
		RuntimeName: "purpose-built",
		Saved:       &mapSavedStore{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return demo
}

func waitForOperation(t *testing.T, store *OperationStore, id string) *EnvironmentOperation {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		operation, ok := store.Get(id)
		if !ok {
			t.Fatalf("operation %s disappeared", id)
		}
		if operation.Status != OperationPending {
			return operation
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("operation did not finish")
	return nil
}

func TestLocalDemoServesStarterAndAssessesSubmissions(t *testing.T) {
	demo := newTestLocalDemo(t, starterFixtureTurns())
	started, err := demo.Operations.Start("owner-1", "workspace-1", "shell", "print a greeting")
	if err != nil {
		t.Fatal(err)
	}
	operation := waitForOperation(t, demo.Operations, started.ID)
	if operation.Status != OperationReady {
		t.Fatalf("operation = %+v, want ready", operation)
	}

	session, err := demo.Practice.GetBrowserSession(operation.SessionID, "owner-1", "workspace-1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(session.Instructions, "Make greet.sh print hello") {
		t.Fatalf("instructions = %q, want EXERCISE.md", session.Instructions)
	}
	served := map[string]string{}
	for _, file := range session.Files {
		served[file.Path] = file.Content
	}
	if served["greet.sh"] != starterGreet {
		t.Fatalf("greet.sh served %q, want the starter", served["greet.sh"])
	}
	for path := range served {
		if strings.HasPrefix(path, ".codegym/") {
			t.Fatalf("protected file %q served to learner", path)
		}
	}

	ctx := t.Context()
	result, err := demo.Practice.Submit(ctx, operation.SessionID, "owner-1")
	if err != nil {
		t.Fatal(err)
	}
	if result.Category != practice.CategoryTestFailure {
		t.Fatalf("starter submit = %s (%s), want test failure", result.Category, result.Summary)
	}

	if err := demo.Practice.WriteBrowserFiles(operation.SessionID, "owner-1", "workspace-1", map[string]string{"greet.sh": referenceGreet}); err != nil {
		t.Fatal(err)
	}
	result, err = demo.Practice.Submit(ctx, operation.SessionID, "owner-1")
	if err != nil {
		t.Fatal(err)
	}
	if result.Category != practice.CategoryPassed {
		t.Fatalf("solved submit = %s (%s), want passed", result.Category, result.Summary)
	}

	records, err := demo.Registry.Store.ListByTechnology(ctx, "shell")
	if err != nil || len(records) != 1 {
		t.Fatalf("registry records = %d, %v; want one promoted artifact", len(records), err)
	}
}

func TestLocalDemoRejectsBuildWithoutStarter(t *testing.T) {
	demo := newTestLocalDemo(t, demoFixtureTurns())
	started, err := demo.Operations.Start("owner-1", "workspace-1", "demo", "learn demo")
	if err != nil {
		t.Fatal(err)
	}
	operation := waitForOperation(t, demo.Operations, started.ID)
	if operation.Status != OperationFailed {
		t.Fatalf("operation = %+v, want failed", operation)
	}
}

func TestStarterAssessmentRejectsStarterThatPasses(t *testing.T) {
	dirs := &SnapshotDirectories{}
	root := t.TempDir()
	for path, content := range map[string]string{
		"greet.sh":                  referenceGreet,
		".codegym/starter/greet.sh": referenceGreet,
	} {
		writeHandoffFixture(t, root, path, content)
	}
	build := environmentregistry.BuildOutput{SnapshotRef: dirs.Register(root)}
	build.Manifest.Artifact = &environmentverify.Artifact{ID: "a1", Version: "1", Technology: "shell"}
	build.Manifest.Workspace = &environmentverify.Workspace{LearnerEditable: []string{"greet.sh"}}
	spec, assessor, err := (&StarterAssessmentSource{Roots: dirs.Resolve}).InputsForBuild(t.Context(), build)
	if err != nil {
		t.Fatal(err)
	}
	if spec.ReviewedBy != DemoUnreviewedReviewer {
		t.Fatalf("reviewedBy = %q, want the unreviewed demo label", spec.ReviewedBy)
	}
	work := t.TempDir()
	writeHandoffFixture(t, work, "greet.sh", spec.WrongSolution["greet.sh"])
	report, err := assessor.Assess(t.Context(), environmentverify.AssessmentWork{WorkDir: work, TestCommand: `[ "$(sh greet.sh)" = hello ]`})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Tests[0].Passed {
		t.Fatal("identical starter should pass the suite, which the verifier then rejects as WRONG_SOLUTION_PASSED")
	}
}
