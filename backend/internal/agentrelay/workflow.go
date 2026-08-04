package agentrelay

import (
	"context"

	"github.com/kvn8888/codegym/backend/internal/workflow"
)

// IssueOperationToken adapts relay issuance to the workflow lifecycle hook.
func (s *Service) IssueOperationToken(ctx context.Context, operation workflow.Operation) (workflow.RelayAccess, error) {
	issued, err := s.Issue(ctx, IssueInput{
		OperationID: operation.ID, WorkspaceID: operation.WorkspaceID, UserID: operation.UserID,
		Deadline: operation.CreatedAt.UTC().Add(s.defaultMaxWallClock),
	})
	if err != nil {
		return workflow.RelayAccess{}, err
	}
	return workflow.RelayAccess{
		Token: issued.Token, ExpiresAt: issued.ExpiresAt, Deadline: issued.Budget.Deadline,
		MaxTotalTokens:   issued.Budget.MaxTotalTokens,
		MaxCostUSDMicros: issued.Budget.MaxCostUSDMicros,
	}, nil
}

// RevokeOperationToken adapts scoped relay revocation to terminal workflow
// transitions.
func (s *Service) RevokeOperationToken(ctx context.Context, operation workflow.Operation) error {
	return s.Revoke(ctx, operation.WorkspaceID, operation.UserID, operation.ID)
}
