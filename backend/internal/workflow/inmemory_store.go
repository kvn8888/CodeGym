package workflow

import (
	"context"
	"sort"
	"sync"
)

type InMemoryStore struct {
	mu         sync.RWMutex
	operations map[string]Operation
	events     map[string][]Event
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		operations: map[string]Operation{},
		events:     map[string][]Event{},
	}
}

func (s *InMemoryStore) EnsureSchema(context.Context) error { return nil }

func (s *InMemoryStore) CreateOperation(_ context.Context, operation Operation) (Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.operations[operation.ID]; exists {
		return Operation{}, ErrConflict
	}
	s.operations[operation.ID] = copyOperation(operation)
	return copyOperation(operation), nil
}

func (s *InMemoryStore) GetOperation(_ context.Context, workspaceID, userID, operationID string) (Operation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	operation, ok := s.operations[operationID]
	if !ok || operation.WorkspaceID != workspaceID || operation.UserID != userID {
		return Operation{}, ErrNotFound
	}
	return copyOperation(operation), nil
}

func (s *InMemoryStore) AppendEvent(
	_ context.Context,
	workspaceID, userID, operationID string,
	event Event,
	operationStatus Status,
) (Operation, Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	operation, ok := s.operations[operationID]
	if !ok || operation.WorkspaceID != workspaceID || operation.UserID != userID {
		return Operation{}, Event{}, ErrNotFound
	}
	if operation.Status == StatusSucceeded || operation.Status == StatusFailed {
		return Operation{}, Event{}, ErrTerminal
	}
	operation.LastSequence++
	event.OperationID = operation.ID
	event.Sequence = operation.LastSequence
	operation.Status = operationStatus
	operation.UpdatedAt = event.Timestamp
	if operationStatus == StatusSucceeded || operationStatus == StatusFailed {
		completedAt := event.Timestamp
		operation.CompletedAt = &completedAt
	}
	s.operations[operationID] = copyOperation(operation)
	s.events[operationID] = append(s.events[operationID], copyEvent(event))
	return copyOperation(operation), copyEvent(event), nil
}

func (s *InMemoryStore) ListEvents(
	_ context.Context,
	workspaceID, userID, operationID string,
	afterSequence int64,
	limit int,
) ([]Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	operation, ok := s.operations[operationID]
	if !ok || operation.WorkspaceID != workspaceID || operation.UserID != userID {
		return nil, ErrNotFound
	}
	out := make([]Event, 0)
	for _, event := range s.events[operationID] {
		if event.Sequence > afterSequence {
			out = append(out, copyEvent(event))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Sequence < out[j].Sequence })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func copyOperation(operation Operation) Operation {
	if operation.CompletedAt != nil {
		completedAt := *operation.CompletedAt
		operation.CompletedAt = &completedAt
	}
	return operation
}

func copyEvent(event Event) Event {
	event.Metadata = cloneMetadata(event.Metadata)
	return event
}
