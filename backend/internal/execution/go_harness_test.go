package execution

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const localGoTestHarness = `package main

import (
    "encoding/json"
    "os"
    "time"
)

type caseResult struct {
    Name string ` + "`json:\"name\"`" + `
    Status string ` + "`json:\"status\"`" + `
    DurationMS int64 ` + "`json:\"duration_ms\"`" + `
    Error *string ` + "`json:\"error\"`" + `
}

func appendEvent(event any) {
    file, err := os.OpenFile(".codegym/cases.jsonl", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
    if err != nil { panic(err) }
    if err := json.NewEncoder(file).Encode(event); err != nil { panic(err) }
    if err := file.Sync(); err != nil { panic(err) }
    if err := file.Close(); err != nil { panic(err) }
}

func main() {
    cases := []struct{Name string; Left, Right, Expected int}{
        {"positive", 2, 3, 5}, {"negative", -2, 5, 3},
    }
    results := make([]caseResult, 0, len(cases))
    for _, item := range cases {
        appendEvent(map[string]any{"event":"case_start", "name":item.Name})
        started := time.Now()
        actual := add(item.Left, item.Right)
        equal, err := compareValues("exact", item.Expected, actual, nil)
        status := "pass"
        var detail *string
        if err != nil || !equal {
            status = "fail"
            message := "unexpected result"
            if err != nil { message = err.Error() }
            detail = &message
        }
        result := caseResult{Name:item.Name, Status:status, DurationMS:max(0, time.Since(started).Milliseconds()), Error:detail}
        results = append(results, result)
        appendEvent(map[string]any{"event":"case_result", "name":result.Name, "status":result.Status, "duration_ms":result.DurationMS, "error":result.Error})
    }
    status := "passed"
    for _, result := range results { if result.Status == "fail" { status = "failed" } }
    payload := map[string]any{"schema":1, "status":status, "compile_error":nil, "cases":results}
    data, _ := json.Marshal(payload)
    if err := os.WriteFile(".codegym/verdict.json", data, 0600); err != nil { panic(err) }
}
`

func TestGoHarnessProtocolAndCompileError(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is required for the local supervisor test")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is required for the Go harness test")
	}

	t.Run("incremental verdict", func(t *testing.T) {
		result, progress := runLocalGoHarness(t, "package main\n\nfunc add(left, right int) int { return left + right }\n")
		if result.Status != JudgeStatusPassed || len(result.Cases) != 2 {
			t.Fatalf("result = %#v", result)
		}
		lines := strings.Split(strings.TrimSpace(progress), "\n")
		if len(lines) != 4 || !strings.Contains(lines[0], `"event":"case_start"`) || !strings.Contains(lines[1], `"event":"case_result"`) {
			t.Fatalf("incremental progress = %q", progress)
		}
	})

	t.Run("compile error", func(t *testing.T) {
		result, _ := runLocalGoHarness(t, "package main\n\nfunc add(left, right int) int { return left + }\n")
		if result.Status != JudgeStatusFailed || result.CompileError == nil || !strings.Contains(strings.ToLower(*result.CompileError), "syntax error") {
			t.Fatalf("result = %#v", result)
		}
		if len(result.Cases) != 0 {
			t.Fatalf("compile-error cases = %#v", result.Cases)
		}
	})
}

func runLocalGoHarness(t *testing.T, solution string) (JudgeResult, string) {
	t.Helper()
	root := t.TempDir()
	protocolDir := filepath.Join(root, ".codegym")
	if err := os.Mkdir(protocolDir, 0o700); err != nil {
		t.Fatalf("mkdir protocol directory: %v", err)
	}
	files := map[string]string{
		"solution.go":                 solution,
		"codegym_comparator.go":       GoComparatorSource,
		"test_solution.go":            localGoTestHarness,
		".codegym/compile_and_run.py": GoCompileRunnerSource,
		".codegym/supervisor.py":      string(SupervisorScript),
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "python3", ".codegym/supervisor.py",
		"--work-dir", ".codegym", "--timeout-seconds", "20", "--memory-mb", "1024", "--output-cap-bytes", "65536",
		"--", "python3", ".codegym/compile_and_run.py",
	)
	command.Dir = root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("run supervisor: %v\n%s", err, output)
	}
	resultData, err := os.ReadFile(filepath.Join(protocolDir, "result.json"))
	if err != nil {
		t.Fatalf("read result.json: %v", err)
	}
	result, err := ParseJudgeResult(resultData)
	if err != nil {
		t.Fatalf("ParseJudgeResult: %v", err)
	}
	progressData, _ := os.ReadFile(filepath.Join(protocolDir, "cases.jsonl"))
	return result, string(progressData)
}
