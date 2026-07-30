package workflow

import (
	"context"
	"errors"
)

var (
	ErrNotFound     = errors.New("workflow operation not found")
	ErrConflict     = errors.New("workflow operation conflict")
	ErrTerminal     = errors.New("workflow operation is already terminal")
	ErrPrerequisite = errors.New("workflow prerequisite has not succeeded")
)

type Store interface {
	EnsureSchema(ctx context.Context) error
	CreateOperation(ctx context.Context, operation Operation) (Operation, error)
	GetOperation(ctx context.Context, workspaceID, userID, operationID string) (Operation, error)
	AppendEvent(
		ctx context.Context,
		workspaceID, userID, operationID string,
		event Event,
		operationStatus Status,
	) (Operation, Event, error)
	ListEvents(
		ctx context.Context,
		workspaceID, userID, operationID string,
		afterSequence int64,
		limit int,
	) ([]Event, error)
}
