package workspace

import (
	"net/http"

	"github.com/kvn8888/codegym/backend/internal/api/response"
	"github.com/kvn8888/codegym/backend/internal/auth"
)

// HeaderWorkspaceID is the optional request header used to select workspace scope.
const HeaderWorkspaceID = "X-CodeGym-Workspace-ID"

// legacyHeaderWorkspaceID is accepted for one release so existing clients keep working.
const legacyHeaderWorkspaceID = "X-CodeGym-Tenant-ID"

// Middleware resolves personal-workspace scope and enforces workspace access control.
//
// Each request operates in exactly one workspace at a time. The workspace is determined by:
//  1. X-CodeGym-Workspace-ID header (or legacy X-CodeGym-Tenant-ID) if present and allowed
//  2. Principal.DefaultWorkspaceID when no header is provided
//
// All data operations (memory profile, events, sessions) are scoped to this workspace.
// If the requested workspace is not in Principal.WorkspaceIDs, the request is rejected
// with 403 Forbidden.
func Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, ok := auth.PrincipalFromContext(r.Context())
			if !ok {
				response.Error(w, http.StatusUnauthorized, "unauthenticated", "Missing authenticated principal.")
				return
			}

			workspaceID := r.Header.Get(HeaderWorkspaceID)
			if workspaceID == "" {
				workspaceID = r.Header.Get(legacyHeaderWorkspaceID)
			}
			if workspaceID == "" {
				workspaceID = principal.DefaultWorkspaceID
			}
			if workspaceID == "" || !principal.HasWorkspace(workspaceID) {
				response.Error(w, http.StatusForbidden, "workspace_forbidden", "User does not have access to this workspace.")
				return
			}

			next.ServeHTTP(w, r.WithContext(WithScope(r.Context(), Scope{WorkspaceID: workspaceID})))
		})
	}
}
