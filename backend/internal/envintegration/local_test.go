package envintegration

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRunLocalStageCommandBoundsOutputAndTerminatesProcessGroup(t *testing.T) {
	started := time.Now()
	exitCode, output, timedOut, err := runLocalStageCommand(context.Background(), "yes overflow", t.TempDir(), 128)
	if !errors.Is(err, ErrLocalOutputExceeded) {
		t.Fatalf("error = %v, want ErrLocalOutputExceeded", err)
	}
	if exitCode != -1 {
		t.Fatalf("exit code = %d, want -1 for an execution failure", exitCode)
	}
	if timedOut {
		t.Fatal("timedOut = true, want output exhaustion to be distinct from a deadline")
	}
	if len(output) > 128 {
		t.Fatalf("captured %d bytes, want at most the 128-byte ceiling", len(output))
	}
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Fatalf("command took %s; the process group was not terminated promptly", elapsed)
	}
}

func TestRunLocalStageCommandPreservesNormalExitBehavior(t *testing.T) {
	exitCode, output, timedOut, err := runLocalStageCommand(context.Background(), "printf ok; exit 7", t.TempDir(), 128)
	if err != nil {
		t.Fatalf("runLocalStageCommand: %v", err)
	}
	if exitCode != 7 || output != "ok" || timedOut {
		t.Fatalf("result = exit %d, output %q, timedOut=%v; want 7/ok/false", exitCode, output, timedOut)
	}
	if strings.Contains(output, "truncated") {
		t.Fatalf("output = %q, want only the command output", output)
	}
}
