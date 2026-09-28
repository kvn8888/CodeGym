package problemverify

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/kvn8888/codegym/backend/internal/execution"
	"github.com/kvn8888/codegym/backend/internal/generation"
	"github.com/kvn8888/codegym/backend/internal/problems"
	"github.com/kvn8888/codegym/backend/internal/submission"
)

const (
	maxReconcileRounds = 2
)

// ErrRejected means the problem never converged on an all-pass harness after
// the bounded repair budget (or the adjudicator asked to regenerate).
var ErrRejected = errors.New("problem verification rejected the generated problem")

// ErrUnavailable means no sandbox runner is configured for verify.
var ErrUnavailable = errors.New("problem verification runner is not configured")

// Verify runs the reference solution against generated hidden tests in an
// internal sandbox (no user submission / workspace auth). The blind pass ships
// immediately on agreement. Only disagreement exposes tests to the reference
// role, for at most two attributed reconciliation rounds.
func Verify(
	ctx context.Context,
	orchestrator *generation.Orchestrator,
	runner execution.Runner,
	definition problems.Definition,
	generated generation.GeneratedProblem,
) (problems.Definition, generation.GeneratedProblem, error) {
	currentDef := definition
	currentGen := generated
	result, runErr := executeReconcileCheck(ctx, runner, currentDef)
	if runErr != nil {
		return problems.Definition{}, generation.GeneratedProblem{}, runErr
	}
	if agreementSucceeded(result) {
		return currentDef, currentGen, nil
	}
	if orchestrator == nil {
		return problems.Definition{}, generation.GeneratedProblem{}, disagreementError(result, "no reconciler is configured")
	}
	contract := currentGen.Specification
	if contract == nil {
		derived := generation.ProblemSpecFromGenerated(currentGen)
		contract = &derived
	}

	for round := 1; round <= maxReconcileRounds; round++ {
		reconciled, record, err := generation.ReconcileProblemArtifacts(ctx, orchestrator, *contract, currentGen, result, round)
		if err != nil {
			return problems.Definition{}, generation.GeneratedProblem{}, fmt.Errorf("%w: reconcile round %d: %v", ErrRejected, round, err)
		}
		log.Printf("problem reconciliation round=%d edited_artifact=%s reason=%q", record.Round, record.EditedArtifact, record.Reason)
		nextDef, err := generation.BuildProblemDefinition(reconciled)
		if err != nil {
			return problems.Definition{}, generation.GeneratedProblem{}, fmt.Errorf("%w: rebuild reconcile round %d: %v", ErrRejected, round, err)
		}
		currentGen = reconciled
		currentDef = nextDef
		result, runErr = executeReconcileCheck(ctx, runner, currentDef)
		if runErr != nil {
			return problems.Definition{}, generation.GeneratedProblem{}, runErr
		}
		if agreementSucceeded(result) {
			return currentDef, currentGen, nil
		}
	}
	return problems.Definition{}, generation.GeneratedProblem{}, disagreementError(result, fmt.Sprintf("did not converge after %d reconcile rounds", maxReconcileRounds))
}

// VerifyAgreement executes one blind reference artifact against one independently
// generated test artifact. Success is publishable evidence; disagreement is a
// rejection for callers that do not opt into the bounded reconcile path.
func VerifyAgreement(ctx context.Context, runner execution.Runner, definition problems.Definition) error {
	result, err := executeAgreementCheck(ctx, runner, definition)
	if err != nil {
		return err
	}
	if result.Total == 0 {
		return fmt.Errorf("%w: verification produced no test results", ErrRejected)
	}
	if result.Failed > 0 {
		return fmt.Errorf("%w: independent reference disagreed with %d of %d cases", ErrRejected, result.Failed, result.Total)
	}
	return nil
}

func executeAgreementCheck(ctx context.Context, runner execution.Runner, definition problems.Definition) (submission.TestResult, error) {
	if runner == nil {
		return submission.TestResult{}, ErrUnavailable
	}
	result, err := runReference(ctx, runner, definition)
	if err != nil {
		return submission.TestResult{}, err
	}
	if result.CompileError != nil && strings.TrimSpace(*result.CompileError) != "" {
		return submission.TestResult{}, fmt.Errorf("%w: reference failed to load: %s", ErrRejected, strings.TrimSpace(*result.CompileError))
	}
	return result, nil
}

func executeReconcileCheck(ctx context.Context, runner execution.Runner, definition problems.Definition) (submission.TestResult, error) {
	if runner == nil {
		return submission.TestResult{}, ErrUnavailable
	}
	return runReference(ctx, runner, definition)
}

func agreementSucceeded(result submission.TestResult) bool {
	return (result.CompileError == nil || strings.TrimSpace(*result.CompileError) == "") && result.Total > 0 && result.Failed == 0
}

func disagreementError(result submission.TestResult, reason string) error {
	if result.CompileError != nil && strings.TrimSpace(*result.CompileError) != "" {
		return fmt.Errorf("%w: %s; reference failed to load: %s", ErrRejected, reason, strings.TrimSpace(*result.CompileError))
	}
	if result.Total == 0 {
		return fmt.Errorf("%w: %s; verification produced no test results", ErrRejected, reason)
	}
	return fmt.Errorf("%w: %s; independent reference disagreed with %d of %d cases", ErrRejected, reason, result.Failed, result.Total)
}

func runReference(ctx context.Context, runner execution.Runner, definition problems.Definition) (submission.TestResult, error) {
	language, ok := execution.LanguageFor(definition.Language)
	if !ok {
		return submission.TestResult{}, fmt.Errorf("%w: unsupported language %q", ErrRejected, definition.Language)
	}
	solutionPath := ""
	for _, file := range definition.Files.Skeleton {
		if file.Entry {
			solutionPath = file.Path
			break
		}
	}
	if solutionPath == "" && len(definition.Files.Skeleton) > 0 {
		solutionPath = definition.Files.Skeleton[0].Path
	}
	if solutionPath == "" {
		return submission.TestResult{}, fmt.Errorf("%w: problem has no solution entry file", ErrRejected)
	}
	files, err := submission.AssembleFiles(
		[]execution.File{{Path: solutionPath, Content: definition.ReferenceSolution}},
		definition.HiddenTestFiles,
	)
	if err != nil {
		return submission.TestResult{}, err
	}
	outcome, err := runner.Run(ctx, execution.RunSpec{
		Language:   language,
		Files:      files,
		Entrypoint: definition.Entrypoint,
		Strategy:   execution.TestStrategy(definition.TestConfig.Strategy),
		Limits: execution.Limits{
			TimeoutSeconds: definition.Runtime.TimeoutSeconds,
			MemoryMB:       definition.Runtime.MemoryMB,
			NetworkMode:    definition.Runtime.NetworkMode,
		},
	})
	if err != nil {
		return submission.TestResult{}, fmt.Errorf("verification sandbox run failed: %w", err)
	}
	var parsed submission.TestResult
	if outcome.Result.Schema == execution.JudgeSchema {
		parsed, err = submission.TestResultFromJudge(outcome.Result)
	} else {
		parsed, err = submission.ParseTestResult(outcome.Output)
	}
	if err != nil {
		return submission.TestResult{}, fmt.Errorf("%w: %v", ErrRejected, err)
	}
	return parsed, nil
}
