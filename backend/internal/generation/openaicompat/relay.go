package openaicompat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// RelayModel is the configured provider model/deployment. It is server-owned
// and must not be selected from an untrusted relay request.
func (a *Adapter) RelayModel() string {
	if a == nil {
		return ""
	}
	return a.model
}

// RelayProvider is the provider id used for server-side pricing and usage.
func (a *Adapter) RelayProvider() string {
	return a.Name()
}

// RelayChatCompletion sends an OpenAI chat-completions payload through the
// configured provider transport. It always overwrites model selection and
// performs the Azure max-token field translation used by the owned adapter.
func (a *Adapter) RelayChatCompletion(ctx context.Context, payload []byte, stream bool) (*http.Response, error) {
	if a == nil {
		return nil, fmt.Errorf("openaicompat: adapter is nil")
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(payload, &body); err != nil {
		return nil, fmt.Errorf("openaicompat: decode relay request: %w", err)
	}
	model, err := json.Marshal(a.model)
	if err != nil {
		return nil, fmt.Errorf("openaicompat: encode relay model: %w", err)
	}
	body["model"] = model
	streamValue, err := json.Marshal(stream)
	if err != nil {
		return nil, fmt.Errorf("openaicompat: encode relay stream flag: %w", err)
	}
	body["stream"] = streamValue
	if stream {
		streamOptions, marshalErr := json.Marshal(map[string]any{"include_usage": true})
		if marshalErr != nil {
			return nil, fmt.Errorf("openaicompat: encode relay stream options: %w", marshalErr)
		}
		body["stream_options"] = streamOptions
	}
	if a.authStyle == AuthAzureAPIKey {
		if maxTokens, ok := body["max_tokens"]; ok {
			if _, explicit := body["max_completion_tokens"]; !explicit {
				body["max_completion_tokens"] = maxTokens
			}
			delete(body, "max_tokens")
		}
	}

	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("openaicompat: encode relay request: %w", err)
	}
	endpoint := a.baseURL + "/chat/completions"
	if a.apiVersion != "" {
		endpoint = appendQuery(endpoint, "api-version", a.apiVersion)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return nil, fmt.Errorf("openaicompat: build relay request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	if stream {
		request.Header.Set("Accept", "text/event-stream")
	} else {
		request.Header.Set("Accept", "application/json")
	}
	if a.authStyle == AuthAzureAPIKey {
		request.Header.Set("api-key", a.apiKey)
	} else {
		request.Header.Set("Authorization", "Bearer "+a.apiKey)
	}
	response, err := a.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("openaicompat: call relay provider: %w", err)
	}
	return response, nil
}

// RedactProviderSecrets removes the configured credential from provider
// payloads before any bytes can cross the sandbox-facing boundary.
func (a *Adapter) RedactProviderSecrets(payload []byte) []byte {
	if a == nil || a.apiKey == "" {
		return append([]byte(nil), payload...)
	}
	return bytes.ReplaceAll(payload, []byte(a.apiKey), []byte("[REDACTED]"))
}
