package api

import (
	"net/http"

	"github.com/kvn8888/codegym/backend/internal/agentrelay"
	"github.com/kvn8888/codegym/backend/internal/api/handlers"
	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/chat"
	"github.com/kvn8888/codegym/backend/internal/execution"
	"github.com/kvn8888/codegym/backend/internal/generation"
	"github.com/kvn8888/codegym/backend/internal/identity"
	"github.com/kvn8888/codegym/backend/internal/intake"
	"github.com/kvn8888/codegym/backend/internal/memory"
	"github.com/kvn8888/codegym/backend/internal/problems"
	"github.com/kvn8888/codegym/backend/internal/session"
	"github.com/kvn8888/codegym/backend/internal/settings"
	"github.com/kvn8888/codegym/backend/internal/submission"
	"github.com/kvn8888/codegym/backend/internal/usage"
	"github.com/kvn8888/codegym/backend/internal/workflow"
	"github.com/kvn8888/codegym/backend/internal/workspace"
)

// Dependencies contains services and middleware inputs required to build the API router.
type Dependencies struct {
	Authenticator   auth.Authenticator
	Identity        *identity.Service
	Memory          *memory.Service
	Sessions        *session.Service
	Execution       *execution.Service
	ExecutionRunner execution.Runner
	Problems        *problems.Service
	Submissions     *submission.Service
	Intakes         *intake.Service
	Chat            *chat.Service
	Workflow        *workflow.Service
	// Generation is nil when no GenAI provider is configured; the generate
	// route stays registered and answers 503 so clients can fall back.
	Generation           *generation.Orchestrator
	MemoryProfiles       *generation.ProfileSynthesizer
	MemoryRefreshTrigger string
	Usage                *usage.Service
	Settings             *settings.Service
	CORSAllowedOrigins   []string
	DatabaseURL          string
	AgentRelay           *agentrelay.HTTPHandler
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
	// The sandbox-facing relay is deliberately outside the Auth0/identity/
	// workspace chain. It authenticates short-lived operation tokens itself.
	mux.HandleFunc("GET /api/v1/agent-relay/v1/models", func(w http.ResponseWriter, r *http.Request) {
		deps.AgentRelay.Models(w, r)
	})
	mux.HandleFunc("POST /api/v1/agent-relay/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		deps.AgentRelay.ChatCompletions(w, r)
	})

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
	intakeHandler := handlers.NewIntakeHandler(deps.Intakes)
	protected.HandleFunc("POST /api/v1/practice-intakes", intakeHandler.Prepare)
	protected.HandleFunc("GET /api/v1/practice-intakes", intakeHandler.List)
	protected.HandleFunc("PATCH /api/v1/practice-intakes/{id}", intakeHandler.Update)
	generateHandler := handlers.NewGenerateHandler(deps.Generation, deps.Memory, profiles, refreshOnSetCompletion, deps.Intakes, deps.Problems, deps.Sessions, deps.ExecutionRunner, deps.Workflow)
	protected.HandleFunc("POST /api/v1/generate", generateHandler.Generate)
	protected.HandleFunc("POST /api/v1/mcq/evaluate", generateHandler.EvaluateFreeResponse)
	protected.HandleFunc("POST /api/v1/memory/profile/maintain", generateHandler.MaintainProfile)
	// Compatibility alias for clients deployed before full-profile synthesis.
	protected.HandleFunc("POST /api/v1/memory/notes/maintain", generateHandler.MaintainProfile)
	costHandler := handlers.NewCostHandler(deps.Usage)
	protected.HandleFunc("GET /api/v1/cost", costHandler.Cost)
	settingsHandler := handlers.NewSettingsHandler(deps.Settings)
	protected.HandleFunc("GET /api/v1/settings/{key}", settingsHandler.Get)
	protected.HandleFunc("PUT /api/v1/settings/{key}", settingsHandler.Put)
	executionHandler := handlers.NewExecutionHandler(deps.Execution)
	protected.HandleFunc("POST /api/v1/executions", executionHandler.Submit)
	protected.HandleFunc("GET /api/v1/executions", executionHandler.List)
	protected.HandleFunc("GET /api/v1/executions/{id}", executionHandler.Get)
	problemHandler := handlers.NewProblemHandler(deps.Problems)
	protected.HandleFunc("GET /api/v1/problems", problemHandler.List)
	protected.HandleFunc("GET /api/v1/problems/{id}", problemHandler.Get)
	protected.HandleFunc("GET /api/v1/problems/{id}/skeleton", problemHandler.Skeleton)
	submissionHandler := handlers.NewSubmissionHandler(deps.Submissions)
	protected.HandleFunc("POST /api/v1/submissions", submissionHandler.Submit)
	protected.HandleFunc("GET /api/v1/submissions/{id}", submissionHandler.Get)
	chatHandler := handlers.NewChatHandler(deps.Chat)
	protected.HandleFunc("POST /api/v1/chat/threads", chatHandler.CreateThread)
	protected.HandleFunc("GET /api/v1/chat/threads", chatHandler.ListThreads)
	protected.HandleFunc("GET /api/v1/chat/threads/{id}/messages", chatHandler.Messages)
	protected.HandleFunc("POST /api/v1/chat/threads/{id}/turns", chatHandler.Turn)
	protected.HandleFunc("POST /api/v1/chat/threads/{id}/reset", chatHandler.Reset)
	protected.HandleFunc("POST /api/v1/chat/threads/{id}/finish", chatHandler.Finish)
	protected.HandleFunc("POST /api/v1/chat/threads/{id}/exit", chatHandler.Exit)
	workflowHandler := handlers.NewWorkflowHandler(deps.Workflow)
	protected.HandleFunc("POST /api/v1/workflow-operations", workflowHandler.Create)
	protected.HandleFunc("GET /api/v1/workflow-operations/{id}/events", workflowHandler.Events)
	protected.HandleFunc("POST /api/v1/workflow-operations/{id}/cancel", workflowHandler.Cancel)

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
