package agentrelay

import (
	"context"
	"sync"
	"time"
)

// InMemoryStore keeps relay operation budgets in process memory.
type InMemoryStore struct {
	mu      sync.RWMutex
	budgets map[string]OperationBudget
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{budgets: map[string]OperationBudget{}}
}

func (s *InMemoryStore) EnsureSchema(context.Context) error { return nil }

func (s *InMemoryStore) Create(_ context.Context, budget OperationBudget) (OperationBudget, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.budgets[budget.OperationID]; exists {
		return OperationBudget{}, ErrConflict
	}
	s.budgets[budget.OperationID] = copyBudget(budget)
	return copyBudget(budget), nil
}

func (s *InMemoryStore) Get(_ context.Context, workspaceID, userID, operationID string) (OperationBudget, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	budget, ok := s.budgets[operationID]
	if !ok || budget.WorkspaceID != workspaceID || budget.UserID != userID {
		return OperationBudget{}, ErrNotFound
	}
	return copyBudget(budget), nil
}

func (s *InMemoryStore) Revoke(_ context.Context, workspaceID, userID, operationID string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	budget, ok := s.budgets[operationID]
	if !ok || budget.WorkspaceID != workspaceID || budget.UserID != userID {
		return ErrNotFound
	}
	value := at.UTC()
	budget.RevokedAt = &value
	budget.UpdatedAt = value
	s.budgets[operationID] = copyBudget(budget)
	return nil
}

func (s *InMemoryStore) AddUsage(
	_ context.Context,
	workspaceID, userID, operationID string,
	delta UsageDelta,
	at time.Time,
) (OperationBudget, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	budget, ok := s.budgets[operationID]
	if !ok || budget.WorkspaceID != workspaceID || budget.UserID != userID {
		return OperationBudget{}, ErrNotFound
	}
	budget.UsedTotalTokens += delta.TotalTokens
	budget.InputTokens += delta.InputTokens
	budget.OutputTokens += delta.OutputTokens
	budget.ReasoningTokens += delta.ReasoningTokens
	budget.CacheReadTokens += delta.CacheReadTokens
	budget.CacheWriteTokens += delta.CacheWriteTokens
	budget.UsedCostUSDMicros += delta.CostUSDMicros
	budget.UpdatedAt = at.UTC()
	s.budgets[operationID] = copyBudget(budget)
	return copyBudget(budget), nil
}

func copyBudget(budget OperationBudget) OperationBudget {
	if budget.RevokedAt != nil {
		revokedAt := *budget.RevokedAt
		budget.RevokedAt = &revokedAt
	}
	return budget
}
