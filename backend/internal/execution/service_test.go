package execution

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/workspace"
)

type fakeRunner struct {
	lastSpec RunSpec
	outcome  RunOutcome
	err      error
}

func (f *fakeRunner) Run(_ context.Context, spec RunSpec) (RunOutcome, error) {
	f.lastSpec = spec
	return f.outcome, f.err
}

func fixedClock() Clock {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	return func() time.Time { return now }
}

func pythonInput() SubmitRunInput {
	return SubmitRunInput{
		ProblemID:  "two-sum",
		Language:   "python",
		Entrypoint: "test_solution.py",
		Strategy:   string(TestStrategyUnit),
		Files: []File{
			{Path: "solution.py", Content: "def two_sum(nums, target): ..."},
			{Path: "test_solution.py", Content: "import solution"},
		},
		Limits: Limits{TimeoutSeconds: 30, MemoryMB: 256, NetworkMode: NetworkModeBlockAll},
	}
}

func TestSubmitRunPasses(t *testing.T) {
	runner := &fakeRunner{outcome: RunOutcome{ExitCode: 0, Output: "PASS 3 cases\n", Duration: 1500 * time.Millisecond}}
	service := NewService(NewInMemoryStore(), runner, fixedClock())

	run, err := service.SubmitRun(scopedContext(), pythonInput())
	if err != nil {
		t.Fatalf("SubmitRun returned error: %v", err)
	}
	if run.Status != StatusPassed {
		t.Fatalf("expected status %s, got %s", StatusPassed, run.Status)
	}
	if run.Mode != "submit" {
		t.Fatalf("absent mode resolved to %q, want submit", run.Mode)
	}
	if !strings.HasPrefix(run.ID, "exec_run_") {
		t.Fatalf("unexpected run id %q", run.ID)
	}
	if run.WorkspaceID != "workspace-1" || run.UserID != "user-1" {
		t.Fatalf("unexpected scope: %s/%s", run.WorkspaceID, run.UserID)
	}
	if run.ExitCode == nil || *run.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %v", run.ExitCode)
	}
	if run.Output != "PASS 3 cases\n" {
		t.Fatalf("unexpected output %q", run.Output)
	}
	if run.DurationMs != 1500 {
		t.Fatalf("expected duration 1500ms, got %d", run.DurationMs)
	}
	if run.CompletedAt == nil {
		t.Fatal("expected CompletedAt to be set")
	}
}

func TestSubmitRunFailsOnNonzeroExit(t *testing.T) {
	runner := &fakeRunner{outcome: RunOutcome{ExitCode: 1, Output: "AssertionError\n"}}
	service := NewService(NewInMemoryStore(), runner, fixedClock())

	run, err := service.SubmitRun(scopedContext(), pythonInput())
	if err != nil {
		t.Fatalf("SubmitRun returned error: %v", err)
	}
	if run.Status != StatusFailed {
		t.Fatalf("expected status %s, got %s", StatusFailed, run.Status)
	}
	if run.Error != "" {
		t.Fatalf("expected no infra error, got %q", run.Error)
	}
}

func TestSubmitRunUsesStructuredJudgeStatus(t *testing.T) {
	exitCode := 0
	runner := &fakeRunner{outcome: RunOutcome{
		ExitCode: 0,
		Result: JudgeResult{
			Schema:   JudgeSchema,
			Status:   JudgeStatusFailed,
			Cases:    []CaseResult{{Name: "case-1", Status: "fail", DurationMs: 2}},
			ExitCode: &exitCode,
		},
		Output: "learner output",
	}}
	service := NewService(NewInMemoryStore(), runner, fixedClock())

	run, err := service.SubmitRun(scopedContext(), pythonInput())
	if err != nil {
		t.Fatalf("SubmitRun returned error: %v", err)
	}
	if run.Status != StatusFailed {
		t.Fatalf("expected structured status %s, got %s", StatusFailed, run.Status)
	}
	if run.ExitCode == nil || *run.ExitCode != 0 {
		t.Fatalf("expected child exit code 0, got %v", run.ExitCode)
	}
}

