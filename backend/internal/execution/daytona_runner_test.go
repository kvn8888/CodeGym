package execution

// Integration tests for DaytonaRunner against the real Daytona API. Run them
// with:
//
//	doppler run -p codegym -c dev -- go test ./internal/execution -run TestDaytonaRunner -count=1
//
// Without DAYTONA_API_KEY set they skip, so `go test ./...` stays green.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/daytona/clients/sdk-go/pkg/types"
)

const passingSolution = `def two_sum(nums, target):
    seen = {}
    for i, n in enumerate(nums):
        if target - n in seen:
            return [seen[target - n], i]
        seen[n] = i
    return []
`

const brokenSolution = `def two_sum(nums, target):
    return []
`

const solutionTests = `import json
import solution
import time

cases = [
    ("basic", ([2, 7, 11, 15], 9), [0, 1]),
    ("unordered", ([3, 2, 4], 6), [1, 2]),
    ("duplicates", ([3, 3], 6), [0, 1]),
]
results = []
for name, args, want in cases:
    started = time.perf_counter()
    got = solution.two_sum(*args)
    passed = got == want
    results.append({"name": name, "status": "pass" if passed else "fail", "duration_ms": int((time.perf_counter() - started) * 1000), "error": None if passed else f"got {got}, want {want}"})
print("CODEGYM_RESULT " + json.dumps({"tests": results, "compile_error": None}, separators=(",", ":")))
print(f"finished {len(cases)} cases")
`

// networkProbe must FAIL inside the sandbox: submission runs are created
// with NetworkBlockAll and may not reach the internet.
const networkProbe = `import json
import urllib.request

try:
    urllib.request.urlopen("https://registry.npmjs.org/", timeout=8)
    print("REACHED")
    raise SystemExit(2)
except SystemExit:
    raise
except Exception as exc:
    print(f"BLOCKED: {type(exc).__name__}")
    print("CODEGYM_RESULT " + json.dumps({"tests":[{"name":"network","status":"pass","duration_ms":0,"error":None}],"compile_error":None}, separators=(",", ":")))
`

func daytonaRunner(t *testing.T) *DaytonaRunner {
	t.Helper()
	apiKey := os.Getenv("DAYTONA_API_KEY")
	if apiKey == "" {
		t.Skip("set DAYTONA_API_KEY (via doppler run -p codegym -c dev) to run Daytona integration tests")
	}
	runner, err := NewDaytonaRunner(apiKey, os.Getenv("DAYTONA_API_URL"))
	if err != nil {
		t.Fatalf("NewDaytonaRunner returned error: %v", err)
	}
	return runner
}

func runnerContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	return ctx
}

func pythonSpec(solution, tests string) RunSpec {
	lang, _ := LanguageFor("python")
	return RunSpec{
		Language:   lang,
		Entrypoint: "test_solution.py",
		Files: []File{
			{Path: "solution.py", Content: solution},
			{Path: "test_solution.py", Content: tests},
		},
		Limits: Limits{TimeoutSeconds: 30, MemoryMB: 256, NetworkMode: NetworkModeBlockAll},
	}
}

func TestBuildSupervisorCommandUsesRunLimits(t *testing.T) {
	spec := pythonSpec(passingSolution, solutionTests)
	spec.Limits = Limits{TimeoutSeconds: 7, MemoryMB: 144, NetworkMode: NetworkModeBlockAll}
	command := buildSupervisorCommand(spec)
	for _, expected := range []string{
		"'--timeout-seconds' '7'",
		"'--memory-mb' '144'",
		"'--output-cap-bytes' '65536'",
		"'--' 'python3' 'test_solution.py'",
	} {
		if !strings.Contains(command, expected) {
			t.Fatalf("supervisor command %q does not contain %q", command, expected)
		}
	}
}

func TestUnitStrategyKeepsSupervisorCommandByteIdentical(t *testing.T) {
	spec := pythonSpec(passingSolution, solutionTests)
	spec.Strategy = TestStrategyUnit
	got := buildSupervisorCommand(spec)
	want := "cd ~/work && 'python3' '.codegym/supervisor.py' '--work-dir' '.codegym' '--timeout-seconds' '30' '--memory-mb' '256' '--output-cap-bytes' '65536' '--' 'python3' 'test_solution.py'"
	if got != want {
		t.Fatalf("unit supervisor command changed\ngot:  %s\nwant: %s", got, want)
	}
}

type fakeDaytonaClient struct {
	sandbox     daytonaRunnerSandbox
	createErr   error
	createCalls int
}

func (f *fakeDaytonaClient) Create(_ context.Context, _ types.SnapshotParams) (daytonaRunnerSandbox, error) {
	f.createCalls++
	return f.sandbox, f.createErr
}

type fakeDaytonaSandbox struct {
	executeResult *types.ExecuteResponse
	executeErr    error
	downloads     map[string][]byte
	downloadErrs  map[string]error
	executeCalls  int
	deleteCalls   int
}

func (f *fakeDaytonaSandbox) CreateFolder(context.Context, string) error { return nil }
func (f *fakeDaytonaSandbox) UploadFile(context.Context, []byte, string) error {
	return nil
}
func (f *fakeDaytonaSandbox) DownloadFile(_ context.Context, path string) ([]byte, error) {
	if err := f.downloadErrs[path]; err != nil {
		return nil, err
	}
	data, ok := f.downloads[path]
	if !ok {
		return nil, errors.New("file not found")
	}
	return data, nil
}
func (f *fakeDaytonaSandbox) ExecuteCommand(context.Context, string, time.Duration) (*types.ExecuteResponse, error) {
	f.executeCalls++
	return f.executeResult, f.executeErr
}
func (f *fakeDaytonaSandbox) Delete(context.Context) error {
	f.deleteCalls++
	return nil
}

