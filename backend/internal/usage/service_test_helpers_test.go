package usage

import (
	"context"
	"testing"

	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/workspace"
)

func testUsageContext(t *testing.T) context.Context {
	t.Helper()
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{
		UserID:             "user-1",
		DefaultWorkspaceID: "ws-1",
		WorkspaceIDs:       []string{"ws-1"},
	})
	return workspace.WithScope(ctx, workspace.Scope{WorkspaceID: "ws-1"})
}
