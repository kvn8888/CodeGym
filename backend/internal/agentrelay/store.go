package agentrelay

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound          = errors.New("relay operation budget not found")
	ErrConflict          = errors.New("relay operation budget already exists")
	ErrInvalidToken      = errors.New("invalid relay token")
	ErrExpiredToken      = errors.New("relay token expired")
	ErrRevokedToken      = errors.New("relay token revoked")
	ErrOperationMismatch = errors.New("relay token is scoped to another operation")
	ErrOperationTerminal = errors.New("workflow operation is terminal")
)

// Store persists relay authorization and budget state. Every read and update
// is scoped by workspace, user, and operation.
type Store interface {
	EnsureSchema(ctx context.Context) error
	Create(ctx context.Context, budget OperationBudget) (OperationBudget, error)
	Get(ctx context.Context, workspaceID, userID, operationID string) (OperationBudget, error)
	Revoke(ctx context.Context, workspaceID, userID, operationID string, at time.Time) error
}
