package tenant

import (
	"net/http"

	"github.com/kvn8888/codegym/backend/internal/api/response"
	"github.com/kvn8888/codegym/backend/internal/auth"
)

// HeaderTenantID is the optional request header used to select tenant scope.
const HeaderTenantID = "X-CodeGym-Tenant-ID"

// Middleware resolves tenant (workspace) scope and enforces tenant access control.
//
// Each request operates in exactly one tenant at a time. The tenant is determined by:
//  1. X-CodeGym-Tenant-ID header (if present and user has access)
//  2. Principal.DefaultTenantID (if header not provided)
//
// All data operations (memory profile, events) are scoped to this tenant.
// A request to tenant A will only see that tenant's memory; same user in tenant B operates on separate data.
//
// This isolation enables multi-workspace support:
// a user in their personal workspace sees only personal memory, but can switch to a team workspace (via header) and see shared team memory instead.
//
// If the requested tenant is not in the user's Principal.TenantIDs, the request is rejected with 403 Forbidden.
func Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, ok := auth.PrincipalFromContext(r.Context())
			if !ok {
				response.Error(w, http.StatusUnauthorized, "unauthenticated", "Missing authenticated principal.")
				return
			}

			tenantID := r.Header.Get(HeaderTenantID)
			if tenantID == "" {
				tenantID = principal.DefaultTenantID
			}
			if tenantID == "" || !principal.HasTenant(tenantID) {
				response.Error(w, http.StatusForbidden, "tenant_forbidden", "User does not have access to this tenant.")
				return
			}

			next.ServeHTTP(w, r.WithContext(WithScope(r.Context(), Scope{TenantID: tenantID})))
		})
	}
}
