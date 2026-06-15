package tenant

import (
	"net/http"

	"github.com/kvn8888/codegym/backend/internal/api/response"
	"github.com/kvn8888/codegym/backend/internal/auth"
)

const HeaderTenantID = "X-CodeGym-Tenant-ID"

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
