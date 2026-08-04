// Command recorder captures every HTTP request and serves a small
// OpenAI-compatible API suitable for testing agent clients without credentials.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	completionID      = "chatcmpl-opencode-spike"
	defaultToolCallID = "call_opencode_spike_progress"
)

type config struct {
	port              int
	portFile          string
	transcript        string
	label             string
	responsesAPI      bool
	scriptedToolCall  bool
	toolCallID        string
	toolCallName      string
	toolCallArguments string
}

type recorder struct {
	cfg config
	mu  sync.Mutex
}

type transcriptEntry struct {
	Timestamp  string              `json:"timestamp"`
	Label      string              `json:"label,omitempty"`
	Method     string              `json:"method"`
	FullPath   string              `json:"full_path"`
	RawQuery   string              `json:"raw_query"`
	Headers    map[string][]string `json:"headers"`
	Body       string              `json:"body"`
	ToolResult *toolResultCheck    `json:"tool_result,omitempty"`
}

type toolResultCheck struct {
	Present            bool   `json:"present"`
	ShapeValid         bool   `json:"shape_valid"`
	ExpectedToolCallID string `json:"expected_tool_call_id"`
	ObservedToolCallID string `json:"observed_tool_call_id,omitempty"`
	ObservedName       string `json:"observed_name,omitempty"`
	Content            any    `json:"content,omitempty"`
	Reason             string `json:"reason,omitempty"`
}

func main() {
	var cfg config
	flag.IntVar(&cfg.port, "port", 0, "TCP port on 127.0.0.1 (0 chooses a free port)")
	flag.StringVar(&cfg.portFile, "port-file", "", "optional file to receive the selected port")
	flag.StringVar(&cfg.transcript, "transcript", "../transcript.jsonl", "JSONL transcript path")
	flag.StringVar(&cfg.label, "label", "", "optional experiment label recorded with every request")
	flag.BoolVar(&cfg.responsesAPI, "responses-api", false, "also implement POST /v1/responses")
	flag.BoolVar(&cfg.scriptedToolCall, "scripted-tool-call", false, "return a tool call, then a final completion")
	flag.StringVar(&cfg.toolCallID, "tool-call-id", defaultToolCallID, "ID for the scripted tool call")
	flag.StringVar(&cfg.toolCallName, "tool-call-name", "", "tool name to force on the first agent turn")
	flag.StringVar(&cfg.toolCallArguments, "tool-call-arguments", "{}", "JSON arguments for the forced tool call")
	flag.Parse()

	if envEnabled("RECORDER_SCRIPTED_TOOL_CALL") {
		cfg.scriptedToolCall = true
	}
	if cfg.scriptedToolCall && cfg.toolCallName == "" {
		cfg.toolCallName = "report_progress"
		cfg.toolCallArguments = `{"step_id":"environment","label":"Setting up"}`
	}
	if envEnabled("RECORDER_RESPONSES_API") {
		cfg.responsesAPI = true
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", cfg.port))
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if cfg.portFile != "" {
		if err := os.MkdirAll(filepath.Dir(cfg.portFile), 0o755); err != nil {
			log.Fatalf("create port-file directory: %v", err)
		}
		if err := os.WriteFile(cfg.portFile, []byte(fmt.Sprintf("%d\n", port)), 0o600); err != nil {
			log.Fatalf("write port file: %v", err)
		}
	}

	r := &recorder{cfg: cfg}
	server := &http.Server{
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("recorder listening on http://127.0.0.1:%d", port)
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Fatalf("serve: %v", err)
	}
}

func envEnabled(name string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	return v == "1" || v == "true" || v == "yes"
}

func (r *recorder) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, `{"error":{"message":"could not read request"}}`, http.StatusBadRequest)
		return
	}
	entry := transcriptEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Label:     r.cfg.label,
		Method:    req.Method,
		FullPath:  req.URL.RequestURI(),
		RawQuery:  req.URL.RawQuery,
		Headers:   req.Header.Clone(),
		Body:      prettyBody(body),
	}

	var payload map[string]any
	_ = json.Unmarshal(body, &payload)
	if r.scriptedToolCall() && isChatCompletions(req.URL.Path) {
		check := inspectToolResult(payload, r.cfg.toolCallID)
		entry.ToolResult = &check
	}
	if err := r.append(entry); err != nil {
		log.Printf("record request: %v", err)
	}
	log.Printf("%s %s body=%d tool_result=%s", req.Method, req.URL.RequestURI(), len(body), toolStatus(entry.ToolResult))

	switch {
	case req.Method == http.MethodGet && (req.URL.Path == "/v1/models" || req.URL.Path == "/models"):
		writeJSON(w, http.StatusOK, map[string]any{
			"object": "list",
			"data":   []any{map[string]any{"id": "spike-model", "object": "model", "created": 1, "owned_by": "codegym-spike"}},
		})
	case req.Method == http.MethodPost && isChatCompletions(req.URL.Path):
		hasToolResult := entry.ToolResult != nil && entry.ToolResult.Present
		if wantsStream(payload) {
			r.writeChatStream(w, hasToolResult)
		} else {
			r.writeChatJSON(w, hasToolResult)
		}
	case req.Method == http.MethodPost && req.URL.Path == "/v1/responses" && r.cfg.responsesAPI:
		r.writeResponsesJSON(w)
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"error": map[string]any{"message": "unimplemented recorder path", "type": "not_found"}})
	}
}

