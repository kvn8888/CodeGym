package agentrelay

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	usagepkg "github.com/kvn8888/codegym/backend/internal/usage"
)

const maxRelayBodyBytes = 8 << 20

// ChatTransport is the narrow raw chat-completions seam implemented by the
// configured generation/openaicompat adapter.
type ChatTransport interface {
	RelayChatCompletion(ctx context.Context, payload []byte, stream bool) (*http.Response, error)
	RelayModel() string
	RelayProvider() string
	RedactProviderSecrets(payload []byte) []byte
}

// HTTPHandler serves the sandbox-facing OpenAI-compatible surface with its own
// single-operation bearer authenticator.
type HTTPHandler struct {
	tokens      *Service
	upstream    ChatTransport
	usage       *usagepkg.Service
	publicModel string
}

func NewHTTPHandler(tokens *Service, upstream ChatTransport, usageService *usagepkg.Service, publicModel string) (*HTTPHandler, error) {
	if tokens == nil {
		return nil, errors.New("agentrelay: token service is required")
	}
	if upstream == nil || strings.TrimSpace(upstream.RelayModel()) == "" {
		return nil, errors.New("agentrelay: upstream transport is required")
	}
	if usageService == nil {
		return nil, errors.New("agentrelay: usage service is required")
	}
	publicModel = strings.TrimSpace(publicModel)
	if publicModel == "" {
		return nil, errors.New("agentrelay: public model is required")
	}
	return &HTTPHandler{tokens: tokens, upstream: upstream, usage: usageService, publicModel: publicModel}, nil
}

// Models implements GET /api/v1/agent-relay/v1/models.
func (h *HTTPHandler) Models(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.authenticate(w, r); !ok {
		return
	}
	writeOpenAIJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data": []any{map[string]any{
			"id": h.publicModel, "object": "model", "created": time.Now().Unix(),
			"owned_by": "codegym",
		}},
	})
}

// ChatCompletions implements POST /api/v1/agent-relay/v1/chat/completions.
func (h *HTTPHandler) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	authorization, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxRelayBodyBytes+1))
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "invalid_json", "Could not read request body.", "")
		return
	}
	if len(body) > maxRelayBodyBytes {
		writeOpenAIError(w, http.StatusRequestEntityTooLarge, "invalid_request_error", "request_too_large", "Request body is too large.", "")
		return
	}
	var request map[string]json.RawMessage
	if err := json.Unmarshal(body, &request); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "invalid_json", "Request body must be valid JSON.", "")
		return
	}
	for _, forbidden := range []string{"base_url", "baseURL", "upstream_url"} {
		if _, exists := request[forbidden]; exists {
			writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "unsupported_parameter", "Upstream routing is server-managed.", forbidden)
			return
		}
	}
	var model string
	if raw, ok := request["model"]; !ok || json.Unmarshal(raw, &model) != nil || strings.TrimSpace(model) == "" {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "missing_required_parameter", "Model is required.", "model")
		return
	}
	if model != h.publicModel {
		writeOpenAIError(w, http.StatusNotFound, "invalid_request_error", "model_not_found", "The requested model is not available.", "model")
		return
	}
	var messages []json.RawMessage
	if raw, ok := request["messages"]; !ok || json.Unmarshal(raw, &messages) != nil || len(messages) == 0 {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "missing_required_parameter", "Messages must be a non-empty array.", "messages")
		return
	}
	stream := false
	if raw, ok := request["stream"]; ok {
		if err := json.Unmarshal(raw, &stream); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "invalid_parameter", "Stream must be a boolean.", "stream")
			return
		}
	}
	if stream {
		h.streamChatCompletions(w, r, body, authorization)
		return
	}

	upstreamResponse, err := h.upstream.RelayChatCompletion(r.Context(), body, false)
	if err != nil {
		writeOpenAIError(w, http.StatusBadGateway, "api_error", "upstream_unavailable", "The upstream model is unavailable.", "")
		return
	}
	defer func() { _ = upstreamResponse.Body.Close() }()
	responseBody, err := io.ReadAll(io.LimitReader(upstreamResponse.Body, maxRelayBodyBytes+1))
	if err != nil || len(responseBody) > maxRelayBodyBytes {
		writeOpenAIError(w, http.StatusBadGateway, "api_error", "invalid_upstream_response", "The upstream model returned an invalid response.", "")
		return
	}
	responseBody = h.upstream.RedactProviderSecrets(responseBody)
	if upstreamResponse.StatusCode < 200 || upstreamResponse.StatusCode >= 300 {
		if isOpenAIErrorEnvelope(responseBody) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(upstreamResponse.StatusCode)
			_, _ = w.Write(responseBody)
			return
		}
		writeOpenAIError(w, http.StatusBadGateway, "api_error", "upstream_error", "The upstream model request failed.", "")
		return
	}
	if !json.Valid(responseBody) {
		writeOpenAIError(w, http.StatusBadGateway, "api_error", "invalid_upstream_response", "The upstream model returned invalid JSON.", "")
		return
	}
	completionUsage, ok := parseCompletionUsage(responseBody)
	if !ok {
		h.revokeAfterAccountingFailure(r.Context(), authorization, errors.New("provider response omitted usage"))
		writeOpenAIError(w, http.StatusBadGateway, "api_error", "usage_accounting_failed", "The upstream model did not return required usage data.", "")
		return
	}
	if err := h.recordUsage(r.Context(), authorization, completionUsage); err != nil {
		h.revokeAfterAccountingFailure(r.Context(), authorization, err)
		writeOpenAIError(w, http.StatusInternalServerError, "api_error", "usage_accounting_failed", "Model usage could not be recorded.", "")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(responseBody)
}

