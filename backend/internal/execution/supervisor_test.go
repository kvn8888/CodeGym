package execution

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSupervisorScriptDeathModes(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is required to test the embedded supervisor")
	}

	tests := []struct {
		name         string
		child        string
		timeout      string
		memoryMB     string
		wantStatus   JudgeStatus
		wantCase     string
		wantStdout   string
		wantStderr   string
		wantSignal   bool
		wantTruncate bool
	}{
		{
			name: "clean exit",
			child: `import json, pathlib, sys
p = pathlib.Path(".codegym")
p.mkdir(exist_ok=True)
(p / "cases.jsonl").write_text(json.dumps({"event":"case_start","name":"clean"}) + "\n" + json.dumps({"event":"case_result","name":"clean","status":"pass","duration_ms":1,"error":None}) + "\n", encoding="utf-8")
(p / "verdict.json").write_text(json.dumps({"schema":1,"status":"passed","compile_error":None,"cases":[{"name":"clean","status":"pass","duration_ms":1,"error":None}]}), encoding="utf-8")
print("learner stdout")
print("learner stderr", file=sys.stderr)
`,
			timeout: "3", memoryMB: "256", wantStatus: JudgeStatusPassed, wantCase: "clean",
			wantStdout: "learner stdout", wantStderr: "learner stderr",
		},
		{
			name: "timeout",
			child: `import json, pathlib, time
p = pathlib.Path(".codegym")
p.mkdir(exist_ok=True)
with (p / "cases.jsonl").open("a", encoding="utf-8") as f:
    f.write(json.dumps({"event":"case_start","name":"sleeping-case"}) + "\n")
    f.flush()
time.sleep(60)
`,
			timeout: "0.15", memoryMB: "256", wantStatus: JudgeStatusTimeout, wantCase: "sleeping-case", wantSignal: true,
		},
		{
			name: "abort",
			child: `import json, os, pathlib
p = pathlib.Path(".codegym")
p.mkdir(exist_ok=True)
with (p / "cases.jsonl").open("a", encoding="utf-8") as f:
    f.write(json.dumps({"event":"case_start","name":"abort-case"}) + "\n")
    f.flush()
os.abort()
`,
			timeout: "3", memoryMB: "256", wantStatus: JudgeStatusCrashed, wantCase: "abort-case", wantSignal: true,
		},
		{
			name: "per-case alarm",
			child: `import json, pathlib, signal, time
p = pathlib.Path(".codegym")
p.mkdir(exist_ok=True)
with (p / "cases.jsonl").open("a", encoding="utf-8") as f:
    f.write(json.dumps({"event":"case_start","name":"alarm-case"}) + "\n")
    f.flush()
signal.signal(signal.SIGALRM, signal.SIG_DFL)
signal.setitimer(signal.ITIMER_REAL, 0.05)
time.sleep(60)
`,
			timeout: "3", memoryMB: "256", wantStatus: JudgeStatusTimeout, wantCase: "alarm-case", wantSignal: true,
		},
		{
			name: "out of memory",
			child: `import json, pathlib, sys
p = pathlib.Path(".codegym")
p.mkdir(exist_ok=True)
with (p / "cases.jsonl").open("a", encoding="utf-8") as f:
    f.write(json.dumps({"event":"case_start","name":"memory-case"}) + "\n")
    f.flush()
if sys.platform == "darwin":
    raise MemoryError("Darwin's framework Python cannot apply a finite RLIMIT_AS")
chunks = []
while True:
    chunks.append(bytearray(8 * 1024 * 1024))
`,
			timeout: "5", memoryMB: "96", wantStatus: JudgeStatusOutOfMemory, wantCase: "memory-case", wantTruncate: true,
		},
		{
			name: "separate capped output",
			child: `import json, pathlib, sys
p = pathlib.Path(".codegym")
p.mkdir(exist_ok=True)
(p / "verdict.json").write_text(json.dumps({"schema":1,"status":"passed","compile_error":None,"cases":[{"name":"output","status":"pass","duration_ms":0,"error":None}]}), encoding="utf-8")
print("o" * 200)
print("e" * 200, file=sys.stderr)
`,
			timeout: "3", memoryMB: "256", wantStatus: JudgeStatusPassed,
			wantStdout: strings.Repeat("o", 32), wantStderr: strings.Repeat("e", 32), wantTruncate: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			supervisorPath := filepath.Join(root, "supervisor.py")
			childPath := filepath.Join(root, "child.py")
			if err := os.WriteFile(supervisorPath, SupervisorScript, 0o600); err != nil {
				t.Fatalf("write supervisor: %v", err)
			}
			if err := os.WriteFile(childPath, []byte(test.child), 0o600); err != nil {
				t.Fatalf("write child: %v", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, python, supervisorPath,
				"--work-dir", ".codegym",
				"--timeout-seconds", test.timeout,
				"--memory-mb", test.memoryMB,
				"--output-cap-bytes", "32",
				"--", python, childPath,
			)
			cmd.Dir = root
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("supervisor failed: %v\n%s", err, output)
			}
			if ctx.Err() != nil {
				t.Fatalf("supervisor exceeded test deadline: %v", ctx.Err())
			}

			data, err := os.ReadFile(filepath.Join(root, ".codegym", "result.json"))
			if err != nil {
				t.Fatalf("read result.json: %v", err)
			}
			var result JudgeResult
			if err := json.Unmarshal(data, &result); err != nil {
				t.Fatalf("result.json is malformed: %v\n%s", err, data)
			}
			if result.Status != test.wantStatus {
				t.Fatalf("status = %s, want %s; result=%s", result.Status, test.wantStatus, data)
			}
			if test.wantCase != "" && (result.FailureDetail == nil || !strings.Contains(*result.FailureDetail, test.wantCase)) &&
				(len(result.Cases) == 0 || result.Cases[0].Name != test.wantCase) {
				t.Fatalf("result does not name case %q: %s", test.wantCase, data)
			}
			if test.wantStdout != "" && !strings.Contains(result.Stdout, test.wantStdout) {
				t.Fatalf("stdout = %q, want substring %q", result.Stdout, test.wantStdout)
			}
			if test.wantStderr != "" && !strings.Contains(result.Stderr, test.wantStderr) {
				t.Fatalf("stderr = %q, want substring %q", result.Stderr, test.wantStderr)
			}
			if test.wantSignal && result.Signal == nil {
				t.Fatalf("signal is nil: %s", data)
			}
			if result.OutputTruncated != test.wantTruncate {
				t.Fatalf("output_truncated = %t, want %t: %s", result.OutputTruncated, test.wantTruncate, data)
			}
		})
	}
}

func TestSupervisorAlwaysWritesCrashResultOnConfigurationError(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is required to test the embedded supervisor")
	}
	root := t.TempDir()
	supervisorPath := filepath.Join(root, "supervisor.py")
	if err := os.WriteFile(supervisorPath, SupervisorScript, 0o600); err != nil {
		t.Fatalf("write supervisor: %v", err)
	}
	cmd := exec.Command(python, supervisorPath, "--work-dir", ".codegym")
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("supervisor should report its own error in result.json: %v\n%s", err, output)
	}
	data, err := os.ReadFile(filepath.Join(root, ".codegym", "result.json"))
	if err != nil {
		t.Fatalf("read result.json: %v", err)
	}
	result, err := ParseJudgeResult(data)
	if err != nil {
		t.Fatalf("ParseJudgeResult: %v", err)
	}
	if result.Status != JudgeStatusCrashed || result.FailureDetail == nil || !strings.Contains(*result.FailureDetail, "supervisor error") {
		t.Fatalf("result = %#v", result)
	}
}
