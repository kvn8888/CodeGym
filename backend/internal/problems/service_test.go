package problems

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/harness"
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
	if len(summaries) != 1 || summaries[0].ID != "two-sum" {
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
}

func TestTwoSumSeedUsesRenderedHarness(t *testing.T) {
	rendered, err := harness.Render(twoSumHarnessSpec())
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	definition := twoSumDefinition()
	if len(definition.HiddenTestFiles) != 1 {
		t.Fatalf("hidden tests = %#v", definition.HiddenTestFiles)
	}
	if definition.HiddenTestFiles[0].Path != rendered.Path ||
		definition.HiddenTestFiles[0].Content != rendered.Content {
		t.Fatal("two-sum seed does not use the canonical rendered harness")
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
	if len(otherList) != 1 || otherList[0].ID != "two-sum" {
		t.Fatalf("other scope should see only global seed: %#v", otherList)
	}
}

func problemTestContext(workspaceID, userID string) context.Context {
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{
		UserID: userID, DefaultWorkspaceID: workspaceID, WorkspaceIDs: []string{workspaceID},
	})
	return workspace.WithScope(ctx, workspace.Scope{WorkspaceID: workspaceID})
}
