package agentruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type ChatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ToolDefinition struct {
	Type     string                 `json:"type"`
	Function ToolDefinitionFunction `json:"function"`
}

type ToolDefinitionFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type CompletionRequest struct {
	Messages []ChatMessage    `json:"messages"`
	Tools    []ToolDefinition `json:"tools,omitempty"`
}

type CompletionResponse struct {
	Message ChatMessage
	Usage   TokenUsage
	CostUSD int64
}

type CompletionClient interface {
	Complete(ctx context.Context, request CompletionRequest) (CompletionResponse, error)
}

type BudgetError struct {
	Reason TerminationReason
	Detail string
}

func (e *BudgetError) Error() string {
	if e.Detail != "" {
		return e.Detail
	}
	return string(e.Reason)
}

type OpenAICompatibleClientConfig struct {
	BaseURL          string
	APIKey           string
	Model            string
	MaxTokens        int
	MaxResponseBytes int64
	HTTPClient       *http.Client
}

type OpenAICompatibleClient struct {
	baseURL          string
	apiKey           string
	model            string
	maxTokens        int
	maxResponseBytes int64
	httpClient       *http.Client
}

func NewOpenAICompatibleClient(config OpenAICompatibleClientConfig) (*OpenAICompatibleClient, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if baseURL == "" || strings.TrimSpace(config.APIKey) == "" || strings.TrimSpace(config.Model) == "" {
		return nil, errors.New("agentruntime: relay base URL, API key, and model are required")
	}
	maxResponseBytes := config.MaxResponseBytes
	if maxResponseBytes <= 0 {
		maxResponseBytes = 2 << 20
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &OpenAICompatibleClient{
		baseURL: baseURL, apiKey: strings.TrimSpace(config.APIKey), model: strings.TrimSpace(config.Model),
		maxTokens: config.MaxTokens, maxResponseBytes: maxResponseBytes, httpClient: httpClient,
	}, nil
}

func (c *OpenAICompatibleClient) Complete(ctx context.Context, request CompletionRequest) (CompletionResponse, error) {
	if c == nil {
		return CompletionResponse{}, errors.New("agentruntime: OpenAI-compatible client is nil")
	}
	body := struct {
		Model      string           `json:"model"`
		Messages   []ChatMessage    `json:"messages"`
		Tools      []ToolDefinition `json:"tools,omitempty"`
		ToolChoice string           `json:"tool_choice,omitempty"`
		MaxTokens  int              `json:"max_tokens,omitempty"`
	}{
		Model: c.model, Messages: request.Messages, Tools: request.Tools,
		MaxTokens: c.maxTokens,
	}
	if len(body.Tools) > 0 {
		body.ToolChoice = "auto"
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("agentruntime: encode relay request: %w", err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("agentruntime: build relay request: %w", err)
	}
	httpRequest.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpRequest.Header.Set("Content-Type", "application/json")
	httpResponse, err := c.httpClient.Do(httpRequest)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("agentruntime: call relay: %w", err)
	}
	defer func() { _ = httpResponse.Body.Close() }()
	responseBody, err := io.ReadAll(io.LimitReader(httpResponse.Body, c.maxResponseBytes+1))
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("agentruntime: read relay response: %w", err)
	}
	if int64(len(responseBody)) > c.maxResponseBytes {
		return CompletionResponse{}, &BudgetError{Reason: TerminationOutputCeiling, Detail: "relay response exceeded the output ceiling"}
	}
	var envelope struct {
		Choices []struct {
			Message ChatMessage `json:"message"`
		} `json:"choices"`
		Usage struct {
			TotalTokens      int64 `json:"total_tokens"`
			PromptTokens     int64 `json:"prompt_tokens"`
			CompletionTokens int64 `json:"completion_tokens"`
			ReasoningTokens  int64 `json:"reasoning_tokens"`
			CacheReadTokens  int64 `json:"cache_read_tokens"`
			CacheWriteTokens int64 `json:"cache_write_tokens"`
			PromptDetails    struct {
				CachedTokens int64 `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
			CompletionDetails struct {
				ReasoningTokens int64 `json:"reasoning_tokens"`
			} `json:"completion_tokens_details"`
		} `json:"usage"`
		Error *struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(responseBody, &envelope); err != nil {
		return CompletionResponse{}, fmt.Errorf("agentruntime: decode relay response: %w", err)
	}
	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		message := strings.TrimSpace(http.StatusText(httpResponse.StatusCode))
		code := ""
		if envelope.Error != nil {
			message = strings.TrimSpace(envelope.Error.Message)
			code = envelope.Error.Code
		}
		switch code {
		case "token_budget_exceeded":
			return CompletionResponse{}, &BudgetError{Reason: TerminationTokenBudget, Detail: message}
		case "cost_budget_exceeded":
			return CompletionResponse{}, &BudgetError{Reason: TerminationCostBudget, Detail: message}
		case "operation_deadline_exceeded":
			return CompletionResponse{}, &BudgetError{Reason: TerminationDeadline, Detail: message}
		default:
			return CompletionResponse{}, fmt.Errorf("agentruntime: relay returned HTTP %d: %s", httpResponse.StatusCode, message)
		}
	}
	if len(envelope.Choices) != 1 {
		return CompletionResponse{}, errors.New("agentruntime: relay response must contain exactly one choice")
	}
	input := envelope.Usage.PromptTokens
	output := envelope.Usage.CompletionTokens
	total := envelope.Usage.TotalTokens
	if total < input+output {
		total = input + output
	}
	reasoning := envelope.Usage.ReasoningTokens
	if envelope.Usage.CompletionDetails.ReasoningTokens > reasoning {
		reasoning = envelope.Usage.CompletionDetails.ReasoningTokens
	}
	cacheRead := envelope.Usage.CacheReadTokens
	if envelope.Usage.PromptDetails.CachedTokens > cacheRead {
		cacheRead = envelope.Usage.PromptDetails.CachedTokens
	}
	return CompletionResponse{
		Message: envelope.Choices[0].Message,
		Usage: TokenUsage{
			Total: total, Input: input, Output: output, Reasoning: reasoning,
			CacheRead: cacheRead, CacheWrite: envelope.Usage.CacheWriteTokens,
		},
	}, nil
}
