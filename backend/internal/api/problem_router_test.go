package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/identity"
	"github.com/kvn8888/codegym/backend/internal/problems"
)

func TestProblemEndpointsNeverExposeHiddenCaseContent(t *testing.T) {
	const (
		problemID      = "visibility-regression"
		hiddenInput    = "HIDDEN_INPUT_SENTINEL_7b3f"
		hiddenExpected = "HIDDEN_EXPECTED_SENTINEL_91ad"
		publicExpected = "PUBLIC_EXPECTED_SENTINEL"
	)
	store := problems.NewInMemoryStore()
	problemService := problems.NewService(store)
	if err := store.Upsert(context.Background(), problems.Definition{
		Problem: problems.Problem{
			Summary:    problems.Summary{ID: problemID, Title: "Visibility regression", Language: "python", Type: "coding"},
			Runtime:    problems.Runtime{Image: "python312", TimeoutSeconds: 30, MemoryMB: 256, NetworkMode: "block-all"},
			Files:      problems.FileManifest{Skeleton: []problems.FileRef{{Path: "solution.py", Entry: true}}},
			TestConfig: problems.TestConfig{Strategy: problems.TestStrategyUnit},
			PublicCases: []problems.PublicCase{{
				Strategy: problems.TestStrategyUnit, Name: "visible-example", Kind: problems.CaseKindExample,
				Expected: publicExpected, Explanation: "Shown before running code.",
			}},
		},
		SkeletonFiles: []problems.File{{Path: "solution.py", Content: "def solve(value):\n    return value\n"}},
		HiddenTestFiles: []problems.File{{
			Path: "test_solution.py", Content: hiddenInput + "\n" + hiddenExpected,
		}},
		ReferenceSolution: hiddenExpected,
		Entrypoint:        "test_solution.py",
		Visibility:        problems.VisibilityGlobal,
	}); err != nil {
		t.Fatal(err)
	}

	router := NewRouter(Dependencies{
		Authenticator: auth.NewDevAuthenticator(auth.DevAuthenticatorConfig{}),
		Identity:      identity.NewService(identity.NewInMemoryStore()),
		Problems:      problemService,
	})
	responses := map[string]string{}
	for _, target := range []string{
		"/api/v1/problems",
		"/api/v1/problems/" + problemID,
		"/api/v1/problems/" + problemID + "/skeleton",
	} {
		response := authedRequest(t, router, http.MethodGet, target, "")
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d: %s", target, response.Code, response.Body.String())
		}
		responses[target] = response.Body.String()
	}

	for target, body := range responses {
		for _, secret := range []string{hiddenInput, hiddenExpected, "test_solution.py"} {
			if strings.Contains(body, secret) {
				t.Fatalf("GET %s leaked hidden case content %q: %s", target, secret, body)
			}
		}
	}
	if detail := responses["/api/v1/problems/"+problemID]; !strings.Contains(detail, publicExpected) || !strings.Contains(detail, "visible-example") {
		t.Fatalf("problem detail did not expose its public worked example: %s", detail)
	}
}