func TestDaytonaRunnerDeadlineReturnsTimeoutWithProgress(t *testing.T) {
	sandbox := &fakeDaytonaSandbox{
		executeErr: fmt.Errorf("toolbox execute: %w", context.DeadlineExceeded),
		downloads: map[string][]byte{
			casesRemotePath: []byte("{\"event\":\"case_result\",\"name\":\"first\",\"status\":\"pass\",\"duration_ms\":3}\n{\"event\":\"case_start\",\"name\":\"slow-case\"}\n"),
		},
	}
	runner := &DaytonaRunner{client: &fakeDaytonaClient{sandbox: sandbox}}

	outcome, err := runner.Run(context.Background(), pythonSpec(passingSolution, solutionTests))
	if err != nil {
		t.Fatalf("Run returned platform error for deadline: %v", err)
	}
	if outcome.Result.Status != JudgeStatusTimeout || len(outcome.Result.Cases) != 1 {
		t.Fatalf("timeout result = %#v", outcome.Result)
	}
	if outcome.Result.FailureDetail == nil ||
		!strings.Contains(*outcome.Result.FailureDetail, "overall execution budget") ||
		!strings.Contains(*outcome.Result.FailureDetail, "slow-case") {
		t.Fatalf("failure_detail = %v", outcome.Result.FailureDetail)
	}
}

func TestDaytonaRunnerDeadlineReturnsTimeoutWithoutProgress(t *testing.T) {
	sandbox := &fakeDaytonaSandbox{
		executeErr:   errors.New("Daytona error: context deadline exceeded"),
		downloadErrs: map[string]error{casesRemotePath: errors.New("unavailable after deadline")},
	}
	runner := &DaytonaRunner{client: &fakeDaytonaClient{sandbox: sandbox}}

	outcome, err := runner.Run(context.Background(), pythonSpec(passingSolution, solutionTests))
	if err != nil {
		t.Fatalf("Run returned platform error for deadline: %v", err)
	}
	if outcome.Result.Status != JudgeStatusTimeout || len(outcome.Result.Cases) != 0 {
		t.Fatalf("timeout result = %#v", outcome.Result)
	}
	if outcome.Result.FailureDetail == nil || strings.Contains(*outcome.Result.FailureDetail, "during case") {
		t.Fatalf("failure_detail = %v", outcome.Result.FailureDetail)
	}
}

func TestDaytonaRunnerExecuteAPIFailureRemainsPlatformError(t *testing.T) {
	sandbox := &fakeDaytonaSandbox{executeErr: errors.New("Daytona API unavailable")}
	runner := &DaytonaRunner{client: &fakeDaytonaClient{sandbox: sandbox}}

	outcome, err := runner.Run(context.Background(), pythonSpec(passingSolution, solutionTests))
	if err == nil || !strings.Contains(err.Error(), "execute submission") {
		t.Fatalf("Run error = %v, outcome = %#v", err, outcome)
	}
	if outcome.Result.Schema != 0 {
		t.Fatalf("API failure unexpectedly synthesized a judge result: %#v", outcome.Result)
	}
}

func TestDaytonaRunnerPassingSubmission(t *testing.T) {
	runner := daytonaRunner(t)

	outcome, err := runner.Run(runnerContext(t), pythonSpec(passingSolution, solutionTests))
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if outcome.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d (output: %s)", outcome.ExitCode, outcome.Output)
	}
	if outcome.Result.Status != JudgeStatusPassed {
		t.Fatalf("expected judge status passed, got %s", outcome.Result.Status)
	}
	if !strings.Contains(outcome.Output, "finished 3 cases") {
		t.Fatalf("expected passing output, got: %s", outcome.Output)
	}
	if outcome.Duration <= 0 {
		t.Fatal("expected a positive duration")
	}
}

func TestDaytonaRunnerFailingSubmission(t *testing.T) {
	runner := daytonaRunner(t)

	outcome, err := runner.Run(runnerContext(t), pythonSpec(brokenSolution, solutionTests))
	if err != nil {
		t.Fatalf("Run must not error on a failing submission (nonzero exit is a result): %v", err)
	}
	if outcome.Result.Status != JudgeStatusFailed {
		t.Fatalf("expected failed judge status, got %s (output: %s)", outcome.Result.Status, outcome.Output)
	}
	if len(outcome.Result.Cases) != 3 || outcome.Result.Cases[0].Status != "fail" {
		t.Fatalf("expected structured failing cases, got: %#v", outcome.Result.Cases)
	}
}

func TestDaytonaRunnerNetworkBlocked(t *testing.T) {
	runner := daytonaRunner(t)

	spec := pythonSpec(passingSolution, networkProbe)
	outcome, err := runner.Run(runnerContext(t), spec)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if strings.Contains(outcome.Output, "REACHED") {
		t.Fatalf("sandbox reached the internet — NetworkBlockAll is not set: %s", outcome.Output)
	}
	if !strings.Contains(outcome.Output, "BLOCKED") {
		t.Fatalf("expected blocked-egress output, got (exit %d): %s", outcome.ExitCode, outcome.Output)
	}
	if outcome.Result.Status != JudgeStatusPassed {
		t.Fatalf("expected passed network probe result, got %s", outcome.Result.Status)
	}
}
