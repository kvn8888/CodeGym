package agentrelay

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/generation/openaicompat"
)

func TestChatCompletionsPreservesToolRoundTripAndMapsModel(t *testing.T) {
	const providerSecret = "provider-secret-must-never-leave-the-backend"
	var mu sync.Mutex
	requests := make([]map[string]any, 0, 2)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" || r.URL.Query().Get("api-version") != "test-version" {
			t.Fatalf("unexpected upstream URL: %s", r.URL.String())
		}
		if r.Header.Get("api-key") != providerSecret {
			t.Fatalf("upstream api-key = %q", r.Header.Get("api-key"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		mu.Lock()
		requests = append(requests, body)
		requestNumber := len(requests)
		mu.Unlock()
		if body["model"] != "azure-deployment" {
			t.Fatalf("upstream model = %#v", body["model"])
		}
		if _, exists := body["max_tokens"]; exists {
			t.Fatal("Azure upstream request retained max_tokens")
		}
		if body["max_completion_tokens"] != float64(512) {
			t.Fatalf("max_completion_tokens = %#v", body["max_completion_tokens"])
		}
		if requestNumber == 1 {
			tools, _ := body["tools"].([]any)
			if len(tools) != 1 {
				t.Fatalf("request tools were not preserved: %#v", body["tools"])
			}
		}

		w.Header().Set("Content-Type", "application/json")
		if requestNumber == 1 {
			writeTestJSON(t, w, map[string]any{
				"id": "chatcmpl-tool", "object": "chat.completion", "model": "azure-deployment",
				"choices": []any{
					map[string]any{
						"index": 0, "finish_reason": "tool_calls",
						"message": map[string]any{
							"role": "assistant", "content": nil,
							"tool_calls": []any{
								map[string]any{
									"id": "call_progress", "type": "function",
									"function": map[string]any{
										"name": "report_progress", "arguments": `{"step_id":"environment","label":"Setting up"}`,
									},
								},
							},
						},
					},
				},
				"usage": map[string]any{"prompt_tokens": 37, "completion_tokens": 11, "total_tokens": 48},
			})
			return
		}
		messages, _ := body["messages"].([]any)
		if len(messages) != 4 {
			t.Fatalf("second request messages = %#v", messages)
		}
		assistant, _ := messages[2].(map[string]any)
		toolResult, _ := messages[3].(map[string]any)
		if assistant["role"] != "assistant" || toolResult["role"] != "tool" || toolResult["tool_call_id"] != "call_progress" {
			t.Fatalf("tool continuation was not preserved: assistant=%#v tool=%#v", assistant, toolResult)
		}
		writeTestJSON(t, w, map[string]any{
			"id": "chatcmpl-final", "object": "chat.completion", "model": "azure-deployment",
			"choices": []any{
				map[string]any{
					"index": 0, "finish_reason": "stop",
					"message": map[string]any{"role": "assistant", "content": "DONE"},
				},
			},
			"usage": map[string]any{"prompt_tokens": 53, "completion_tokens": 3, "total_tokens": 56},
		})
	}))
	defer upstream.Close()

	handler, token := newTestHTTPHandler(t, upstream.URL, providerSecret)
	first := `{
		"model":"codegym-agent","max_tokens":512,"temperature":0.2,
		"messages":[{"role":"system","content":"Use tools"},{"role":"user","content":"Report progress"}],
		"tools":[{"type":"function","function":{"name":"report_progress","description":"Report progress","parameters":{"type":"object"}}}],
		"tool_choice":"auto"
	}`
	firstResponse := relayRequest(t, handler, token, first)
	if firstResponse.Code != http.StatusOK {
		t.Fatalf("first response = %d %s", firstResponse.Code, firstResponse.Body.String())
	}
	var firstBody map[string]any
	if err := json.Unmarshal(firstResponse.Body.Bytes(), &firstBody); err != nil {
		t.Fatal(err)
	}
	choices := firstBody["choices"].([]any)
	message := choices[0].(map[string]any)["message"].(map[string]any)
	toolCalls := message["tool_calls"].([]any)
	call := toolCalls[0].(map[string]any)
	if call["id"] != "call_progress" || call["type"] != "function" {
		t.Fatalf("tool call changed: %#v", call)
	}

	second := `{
		"model":"codegym-agent","max_tokens":512,
		"messages":[
			{"role":"system","content":"Use tools"},
			{"role":"user","content":"Report progress"},
			{"role":"assistant","content":"","tool_calls":[{"id":"call_progress","type":"function","function":{"name":"report_progress","arguments":"{\"step_id\":\"environment\",\"label\":\"Setting up\"}"}}]},
			{"role":"tool","tool_call_id":"call_progress","content":"{\"accepted\":true}"}
		],
		"tools":[{"type":"function","function":{"name":"report_progress","parameters":{"type":"object"}}}]
	}`
	secondResponse := relayRequest(t, handler, token, second)
	if secondResponse.Code != http.StatusOK || !strings.Contains(secondResponse.Body.String(), `"content":"DONE"`) {
		t.Fatalf("second response = %d %s", secondResponse.Code, secondResponse.Body.String())
	}
	if len(requests) != 2 {
		t.Fatalf("upstream request count = %d", len(requests))
	}
}

