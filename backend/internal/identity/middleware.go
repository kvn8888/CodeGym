package identity

import (
	"net/http"

	"github.com/kvn8888/codegym/backend/internal/api/response"
	"github.com/kvn8888/codegym/backend/internal/auth"
)

// Middleware ensures the authenticated principal has a bootstrapped personal
// workspace before proceeding.
func Middleware(service *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, ok := auth.PrincipalFromContext(r.Context())
			if !ok {
				response.Error(w, http.StatusUnauthorized, "unauthenticated", "Missing authenticated principal.")
				return
			}

			if err := service.EnsurePersonalWorkspace(r.Context(), principal); err != nil {
				response.Error(w, http.StatusInternalServerError, "workspace_bootstrap_failed", "Could not bootstrap personal workspace.")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
