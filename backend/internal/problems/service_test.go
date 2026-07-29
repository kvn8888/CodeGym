package problems

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
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
