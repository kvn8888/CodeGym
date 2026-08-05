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
	"github.com/kvn8888/codegym/backend/internal/agentrelay"
	"github.com/kvn8888/codegym/backend/internal/api"
	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/chat"
	"github.com/kvn8888/codegym/backend/internal/config"
	"github.com/kvn8888/codegym/backend/internal/execution"
	"github.com/kvn8888/codegym/backend/internal/generation"
	"github.com/kvn8888/codegym/backend/internal/generation/openaicompat"
	"github.com/kvn8888/codegym/backend/internal/identity"
	"github.com/kvn8888/codegym/backend/internal/intake"
	"github.com/kvn8888/codegym/backend/internal/memory"
	"github.com/kvn8888/codegym/backend/internal/problems"
	"github.com/kvn8888/codegym/backend/internal/session"
	"github.com/kvn8888/codegym/backend/internal/settings"
	"github.com/kvn8888/codegym/backend/internal/submission"
	"github.com/kvn8888/codegym/backend/internal/usage"
	"github.com/kvn8888/codegym/backend/internal/workflow"
)

// main wires configuration, persistence adapters, services, and the HTTP router,
// then starts the API server.
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("could not load config: %v", err)
	}
	log.Printf("CodeGym environment resolved environment=%s", cfg.Environment)

	authenticator, err := buildAuthenticator(cfg)
	if err != nil {
		log.Fatalf("could not configure auth: %v", err)
	}

	var memoryStore memory.Store = memory.NewInMemoryStore()
	var identityStore identity.Store = identity.NewInMemoryStore()
	var sessionStore session.Store = session.NewInMemoryStore()
	var usageStore usage.Store = usage.NewInMemoryStore()
	var executionStore execution.Store = execution.NewInMemoryStore()
	var problemStore problems.Store = problems.NewInMemoryStore()
	var intakeStore intake.Store = intake.NewInMemoryStore()
	var chatStore chat.Store = chat.NewInMemoryStore()
	var workflowStore workflow.Store = workflow.NewInMemoryStore()
	var relayStore agentrelay.Store = agentrelay.NewInMemoryStore()
	var settingsStore settings.Store = settings.NewInMemoryStore()

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
		postgresProblemStore := problems.NewPostgresStore(pool)
		postgresIntakeStore := intake.NewPostgresStore(pool)
		postgresChatStore := chat.NewPostgresStore(pool)
		postgresWorkflowStore := workflow.NewPostgresStore(pool)
		postgresRelayStore := agentrelay.NewPostgresStore(pool)
		postgresSettingsStore := settings.NewPostgresStore(pool)
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
		if err := postgresProblemStore.EnsureSchema(ctx); err != nil {
			log.Fatalf("could not bootstrap problem schema: %v", err)
		}
		if err := postgresIntakeStore.EnsureSchema(ctx); err != nil {
			log.Fatalf("could not bootstrap practice intake schema: %v", err)
		}
		if err := postgresChatStore.EnsureSchema(ctx); err != nil {
			log.Fatalf("could not bootstrap chat schema: %v", err)
		}
		if err := postgresWorkflowStore.EnsureSchema(ctx); err != nil {
			log.Fatalf("could not bootstrap workflow progress schema: %v", err)
		}
		if err := postgresRelayStore.EnsureSchema(ctx); err != nil {
			log.Fatalf("could not bootstrap agent relay schema: %v", err)
		}
		if err := postgresSettingsStore.EnsureSchema(ctx); err != nil {
			log.Fatalf("could not bootstrap runtime settings schema: %v", err)
		}

		identityStore = postgresIdentityStore
		memoryStore = postgresMemoryStore
		sessionStore = postgresSessionStore
		usageStore = postgresUsageStore
		executionStore = postgresExecutionStore
		problemStore = postgresProblemStore
		intakeStore = postgresIntakeStore
		chatStore = postgresChatStore
		workflowStore = postgresWorkflowStore
		relayStore = postgresRelayStore
		settingsStore = postgresSettingsStore
		log.Print("CodeGym API using Postgres identity, memory, session, chat, workflow, relay, runtime settings, genai usage, execution, and problem stores")
	} else {
		log.Print("CodeGym API using in-memory identity, memory, session, workflow, relay, runtime settings, genai usage, execution, and problem stores; set NEON_CONNECTION_STRING to enable Postgres")
	}

	settingsService := settings.NewService(settingsStore, nil)
	var executionRunner execution.Runner
	if cfg.DaytonaAPIKey != "" {
		daytonaRunner, err := execution.NewDaytonaRunner(cfg.DaytonaAPIKey, cfg.DaytonaAPIURL, cfg.Environment)
		if err != nil {
			log.Fatalf("could not configure Daytona runner: %v", err)
		}
		executionRunner = daytonaRunner.WithHedgeCountProvider(settingsService)
		log.Print("CodeGym API execution runner: Daytona")
		if !cfg.SandboxSweeper.Disabled {
			sweeper := execution.NewSweeper(
				daytonaRunner,
				cfg.SandboxSweeper.MaxAge,
				cfg.SandboxSweeper.Interval,
				nil,
			)
			go sweeper.Run(ctx)
			log.Printf("CodeGym Daytona orphan sweeper scheduled every %s max_age=%s environment=%s",
				cfg.SandboxSweeper.Interval, cfg.SandboxSweeper.MaxAge, cfg.Environment)
		} else {
			log.Print("CodeGym Daytona orphan sweeper disabled")
		}
	} else {
		log.Print("CodeGym API execution runner disabled; set DAYTONA_API_KEY to enable")
	}

	identityService := identity.NewService(identityStore)
	memoryService := memory.NewService(memoryStore, nil)
	sessionService := session.NewService(sessionStore, nil)
	usageService := usage.NewService(usageStore, nil)
	executionService := execution.NewService(executionStore, executionRunner, nil)
	problemService := problems.NewService(problemStore)
	if cfg.SeedDemo {
		if err := problemService.EnsureSeed(ctx); err != nil {
			log.Fatalf("could not seed demo problem: %v", err)
		}
		log.Print("CodeGym demo problem seeded: two-sum")
	}
	var generationOrchestrator *generation.Orchestrator
	var generationStreamer generation.Streamer
	var relayUpstream *openaicompat.Adapter
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
			if relayUpstream == nil {
				relayUpstream = adapter
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
		generationStreamer = generationOrchestrator
		log.Printf("CodeGym generation enabled providers=%v order=%v", names, cfg.GenAIProviderOrder)
	} else {
		log.Print("CodeGym generation disabled; set META_MUSE_SPARK_API, CODEGYM_GENAI_AZURE_API_KEY, or CODEGYM_GEMINI_API_KEY to enable POST /api/v1/generate")
	}
	profileSynthesizer := generation.NewProfileSynthesizer(generationOrchestrator, memoryService)
	workflowService := workflow.NewService(workflowStore, nil)
	var relayHTTPHandler *agentrelay.HTTPHandler
	if cfg.Relay.Enabled() {
		if relayUpstream == nil {
			log.Print("CodeGym agent relay disabled: token secret is set but no GenAI provider is configured")
		} else {
			relayService, err := agentrelay.NewService(relayStore, agentrelay.ServiceConfig{
				Environment: cfg.Environment,
				TokenSecret: cfg.Relay.TokenSecret, TokenTTL: cfg.Relay.TokenTTL,
				DefaultMaxTotalTokens:   cfg.Relay.MaxTotalTokens,
				DefaultMaxCostUSDMicros: cfg.Relay.MaxCostUSDMicros,
				DefaultMaxWallClock:     cfg.Relay.MaxWallClock,
				OperationChecker:        workflowService,
			})
			if err != nil {
				log.Fatalf("could not configure agent relay tokens: %v", err)
			}
			workflowService.WithOperationTokens(relayService)
			relayHTTPHandler, err = agentrelay.NewHTTPHandler(
				relayService, relayUpstream, usageService, cfg.Relay.PublicModel,
			)
			if err != nil {
				log.Fatalf("could not configure agent relay HTTP handler: %v", err)
			}
			log.Printf("CodeGym agent relay enabled provider=%s public_model=%s token_ttl=%s max_tokens=%d max_cost_usd=%.6f max_wall_clock=%s",
				relayUpstream.RelayProvider(), cfg.Relay.PublicModel, cfg.Relay.TokenTTL,
				cfg.Relay.MaxTotalTokens, usage.MicrosToUSD(cfg.Relay.MaxCostUSDMicros),
				cfg.Relay.MaxWallClock)
		}
	} else {
		log.Print("CodeGym agent relay disabled; set CODEGYM_RELAY_TOKEN_SECRET to enable")
	}
	intakeService := intake.NewService(intakeStore, memoryService, generationOrchestrator, nil)
	submissionService := submission.NewService(problemService, executionService, sessionService, memoryService, profileSynthesizer)
	chatService := chat.NewService(
		chatStore,
		sessionService,
		problemService,
		memoryService,
		generationStreamer,
		generationOrchestrator,
		profileSynthesizer,
		nil,
	)

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
		ExecutionRunner:      executionRunner,
		Problems:             problemService,
		Submissions:          submissionService,
		Intakes:              intakeService,
		Chat:                 chatService,
		Workflow:             workflowService,
		Generation:           generationOrchestrator,
		MemoryProfiles:       profileSynthesizer,
		MemoryRefreshTrigger: cfg.MemoryWorker.Trigger,
		Usage:                usageService,
		Settings:             settingsService,
		CORSAllowedOrigins:   cfg.CORSAllowedOrigins,
		DatabaseURL:          cfg.DatabaseURL,
		Environment:          cfg.Environment,
		AgentRelay:           relayHTTPHandler,
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
