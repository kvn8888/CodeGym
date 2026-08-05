package submission_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/environment"
	"github.com/kvn8888/codegym/backend/internal/execution"
	"github.com/kvn8888/codegym/backend/internal/generation"
	"github.com/kvn8888/codegym/backend/internal/problems"
	"github.com/kvn8888/codegym/backend/internal/submission"
	"github.com/kvn8888/codegym/backend/internal/workspace"
)

// TestRunModeDaytonaPublicCases is the focused public/full-suite acceptance
// gate. It skips without credentials so the ordinary unit suite stays local.
func TestRunModeDaytonaPublicCases(t *testing.T) {
	apiKey := os.Getenv("DAYTONA_API_KEY")
	if apiKey == "" {
		t.Skip("set DAYTONA_API_KEY via doppler run -p codegym -c dev")
	}
	runner, err := execution.NewDaytonaRunner(apiKey, os.Getenv("DAYTONA_API_URL"), environment.Dev)
	if err != nil {
		t.Fatalf("NewDaytonaRunner: %v", err)
	}
	problemStore := problems.NewInMemoryStore()
	problemService := problems.NewService(problemStore)
	if err := problemService.EnsureSeed(context.Background()); err != nil {
		t.Fatalf("EnsureSeed: %v", err)
	}
	executionService := execution.NewService(execution.NewInMemoryStore(), runner, nil)
	submissionService := submission.NewService(problemService, executionService, nil, nil, nil)
	ctx := runModeTestContext()

	twoSum, err := problemService.Get(ctx, "two-sum")
	if err != nil {
		t.Fatal(err)
	}
	pythonRun := submitAndGetDaytona(t, ctx, submissionService, submission.SubmitInput{
		ProblemID: "two-sum", Mode: submission.ModeRun,
		Files: []execution.File{{Path: "solution.py", Content: daytonaCorrectTwoSum}},
	})
	if pythonRun.Mode != submission.ModeRun || pythonRun.Result == nil ||
		pythonRun.ExecutedCount != len(twoSum.PublicCases) || pythonRun.Result.Total != len(twoSum.PublicCases) ||
		pythonRun.ExecutedCount != 2 {
		t.Fatalf("Python run result = %#v; public cases=%d", pythonRun, len(twoSum.PublicCases))
	}

	// Mode is intentionally absent: this proves the compatibility default still
	// selects the full submit suite at the real sandbox boundary.
	pythonSubmit := submitAndGetDaytona(t, ctx, submissionService, submission.SubmitInput{
		ProblemID: "two-sum",
		Files:     []execution.File{{Path: "solution.py", Content: daytonaCorrectTwoSum}},
	})
	if pythonSubmit.Mode != submission.ModeSubmit || pythonSubmit.Result == nil ||
		pythonSubmit.ExecutedCount != 4 || pythonSubmit.Result.Total != 4 ||
		pythonSubmit.ExecutedCount <= pythonRun.ExecutedCount {
		t.Fatalf("Python submit result = %#v; run count=%d", pythonSubmit, pythonRun.ExecutedCount)
	}

	httpProblem, err := problemService.Get(ctx, "go-http-items")
	if err != nil {
		t.Fatal(err)
	}
	httpDefinition, err := problemService.GetDefinition(ctx, "go-http-items")
	if err != nil {
		t.Fatal(err)
	}
	httpRun := submitAndGetDaytona(t, ctx, submissionService, submission.SubmitInput{
		ProblemID: "go-http-items", Mode: submission.ModeRun,
		Files: []execution.File{{Path: "main.go", Content: httpDefinition.ReferenceSolution}},
	})
	if httpRun.Mode != submission.ModeRun || httpRun.Result == nil ||
		httpRun.ExecutedCount != len(httpProblem.PublicCases) || httpRun.Result.Total != len(httpProblem.PublicCases) ||
		httpRun.ExecutedCount != 2 {
		t.Fatalf("Go HTTP run result = %#v; public cases=%d", httpRun, len(httpProblem.PublicCases))
	}

	stressDefinition := hiddenStressDefinition(t)
	stressDefinition.ID = "hidden-disclosure-stress"
	stressDefinition.Visibility = problems.VisibilityGlobal
	if err := problemStore.Upsert(ctx, stressDefinition); err != nil {
		t.Fatalf("store stress problem: %v", err)
	}
	stressSubmit := submitAndGetDaytona(t, ctx, submissionService, submission.SubmitInput{
		ProblemID: stressDefinition.ID, Mode: submission.ModeSubmit,
		Files: []execution.File{{Path: "solution.py", Content: `def find_pair(nums, target):
    return nums
`}},
	})
	if stressSubmit.Result == nil || stressSubmit.Result.Status != "fail" {
		t.Fatalf("stress submit result = %#v", stressSubmit)
	}
	hiddenError := ""
	for _, testCase := range stressSubmit.Result.TestCases {
		if testCase.Name == "hidden-stress" && testCase.Error != nil {
			hiddenError = *testCase.Error
			break
		}
	}
	if !strings.Contains(hiddenError, "expected [1498,1499], got [0,1") ||
		!strings.HasSuffix(hiddenError, "... [truncated]") || len(hiddenError) > submission.MaxRevealedFailureBytes {
		t.Fatalf("hidden disclosure was not revealed and capped: bytes=%d detail=%q", len(hiddenError), hiddenError)
	}
}

