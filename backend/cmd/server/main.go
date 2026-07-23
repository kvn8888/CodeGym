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
	"github.com/kvn8888/codegym/backend/internal/execution"
	"github.com/kvn8888/codegym/backend/internal/generation"
	"github.com/kvn8888/codegym/backend/internal/generation/openaicompat"
	"github.com/kvn8888/codegym/backend/internal/identity"
	"github.com/kvn8888/codegym/backend/internal/memory"
	"github.com/kvn8888/codegym/backend/internal/session"
	"github.com/kvn8888/codegym/backend/internal/usage"
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
	var usageStore usage.Store = usage.NewInMemoryStore()
	var executionStore execution.Store = execution.NewInMemoryStore()

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
		postgresUsageStore := usage.NewPostgresStore(pool)
		postgresExecutionStore := execution.NewPostgresStore(pool)
		if err := postgresIdentityStore.EnsureSchema(ctx); err != nil {
			log.Fatalf("could not bootstrap identity schema: %v", err)
		}
		if err := postgresMemoryStore.EnsureSchema(ctx); err != nil {
			log.Fatalf("could not bootstrap memory schema: %v", err)
		}
		if err := postgresSessionStore.EnsureSchema(ctx); err != nil {
			log.Fatalf("could not bootstrap session schema: %v", err)
		}
		if err := postgresUsageStore.EnsureSchema(ctx); err != nil {
			log.Fatalf("could not bootstrap genai usage schema: %v", err)
		}
		if err := postgresExecutionStore.EnsureSchema(ctx); err != nil {
			log.Fatalf("could not bootstrap execution schema: %v", err)
		}

		identityStore = postgresIdentityStore
		memoryStore = postgresMemoryStore
		sessionStore = postgresSessionStore
		usageStore = postgresUsageStore
		executionStore = postgresExecutionStore
		log.Print("CodeGym API using Postgres identity, memory, session, genai usage, and execution stores")
	} else {
		log.Print("CodeGym API using in-memory identity, memory, session, genai usage, and execution stores; set NEON_CONNECTION_STRING to enable Postgres")
	}

	var executionRunner execution.Runner
	if cfg.DaytonaAPIKey != "" {
		daytonaRunner, err := execution.NewDaytonaRunner(cfg.DaytonaAPIKey, cfg.DaytonaAPIURL)
		if err != nil {
			log.Fatalf("could not configure Daytona runner: %v", err)
		}
		executionRunner = daytonaRunner
		log.Print("CodeGym API execution runner: Daytona")
	} else {
		log.Print("CodeGym API execution runner disabled; set DAYTONA_API_KEY to enable")
	}

	identityService := identity.NewService(identityStore)
	memoryService := memory.NewService(memoryStore, nil)
	sessionService := session.NewService(sessionStore, nil)
	usageService := usage.NewService(usageStore, nil)
	executionService := execution.NewService(executionStore, executionRunner, nil)

	var generationOrchestrator *generation.Orchestrator
	if cfg.AnyGenAIEnabled() {
		named := make([]generation.NamedGenerator, 0, len(cfg.GenAIProviders))
		names := make([]string, 0, len(cfg.GenAIProviders))
		for _, provider := range cfg.GenAIProviders {
			if warning := provider.PairingWarning(); warning != "" {
				log.Printf("WARNING: provider=%s %s base_url=%s model=%s",
					provider.Name, warning, provider.BaseURL, provider.Model)
			}
			adapter, err := openaicompat.New(openaicompat.Config{
				Name:             provider.Name,
				BaseURL:          provider.BaseURL,
				APIKey:           provider.APIKey,
				Model:            provider.Model,
				AuthStyle:        provider.AuthStyle,
				APIVersion:       provider.APIVersion,
				DefaultMaxTokens: provider.DefaultMaxTokens,
			})
			if err != nil {
				log.Fatalf("could not configure GenAI provider %s: %v", provider.Name, err)
			}
			named = append(named, generation.NamedGenerator{Name: provider.Name, Generator: adapter})
			names = append(names, provider.Name)
			log.Printf("CodeGym GenAI provider registered name=%s base_url=%s model=%s",
				provider.Name, provider.BaseURL, provider.Model)
		}
		routerGenerator, err := generation.NewRouter(named)
		if err != nil {
			log.Fatalf("could not configure GenAI router: %v", err)
		}
		generationOrchestrator = generation.NewOrchestrator(memoryService, routerGenerator).WithUsage(usageService)
		log.Printf("CodeGym generation enabled providers=%v order=%v", names, cfg.GenAIProviderOrder)
	} else {
		log.Print("CodeGym generation disabled; set META_MUSE_SPARK_API, CODEGYM_GENAI_AZURE_API_KEY, or CODEGYM_GEMINI_API_KEY to enable POST /api/v1/generate")
	}
	profileSynthesizer := generation.NewProfileSynthesizer(generationOrchestrator, memoryService)

	if cfg.MemoryWorker.DailyEnabled() {
		worker := memory.NewWorker(profileSynthesizer, cfg.MemoryWorker.Interval)
		go worker.Run(ctx)
		log.Printf("CodeGym LLM memory profile worker scheduled every %s trigger=%s", cfg.MemoryWorker.Interval, cfg.MemoryWorker.Trigger)
	} else {
		log.Printf("CodeGym daily memory worker disabled trigger=%s", cfg.MemoryWorker.Trigger)
	}

	router := api.NewRouter(api.Dependencies{
		Authenticator:        authenticator,
		Identity:             identityService,
		Memory:               memoryService,
		Sessions:             sessionService,
		Execution:            executionService,
		Generation:           generationOrchestrator,
		MemoryProfiles:       profileSynthesizer,
		MemoryRefreshTrigger: cfg.MemoryWorker.Trigger,
		Usage:                usageService,
		CORSAllowedOrigins:   cfg.CORSAllowedOrigins,
		DatabaseURL:          cfg.DatabaseURL,
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
		StaticToken:        cfg.DevAuthToken,
		DefaultUserID:      cfg.DevUserID,
		DefaultWorkspaceID: cfg.DevWorkspaceID,
	}), nil
}
