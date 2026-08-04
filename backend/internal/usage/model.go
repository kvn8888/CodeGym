package usage

import "time"

// Record is one GenAI completion's usage + estimated cost.
type Record struct {
	ID               string    `json:"id"`
	WorkspaceID      string    `json:"workspace_id"`
	UserID           string    `json:"user_id"`
	OperationID      string    `json:"operation_id,omitempty"`
	Provider         string    `json:"provider"`
	Model            string    `json:"model"`
	Kind             string    `json:"kind,omitempty"`
	TotalTokens      int       `json:"total_tokens"`
	TokensIn         int       `json:"tokens_in"`
	TokensOut        int       `json:"tokens_out"`
	ReasoningTokens  int       `json:"reasoning_tokens"`
	CacheReadTokens  int       `json:"cache_read_tokens"`
	CacheWriteTokens int       `json:"cache_write_tokens"`
	CostUSDMicros    int64     `json:"cost_usd_micros"`
	CreatedAt        time.Time `json:"created_at"`
}

// Aggregate is the rollup returned by GET /api/v1/cost.
type Aggregate struct {
	Currency       string          `json:"currency"`
	PricingAsOf    string          `json:"pricing_as_of"`
	TotalTokensIn  int64           `json:"total_tokens_in"`
	TotalTokensOut int64           `json:"total_tokens_out"`
	TotalCostUSD   float64         `json:"total_cost_usd"`
	CallCount      int             `json:"call_count"`
	ByProvider     []ProviderSlice `json:"by_provider"`
	ByModel        []ModelSlice    `json:"by_model"`
	Rates          []Rate          `json:"rates"`
}

// ProviderSlice is usage rolled up by provider id.
type ProviderSlice struct {
	Provider  string  `json:"provider"`
	TokensIn  int64   `json:"tokens_in"`
	TokensOut int64   `json:"tokens_out"`
	CostUSD   float64 `json:"cost_usd"`
	CallCount int     `json:"call_count"`
}

// ModelSlice is usage rolled up by provider+model.
type ModelSlice struct {
	Provider  string  `json:"provider"`
	Model     string  `json:"model"`
	TokensIn  int64   `json:"tokens_in"`
	TokensOut int64   `json:"tokens_out"`
	CostUSD   float64 `json:"cost_usd"`
	CallCount int     `json:"call_count"`
}

// RecordInput is the service input for appending a usage row.
type RecordInput struct {
	OperationID      string
	Provider         string
	Model            string
	Kind             string
	TotalTokens      int
	TokensIn         int
	TokensOut        int
	ReasoningTokens  int
	CacheReadTokens  int
	CacheWriteTokens int
}

// Scope is an already-authenticated server-owned usage scope. Browser-facing
// handlers should keep using Record, which derives this from request context.
type Scope struct {
	WorkspaceID string
	UserID      string
}
