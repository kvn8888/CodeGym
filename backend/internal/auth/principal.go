package auth

// Principal describes the authenticated user and the workspaces they can access.
//
// A Principal is an authenticated identity (UserID) plus the set of workspaces
// that user may use. Each workspace is a persistent data partition for memory
// profiles, events, sessions, and other scoped data.
//
// Most users have a single personal workspace created on first auth.
// DefaultWorkspaceID is used when the request does not select a workspace
// via X-CodeGym-Workspace-ID. HasWorkspace validates access.
type Principal struct {
	UserID             string       `json:"user_id"`
	DefaultWorkspaceID string       `json:"default_workspace_id"`
	WorkspaceIDs       []string     `json:"workspace_ids"`
	UserMetadata       UserMetadata `json:"user_metadata,omitempty"`
}

type UserMetadata struct {
	Email       string `json:"email,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
}

// HasWorkspace reports whether the principal is allowed to access workspaceID.
func (p Principal) HasWorkspace(workspaceID string) bool {
	for _, candidate := range p.WorkspaceIDs {
		if candidate == workspaceID {
			return true
		}
	}
	return false
}
