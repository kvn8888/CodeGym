package agentrelay

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound            = errors.New("relay operation budget not found")
	ErrConflict            = errors.New("relay operation budget already exists")
	ErrInvalidToken        = errors.New("invalid relay token")
	ErrEnvironmentMismatch = errors.New("relay token is scoped to another environment")
	ErrExpiredToken        = errors.New("relay token expired")
	ErrRevokedToken        = errors.New("relay token revoked")
	ErrOperationMismatch   = errors.New("relay token is scoped to another operation")
	ErrOperationTerminal   = errors.New("workflow operation is terminal")
	ErrTokenBudget         = errors.New("relay token budget is exhausted")
	ErrCostBudget          = errors.New("relay cost budget is exhausted")
	ErrDeadline            = errors.New("relay operation deadline has passed")
)

// Store persists relay authorization and budget state. Every read and update
// is scoped by workspace, user, and operation.
type Store interface {
	EnsureSchema(ctx context.Context) error
	Create(ctx context.Context, budget OperationBudget) (OperationBudget, error)
	Get(ctx context.Context, workspaceID, userID, operationID string) (OperationBudget, error)
	Revoke(ctx context.Context, workspaceID, userID, operationID string, at time.Time) error
	AddUsage(
		ctx context.Context,
		workspaceID, userID, operationID string,
		delta UsageDelta,
		at time.Time,
	) (OperationBudget, error)
}