func (h *HTTPHandler) streamChatCompletions(w http.ResponseWriter, r *http.Request, body []byte, authorization Authorization) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeOpenAIError(w, http.StatusInternalServerError, "api_error", "streaming_unsupported", "Streaming is not available.", "stream")
		return
	}
	upstreamResponse, err := h.upstream.RelayChatCompletion(r.Context(), body, true)
	if err != nil {
		writeOpenAIError(w, http.StatusBadGateway, "api_error", "upstream_unavailable", "The upstream model is unavailable.", "")
		return
	}
	defer func() { _ = upstreamResponse.Body.Close() }()
	if upstreamResponse.StatusCode < 200 || upstreamResponse.StatusCode >= 300 {
		responseBody, readErr := io.ReadAll(io.LimitReader(upstreamResponse.Body, maxRelayBodyBytes+1))
		if readErr == nil && len(responseBody) <= maxRelayBodyBytes {
			responseBody = h.upstream.RedactProviderSecrets(responseBody)
			if isOpenAIErrorEnvelope(responseBody) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(upstreamResponse.StatusCode)
				_, _ = w.Write(responseBody)
				return
			}
		}
		writeOpenAIError(w, http.StatusBadGateway, "api_error", "upstream_error", "The upstream model request failed.", "")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Relay complete SSE lines as they arrive. This preserves tool-call deltas
	// and usage frames while allowing provider credentials to be redacted before
	// any line crosses the sandbox-facing boundary.
	scanner := bufio.NewScanner(upstreamResponse.Body)
	scanner.Buffer(make([]byte, 0, 64<<10), maxRelayBodyBytes)
	sawDone := false
	var completionUsage CompletionUsage
	sawUsage := false
	for scanner.Scan() {
		line := h.upstream.RedactProviderSecrets(scanner.Bytes())
		if parsed, ok := parseSSEUsage(line); ok {
			completionUsage = parsed
			sawUsage = true
		}
		if strings.TrimSpace(string(line)) == "data: [DONE]" {
			sawDone = true
		}
		if _, err := w.Write(append(append([]byte(nil), line...), '\n')); err != nil {
			return
		}
		if len(line) == 0 {
			flusher.Flush()
		}
	}
	if sawDone && sawUsage {
		if err := h.recordUsage(r.Context(), authorization, completionUsage); err != nil {
			h.revokeAfterAccountingFailure(r.Context(), authorization, err)
			log.Printf("agent relay streaming usage accounting failed operation_id=%s: %v", authorization.Claims.OperationID, err)
		}
	} else if sawDone {
		h.revokeAfterAccountingFailure(r.Context(), authorization, errors.New("provider stream omitted usage"))
		log.Printf("agent relay provider stream omitted usage operation_id=%s", authorization.Claims.OperationID)
	}
	if !sawDone {
		h.revokeAfterAccountingFailure(r.Context(), authorization, errors.New("provider stream ended before DONE"))
		errorFrame, _ := json.Marshal(map[string]any{"error": map[string]any{
			"message": "The upstream model stream ended unexpectedly.",
			"type":    "api_error", "param": nil, "code": "upstream_stream_ended",
		}})
		_, _ = w.Write([]byte("data: " + string(errorFrame) + "\n\ndata: [DONE]\n\n"))
		flusher.Flush()
	}
}