func TestSubmitRunMapsSupervisorDeathStatuses(t *testing.T) {
	tests := []struct {
		judge JudgeStatus
		want  Status
	}{
		{JudgeStatusTimeout, StatusTimeout},
		{JudgeStatusOutOfMemory, StatusOutOfMemory},
		{JudgeStatusCrashed, StatusCrashed},
	}
	for _, test := range tests {
		t.Run(string(test.judge), func(t *testing.T) {
			detail := "died during case 'case-2'"
			runner := &fakeRunner{outcome: RunOutcome{Result: JudgeResult{
				Schema: JudgeSchema, Status: test.judge, Cases: []CaseResult{},
				FailureDetail: &detail, Stdout: "debug output",
			}}}
			service := NewService(NewInMemoryStore(), runner, fixedClock())

			run, err := service.SubmitRun(scopedContext(), pythonInput())
			if err != nil {
				t.Fatalf("SubmitRun returned error: %v", err)
			}
			if run.Status != test.want || run.JudgeResult == nil || run.JudgeResult.Stdout != "debug output" {
				t.Fatalf("run = %#v", run)
			}
		})
	}
}

func TestSubmitRunRunnerErrorYieldsErrorStatus(t *testing.T) {
	runner := &fakeRunner{err: errors.New("sandbox create failed")}
	store := NewInMemoryStore()
	service := NewService(store, runner, fixedClock())

	run, err := service.SubmitRun(scopedContext(), pythonInput())
	if err != nil {
		t.Fatalf("SubmitRun returned error: %v", err)
	}
	if run.Status != StatusError {
		t.Fatalf("expected status %s, got %s", StatusError, run.Status)
	}
	if run.Error != "sandbox create failed" {
		t.Fatalf("unexpected error %q", run.Error)
	}
	if run.ExitCode != nil {
		t.Fatalf("expected nil exit code, got %v", *run.ExitCode)
	}

	persisted, err := service.GetRun(scopedContext(), run.ID)
	if err != nil {
		t.Fatalf("GetRun returned error: %v", err)
	}
	if persisted.Status != StatusError {
		t.Fatalf("expected persisted status %s, got %s", StatusError, persisted.Status)
	}
}

func TestSubmitRunValidation(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*SubmitRunInput)
	}{
		{"unknown language", func(in *SubmitRunInput) { in.Language = "cobol" }},
		{"unknown strategy", func(in *SubmitRunInput) { in.Strategy = "custom" }},
		{"unknown submission mode", func(in *SubmitRunInput) { in.Mode = "preview" }},
		{"missing limits", func(in *SubmitRunInput) { in.Limits = Limits{} }},
		{"zero timeout", func(in *SubmitRunInput) { in.Limits.TimeoutSeconds = 0 }},
		{"negative timeout", func(in *SubmitRunInput) { in.Limits.TimeoutSeconds = -1 }},
		{"zero memory", func(in *SubmitRunInput) { in.Limits.MemoryMB = 0 }},
		{"negative memory", func(in *SubmitRunInput) { in.Limits.MemoryMB = -1 }},
		{"missing network mode", func(in *SubmitRunInput) { in.Limits.NetworkMode = "" }},
		{"unsupported network mode", func(in *SubmitRunInput) { in.Limits.NetworkMode = "bridge" }},
		{"missing entrypoint", func(in *SubmitRunInput) { in.Entrypoint = "" }},
		{"entrypoint not among files", func(in *SubmitRunInput) { in.Entrypoint = "other.py" }},
		{"empty files", func(in *SubmitRunInput) { in.Files = nil }},
		{"absolute path", func(in *SubmitRunInput) { in.Files[0].Path = "/etc/passwd" }},
		{"parent traversal", func(in *SubmitRunInput) { in.Files[0].Path = "../escape.py" }},
		{"home prefix", func(in *SubmitRunInput) { in.Files[0].Path = "~/escape.py" }},
		{"reserved protocol directory", func(in *SubmitRunInput) { in.Files[0].Path = ".codegym/result.json" }},
		{"duplicate paths", func(in *SubmitRunInput) { in.Files[1].Path = in.Files[0].Path }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &fakeRunner{}
			service := NewService(NewInMemoryStore(), runner, fixedClock())
			input := pythonInput()
			tc.mutate(&input)

			if _, err := service.SubmitRun(scopedContext(), input); err == nil {
				t.Fatal("expected validation error")
			}
			if runner.lastSpec.Entrypoint != "" {
				t.Fatal("runner must not be called on validation failure")
			}
		})
	}
}

