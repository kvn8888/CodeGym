package tenant

import (
	"net/http"

	"github.com/kvn8888/codegym/backend/internal/workspace"
)

// HeaderTenantID is a legacy alias for workspace.HeaderWorkspaceID support.
const HeaderTenantID = "X-CodeGym-Tenant-ID"

// Middleware is a legacy compatibility shim.
//
// Workspace naming is canonical. This function delegates to workspace.Middleware,
// which accepts both X-CodeGym-Workspace-ID and legacy X-CodeGym-Tenant-ID.
func Middleware() func(http.Handler) http.Handler {
	return workspace.Middleware()
}
