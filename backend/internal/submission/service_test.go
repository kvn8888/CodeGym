package submission

import (
	"testing"

	"github.com/kvn8888/codegym/backend/internal/execution"
)

func TestViewFromRunSurfacesSupervisorDeathsAndStdout(t *testing.T) {
	tests := []struct {
		runStatus   execution.Status
		judgeStatus execution.JudgeStatus
		wantStatus  Status
	}{
		{execution.StatusTimeout, execution.JudgeStatusTimeout, StatusTimeout},
		{execution.StatusOutOfMemory, execution.JudgeStatusOutOfMemory, StatusOutOfMemory},
		{execution.StatusCrashed, execution.JudgeStatusCrashed, StatusCrashed},
	}
	for _, test := range tests {
		t.Run(string(test.wantStatus), func(t *testing.T) {
			detail := "failed during case 'case-7'"
			view := viewFromRun(execution.Run{
				Status: test.runStatus,
				JudgeResult: &execution.JudgeResult{
					Schema: execution.JudgeSchema, Status: test.judgeStatus,
					FailureDetail: &detail, Stdout: "learner debug\n", OutputTruncated: true,
				},
			})
			if view.Status != test.wantStatus || view.FailureDetail == nil || *view.FailureDetail != detail {
				t.Fatalf("view = %#v", view)
			}
			if view.Stdout != "learner debug\n" || !view.OutputTruncated || view.Result != nil {
				t.Fatalf("view output = %#v", view)
			}
		})
	}
}

func TestViewFromRunUsesStructuredCompletedVerdict(t *testing.T) {
	exitCode := 0
	view := viewFromRun(execution.Run{
		Status: execution.StatusPassed, DurationMs: 12,
		JudgeResult: &execution.JudgeResult{
			Schema: execution.JudgeSchema, Status: execution.JudgeStatusPassed,
			ExitCode: &exitCode, DurationMs: 12, Stdout: "hello\n",
			Cases: []execution.CaseResult{{Name: "case-1", Status: "pass", DurationMs: 2}},
		},
	})
	if view.Status != StatusCompleted || view.Result == nil || view.Result.Passed != 1 || view.Stdout != "hello\n" {
		t.Fatalf("view = %#v", view)
	}
}

func TestViewFromRunSurfacesCompileError(t *testing.T) {
	compileError := "solution.go:3: syntax error: unexpected }"
	exitCode := 0
	view := viewFromRun(execution.Run{
		Status: execution.StatusFailed, DurationMs: 18,
		JudgeResult: &execution.JudgeResult{
			Schema: execution.JudgeSchema, Status: execution.JudgeStatusFailed,
			ExitCode: &exitCode, DurationMs: 18, Cases: []execution.CaseResult{},
			CompileError: &compileError,
		},
	})
	if view.Status != StatusCompleted || view.Result == nil || view.Result.Status != "fail" {
		t.Fatalf("view = %#v", view)
	}
	if view.Result.CompileError == nil || *view.Result.CompileError != compileError || view.Result.Total != 0 {
		t.Fatalf("compile result = %#v", view.Result)
	}
}
