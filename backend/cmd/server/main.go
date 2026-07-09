package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kvn8888/codegym/backend/internal/api"
	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/config"
	"github.com/kvn8888/codegym/backend/internal/generation"
	"github.com/kvn8888/codegym/backend/internal/generation/openaicompat"
	"github.com/kvn8888/codegym/backend/internal/identity"
	"github.com/kvn8888/codegym/backend/internal/memory"
	"github.com/kvn8888/codegym/backend/internal/session"
)

// main wires configuration, persistence adapters, services, and the HTTP router,
// then starts the API server.
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()

	authenticator, err := buildAuthenticator(cfg)
	if err != nil {
		log.Fatalf("could not configure auth: %v", err)
	}

	var memoryStore memory.Store = memory.NewInMemoryStore()
	var identityStore identity.Store = identity.NewInMemoryStore()
	var sessionStore session.Store = session.NewInMemoryStore()

	if cfg.DatabaseURL != "" {
		pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
		if err != nil {
			log.Fatalf("could not configure Postgres pool: %v", err)
		}
		defer pool.Close()

		if err := pool.Ping(ctx); err != nil {
			log.Fatalf("could not connect to Postgres: %v", err)
		}

		postgresIdentityStore := identity.NewPostgresStore(pool)
		postgresMemoryStore := memory.NewPostgresStore(pool)
		postgresSessionStore := session.NewPostgresStore(pool)
		if err := postgresIdentityStore.EnsureSchema(ctx); err != nil {
			log.Fatalf("could not bootstrap identity schema: %v", err)
		}
		if err := postgresMemoryStore.EnsureSchema(ctx); err != nil {
			log.Fatalf("could not bootstrap memory schema: %v", err)
		}
		if err := postgresSessionStore.EnsureSchema(ctx); err != nil {
			log.Fatalf("could not bootstrap session schema: %v", err)
		}

		identityStore = postgresIdentityStore
		memoryStore = postgresMemoryStore
		sessionStore = postgresSessionStore
		log.Print("CodeGym API using Postgres identity, memory, and session stores")
	} else {
		log.Print("CodeGym API using in-memory identity, memory, and session stores; set NEON_CONNECTION_STRING to enable Postgres")
	}

	identityService := identity.NewService(identityStore)
	memoryService := memory.NewService(memoryStore, nil)
	sessionService := session.NewService(sessionStore, nil)

	var generationOrchestrator *generation.Orchestrator
	if cfg.GenAI.Enabled() {
		if warning := cfg.GenAI.PairingWarning(); warning != "" {
			log.Printf("WARNING: %s base_url=%s model=%s", warning, cfg.GenAI.BaseURL, cfg.GenAI.Model)
		}
		generator, err := openaicompat.New(openaicompat.Config{
			BaseURL: cfg.GenAI.BaseURL,
			APIKey:  cfg.GenAI.APIKey,
			Model:   cfg.GenAI.Model,
		})
		if err != nil {
			log.Fatalf("could not configure GenAI adapter: %v", err)
		}
		generationOrchestrator = generation.NewOrchestrator(memoryService, generator)
		log.Printf("CodeGym generation enabled via %s (model %s)", cfg.GenAI.BaseURL, cfg.GenAI.Model)
	} else {
		log.Print("CodeGym generation disabled; set CODEGYM_GENAI_API_KEY to enable POST /api/v1/generate")
	}

	if !cfg.MemoryWorker.Disabled {
		worker := memory.NewWorker(memoryService, cfg.MemoryWorker.Interval)
		go worker.Run(ctx)
		log.Printf("CodeGym memory worker scheduled every %s", cfg.MemoryWorker.Interval)
	} else {
		log.Print("CodeGym memory worker disabled")
	}

	router := api.NewRouter(api.Dependencies{
		Authenticator:      authenticator,
		Identity:           identityService,
		Memory:             memoryService,
		Sessions:           sessionService,
		Generation:         generationOrchestrator,
		CORSAllowedOrigins: cfg.CORSAllowedOrigins,
		DatabaseURL:        cfg.DatabaseURL,
	})

	log.Printf("CodeGym API listening on %s", cfg.Addr())
	server := &http.Server{
		Addr:              cfg.Addr(),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("CodeGym API graceful shutdown failed: %v", err)
		}
	}()

	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func buildAuthenticator(cfg config.Config) (auth.Authenticator, error) {
	if cfg.UseAuth0() {
		log.Print("CodeGym API using Auth0 bearer token authentication")
		return auth.NewAuth0Authenticator(auth.Auth0AuthenticatorConfig{
			Domain:           cfg.Auth0Domain,
			IssuerURL:        cfg.Auth0IssuerURL,
			Audience:         cfg.Auth0Audience,
			AllowedClockSkew: cfg.Auth0ClockSkew,
		})
	}

	log.Print("CodeGym API using dev bearer token authentication")
	return auth.NewDevAuthenticator(auth.DevAuthenticatorConfig{
		StaticToken:     cfg.DevAuthToken,
		DefaultUserID:   cfg.DevUserID,
		DefaultTenantID: cfg.DevTenantID,
	}), nil
}