func isChatCompletions(path string) bool {
	return path == "/v1/chat/completions" || path == "/chat/completions" || strings.HasSuffix(path, "/chat/completions")
}

func prettyBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var decoded any
	if json.Unmarshal(body, &decoded) == nil {
		pretty, err := json.MarshalIndent(decoded, "", "  ")
		if err == nil {
			return string(pretty)
		}
	}
	return string(body)
}

func (r *recorder) append(entry transcriptEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(r.cfg.transcript), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(r.cfg.transcript, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(entry)
}

func inspectToolResult(payload map[string]any, expectedToolCallID string) toolResultCheck {
	check := toolResultCheck{ExpectedToolCallID: expectedToolCallID}
	messages, _ := payload["messages"].([]any)
	for _, raw := range messages {
		message, _ := raw.(map[string]any)
		if message["role"] != "tool" {
			continue
		}
		check.Present = true
		check.ObservedToolCallID, _ = message["tool_call_id"].(string)
		check.ObservedName, _ = message["name"].(string)
		check.Content = message["content"]
		if check.ObservedToolCallID != expectedToolCallID {
			check.Reason = "role=tool message has the wrong or missing tool_call_id"
			return check
		}
		if message["content"] == nil {
			check.Reason = "role=tool message has no content"
			return check
		}
		check.ShapeValid = true
		return check
	}
	check.Reason = "no role=tool message observed"
	return check
}

func (r *recorder) scriptedToolCall() bool {
	return r.cfg.toolCallName != ""
}

func toolStatus(check *toolResultCheck) string {
	if check == nil {
		return "n/a"
	}
	if check.ShapeValid {
		return "valid"
	}
	if check.Present {
		return "invalid"
	}
	return "absent"
}

func wantsStream(payload map[string]any) bool {
	stream, _ := payload["stream"].(bool)
	return stream
}

func (r *recorder) writeChatJSON(w http.ResponseWriter, hasToolResult bool) {
	message, finish := r.nextMessage(hasToolResult)
	writeJSON(w, http.StatusOK, map[string]any{
		"id": completionID, "object": "chat.completion", "created": time.Now().Unix(), "model": "spike-model",
		"choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finish}},
		"usage":   map[string]any{"prompt_tokens": 37, "completion_tokens": 11, "total_tokens": 48},
	})
}

func (r *recorder) nextMessage(hasToolResult bool) (map[string]any, string) {
	if r.scriptedToolCall() && !hasToolResult {
		return map[string]any{
			"role": "assistant", "content": nil,
			"tool_calls": []any{map[string]any{
				"id": r.cfg.toolCallID, "type": "function",
				"function": map[string]any{"name": r.cfg.toolCallName, "arguments": r.cfg.toolCallArguments},
			}},
		}, "tool_calls"
	}
	return map[string]any{"role": "assistant", "content": "DONE — the work is done."}, "stop"
}

func (r *recorder) writeChatStream(w http.ResponseWriter, hasToolResult bool) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	writer := bufio.NewWriter(w)
	writeChunk := func(chunk map[string]any) {
		data, _ := json.Marshal(chunk)
		fmt.Fprintf(writer, "data: %s\n\n", data)
		writer.Flush()
	}
	base := map[string]any{"id": completionID, "object": "chat.completion.chunk", "created": time.Now().Unix(), "model": "spike-model"}
	if r.scriptedToolCall() && !hasToolResult {
		base["choices"] = []any{map[string]any{
			"index": 0, "finish_reason": nil,
			"delta": map[string]any{"role": "assistant", "content": nil, "tool_calls": []any{map[string]any{
				"index": 0, "id": r.cfg.toolCallID, "type": "function",
				"function": map[string]any{"name": r.cfg.toolCallName, "arguments": r.cfg.toolCallArguments},
			}}},
		}}
		writeChunk(base)
		base["choices"] = []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "tool_calls"}}
	} else {
		base["choices"] = []any{map[string]any{"index": 0, "finish_reason": nil, "delta": map[string]any{"role": "assistant", "content": "DONE — the work is done."}}}
		writeChunk(base)
		base["choices"] = []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}}
	}
	base["usage"] = map[string]any{"prompt_tokens": 37, "completion_tokens": 11, "total_tokens": 48}
	writeChunk(base)
	fmt.Fprint(writer, "data: [DONE]\n\n")
	writer.Flush()
}

func (r *recorder) writeResponsesJSON(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, map[string]any{
		"id": "resp_opencode_spike", "object": "response", "created_at": time.Now().Unix(), "status": "completed",
		"model": "spike-model", "output": []any{map[string]any{
			"id": "msg_opencode_spike", "type": "message", "status": "completed", "role": "assistant",
			"content": []any{map[string]any{"type": "output_text", "text": "DONE — the work is done.", "annotations": []any{}}},
		}},
		"usage": map[string]any{"input_tokens": 37, "output_tokens": 11, "total_tokens": 48},
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
