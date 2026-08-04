package workflow

import "context"

// OperationTokenLifecycle keeps workflow ownership independent from the relay
// implementation while allowing operation start/terminal transitions to issue
// and revoke sandbox credentials.
type OperationTokenLifecycle interface {
	IssueOperationToken(ctx context.Context, operation Operation) (RelayAccess, error)
	RevokeOperationToken(ctx context.Context, operation Operation) error
}

// OperationActive implements the relay's fail-closed operation checker without
// relying on browser authentication context.
func (s *Service) OperationActive(ctx context.Context, workspaceID, userID, operationID string) (bool, error) {
	if s == nil || s.store == nil {
		return false, ErrNotFound
	}
	operation, err := s.store.GetOperation(ctx, workspaceID, userID, operationID)
	if err != nil {
		return false, err
	}
	return operation.Status != StatusSucceeded && operation.Status != StatusFailed, nil
}