func submitAndGetDaytona(t *testing.T, parent context.Context, service *submission.Service, input submission.SubmitInput) submission.View {
	t.Helper()
	ctx, cancel := context.WithTimeout(parent, 3*time.Minute)
	defer cancel()
	accepted, err := service.Submit(ctx, input)
	if err != nil {
		t.Fatalf("Submit(%s, %s): %v", input.ProblemID, input.Mode, err)
	}
	view, err := service.Get(ctx, accepted.SubmissionID)
	if err != nil {
		t.Fatalf("Get(%s): %v", accepted.SubmissionID, err)
	}
	return view
}

func hiddenStressDefinition(t *testing.T) problems.Definition {
	t.Helper()
	largeInput := make([]int, 1500)
	for index := range largeInput {
		largeInput[index] = index
	}
	generated := generation.GeneratedProblem{
		Language: "python", Title: "Hidden disclosure stress", Description: "Return two indices whose values sum to target.",
		Category: "algorithms", Subcategory: "arrays", Tags: []string{"arrays"},
		Difficulty: 2, EstimatedMinutes: 20, FunctionName: "find_pair",
		Parameters: []generation.ProblemParameter{{Name: "nums", Type: "list[int]"}, {Name: "target", Type: "int"}},
		ReturnType: "list[int]", Hints: []string{"Track complements."},
		ReferenceSolution: `def find_pair(nums, target):
    seen = {}
    for index, value in enumerate(nums):
        if target - value in seen:
            return [seen[target - value], index]
        seen[value] = index
    return []`,
		TestCases: []generation.ProblemTestCase{
			{CaseMetadata: problems.CaseMetadata{Kind: problems.CaseKindExample}, Name: "public-example", Args: rawValues([]int{2, 7, 11, 15}, 9), Expected: rawJSON([]int{0, 1})},
			{CaseMetadata: problems.CaseMetadata{Kind: problems.CaseKindFunctional}, Name: "public-functional", Args: rawValues([]int{3, 2, 4}, 6), Expected: rawJSON([]int{1, 2})},
			{CaseMetadata: problems.CaseMetadata{Kind: problems.CaseKindStress, Hidden: true}, Name: "hidden-stress", Args: rawValues(largeInput, 2997), Expected: rawJSON([]int{1498, 1499})},
			{CaseMetadata: problems.CaseMetadata{Kind: problems.CaseKindHidden, Hidden: true}, Name: "hidden-duplicates", Args: rawValues([]int{3, 3}, 6), Expected: rawJSON([]int{0, 1})},
		},
	}
	raw, err := json.Marshal(generated)
	if err != nil {
		t.Fatal(err)
	}
	validated, err := generation.ValidateGeneratedProblem(raw)
	if err != nil {
		t.Fatalf("validate stress problem: %v", err)
	}
	definition, err := generation.BuildProblemDefinition(validated)
	if err != nil {
		t.Fatalf("build stress problem: %v", err)
	}
	return definition
}

func rawValues(nums []int, target int) []json.RawMessage {
	return []json.RawMessage{rawJSON(nums), rawJSON(target)}
}

func rawJSON(value any) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return encoded
}

func runModeTestContext() context.Context {
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{
		UserID: "daytona-run-mode", DefaultWorkspaceID: "workspace-daytona-run-mode",
		WorkspaceIDs: []string{"workspace-daytona-run-mode"},
	})
	return workspace.WithScope(ctx, workspace.Scope{WorkspaceID: "workspace-daytona-run-mode"})
}

const daytonaCorrectTwoSum = `def two_sum(nums, target):
    seen = {}
    for index, value in enumerate(nums):
        complement = target - value
        if complement in seen:
            return [seen[complement], index]
        seen[value] = index
    return []
`