func (h *HTTPHandler) authenticate(w http.ResponseWriter, r *http.Request) (Authorization, bool) {
	if h == nil || h.tokens == nil || h.upstream == nil || h.usage == nil {
		writeOpenAIError(w, http.StatusServiceUnavailable, "api_error", "relay_unconfigured", "The model relay is not configured.", "")
		return Authorization{}, false
	}
	token, ok := relayBearerToken(r.Header.Get("Authorization"))
	if !ok {
		writeOpenAIError(w, http.StatusUnauthorized, "authentication_error", "invalid_api_key", "Missing relay bearer token.", "")
		return Authorization{}, false
	}
	authorization, err := h.tokens.Authenticate(r.Context(), token)
	if err != nil {
		code := "invalid_api_key"
		message := "Invalid relay bearer token."
		if errors.Is(err, ErrExpiredToken) {
			code, message = "token_expired", "Relay bearer token has expired."
		} else if errors.Is(err, ErrRevokedToken) || errors.Is(err, ErrOperationTerminal) {
			code, message = "token_revoked", "Relay bearer token is no longer active."
		} else if errors.Is(err, ErrTokenBudget) {
			writeOpenAIError(w, http.StatusTooManyRequests, "rate_limit_error", "token_budget_exceeded", "The operation token budget is exhausted.", "")
			return Authorization{}, false
		} else if errors.Is(err, ErrCostBudget) {
			writeOpenAIError(w, http.StatusTooManyRequests, "rate_limit_error", "cost_budget_exceeded", "The operation cost budget is exhausted.", "")
			return Authorization{}, false
		} else if errors.Is(err, ErrDeadline) {
			writeOpenAIError(w, http.StatusRequestTimeout, "invalid_request_error", "operation_deadline_exceeded", "The operation wall-clock deadline has passed.", "")
			return Authorization{}, false
		}
		writeOpenAIError(w, http.StatusUnauthorized, "authentication_error", code, message, "")
		return Authorization{}, false
	}
	return authorization, true
}

// CompletionUsage is the normalized chat-completions usage shape. Detail
// categories remain informational subsets of input/output and are not added to
// TotalTokens or priced twice.
type CompletionUsage struct {
	TotalTokens      int64
	InputTokens      int64
	OutputTokens     int64
	ReasoningTokens  int64
	CacheReadTokens  int64
	CacheWriteTokens int64
}

type completionUsageEnvelope struct {
	Usage struct {
		PromptTokens     int64 `json:"prompt_tokens"`
		CompletionTokens int64 `json:"completion_tokens"`
		TotalTokens      int64 `json:"total_tokens"`
		InputTokens      int64 `json:"input_tokens"`
		OutputTokens     int64 `json:"output_tokens"`
		ReasoningTokens  int64 `json:"reasoning_tokens"`
		CacheReadTokens  int64 `json:"cache_read_tokens"`
		CacheWriteTokens int64 `json:"cache_write_tokens"`
		PromptDetails    struct {
			CachedTokens     int64 `json:"cached_tokens"`
			CacheReadTokens  int64 `json:"cache_read_tokens"`
			CacheWriteTokens int64 `json:"cache_write_tokens"`
		} `json:"prompt_tokens_details"`
		CompletionDetails struct {
			ReasoningTokens int64 `json:"reasoning_tokens"`
		} `json:"completion_tokens_details"`
	} `json:"usage"`
}

