package api

import (
	"net/http"

	"github.com/kvn8888/codegym/backend/internal/api/handlers"
	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/generation"
	"github.com/kvn8888/codegym/backend/internal/identity"
	"github.com/kvn8888/codegym/backend/internal/memory"
	"github.com/kvn8888/codegym/backend/internal/session"
	"github.com/kvn8888/codegym/backend/internal/usage"
	"github.com/kvn8888/codegym/backend/internal/workspace"
)

// Dependencies contains services and middleware inputs required to build the API router.
type Dependencies struct {
	Authenticator auth.Authenticator
	Identity      *identity.Service
	Memory        *memory.Service
	Sessions      *session.Service
	// Generation is nil when no GenAI provider is configured; the generate
	// route stays registered and answers 503 so clients can fall back.
	Generation           *generation.Orchestrator
	MemoryProfiles       *generation.ProfileSynthesizer
	MemoryRefreshTrigger string
	Usage                *usage.Service
	CORSAllowedOrigins   []string
	DatabaseURL          string
}

// NewRouter builds the top-level HTTP handler tree for public and protected
// endpoints.
//
// Public routes:
//   - GET /health
//   - GET /ready
//
// Protected routes under /api/v1 pass through auth, identity, and workspace
// middleware. CORS middleware is applied at the top level.
func NewRouter(deps Dependencies) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", handlers.Health)
	mux.HandleFunc("GET /ready", handlers.NewReadyHandler(deps.DatabaseURL))

	protected := http.NewServeMux()
	profileHandler := handlers.NewProfileHandler(deps.Identity)
	protected.HandleFunc("GET /api/v1/me", profileHandler.Me)
	protected.HandleFunc("PATCH /api/v1/me", profileHandler.UpdateMe)
	profiles := deps.MemoryProfiles
	if profiles == nil {
		profiles = generation.NewProfileSynthesizer(deps.Generation, deps.Memory)
	}
	refreshOnSetCompletion := deps.MemoryRefreshTrigger == "" ||
		deps.MemoryRefreshTrigger == "both" || deps.MemoryRefreshTrigger == "set-completion"
	memoryHandler := handlers.NewMemoryHandler(deps.Memory, profiles)
	protected.HandleFunc("GET /api/v1/memory/profile", memoryHandler.Profile)
	protected.HandleFunc("POST /api/v1/memory/profile/refresh", memoryHandler.RefreshProfile)
	protected.HandleFunc("GET /api/v1/memory/events", memoryHandler.ListEvents)
	protected.HandleFunc("POST /api/v1/memory/events", memoryHandler.RecordEvent)
	sessionHandler := handlers.NewSessionHandler(deps.Sessions)
	protected.HandleFunc("GET /api/v1/sessions", sessionHandler.List)
	protected.HandleFunc("POST /api/v1/sessions", sessionHandler.Create)
	protected.HandleFunc("GET /api/v1/sessions/{id}", sessionHandler.Get)
	protected.HandleFunc("PATCH /api/v1/sessions/{id}", sessionHandler.Patch)
	protected.HandleFunc("PUT /api/v1/sessions/{id}/files", sessionHandler.UpsertFiles)
	generateHandler := handlers.NewGenerateHandler(deps.Generation, deps.Memory, profiles, refreshOnSetCompletion)
	protected.HandleFunc("POST /api/v1/generate", generateHandler.Generate)
	protected.HandleFunc("POST /api/v1/memory/profile/maintain", generateHandler.MaintainProfile)
	// Compatibility alias for clients deployed before full-profile synthesis.
	protected.HandleFunc("POST /api/v1/memory/notes/maintain", generateHandler.MaintainProfile)
	costHandler := handlers.NewCostHandler(deps.Usage)
	protected.HandleFunc("GET /api/v1/cost", costHandler.Cost)

	protectedChain := chain(
		protected,
		auth.Middleware(deps.Authenticator),
		identity.Middleware(deps.Identity),
		workspace.Middleware(),
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