func TestChatCompletionsRejectsUnknownModelAndRedactsProviderCredential(t *testing.T) {
	const providerSecret = "provider-secret-must-never-leave-the-backend"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"message":"credential `+providerSecret+` rejected","type":"invalid_request_error","code":"bad_key"}}`)
	}))
	defer upstream.Close()
	handler, token := newTestHTTPHandler(t, upstream.URL, providerSecret)

	unknown := relayRequest(t, handler, token, `{"model":"arbitrary-provider-model","messages":[{"role":"user","content":"hi"}]}`)
	if unknown.Code != http.StatusNotFound || !strings.Contains(unknown.Body.String(), `"code":"model_not_found"`) {
		t.Fatalf("unknown model response = %d %s", unknown.Code, unknown.Body.String())
	}

	response := relayRequest(t, handler, token, `{"model":"codegym-agent","messages":[{"role":"user","content":"hi"}]}`)
	if response.Code != http.StatusBadRequest || strings.Contains(response.Body.String(), providerSecret) || !strings.Contains(response.Body.String(), "[REDACTED]") {
		t.Fatalf("credential redaction response = %d %s", response.Code, response.Body.String())
	}
}

func TestChatCompletionsStreamsWellFormedSSEThroughDone(t *testing.T) {
	const providerSecret = "provider-secret-must-never-leave-the-backend"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["stream"] != true {
			t.Fatalf("upstream stream flag = %#v", body["stream"])
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, `data: {"id":"chatcmpl-stream","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_progress","type":"function","function":{"name":"report_progress","arguments":"{\"step_id\":\"environment\"}"}}]},"finish_reason":null}]}`+"\n\n")
		flusher.Flush()
		_, _ = io.WriteString(w, `data: {"id":"chatcmpl-stream","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":37,"completion_tokens":11,"total_tokens":48}}`+"\n\n")
		flusher.Flush()
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer upstream.Close()
	handler, token := newTestHTTPHandler(t, upstream.URL, providerSecret)

	response := relayRequest(t, handler, token, `{"model":"codegym-agent","stream":true,"messages":[{"role":"user","content":"report"}],"tools":[{"type":"function","function":{"name":"report_progress","parameters":{"type":"object"}}}]}`)
	if response.Code != http.StatusOK {
		t.Fatalf("stream response = %d %s", response.Code, response.Body.String())
	}
	if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/event-stream") {
		t.Fatalf("stream content type = %q", contentType)
	}
	frames := strings.Split(strings.TrimSpace(response.Body.String()), "\n\n")
	if len(frames) != 3 {
		t.Fatalf("SSE frames = %d: %q", len(frames), response.Body.String())
	}
	for index, frame := range frames[:2] {
		if !strings.HasPrefix(frame, "data: ") || !json.Valid([]byte(strings.TrimPrefix(frame, "data: "))) {
			t.Fatalf("frame %d is not valid SSE JSON: %q", index, frame)
		}
	}
	if !strings.Contains(frames[0], `"tool_calls"`) {
		t.Fatalf("streamed tool call was lost: %q", frames[0])
	}
	if frames[2] != "data: [DONE]" {
		t.Fatalf("terminal frame = %q", frames[2])
	}
}

func newTestHTTPHandler(t *testing.T, upstreamURL, providerSecret string) (*HTTPHandler, string) {
	t.Helper()
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	checker := &operationChecker{active: map[string]bool{"ws-a\x00user-a\x00op-a": true}}
	tokens := newTestService(t, NewInMemoryStore(), checker, &now)
	issued, err := tokens.Issue(t.Context(), IssueInput{OperationID: "op-a", WorkspaceID: "ws-a", UserID: "user-a"})
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := openaicompat.New(openaicompat.Config{
		Name: "azure", BaseURL: upstreamURL, APIKey: providerSecret,
		Model: "azure-deployment", AuthStyle: openaicompat.AuthAzureAPIKey,
		APIVersion: "test-version", HTTPClient: http.DefaultClient,
	})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewHTTPHandler(tokens, adapter, "codegym-agent")
	if err != nil {
		t.Fatal(err)
	}
	return handler, issued.Token
}

func relayRequest(t *testing.T, handler *HTTPHandler, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/agent-relay/v1/chat/completions", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ChatCompletions(response, request)
	return response
}

func writeTestJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatal(err)
	}
}
