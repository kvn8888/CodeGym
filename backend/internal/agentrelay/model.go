package agentrelay

import "time"

// OperationBudget is the durable authorization and budget state for one
// workflow operation. The bearer token is stateless; this row is the
// revocation and accounting authority consulted after signature validation.
type OperationBudget struct {
	ID                string
	OperationID       string
	WorkspaceID       string
	UserID            string
	MaxTotalTokens    int64
	MaxCostUSDMicros  int64
	Deadline          time.Time
	UsedTotalTokens   int64
	InputTokens       int64
	OutputTokens      int64
	ReasoningTokens   int64
	CacheReadTokens   int64
	CacheWriteTokens  int64
	UsedCostUSDMicros int64
	RevokedAt         *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// Claims are the authenticated, immutable scope carried by a relay token.
type Claims struct {
	TokenID     string
	BudgetID    string
	OperationID string
	WorkspaceID string
	UserID      string
	IssuedAt    time.Time
	ExpiresAt   time.Time
}

// Authorization is returned only after the token and its scoped store row are
// both validated.
type Authorization struct {
	Claims Claims
	Budget OperationBudget
}

// IssueInput describes the workflow operation receiving a relay token.
type IssueInput struct {
	OperationID      string
	WorkspaceID      string
	UserID           string
	Deadline         time.Time
	MaxTotalTokens   int64
	MaxCostUSDMicros int64
}

// IssueResult returns the opaque bearer token and its server-owned limits.
type IssueResult struct {
	Token     string
	ExpiresAt time.Time
	Budget    OperationBudget
}
