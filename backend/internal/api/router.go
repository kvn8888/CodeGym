package api

import (
	"net/http"

	"github.com/kvn8888/codegym/backend/internal/api/handlers"
	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/identity"
	"github.com/kvn8888/codegym/backend/internal/memory"
	"github.com/kvn8888/codegym/backend/internal/tenant"
)

// Dependencies contains services and middleware inputs required to build the API router.
type Dependencies struct {
	Authenticator      auth.Authenticator
	Identity           *identity.Service
	Memory             *memory.Service
	CORSAllowedOrigins []string
	DatabaseURL        string
}

// NewRouter builds the top-level HTTP handler tree for public and protected
// endpoints.
//
// Public routes:
//   - GET /health
//   - GET /ready
//
// Protected routes under /api/v1 pass through auth, identity, and tenant
// middleware. CORS middleware is applied at the top level.
func NewRouter(deps Dependencies) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", handlers.Health)
	mux.HandleFunc("GET /ready", handlers.NewReadyHandler(deps.DatabaseURL))

	protected := http.NewServeMux()
	memoryHandler := handlers.NewMemoryHandler(deps.Memory)
	protected.HandleFunc("GET /api/v1/memory/profile", memoryHandler.Profile)
	protected.HandleFunc("POST /api/v1/memory/profile/refresh", memoryHandler.RefreshProfile)
	protected.HandleFunc("GET /api/v1/memory/events", memoryHandler.ListEvents)
	protected.HandleFunc("POST /api/v1/memory/events", memoryHandler.RecordEvent)

	protectedChain := chain(
		protected,
		auth.Middleware(deps.Authenticator),
		identity.Middleware(deps.Identity),
		tenant.Middleware(),
	)
	mux.Handle("/api/v1/", protectedChain)

	return chain(mux, CORSMiddleware(deps.CORSAllowedOrigins))
}

// chain wraps a handler with middleware in declaration order.
//
// For example, chain(h, m1, m2) looks like the following m1(m2(h)).
func chain(handler http.Handler, middleware ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middleware) - 1; i >= 0; i-- {
		handler = middleware[i](handler)
	}
	return handler
}