func TestSubmitRunTooManyFiles(t *testing.T) {
	input := pythonInput()
	for i := 0; i < maxFiles; i++ {
		input.Files = append(input.Files, File{Path: string(rune('a'+i)) + ".py", Content: "x"})
	}
	service := NewService(NewInMemoryStore(), &fakeRunner{}, fixedClock())

	if _, err := service.SubmitRun(scopedContext(), input); err == nil {
		t.Fatal("expected file-count validation error")
	}
}

func TestSubmitRunUnavailableWithoutRunner(t *testing.T) {
	service := NewService(NewInMemoryStore(), nil, fixedClock())

	_, err := service.SubmitRun(scopedContext(), pythonInput())
	if !errors.Is(err, ErrRunnerUnavailable) {
		t.Fatalf("expected ErrRunnerUnavailable, got %v", err)
	}
}

func TestGetRunScopedToWorkspace(t *testing.T) {
	runner := &fakeRunner{outcome: RunOutcome{ExitCode: 0}}
	service := NewService(NewInMemoryStore(), runner, fixedClock())

	run, err := service.SubmitRun(scopedContext(), pythonInput())
	if err != nil {
		t.Fatalf("SubmitRun returned error: %v", err)
	}

	otherCtx := auth.WithPrincipal(context.Background(), auth.Principal{
		UserID:             "user-2",
		DefaultWorkspaceID: "workspace-2",
		WorkspaceIDs:       []string{"workspace-2"},
	})
	otherCtx = workspace.WithScope(otherCtx, workspace.Scope{WorkspaceID: "workspace-2"})

	if _, err := service.GetRun(otherCtx, run.ID); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("expected ErrRunNotFound for cross-workspace get, got %v", err)
	}
}

func TestListRunsNewestFirst(t *testing.T) {
	runner := &fakeRunner{outcome: RunOutcome{ExitCode: 0}}
	service := NewService(NewInMemoryStore(), runner, fixedClock())

	first, err := service.SubmitRun(scopedContext(), pythonInput())
	if err != nil {
		t.Fatalf("SubmitRun returned error: %v", err)
	}
	second, err := service.SubmitRun(scopedContext(), pythonInput())
	if err != nil {
		t.Fatalf("SubmitRun returned error: %v", err)
	}

	runs, err := service.ListRuns(scopedContext())
	if err != nil {
		t.Fatalf("ListRuns returned error: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(runs))
	}
	if runs[0].ID != second.ID || runs[1].ID != first.ID {
		t.Fatal("expected newest run first")
	}
}

func TestRunnerReceivesResolvedSpec(t *testing.T) {
	runner := &fakeRunner{outcome: RunOutcome{ExitCode: 0}}
	service := NewService(NewInMemoryStore(), runner, fixedClock())

	if _, err := service.SubmitRun(scopedContext(), pythonInput()); err != nil {
		t.Fatalf("SubmitRun returned error: %v", err)
	}
	if runner.lastSpec.Language.Name != "python" {
		t.Fatalf("expected resolved python language, got %q", runner.lastSpec.Language.Name)
	}
	if runner.lastSpec.Entrypoint != "test_solution.py" {
		t.Fatalf("unexpected entrypoint %q", runner.lastSpec.Entrypoint)
	}
	if runner.lastSpec.Strategy != TestStrategyUnit {
		t.Fatalf("unexpected strategy %q", runner.lastSpec.Strategy)
	}
	if len(runner.lastSpec.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(runner.lastSpec.Files))
	}
	if runner.lastSpec.Limits != (Limits{TimeoutSeconds: 30, MemoryMB: 256, NetworkMode: NetworkModeBlockAll}) {
		t.Fatalf("unexpected limits %#v", runner.lastSpec.Limits)
	}
	if got := runner.lastSpec.Language.RunCommand("test_solution.py"); got != "cd ~/work && python3 test_solution.py" {
		t.Fatalf("unexpected run command %q", got)
	}
}

func scopedContext() context.Context {
	ctx := context.Background()
	ctx = auth.WithPrincipal(ctx, auth.Principal{
		UserID:             "user-1",
		DefaultWorkspaceID: "workspace-1",
		WorkspaceIDs:       []string{"workspace-1"},
	})
	return workspace.WithScope(ctx, workspace.Scope{WorkspaceID: "workspace-1"})
}
