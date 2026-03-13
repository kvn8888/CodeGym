package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/kevinc/codegym/internal/api/handlers"
	"github.com/kevinc/codegym/internal/api/response"
	"github.com/kevinc/codegym/internal/store/problemstore"
)

func NewRouter(problemStore *problemstore.Store) http.Handler {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(corsMiddleware)

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		response.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// API v1
	r.Route("/api/v1", func(r chi.Router) {
		// Problems
		problems := handlers.NewProblemsHandler(problemStore)
		r.Route("/problems", func(r chi.Router) {
			r.Get("/", problems.List)
			r.Get("/{id}", problems.Get)
			r.Get("/{id}/skeleton", problems.GetSkeleton)
		})

		// Submissions (placeholder)
		r.Route("/submissions", func(r chi.Router) {
			r.Post("/", notImplemented)
			r.Get("/{id}", notImplemented)
		})

		// Generation (placeholder)
		r.Route("/generate", func(r chi.Router) {
			r.Post("/", notImplemented)
			r.Get("/{id}/status", notImplemented)
		})

		// User (placeholder)
		r.Route("/users", func(r chi.Router) {
			r.Get("/me", notImplemented)
			r.Get("/me/progress", notImplemented)
		})

		// Auth (placeholder)
		r.Route("/auth", func(r chi.Router) {
			r.Post("/login", notImplemented)
			r.Post("/register", notImplemented)
		})
	})

	return r
}

func notImplemented(w http.ResponseWriter, r *http.Request) {
	response.Err(w, http.StatusNotImplemented, "not_implemented", "this endpoint is not yet implemented")
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
