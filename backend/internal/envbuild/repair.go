package envbuild

import (
	"context"
	"fmt"
	"time"
)

// Verdict mirrors the shared contract verifier result. Only VerdictVerified
// marks an environment ready; everything else keeps it out of promotion.
type Verdict string

const (
	VerdictVerified Verdict = "verified"
	VerdictRejected Verdict = "rejected"
)

// Verification is the verifier's answer for one preparation round. Track B
// owns the real implementation; this issue exercises the loop with
// predetermined responses in this shape.
type Verification struct {
	Verdict     Verdict
	FailedStage string
	Repairable  bool
	Diagnostics []Diagnostic
}

// Verifier judges one preparation round. It never builds and never repairs;
// it only answers.
type Verifier interface {
	Verify(ctx context.Context, result BuildResult) (Verification, error)
}

// RepairBudget bounds a whole preparation attempt across all rounds. These
// are structural runaway guards, not spending policy: the repair-iteration
// cap stops unbounded loops, and the wall-time and token ceilings bound
// pathological runs while spend ceilings stay off per the recorded budget
// decision.
type RepairBudget struct {
	MaxRepairs  int
	MaxWallTime time.Duration
	MaxTokens   int64
}

// DefaultRepairBudget is the standing bound for repair loops.
func DefaultRepairBudget() RepairBudget {
	return RepairBudget{
		MaxRepairs:  3,
		MaxWallTime: 30 * time.Minute,
		MaxTokens:   2_000_000,
	}
}

// AttemptFunc performs one preparation round. Diagnostics carry the previous
// round's verifier answer for the repair to act on; round zero receives none.
type AttemptFunc func(ctx context.Context, round int, diagnostics []Diagnostic) (BuildResult, error)

// RepairLoop runs preparation rounds until the verifier passes, the fault is
// irrecoverable, or the budget is exhausted. An agent success claim never
// short-circuits it: every round ends at a verifier answer.
type RepairLoop struct {
	Budget RepairBudget
}

// Run executes the loop. It returns the final round's result with telemetry
// accumulated across all rounds. A nil error means the verifier passed; any
// terminal failure returns both the result and the error.
func (l *RepairLoop) Run(ctx context.Context, verifier Verifier, attempt AttemptFunc) (BuildResult, error) {
	budget := l.Budget
	if budget.MaxRepairs <= 0 {
		budget.MaxRepairs = DefaultRepairBudget().MaxRepairs
	}
	var (
		result      BuildResult
		diagnostics []Diagnostic
		repairs     int
		wallTimeMs  int64
		tokensIn    int64
		tokensOut   int64
		costUsd     float64
	)
	for round := 0; ; round++ {
		var err error
		result, err = attempt(ctx, round, diagnostics)
		if err != nil {
			return result, err
		}
		wallTimeMs += result.Telemetry.WallTimeMs
		tokensIn += result.Telemetry.TokensIn
		tokensOut += result.Telemetry.TokensOut
		costUsd += result.Telemetry.EstimatedCostUsd
		verification, err := verifier.Verify(ctx, result)
		if err != nil {
			return result, err
		}
		if verification.Verdict == VerdictVerified {
			result.Telemetry.WallTimeMs = wallTimeMs
			result.Telemetry.TokensIn = tokensIn
			result.Telemetry.TokensOut = tokensOut
			result.Telemetry.EstimatedCostUsd = costUsd
			result.Telemetry.RepairIterations = repairs
			return result, nil
		}
		stage := verification.FailedStage
		if stage == "" {
			stage = "prepare"
		}
		if !verification.Repairable {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{
				Severity: "error",
				Stage:    stage,
				Code:     "IRRECOVERABLE_FAULT",
				Message:  fmt.Sprintf("verifier rejected the environment as irrecoverable at stage %s", stage),
				Details:  map[string]any{"failed_stage": stage},
			})
			result.Telemetry.RepairIterations = repairs
			return result, fmt.Errorf("envbuild: irrecoverable fault at stage %s", stage)
		}
		repairsDone := repairs
		if repairsDone >= budget.MaxRepairs || time.Duration(wallTimeMs)*time.Millisecond >= budget.MaxWallTime || tokensIn+tokensOut >= budget.MaxTokens {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{
				Severity: "error",
				Stage:    stage,
				Code:     "REPAIR_BUDGET_EXHAUSTED",
				Message:  fmt.Sprintf("repair budget exhausted after %d repairs at stage %s", repairsDone, stage),
				Details:  map[string]any{"failed_stage": stage, "repairs": repairsDone},
			})
			result.Telemetry.WallTimeMs = wallTimeMs
			result.Telemetry.TokensIn = tokensIn
			result.Telemetry.TokensOut = tokensOut
			result.Telemetry.EstimatedCostUsd = costUsd
			result.Telemetry.RepairIterations = repairsDone
			return result, fmt.Errorf("envbuild: repair budget exhausted after %d repairs", repairsDone)
		}
		diagnostics = verification.Diagnostics
		repairs++
	}
}
