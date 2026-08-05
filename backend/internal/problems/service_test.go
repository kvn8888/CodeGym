package problems

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/workspace"
)

func TestEnsureSeedIsIdempotentAndKeepsHiddenArtifactsPrivate(t *testing.T) {
	ctx := context.Background()
	service := NewService(NewInMemoryStore())
	if err := service.EnsureSeed(ctx); err != nil {
		t.Fatalf("first EnsureSeed: %v", err)
	}
	if err := service.EnsureSeed(ctx); err != nil {
		t.Fatalf("second EnsureSeed: %v", err)
	}

	summaries, err := service.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(summaries) != 2 || !containsProblemID(summaries, "two-sum") || !containsProblemID(summaries, goHTTPItemsID) {
		t.Fatalf("summaries = %#v", summaries)
	}

	problem, err := service.Get(ctx, "two-sum")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	skeleton, err := service.GetSkeleton(ctx, "two-sum")
	if err != nil {
		t.Fatalf("GetSkeleton: %v", err)
	}
	definition, err := service.GetDefinition(ctx, "two-sum")
	if err != nil {
		t.Fatalf("GetDefinition: %v", err)
	}
	if definition.ReferenceSolution == "" || len(definition.HiddenTestFiles) == 0 {
		t.Fatal("seed is missing server-only execution artifacts")
	}
	if counts := CountUnitCaseVisibility(twoSumCases()); counts != (CaseVisibilityCounts{Public: 2, Hidden: 2}) ||
		len(definition.PublicCases) != counts.Public || len(definition.PublicTestFiles) != 2 {
		t.Fatalf("two-sum public/hidden mix = %#v definition=%#v", counts, definition)
	}
	publicSeedHarness := definition.PublicTestFiles[0].Content
	if strings.Contains(publicSeedHarness, "handles duplicate values") || strings.Contains(publicSeedHarness, "handles negative values") ||
		!strings.Contains(publicSeedHarness, "finds a pair without relying on order") {
		t.Fatalf("two-sum public harness has wrong cases: %s", publicSeedHarness)
	}
	seedHarness := definition.HiddenTestFiles[0].Content
	for _, protocol := range []string{"cases.jsonl", "verdict.json", "signal.setitimer"} {
		if !strings.Contains(seedHarness, protocol) {
			t.Fatalf("seed harness is missing %q", protocol)
		}
	}
	if strings.Contains(seedHarness, "CODEGYM_RESULT") {
		t.Fatal("seed harness still uses the in-band result protocol")
	}

	publicJSON, err := json.Marshal(problem)
	if err != nil {
		t.Fatalf("marshal public problem: %v", err)
	}
	skeletonJSON, err := json.Marshal(skeleton)
	if err != nil {
		t.Fatalf("marshal skeleton: %v", err)
	}
	for _, body := range []string{string(publicJSON), string(skeletonJSON)} {
		if strings.Contains(body, "CODEGYM_RESULT") ||
			strings.Contains(body, "seen[complement]") ||
			strings.Contains(body, "test_solution.py") {
			t.Fatalf("public response leaked a hidden artifact: %s", body)
		}
	}
	if len(skeleton.Files) != 1 || skeleton.Files[0].Content != twoSumSkeleton {
		t.Fatalf("skeleton = %#v", skeleton)
	}
	if definition.TestConfig.Strategy != TestStrategyUnit || definition.TestConfig.Comparator.Kind != ComparatorSorted || definition.Entrypoint != "test_solution.py" {
		t.Fatalf("two-sum seed changed: %#v", definition)
	}

	httpProblem, err := service.Get(ctx, goHTTPItemsID)
	if err != nil {
		t.Fatalf("Get HTTP seed: %v", err)
	}
	httpSkeleton, err := service.GetSkeleton(ctx, goHTTPItemsID)
	if err != nil {
		t.Fatalf("GetSkeleton HTTP seed: %v", err)
	}
	httpDefinition, err := service.GetDefinition(ctx, goHTTPItemsID)
	if err != nil {
		t.Fatalf("GetDefinition HTTP seed: %v", err)
	}
	if httpDefinition.TestConfig.Strategy != TestStrategyHTTP || httpDefinition.Language != "go" ||
		httpDefinition.Entrypoint != "codegym_http_compile.py" || httpDefinition.Runtime.TimeoutSeconds != 120 ||
		len(httpDefinition.HiddenTestFiles) != 4 || len(httpDefinition.PublicTestFiles) != 4 || len(httpDefinition.PublicCases) != 2 {
		t.Fatalf("HTTP seed wiring = %#v", httpDefinition)
	}
	if len(httpSkeleton.Files) != 1 || httpSkeleton.Files[0].Path != "main.go" || !strings.Contains(httpSkeleton.Files[0].Content, `os.Getenv("PORT")`) {
		t.Fatalf("HTTP seed skeleton = %#v", httpSkeleton)
	}
	httpPublicJSON, err := json.Marshal(httpProblem)
	if err != nil {
		t.Fatalf("marshal public HTTP seed: %v", err)
	}
	if !strings.Contains(string(httpPublicJSON), "creates-an-item") || strings.Contains(string(httpPublicJSON), "increments-item-identifiers") || strings.Contains(string(httpPublicJSON), "codegym_http_cases") {
		t.Fatalf("public HTTP seed projection is incorrect: %s", httpPublicJSON)
	}
	publicHTTPHarness := httpDefinition.PublicTestFiles[2].Content
	fullHTTPHarness := httpDefinition.HiddenTestFiles[2].Content
	if strings.Contains(publicHTTPHarness, "returns-not-found-for-an-unknown-item") ||
		!strings.Contains(fullHTTPHarness, "returns-not-found-for-an-unknown-item") {
		t.Fatalf("HTTP seed public/hidden harness split is incorrect\npublic=%s\nfull=%s", publicHTTPHarness, fullHTTPHarness)
	}
}

