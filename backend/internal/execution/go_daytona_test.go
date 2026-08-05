package execution_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/environment"
	"github.com/kvn8888/codegym/backend/internal/execution"
	"github.com/kvn8888/codegym/backend/internal/generation"
	"github.com/kvn8888/codegym/backend/internal/problems"
	"github.com/kvn8888/codegym/backend/internal/submission"
)

const passingGoSumSolution = `package main

func sumSlice(values []int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}
`

const failingGoSumSolution = `package main

func sumSlice(values []int) int { return 0 }
`

const brokenGoSumSolution = `package main

func sumSlice(values []int) int {
	return
}
`

const hangingGoSumSolution = `package main

func sumSlice(values []int) int {
	for {
	}
}
`

const alternateGoPairSolution = `package main

func anyPair(nums []int, target int) []int {
	for right := len(nums) - 1; right >= 0; right-- {
		for left := right - 1; left >= 0; left-- {
			if nums[left]+nums[right] == target {
				return []int{left, right}
			}
		}
	}
	return []int{}
}
`

// TestGoDaytonaEndToEnd is the gated acceptance matrix for the registered Go
// snapshot, generated Go harness, compile wrapper, and language-agnostic
// supervisor.
func TestGoDaytonaEndToEnd(t *testing.T) {
	apiKey := os.Getenv("DAYTONA_API_KEY")
	if apiKey == "" {
		t.Skip("set DAYTONA_API_KEY via doppler run -p codegym -c dev")
	}
	runner, err := execution.NewDaytonaRunner(apiKey, os.Getenv("DAYTONA_API_URL"), environment.Dev)
	if err != nil {
		t.Fatalf("NewDaytonaRunner: %v", err)
	}

	sumDefinition := buildGoDefinition(t, goSumGenerated())
	tests := []struct {
		name       string
		solution   string
		timeout    int
		wantStatus execution.JudgeStatus
		check      func(*testing.T, execution.RunOutcome)
	}{
		{
			name: "passing solution", solution: passingGoSumSolution, wantStatus: execution.JudgeStatusPassed,
			check: func(t *testing.T, outcome execution.RunOutcome) {
				if len(outcome.Result.Cases) != 4 {
					t.Fatalf("cases = %#v", outcome.Result.Cases)
				}
			},
		},
		{
			name: "failing solution", solution: failingGoSumSolution, wantStatus: execution.JudgeStatusFailed,
			check: func(t *testing.T, outcome execution.RunOutcome) {
				if len(outcome.Result.Cases) != 4 || outcome.Result.Cases[0].Status != "fail" || outcome.Result.Cases[0].Error == nil {
					t.Fatalf("expected per-case failure detail, got %#v", outcome.Result.Cases)
				}
			},
		},
		{
			name: "compile error", solution: brokenGoSumSolution, wantStatus: execution.JudgeStatusFailed,
			check: func(t *testing.T, outcome execution.RunOutcome) {
				if outcome.Result.CompileError == nil || strings.TrimSpace(*outcome.Result.CompileError) == "" {
					t.Fatalf("compile_error = %v", outcome.Result.CompileError)
				}
				if len(outcome.Result.Cases) != 0 {
					t.Fatalf("compile-error cases = %#v", outcome.Result.Cases)
				}
			},
		},
		{
			name: "infinite loop", solution: hangingGoSumSolution, timeout: 5, wantStatus: execution.JudgeStatusTimeout,
			check: func(t *testing.T, outcome execution.RunOutcome) {
				if outcome.Result.FailureDetail == nil || !strings.Contains(*outcome.Result.FailureDetail, "positive values") {
					t.Fatalf("failure_detail = %v, want in-flight case", outcome.Result.FailureDetail)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			outcome := runGoDefinition(t, runner, sumDefinition, test.solution, test.timeout)
			if outcome.Result.Status != test.wantStatus {
				t.Fatalf("status = %s, want %s; compile_error=%q result=%#v", outcome.Result.Status, test.wantStatus, stringPointerValue(outcome.Result.CompileError), outcome.Result)
			}
			test.check(t, outcome)
		})
	}

	t.Run("different valid answer passes checker and fails exact", func(t *testing.T) {
		checkerGenerated := goAnyPairGenerated(problems.Comparator{Kind: problems.ComparatorChecker})
		checkerDefinition := buildGoDefinition(t, checkerGenerated)
		checkerOutcome := runGoDefinition(t, runner, checkerDefinition, alternateGoPairSolution, 0)
		if checkerOutcome.Result.Status != execution.JudgeStatusPassed {
			t.Fatalf("checker status = %s; compile_error=%q result=%#v", checkerOutcome.Result.Status, stringPointerValue(checkerOutcome.Result.CompileError), checkerOutcome.Result)
		}

		exactGenerated := goAnyPairGenerated(problems.Comparator{Kind: problems.ComparatorExact})
		exactGenerated.Checker = ""
		exactDefinition := buildGoDefinition(t, exactGenerated)
		exactOutcome := runGoDefinition(t, runner, exactDefinition, alternateGoPairSolution, 0)
		if exactOutcome.Result.Status != execution.JudgeStatusFailed {
			t.Fatalf("exact status = %s, want failed; result=%#v", exactOutcome.Result.Status, exactOutcome.Result)
		}
		if len(exactOutcome.Result.Cases) == 0 || exactOutcome.Result.Cases[0].Error == nil ||
			!strings.Contains(*exactOutcome.Result.Cases[0].Error, "got [2,3]") {
			t.Fatalf("exact result does not prove alternate representative: %#v", exactOutcome.Result.Cases)
		}
	})
}

func buildGoDefinition(t *testing.T, generated generation.GeneratedProblem) problems.Definition {
	t.Helper()
	raw, err := json.Marshal(generated)
	if err != nil {
		t.Fatalf("marshal generated Go problem: %v", err)
	}
	validated, err := generation.ValidateGeneratedProblemForLanguage(raw, "go")
	if err != nil {
		t.Fatalf("ValidateGeneratedProblemForLanguage: %v", err)
	}
	definition, err := generation.BuildProblemDefinition(validated)
	if err != nil {
		t.Fatalf("BuildProblemDefinition: %v", err)
	}
	return definition
}

func runGoDefinition(t *testing.T, runner execution.Runner, definition problems.Definition, solution string, timeoutSeconds int) execution.RunOutcome {
	t.Helper()
	solutionPath := definition.Files.Skeleton[0].Path
	files, err := submission.AssembleFiles(
		[]execution.File{{Path: solutionPath, Content: solution}},
		definition.HiddenTestFiles,
	)
	if err != nil {
		t.Fatalf("AssembleFiles: %v", err)
	}
	language, ok := execution.LanguageFor("go")
	if !ok {
		t.Fatal("go language is not registered")
	}
	if language.Snapshot != execution.GoSnapshotName {
		t.Fatalf("Go snapshot = %q, want %q", language.Snapshot, execution.GoSnapshotName)
	}
	if timeoutSeconds == 0 {
		timeoutSeconds = definition.Runtime.TimeoutSeconds
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	outcome, err := runner.Run(ctx, execution.RunSpec{
		Language: language, Files: files, Entrypoint: definition.Entrypoint,
		Strategy: execution.TestStrategy(definition.TestConfig.Strategy),
		Limits: execution.Limits{
			TimeoutSeconds: timeoutSeconds,
			MemoryMB:       definition.Runtime.MemoryMB,
			NetworkMode:    definition.Runtime.NetworkMode,
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return outcome
}

func goSumGenerated() generation.GeneratedProblem {
	return generation.GeneratedProblem{
		Language: "go", Title: "Sum Slice", Description: "Return the sum of all integers.",
		Category: "algorithms", Subcategory: "arrays", Tags: []string{"arrays"},
		Difficulty: 1, EstimatedMinutes: 10, FunctionName: "sumSlice",
		Parameters: []generation.ProblemParameter{{Name: "values", Type: "[]int"}}, ReturnType: "int",
		Hints: []string{"Accumulate each value."}, Comparator: problems.Comparator{Kind: problems.ComparatorExact},
		ReferenceSolution: passingGoSumSolution,
		TestCases: []generation.ProblemTestCase{
			{CaseMetadata: problems.CaseMetadata{Kind: problems.CaseKindExample}, Name: "positive values", Args: goRaws([]any{[]int{1, 2, 3}}), Expected: goRaw(6)},
			{CaseMetadata: problems.CaseMetadata{Kind: problems.CaseKindFunctional}, Name: "negative values", Args: goRaws([]any{[]int{-4, 1}}), Expected: goRaw(-3)},
			{CaseMetadata: problems.CaseMetadata{Kind: problems.CaseKindEdge, Hidden: true}, Name: "empty slice", Args: goRaws([]any{[]int{}}), Expected: goRaw(0)},
			{CaseMetadata: problems.CaseMetadata{Kind: problems.CaseKindHidden, Hidden: true}, Name: "mixed signs", Args: goRaws([]any{[]int{-2, 5, -1}}), Expected: goRaw(2)},
		},
	}
}

func goAnyPairGenerated(comparator problems.Comparator) generation.GeneratedProblem {
	return generation.GeneratedProblem{
		Language: "go", Title: "Any Pair", Description: "Return any two distinct indices whose values sum to target.",
		Category: "algorithms", Subcategory: "arrays", Tags: []string{"arrays"},
		Difficulty: 2, EstimatedMinutes: 20, FunctionName: "anyPair",
		Parameters: []generation.ProblemParameter{{Name: "nums", Type: "[]int"}, {Name: "target", Type: "int"}},
		ReturnType: "[]int", Hints: []string{"Track complements."}, Comparator: comparator,
		ReferenceSolution: `package main

func anyPair(nums []int, target int) []int {
	for left := 0; left < len(nums); left++ {
		for right := left + 1; right < len(nums); right++ {
			if nums[left]+nums[right] == target { return []int{left, right} }
		}
	}
	return []int{}
}
`,
		Checker: `package main

func check(args []any, actual, expected any) (bool, string) {
	nums, numsOK := args[0].([]int)
	target, targetOK := args[1].(int)
	pair, pairOK := actual.([]int)
	if !numsOK || !targetOK || !pairOK || len(pair) != 2 {
		return false, "answer must contain two indices"
	}
	left, right := pair[0], pair[1]
	if left == right || left < 0 || right < 0 || left >= len(nums) || right >= len(nums) {
		return false, "indices must be distinct and in range"
	}
	return nums[left]+nums[right] == target, "selected values do not sum to target"
}
`,
		TestCases: []generation.ProblemTestCase{
			{CaseMetadata: problems.CaseMetadata{Kind: problems.CaseKindExample}, Name: "first", Args: goRaws([]any{[]int{1, 4, 2, 3}, 5}), Expected: goRaw([]int{0, 1})},
			{CaseMetadata: problems.CaseMetadata{Kind: problems.CaseKindFunctional}, Name: "second", Args: goRaws([]any{[]int{2, 6, 3, 5}, 8}), Expected: goRaw([]int{0, 1})},
			{CaseMetadata: problems.CaseMetadata{Kind: problems.CaseKindEdge, Hidden: true}, Name: "negative", Args: goRaws([]any{[]int{-1, 5, 1, 3}, 4}), Expected: goRaw([]int{0, 1})},
			{CaseMetadata: problems.CaseMetadata{Kind: problems.CaseKindHidden, Hidden: true}, Name: "zero", Args: goRaws([]any{[]int{0, 10, 4, 6}, 10}), Expected: goRaw([]int{0, 1})},
		},
	}
}

func goRaw(value any) json.RawMessage {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return raw
}

func goRaws(values []any) []json.RawMessage {
	result := make([]json.RawMessage, 0, len(values))
	for _, value := range values {
		result = append(result, goRaw(value))
	}
	return result
}

func stringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