func parseCompletionUsage(payload []byte) (CompletionUsage, bool) {
	var envelope completionUsageEnvelope
	if json.Unmarshal(payload, &envelope) != nil {
		return CompletionUsage{}, false
	}
	input := envelope.Usage.PromptTokens
	if input == 0 {
		input = envelope.Usage.InputTokens
	}
	output := envelope.Usage.CompletionTokens
	if output == 0 {
		output = envelope.Usage.OutputTokens
	}
	total := envelope.Usage.TotalTokens
	if total == 0 {
		total = input + output
	}
	total = max64(total, input+output)
	reasoning := max64(envelope.Usage.ReasoningTokens, envelope.Usage.CompletionDetails.ReasoningTokens)
	cacheRead := max64(envelope.Usage.CacheReadTokens, envelope.Usage.PromptDetails.CachedTokens)
	cacheRead = max64(cacheRead, envelope.Usage.PromptDetails.CacheReadTokens)
	cacheWrite := max64(envelope.Usage.CacheWriteTokens, envelope.Usage.PromptDetails.CacheWriteTokens)
	parsed := CompletionUsage{
		TotalTokens: max64(0, total), InputTokens: max64(0, input),
		OutputTokens: max64(0, output), ReasoningTokens: max64(0, reasoning),
		CacheReadTokens: max64(0, cacheRead), CacheWriteTokens: max64(0, cacheWrite),
	}
	return parsed, parsed.TotalTokens > 0 || parsed.InputTokens > 0 || parsed.OutputTokens > 0
}

func parseSSEUsage(line []byte) (CompletionUsage, bool) {
	trimmed := strings.TrimSpace(string(line))
	if !strings.HasPrefix(trimmed, "data:") {
		return CompletionUsage{}, false
	}
	data := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
	if data == "" || data == "[DONE]" {
		return CompletionUsage{}, false
	}
	return parseCompletionUsage([]byte(data))
}

func (h *HTTPHandler) recordUsage(ctx context.Context, authorization Authorization, completion CompletionUsage) error {
	input := boundedInt(completion.InputTokens)
	output := boundedInt(completion.OutputTokens)
	cost := usagepkg.EstimateCostMicros(h.upstream.RelayProvider(), h.upstream.RelayModel(), input, output)
	delta := UsageDelta{
		TotalTokens: completion.TotalTokens, InputTokens: completion.InputTokens,
		OutputTokens: completion.OutputTokens, ReasoningTokens: completion.ReasoningTokens,
		CacheReadTokens: completion.CacheReadTokens, CacheWriteTokens: completion.CacheWriteTokens,
		CostUSDMicros: cost,
	}
	if _, err := h.tokens.AddUsage(ctx, authorization, delta); err != nil {
		return err
	}
	_, err := h.usage.RecordScoped(ctx, usagepkg.Scope{
		WorkspaceID: authorization.Claims.WorkspaceID, UserID: authorization.Claims.UserID,
	}, usagepkg.RecordInput{
		OperationID: authorization.Claims.OperationID, Provider: h.upstream.RelayProvider(),
		Model: h.upstream.RelayModel(), Kind: "agent_relay",
		TotalTokens: boundedInt(completion.TotalTokens), TokensIn: input, TokensOut: output,
		ReasoningTokens:  boundedInt(completion.ReasoningTokens),
		CacheReadTokens:  boundedInt(completion.CacheReadTokens),
		CacheWriteTokens: boundedInt(completion.CacheWriteTokens),
	})
	return err
}

func (h *HTTPHandler) revokeAfterAccountingFailure(ctx context.Context, authorization Authorization, cause error) {
	if err := h.tokens.Revoke(ctx, authorization.Claims.WorkspaceID, authorization.Claims.UserID, authorization.Claims.OperationID); err != nil {
		log.Printf("agent relay revoke after accounting failure operation_id=%s cause=%v revoke_error=%v", authorization.Claims.OperationID, cause, err)
	}
}

func boundedInt(value int64) int {
	if value <= 0 {
		return 0
	}
	if value > int64(math.MaxInt) {
		return math.MaxInt
	}
	return int(value)
}

func relayBearerToken(header string) (string, bool) {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return "", false
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	return token, token != ""
}

func isOpenAIErrorEnvelope(body []byte) bool {
	var envelope struct {
		Error *struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	return json.Unmarshal(body, &envelope) == nil && envelope.Error != nil && envelope.Error.Message != ""
}

func writeOpenAIError(w http.ResponseWriter, status int, errorType, code, message, param string) {
	errorBody := map[string]any{
		"message": message,
		"type":    errorType,
		"param":   nil,
		"code":    code,
	}
	if param != "" {
		errorBody["param"] = param
	}
	writeOpenAIJSON(w, status, map[string]any{"error": errorBody})
}

func writeOpenAIJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
