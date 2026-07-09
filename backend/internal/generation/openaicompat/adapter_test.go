package openaicompat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kvn8888/codegym/backend/internal/generation"
)

func chatCompletionBody(t *testing.T, content string) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"model": "anthropic/claude-haiku-4.5",
		"choices": []map[string]any{
			{"message": map[string]any{"role": "assistant", "content": content}},
		},
		"usage": map[string]any{"prompt_tokens": 120, "completion_tokens": 250},
	})
	if err != nil {
		t.Fatalf("encode fake response: %v", err)
	}
	return string(body)
}

func testRequest() generation.GenerateRequest {
	return generation.GenerateRequest{
		Kind:         generation.KindMCQ,
		Spec:         json.RawMessage(`{"topic":"hash tables","count":2}`),
		Instructions: "You are the MCQ generator.",
		Schema: generation.Schema{
			Name:       "mcq_set",
			Version:    "1",
			JSONSchema: json.RawMessage(`{"type":"array"}`),
		},
		MemoryContext: generation.MemoryContext{
			Summary:     "22 events.",
			GrowthEdges: []string{"Caching"},
		},
	}
}

func TestGenerateReturnsParsedObject(t *testing.T) {
	var captured struct {
		path     string
		auth     string
		body     map[string]any
		received bool
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.received = true
		captured.path = r.URL.Path
		captured.auth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&captured.body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(chatCompletionBody(t, `[{"id":"mq1"}]`)))
	}))
	defer server.Close()

	adapter, err := New(Config{BaseURL: server.URL, APIKey: "test-key", Model: "test-model"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	result, err := adapter.Generate(context.Background(), testRequest())
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if !captured.received {
		t.Fatal("provider was never called")
	}
	if captured.path != "/chat/completions" {
		t.Errorf("path = %q, want /chat/completions", captured.path)
	}
	if captured.auth != "Bearer test-key" {
		t.Errorf("auth header = %q", captured.auth)
	}
	if captured.body["model"] != "test-model" {
		t.Errorf("model = %v, want test-model", captured.body["model"])
	}

	messages, ok := captured.body["messages"].([]any)
	if !ok || len(messages) != 2 {
		t.Fatalf("messages = %v, want system+user", captured.body["messages"])
	}
	system := messages[0].(map[string]any)["content"].(string)
	if !strings.Contains(system, "You are the MCQ generator.") {
		t.Error("system message is missing orchestration instructions")
	}
	if !strings.Contains(system, "untrusted reference data") {
		t.Error("system message is missing memory injection guard")
	}

	if string(result.Object) != `[{"id":"mq1"}]` {
		t.Errorf("object = %s", result.Object)
	}
	if result.Provider != "openai_compat" {
		t.Errorf("provider = %q", result.Provider)
	}
	if result.TokensIn != 120 || result.TokensOut != 250 {
		t.Errorf("tokens = %d/%d, want 120/250", result.TokensIn, result.TokensOut)
	}
}

func TestGenerateStripsMarkdownFences(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(chatCompletionBody(t, "```json\n[{\"id\":\"mq1\"}]\n```")))
	}))
	defer server.Close()

	adapter, err := New(Config{BaseURL: server.URL, APIKey: "k", Model: "m"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	result, err := adapter.Generate(context.Background(), testRequest())
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if string(result.Object) != `[{"id":"mq1"}]` {
		t.Errorf("object = %s", result.Object)
	}
}

func TestGenerateRejectsNonJSONOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(chatCompletionBody(t, "Sure! Here are your questions: 1) ...")))
	}))
	defer server.Close()

	adapter, err := New(Config{BaseURL: server.URL, APIKey: "k", Model: "m"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := adapter.Generate(context.Background(), testRequest()); err == nil {
		t.Fatal("expected error for non-JSON model output")
	}
}

func TestGenerateSurfacesProviderError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"rate limited","type":"rate_limit"}}`))
	}))
	defer server.Close()

	adapter, err := New(Config{BaseURL: server.URL, APIKey: "k", Model: "m"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = adapter.Generate(context.Background(), testRequest())
	if err == nil || !strings.Contains(err.Error(), "rate limited") {
		t.Fatalf("err = %v, want provider error containing message", err)
	}
}

func TestGeneratePrefersModelPolicyModel(t *testing.T) {
	var gotModel string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotModel, _ = body["model"].(string)
		_, _ = w.Write([]byte(chatCompletionBody(t, `{}`)))
	}))
	defer server.Close()

	adapter, err := New(Config{BaseURL: server.URL, APIKey: "k", Model: "default-model"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	request := testRequest()
	request.ModelPolicy.PreferredModel = "override-model"
	if _, err := adapter.Generate(context.Background(), request); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if gotModel != "override-model" {
		t.Errorf("model = %q, want override-model", gotModel)
	}
}

func TestNewRequiresConfig(t *testing.T) {
	if _, err := New(Config{APIKey: "k", Model: "m"}); err == nil {
		t.Error("expected error for missing BaseURL")
	}
	if _, err := New(Config{BaseURL: "https://x", Model: "m"}); err == nil {
		t.Error("expected error for missing APIKey")
	}
	if _, err := New(Config{BaseURL: "https://x", APIKey: "k"}); err == nil {
		t.Error("expected error for missing Model")
	}
}
