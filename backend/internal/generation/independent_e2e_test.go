package generation_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/config"
	"github.com/kvn8888/codegym/backend/internal/execution"
	"github.com/kvn8888/codegym/backend/internal/generation"
	"github.com/kvn8888/codegym/backend/internal/generation/openaicompat"
	"github.com/kvn8888/codegym/backend/internal/memory"
	"github.com/kvn8888/codegym/backend/internal/problemverify"
	"github.com/kvn8888/codegym/backend/internal/workspace"
)

type recordedGenerationCall struct {
	Request generation.GenerateRequest
	Result  generation.GenerateResult
}

type recordingGenerator struct {
	next  generation.Generator
	mu    sync.Mutex
	calls []recordedGenerationCall
}

func (r *recordingGenerator) Generate(ctx context.Context, request generation.GenerateRequest) (generation.GenerateResult, error) {
	result, err := r.next.Generate(ctx, request)
	if err == nil {
		r.mu.Lock()
		r.calls = append(r.calls, recordedGenerationCall{Request: request, Result: result})
		r.mu.Unlock()
	}
	return result, err
}

func (r *recordingGenerator) reset() {
	r.mu.Lock()
	r.calls = nil
	r.mu.Unlock()
}

func (r *recordingGenerator) snapshot() []recordedGenerationCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]recordedGenerationCall(nil), r.calls...)
}

// TestIndependentGeneration is the credential-gated acceptance test for the
// real provider plus Daytona verification path. It intentionally generates
// exactly two problems: one Python unit problem and one Go HTTP problem.
func TestIndependentGeneration(t *testing.T) {
	cfg := config.Load()
	if cfg.DaytonaAPIKey == "" || len(cfg.GenAIProviders) == 0 {
		t.Skip("set Daytona and GenAI credentials via doppler run -p codegym -c dev")
	}
	runner, err := execution.NewDaytonaRunner(cfg.DaytonaAPIKey, cfg.DaytonaAPIURL)
	if err != nil {
		t.Fatalf("NewDaytonaRunner: %v", err)
	}

	named := make([]generation.NamedGenerator, 0, len(cfg.GenAIProviders))
	for _, provider := range cfg.GenAIProviders {
		adapter, err := openaicompat.New(openaicompat.Config{
			Name: provider.Name, BaseURL: provider.BaseURL, APIKey: provider.APIKey,
			Model: provider.Model, AuthStyle: provider.AuthStyle,
			APIVersion: provider.APIVersion, DefaultMaxTokens: provider.DefaultMaxTokens,
		})
		if err != nil {
			t.Fatalf("configure GenAI provider %s: %v", provider.Name, err)
		}
		named = append(named, generation.NamedGenerator{Name: provider.Name, Generator: adapter})
	}
	router, err := generation.NewRouter(named)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	recorder := &recordingGenerator{next: router}
	orchestrator := generation.NewOrchestrator(memory.NewService(memory.NewInMemoryStore(), nil), recorder)

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	ctx = auth.WithPrincipal(ctx, auth.Principal{
		UserID: "independent-generation-e2e", DefaultWorkspaceID: "independent-generation-e2e",
		WorkspaceIDs: []string{"independent-generation-e2e"},
	})
	ctx = workspace.WithScope(ctx, workspace.Scope{WorkspaceID: "independent-generation-e2e"})

	tests := []struct {
		name     string
		request  generation.ProblemSpec
		strategy string
	}{
		{
			name: "python unit",
			request: generation.ProblemSpec{
				Topic: "stable deduplication of integer lists", Language: "python", Difficulty: "easy",
				Prompt: "Create a Python unit-strategy problem with one typed function and deterministic edge behavior.",
			},
			strategy: "unit",
		},
		{
			name: "go http",
			request: generation.ProblemSpec{
				Topic: "Go net/http JSON resource API", Language: "go", Difficulty: "medium",
				Prompt: "Create a Go http-strategy problem. Include a JSON response id that is explicitly a string, not a number.",
			},
			strategy: "http",
		},
	}
	totalReconciliations := 0
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder.reset()
			definition, spec, generated, _, err := generation.GenerateIndependentProblem(ctx, orchestrator, test.request)
			if err != nil {
				t.Fatalf("GenerateIndependentProblem: %v", err)
			}
			if len(spec.AmbiguityResolutions) == 0 || spec.Signature == "" || spec.Comparator.Kind == "" {
				t.Fatalf("generated spec omitted alignment fields: %#v", spec)
			}
			if string(spec.Strategy) != test.strategy {
				t.Fatalf("strategy = %q, want %q", spec.Strategy, test.strategy)
			}
			assertBlindArtifactIsolation(t, recorder.snapshot())

			verifiedDefinition, verified, err := problemverify.Verify(ctx, orchestrator, runner, definition, generated)
			if err != nil {
				t.Fatalf("Verify: %v", err)
			}
			if verifiedDefinition.ReferenceSolution == "" || verified.Specification == nil {
				t.Fatalf("verified result lost generation evidence")
			}
			totalReconciliations += len(verified.Reconciliations)
			t.Logf("verified provider-backed %s problem; reconciliation_rounds=%d", test.name, len(verified.Reconciliations))
		})
	}
	t.Logf("independent generation reconciliation total=%d problems=2", totalReconciliations)
}

func assertBlindArtifactIsolation(t *testing.T, calls []recordedGenerationCall) {
	t.Helper()
	var testsCall, referenceCall *recordedGenerationCall
	for index := range calls {
		switch calls[index].Request.Schema.Name {
		case "problem_tests":
			testsCall = &calls[index]
		case "problem_reference":
			referenceCall = &calls[index]
		}
	}
	if testsCall == nil || referenceCall == nil {
		t.Fatalf("missing blind artifact calls: %#v", calls)
	}
	testsPrompt := testsCall.Request.Instructions + "\n" + string(testsCall.Request.Spec)
	referencePrompt := referenceCall.Request.Instructions + "\n" + string(referenceCall.Request.Spec)
	if strings.Contains(testsPrompt, `"reference_solution"`) || strings.Contains(testsPrompt, referenceSource(t, referenceCall.Result.Object)) {
		t.Fatal("tests call received the reference artifact")
	}
	if strings.Contains(referencePrompt, `"test_cases"`) || strings.Contains(referencePrompt, `"http_test_cases"`) {
		t.Fatal("reference call received the tests artifact")
	}
}

func referenceSource(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var artifact struct {
		ReferenceSolution string `json:"reference_solution"`
	}
	if err := json.Unmarshal(raw, &artifact); err != nil {
		t.Fatalf("decode reference artifact: %v", err)
	}
	if strings.TrimSpace(artifact.ReferenceSolution) == "" {
		t.Fatal("reference artifact is empty")
	}
	return artifact.ReferenceSolution
}
