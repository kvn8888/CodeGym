package auth

// Principal describes the authenticated user and the tenant workspaces they can access.
//
// A Principal represents an authenticated identity (UserID) along with the set of
// workspaces (tenants) that user is a member of. Each tenant is a persistent data
// partition that holds that user's memory profile, events, and other scoped data.
//
// Most users will have a single personal tenant created automatically on first auth.
// The multi-tenant design supports future team/org workspaces: a user can become a
// member of multiple tenants (e.g., personal-workspace AND team-data-science), and
// switch context via X-CodeGym-Tenant-ID header to operate in different workspaces.
// This architecture enables shared team memory, collaborative problem tracking, and
// multi-workspace subscription models down the road.
//
// DefaultTenantID is the workspace used when no explicit tenant is specified in the request.
// HasTenant(tenantID) validates that the user is allowed to access a given tenant.
type Principal struct {
	UserID          string       `json:"user_id"`
	DefaultTenantID string       `json:"default_tenant_id"`
	TenantIDs       []string     `json:"tenant_ids"`
	UserMetadata    UserMetadata `json:"user_metadata,omitempty"`
}

type UserMetadata struct {
	Email       string `json:"email,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
}

// HasTenant reports whether the principal is allowed to access tenantID.
func (p Principal) HasTenant(tenantID string) bool {
	for _, candidate := range p.TenantIDs {
		if candidate == tenantID {
			return true
		}
	}
	return false
}