func TestGeneratedProblemsAreVisibleOnlyToTheirOwnerScope(t *testing.T) {
	service := NewService(NewInMemoryStore())
	if err := service.EnsureSeed(context.Background()); err != nil {
		t.Fatal(err)
	}
	owner := problemTestContext("workspace-1", "user-1")
	generated, err := service.PersistGenerated(owner, Definition{
		Problem: Problem{Summary: Summary{Title: "Private Graph Problem", Language: "python"}},
	})
	if err != nil {
		t.Fatalf("PersistGenerated: %v", err)
	}
	if generated.ID == "" {
		t.Fatal("generated problem did not receive an id")
	}
	if _, err := service.Get(owner, generated.ID); err != nil {
		t.Fatalf("owner Get: %v", err)
	}
	if _, err := service.Get(problemTestContext("workspace-2", "user-1"), generated.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-workspace Get err=%v, want ErrNotFound", err)
	}
	if _, err := service.Get(problemTestContext("workspace-1", "user-2"), generated.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-user Get err=%v, want ErrNotFound", err)
	}
	otherList, err := service.List(problemTestContext("workspace-2", "user-2"))
	if err != nil {
		t.Fatal(err)
	}
	if len(otherList) != 2 || !containsProblemID(otherList, "two-sum") || !containsProblemID(otherList, goHTTPItemsID) {
		t.Fatalf("other scope should see both global seeds: %#v", otherList)
	}
}

func containsProblemID(summaries []Summary, id string) bool {
	for _, summary := range summaries {
		if summary.ID == id {
			return true
		}
	}
	return false
}

func problemTestContext(workspaceID, userID string) context.Context {
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{
		UserID: userID, DefaultWorkspaceID: workspaceID, WorkspaceIDs: []string{workspaceID},
	})
	return workspace.WithScope(ctx, workspace.Scope{WorkspaceID: workspaceID})
}
