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
	// testRequest schema is type:array, so json_object must be omitted.
	if _, ok := captured.body["response_format"]; ok {
		t.Errorf("response_format = %#v, want omitted for array schema", captured.body["response_format"])
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
		t.Errorf("provider = %q, want default openai_compat", result.Provider)
	}
	if result.TokensIn != 120 || result.TokensOut != 250 {
		t.Errorf("tokens = %d/%d, want 120/250", result.TokensIn, result.TokensOut)
	}
}

func TestGenerateRetriesWithoutResponseFormatWhenProviderRejectsIt(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if attempts == 1 {
			if _, ok := body["response_format"]; !ok {
				t.Fatal("first request missing response_format")
			}
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"response_format is not supported","type":"invalid_request_error"}}`))
			return
		}
		if _, ok := body["response_format"]; ok {
			t.Fatal("retry still included response_format")
		}
		_, _ = w.Write([]byte(chatCompletionBody(t, `[{"id":"mq1"}]`)))
	}))
	defer server.Close()

	adapter, err := New(Config{BaseURL: server.URL, APIKey: "k", Model: "m"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Object schema triggers json_object response_format (array schemas omit it).
	request := testRequest()
	request.Schema.JSONSchema = json.RawMessage(`{"type":"object"}`)
	result, err := adapter.Generate(context.Background(), request)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
	if string(result.Object) != `[{"id":"mq1"}]` {
		t.Errorf("object = %s", result.Object)
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

func TestGenerateExtractsProseWrappedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(chatCompletionBody(t, "Sure, here is the set:\n[{\"id\":\"mq1\",\"text\":\"Q?\"}]\nDone.")))
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
	if string(result.Object) != `[{"id":"mq1","text":"Q?"}]` {
		t.Errorf("object = %s", result.Object)
	}
}

func TestGenerateRejectsNonJSONOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(chatCompletionBody(t, "Sure! Here are your questions: one, two, three.")))
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

func TestGenerateSurfacesEmptyContentDetails(t *testing.T) {
	body := `{
		"model":"gemini-flash-latest",
		"choices":[{"message":{"content":"","refusal":{"reason":"safety"}},"finish_reason":"SAFETY"}],
		"candidates":[{"finishReason":"SAFETY"}]
	}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	adapter, err := New(Config{BaseURL: server.URL, APIKey: "k", Model: "m"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = adapter.Generate(context.Background(), testRequest())
	if err == nil {
		t.Fatal("expected error for empty content")
	}
	if !strings.Contains(err.Error(), "invalid_output") ||
		!strings.Contains(err.Error(), "finish_reason=SAFETY") ||
		!strings.Contains(err.Error(), "refusal=") {
		t.Fatalf("err = %v, want empty-content diagnostic details", err)
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

func TestGenerateAzureAuthAndAPIVersion(t *testing.T) {
	var captured struct {
		apiKey         string
		auth           string
		apiVersion     string
		maxTokens      any
		maxCompletion  any
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.apiKey = r.Header.Get("api-key")
		captured.auth = r.Header.Get("Authorization")
		captured.apiVersion = r.URL.Query().Get("api-version")
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		captured.maxTokens = body["max_tokens"]
		captured.maxCompletion = body["max_completion_tokens"]
		_, _ = w.Write([]byte(chatCompletionBody(t, `{"ok":true}`)))
	}))
	defer server.Close()

	adapter, err := New(Config{
		Name:             "azure",
		BaseURL:          server.URL + "/openai/deployments/gpt-4o",
		APIKey:           "azure-secret",
		Model:            "gpt-4o",
		AuthStyle:        AuthAzureAPIKey,
		APIVersion:       "2024-10-21-preview",
		DefaultMaxTokens: 2048,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	result, err := adapter.Generate(context.Background(), testRequest())
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if result.Provider != "azure" {
		t.Errorf("provider = %q", result.Provider)
	}
	if captured.apiKey != "azure-secret" {
		t.Errorf("api-key = %q", captured.apiKey)
	}
	if captured.auth != "" {
		t.Errorf("Authorization should be empty for azure_api_key, got %q", captured.auth)
	}
	if captured.apiVersion != "2024-10-21-preview" {
		t.Errorf("api-version = %q", captured.apiVersion)
	}
	// Azure uses max_completion_tokens, not max_tokens.
	if captured.maxTokens != nil {
		t.Errorf("max_tokens = %#v, want omitted for Azure", captured.maxTokens)
	}
	if captured.maxCompletion != float64(2048) {
		t.Errorf("max_completion_tokens = %#v, want default 2048", captured.maxCompletion)
	}
}

func TestGenerateNamedProviderAndDefaultMaxTokens(t *testing.T) {
	var maxTokens any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		maxTokens = body["max_tokens"]
		_, _ = w.Write([]byte(chatCompletionBody(t, `[]`)))
	}))
	defer server.Close()

	adapter, err := New(Config{
		Name:             "meta",
		BaseURL:          server.URL,
		APIKey:           "k",
		Model:            "muse-spark-1.1",
		DefaultMaxTokens: 4096,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	result, err := adapter.Generate(context.Background(), testRequest())
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if result.Provider != "meta" {
		t.Errorf("provider = %q", result.Provider)
	}
	if maxTokens != float64(4096) {
		t.Errorf("max_tokens = %#v", maxTokens)
	}
}
