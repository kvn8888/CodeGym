package api

import (
	"net/http"

	"github.com/kvn8888/codegym/backend/internal/api/handlers"
	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/execution"
	"github.com/kvn8888/codegym/backend/internal/identity"
	"github.com/kvn8888/codegym/backend/internal/memory"
	"github.com/kvn8888/codegym/backend/internal/tenant"
)

type Dependencies struct {
	Authenticator auth.Authenticator
	Identity      *identity.Service
	Memory        *memory.Service
	Execution     *execution.Service
}

func NewRouter(deps Dependencies) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handlers.Health)

	protected := http.NewServeMux()
	memoryHandler := handlers.NewMemoryHandler(deps.Memory)
	protected.HandleFunc("GET /api/v1/memory/profile", memoryHandler.Profile)
	protected.HandleFunc("GET /api/v1/memory/events", memoryHandler.ListEvents)
	protected.HandleFunc("POST /api/v1/memory/events", memoryHandler.RecordEvent)

	executionHandler := handlers.NewExecutionHandler(deps.Execution)
	protected.HandleFunc("POST /api/v1/executions", executionHandler.Submit)
	protected.HandleFunc("GET /api/v1/executions", executionHandler.List)
	protected.HandleFunc("GET /api/v1/executions/{id}", executionHandler.Get)

	protectedChain := chain(
		protected,
		auth.Middleware(deps.Authenticator),
		identity.Middleware(deps.Identity),
		tenant.Middleware(),
	)
	mux.Handle("/api/v1/", protectedChain)

	return mux
}

func chain(handler http.Handler, middleware ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middleware) - 1; i >= 0; i-- {
		handler = middleware[i](handler)
	}
	return handler
}
