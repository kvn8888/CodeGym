package envbuild

import (
	"context"
	"testing"
	"time"
)

type scriptedVerifier struct {
	calls   int
	respond func(call int, result BuildResult) (Verification, error)
}

func (s *scriptedVerifier) Verify(_ context.Context, result BuildResult) (Verification, error) {
	s.calls++
	return s.respond(s.calls, result)
}

type stubAttempt struct {
	calls         int
	received      [][]Diagnostic
	tokensInStep  int64
	tokensOutStep int64
}

func (s *stubAttempt) run(_ context.Context, _ int, diagnostics []Diagnostic) (BuildResult, error) {
	s.calls++
	s.received = append(s.received, append([]Diagnostic(nil), diagnostics...))
	return BuildResult{
		BuilderRunID: "build_stub",
		Telemetry: BuilderTelemetry{
			WallTimeMs: 10,
			TokensIn:   s.tokensInStep,
			TokensOut:  s.tokensOutStep,
		},
	}, nil
}

func repairableRejection() Verification {
	return Verification{
		Verdict:     VerdictRejected,
		FailedStage: "build",
		Repairable:  true,
		Diagnostics: []Diagnostic{{
			Severity: "error",
			Stage:    "build",
			Code:     "DEPENDENCY_MISSING",
			Message:  "test dependency is missing; install it and rerun the build",
		}},
	}
}

func TestRepairLoopSucceedsOnSecondAttempt(t *testing.T) {
	verifier := &scriptedVerifier{}
	verifier.respond = func(call int, _ BuildResult) (Verification, error) {
		if call == 1 {
			return repairableRejection(), nil
		}
		return Verification{Verdict: VerdictVerified}, nil
	}
	stub := &stubAttempt{tokensInStep: 100, tokensOutStep: 50}
	loop := &RepairLoop{Budget: DefaultRepairBudget()}
	result, err := loop.Run(t.Context(), verifier, stub.run)
	if err != nil {
		t.Fatal(err)
	}
	if stub.calls != 2 || verifier.calls != 2 {
		t.Fatalf("attempts = %d, verifications = %d, want 2 and 2", stub.calls, verifier.calls)
	}
	if len(stub.received[1]) != 1 || stub.received[1][0].Code != "DEPENDENCY_MISSING" {
		t.Fatalf("round 2 received %#v, want round 1 diagnostics", stub.received)
	}
	if result.Telemetry.TokensIn != 200 || result.Telemetry.TokensOut != 100 {
		t.Fatalf("telemetry = %#v, want accumulated sums", result.Telemetry)
	}
	if result.Telemetry.RepairIterations != 1 {
		t.Fatalf("repair iterations = %d, want 1", result.Telemetry.RepairIterations)
	}
}

func TestRepairLoopExhaustsBudget(t *testing.T) {
	verifier := &scriptedVerifier{}
	verifier.respond = func(int, BuildResult) (Verification, error) {
		return repairableRejection(), nil
	}
	stub := &stubAttempt{}
	loop := &RepairLoop{Budget: RepairBudget{MaxRepairs: 2, MaxWallTime: time.Hour, MaxTokens: 1_000_000}}
	result, err := loop.Run(t.Context(), verifier, stub.run)
	if err == nil {
		t.Fatal("expected budget exhaustion error")
	}
	if stub.calls != 3 {
		t.Fatalf("attempts = %d, want 1 initial plus 2 repairs", stub.calls)
	}
	found := false
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == "REPAIR_BUDGET_EXHAUSTED" {
			found = true
		}
	}
	if !found {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
	if result.Telemetry.RepairIterations != 2 {
		t.Fatalf("repair iterations = %d, want 2", result.Telemetry.RepairIterations)
	}
}

func TestRepairLoopStopsOnIrrecoverable(t *testing.T) {
	verifier := &scriptedVerifier{}
	verifier.respond = func(int, BuildResult) (Verification, error) {
		return Verification{Verdict: VerdictRejected, FailedStage: "platform", Repairable: false}, nil
	}
	stub := &stubAttempt{}
	loop := &RepairLoop{Budget: DefaultRepairBudget()}
	result, err := loop.Run(t.Context(), verifier, stub.run)
	if err == nil {
		t.Fatal("expected irrecoverable error")
	}
	if stub.calls != 1 {
		t.Fatalf("attempts = %d, want exactly 1", stub.calls)
	}
	found := false
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == "IRRECOVERABLE_FAULT" && diagnostic.Stage == "platform" {
			found = true
		}
	}
	if !found {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
}
