package gemini

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kvn8888/codegym/backend/internal/generation"
)

func interactionBody(t *testing.T, content string) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"status": "completed",
		"model":  "gemini-3.5-flash",
		"steps": []map[string]any{
			{"type": "thought", "signature": "opaque"},
			{
				"type": "model_output",
				"content": []map[string]any{
					{"type": "text", "text": content},
				},
			},
		},
		"usage": map[string]any{
			"total_input_tokens":  12,
			"total_output_tokens": 34,
			"total_tokens":        99,
		},
	})
	if err != nil {
		t.Fatalf("encode fake response: %v", err)
	}
	return string(body)
}

func testRequest() generation.GenerateRequest {
	temp := 0.2
	return generation.GenerateRequest{
		Kind:         generation.KindMCQ,
		Spec:         json.RawMessage(`{"topic":"hash tables","count":2}`),
		Instructions: "You are the MCQ generator.",
		Schema: generation.Schema{
			Name:       "mcq_set",
			Version:    "1",
			JSONSchema: json.RawMessage(`{"type":"array","items":{"type":"object"}}`),
		},
		ModelPolicy: generation.ModelPolicy{
			MaxTokens:   4096,
			Temperature: &temp,
		},
		MemoryContext: generation.MemoryContext{
			Summary:     "22 events.",
			GrowthEdges: []string{"Caching"},
		},
	}
}

func TestGenerateUsesInteractionsSchema(t *testing.T) {
	var captured struct {
		path     string
		apiKey   string
		body     map[string]any
		received bool
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.received = true
		captured.path = r.URL.Path
		captured.apiKey = r.Header.Get("x-goog-api-key")
		_ = json.NewDecoder(r.Body).Decode(&captured.body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(interactionBody(t, `[{"id":"mq1"}]`)))
	}))
	defer server.Close()

	adapter, err := New(Config{BaseURL: server.URL, APIKey: "test-key", Model: "gemini-3.5-flash"})
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
	if captured.path != "/interactions" {
		t.Errorf("path = %q, want /interactions", captured.path)
	}
	if captured.apiKey != "test-key" {
		t.Errorf("x-goog-api-key = %q", captured.apiKey)
	}
	if captured.body["model"] != "gemini-3.5-flash" {
		t.Errorf("model = %v, want gemini-3.5-flash", captured.body["model"])
	}
	if captured.body["store"] != false {
		t.Errorf("store = %v, want false", captured.body["store"])
	}

	format, ok := captured.body["response_format"].(map[string]any)
	if !ok {
		t.Fatalf("response_format = %#v, want object", captured.body["response_format"])
	}
	if format["type"] != "text" || format["mime_type"] != "application/json" {
		t.Fatalf("response_format = %#v, want text application/json", format)
	}
	schema, ok := format["schema"].(map[string]any)
	if !ok || schema["type"] != "array" {
		t.Fatalf("schema = %#v, want array schema", format["schema"])
	}

	config, ok := captured.body["generation_config"].(map[string]any)
	if !ok {
		t.Fatalf("generation_config = %#v, want object", captured.body["generation_config"])
	}
	if config["max_output_tokens"] != float64(4096) || config["temperature"] != 0.2 {
		t.Fatalf("generation_config = %#v", config)
	}

	system, _ := captured.body["system_instruction"].(string)
	if !strings.Contains(system, "You are the MCQ generator.") {
		t.Error("system instruction is missing orchestration instructions")
	}
	if !strings.Contains(system, "untrusted reference data") {
		t.Error("system instruction is missing memory injection guard")
	}
	input, _ := captured.body["input"].(string)
	if !strings.Contains(input, `"topic":"hash tables"`) {
		t.Errorf("input = %q, want serialized spec", input)
	}

	if string(result.Object) != `[{"id":"mq1"}]` {
		t.Errorf("object = %s", result.Object)
	}
	if result.Provider != "gemini_interactions" {
		t.Errorf("provider = %q", result.Provider)
	}
	if result.TokensIn != 12 || result.TokensOut != 34 || result.CostUnits != 99 {
		t.Errorf("tokens = %d/%d/%d, want 12/34/99", result.TokensIn, result.TokensOut, result.CostUnits)
	}
}

func TestGenerateNormalizesOpenAICompatBaseURL(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(interactionBody(t, `{}`)))
	}))
	defer server.Close()

	adapter, err := New(Config{BaseURL: server.URL + "/openai", APIKey: "k", Model: "m"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := adapter.Generate(context.Background(), testRequest()); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if gotPath != "/interactions" {
		t.Fatalf("path = %q, want /interactions", gotPath)
	}
}

func TestGenerateSurfacesProviderError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":400,"message":"bad schema","status":"INVALID_ARGUMENT"}}`))
	}))
	defer server.Close()

	adapter, err := New(Config{BaseURL: server.URL, APIKey: "k", Model: "m"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = adapter.Generate(context.Background(), testRequest())
	if err == nil || !strings.Contains(err.Error(), "bad schema") {
		t.Fatalf("err = %v, want provider error containing message", err)
	}
}

func TestGenerateSurfacesMissingModelOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"completed","steps":[{"type":"thought","signature":"opaque"}]}`))
	}))
	defer server.Close()

	adapter, err := New(Config{BaseURL: server.URL, APIKey: "k", Model: "m"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = adapter.Generate(context.Background(), testRequest())
	if err == nil || !strings.Contains(err.Error(), "invalid_output") {
		t.Fatalf("err = %v, want invalid output", err)
	}
}

func TestGeneratePrefersModelPolicyModel(t *testing.T) {
	var gotModel string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotModel, _ = body["model"].(string)
		_, _ = w.Write([]byte(interactionBody(t, `{}`)))
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
