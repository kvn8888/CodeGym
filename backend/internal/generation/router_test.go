package generation

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type routeScript struct {
	name     string
	err      error
	result   GenerateResult
	calls    int
	failOnce bool
	failed   bool
}

func (s *routeScript) Generate(_ context.Context, _ GenerateRequest) (GenerateResult, error) {
	s.calls++
	if s.failOnce && !s.failed {
		s.failed = true
		return GenerateResult{}, s.err
	}
	if s.err != nil && !s.failOnce {
		return GenerateResult{}, s.err
	}
	result := s.result
	if result.Provider == "" {
		result.Provider = s.name
	}
	if result.Object == nil {
		result.Object = json.RawMessage(`{"ok":true}`)
	}
	return result, nil
}

func TestRouterUsesFirstProvider(t *testing.T) {
	first := &routeScript{name: "meta", result: GenerateResult{Model: "muse"}}
	second := &routeScript{name: "azure"}
	router, err := NewRouter([]NamedGenerator{
		{Name: "meta", Generator: first},
		{Name: "azure", Generator: second},
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	result, err := router.Generate(context.Background(), GenerateRequest{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if result.Provider != "meta" {
		t.Fatalf("provider = %q", result.Provider)
	}
	if first.calls != 1 || second.calls != 0 {
		t.Fatalf("calls meta=%d azure=%d", first.calls, second.calls)
	}
}

func TestRouterFallsBackOnProviderError(t *testing.T) {
	first := &routeScript{
		name: "meta",
		err:  &ProviderError{StatusCode: 429, Message: "rate limited"},
	}
	second := &routeScript{name: "azure", result: GenerateResult{Model: "gpt"}}
	router, err := NewRouter([]NamedGenerator{
		{Name: "meta", Generator: first},
		{Name: "azure", Generator: second},
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	result, err := router.Generate(context.Background(), GenerateRequest{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if result.Provider != "azure" {
		t.Fatalf("provider = %q, want azure", result.Provider)
	}
	if first.calls != 1 || second.calls != 1 {
		t.Fatalf("calls meta=%d azure=%d", first.calls, second.calls)
	}
}

func TestRouterDoesNotFallbackOnInvalidOutput(t *testing.T) {
	first := &routeScript{
		name: "meta",
		err:  &InvalidOutputError{Reason: "not json"},
	}
	second := &routeScript{name: "azure"}
	router, err := NewRouter([]NamedGenerator{
		{Name: "meta", Generator: first},
		{Name: "azure", Generator: second},
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	_, err = router.Generate(context.Background(), GenerateRequest{})
	if err == nil {
		t.Fatal("expected invalid_output error")
	}
	var invalid *InvalidOutputError
	if !errors.As(err, &invalid) {
		t.Fatalf("error type = %T, want InvalidOutputError", err)
	}
	if second.calls != 0 {
		t.Fatalf("azure should not be called, calls=%d", second.calls)
	}
}

func TestRouterHonorsPreferredAndAllowed(t *testing.T) {
	meta := &routeScript{name: "meta"}
	azure := &routeScript{name: "azure"}
	gemini := &routeScript{name: "gemini"}
	router, err := NewRouter([]NamedGenerator{
		{Name: "meta", Generator: meta},
		{Name: "azure", Generator: azure},
		{Name: "gemini", Generator: gemini},
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	result, err := router.Generate(context.Background(), GenerateRequest{
		ModelPolicy: ModelPolicy{
			PreferredProvider: "gemini",
			AllowedProviders:  []string{"azure", "gemini"},
		},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if result.Provider != "gemini" {
		t.Fatalf("provider = %q, want gemini", result.Provider)
	}
	if meta.calls != 0 || azure.calls != 0 || gemini.calls != 1 {
		t.Fatalf("calls meta=%d azure=%d gemini=%d", meta.calls, azure.calls, gemini.calls)
	}
}

func TestRouterAllProvidersFail(t *testing.T) {
	router, err := NewRouter([]NamedGenerator{
		{Name: "meta", Generator: &routeScript{name: "meta", err: &ProviderError{Message: "down"}}},
		{Name: "azure", Generator: &routeScript{name: "azure", err: &ProviderError{Message: "down"}}},
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	_, err = router.Generate(context.Background(), GenerateRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "all GenAI providers failed") {
		t.Fatalf("error = %v", err)
	}
}
