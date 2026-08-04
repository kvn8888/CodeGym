package agentrelay

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
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
	publicModel string
}

func NewHTTPHandler(tokens *Service, upstream ChatTransport, publicModel string) (*HTTPHandler, error) {
	if tokens == nil {
		return nil, errors.New("agentrelay: token service is required")
	}
	if upstream == nil || strings.TrimSpace(upstream.RelayModel()) == "" {
		return nil, errors.New("agentrelay: upstream transport is required")
	}
	publicModel = strings.TrimSpace(publicModel)
	if publicModel == "" {
		return nil, errors.New("agentrelay: public model is required")
	}
	return &HTTPHandler{tokens: tokens, upstream: upstream, publicModel: publicModel}, nil
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
	if _, ok := h.authenticate(w, r); !ok {
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
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "streaming_unavailable", "Streaming is not available.", "stream")
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
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(responseBody)
}

func (h *HTTPHandler) authenticate(w http.ResponseWriter, r *http.Request) (Authorization, bool) {
	if h == nil || h.tokens == nil || h.upstream == nil {
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
		}
		writeOpenAIError(w, http.StatusUnauthorized, "authentication_error", code, message, "")
		return Authorization{}, false
	}
	return authorization, true
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
