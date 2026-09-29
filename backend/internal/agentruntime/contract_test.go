package agentruntime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRuntimeContract(t *testing.T) {
	t.Run("semantic failure despite exit zero", func(t *testing.T) {
		exitCode := 0
		result := RunResult{
			Telemetry: Telemetry{Runtime: "test", ExitCode: &exitCode, Termination: TerminationCompleted},
			Manifest: ManifestClaim{Status: ManifestPresent, Manifest: &ResultManifest{
				Version: ManifestVersion, Completed: true, Summary: "everything passed",
			}},
			Prose: "Success",
		}
		verification, err := Evaluate(t.Context(), VerifierFunc(func(context.Context, TaskSpec, RunResult) (Verification, error) {
			return Verification{Passed: false, Detail: "HTTP endpoint returned the wrong payload"}, nil
		}), validTask(t), result)
		if err != nil {
			t.Fatal(err)
		}
		if verification.Passed {
			t.Fatal("exit zero, successful prose, and a completed manifest overrode the verifier")
		}
	})

	t.Run("cancellation mid run", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		runtime := blockingRuntime{started: make(chan struct{})}
		finished := make(chan struct{})
		var result RunResult
		var runErr error
		go func() {
			defer close(finished)
			result, runErr = runtime.Run(ctx, validTask(t))
		}()
		<-runtime.started
		cancel()
		<-finished
		if !errors.Is(runErr, context.Canceled) {
			t.Fatalf("Run error = %v, want context.Canceled", runErr)
		}
		if result.Telemetry.Termination != TerminationCancelled {
			t.Fatalf("termination = %q", result.Telemetry.Termination)
		}
	})

	t.Run("malformed and absent manifest", func(t *testing.T) {
		workingDirectory := t.TempDir()
		if claim := LoadManifest(workingDirectory); claim.Status != ManifestAbsent {
			t.Fatalf("absent manifest status = %q", claim.Status)
		}
		manifestPath := filepath.Join(workingDirectory, ManifestRelativePath)
		if err := os.MkdirAll(filepath.Dir(manifestPath), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(manifestPath, []byte(`{"version":1,"completed":true`), 0o600); err != nil {
			t.Fatal(err)
		}
		if claim := LoadManifest(workingDirectory); claim.Status != ManifestMalformed {
			t.Fatalf("malformed manifest status = %q", claim.Status)
		}
	})

	t.Run("budget exhaustion", func(t *testing.T) {
		result := RunResult{
			Telemetry: Telemetry{Runtime: "test", Termination: TerminationTokenBudget},
			Manifest:  ManifestClaim{Status: ManifestAbsent},
		}
		verification, err := Evaluate(t.Context(), VerifierFunc(func(_ context.Context, _ TaskSpec, got RunResult) (Verification, error) {
			if got.Telemetry.Termination != TerminationTokenBudget {
				t.Fatalf("termination = %q", got.Telemetry.Termination)
			}
			return Verification{Passed: false, Detail: "relay token budget exhausted"}, nil
		}), validTask(t), result)
		if err != nil {
			t.Fatal(err)
		}
		if verification.Passed {
			t.Fatal("budget-exhausted run passed verification")
		}
	})
}

type blockingRuntime struct {
	started chan struct{}
}

func (blockingRuntime) Name() string { return "blocking" }

func (r blockingRuntime) Run(ctx context.Context, _ TaskSpec) (RunResult, error) {
	close(r.started)
	<-ctx.Done()
	reason := TerminationCancelled
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		reason = TerminationDeadline
	}
	return RunResult{Telemetry: Telemetry{Runtime: r.Name(), Termination: reason}}, ctx.Err()
}

func validTask(t *testing.T) TaskSpec {
	t.Helper()
	return TaskSpec{
		Goal: "build the fixture", WorkingDirectory: t.TempDir(),
		AllowedTools: []Tool{ToolReadFile}, TurnCeiling: 2,
		Deadline: time.Now().Add(time.Minute),
	}
}
