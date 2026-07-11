package main

import (
	"context"
	"log"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kvn8888/codegym/backend/internal/api"
	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/config"
	"github.com/kvn8888/codegym/backend/internal/execution"
	"github.com/kvn8888/codegym/backend/internal/identity"
	"github.com/kvn8888/codegym/backend/internal/memory"
)

func main() {
	ctx := context.Background()
	cfg := config.Load()

	authenticator := auth.NewDevAuthenticator(auth.DevAuthenticatorConfig{
		StaticToken:     cfg.DevAuthToken,
		DefaultUserID:   cfg.DevUserID,
		DefaultTenantID: cfg.DevTenantID,
	})

	var memoryStore memory.Store = memory.NewInMemoryStore()
	var identityStore identity.Store = identity.NewInMemoryStore()
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
		postgresExecutionStore := execution.NewPostgresStore(pool)
		if err := postgresIdentityStore.EnsureSchema(ctx); err != nil {
			log.Fatalf("could not bootstrap identity schema: %v", err)
		}
		if err := postgresMemoryStore.EnsureSchema(ctx); err != nil {
			log.Fatalf("could not bootstrap memory schema: %v", err)
		}
		if err := postgresExecutionStore.EnsureSchema(ctx); err != nil {
			log.Fatalf("could not bootstrap execution schema: %v", err)
		}

		identityStore = postgresIdentityStore
		memoryStore = postgresMemoryStore
		executionStore = postgresExecutionStore
		log.Print("CodeGym API using Postgres identity, memory, and execution stores")
	} else {
		log.Print("CodeGym API using in-memory identity, memory, and execution stores; set NEON_CONNECTION_STRING to enable Postgres")
	}

	var runner execution.Runner
	if cfg.DaytonaAPIKey != "" {
		daytonaRunner, err := execution.NewDaytonaRunner(cfg.DaytonaAPIKey, cfg.DaytonaAPIURL)
		if err != nil {
			log.Fatalf("could not configure Daytona runner: %v", err)
		}
		runner = daytonaRunner
		log.Print("CodeGym API execution runner: Daytona")
	} else {
		log.Print("CodeGym API execution runner disabled; set DAYTONA_API_KEY to enable")
	}

	identityService := identity.NewService(identityStore)
	memoryService := memory.NewService(memoryStore, nil)
	executionService := execution.NewService(executionStore, runner, nil)

	router := api.NewRouter(api.Dependencies{
		Authenticator: authenticator,
		Identity:      identityService,
		Memory:        memoryService,
		Execution:     executionService,
	})

	log.Printf("CodeGym API listening on %s", cfg.Addr())
	if err := http.ListenAndServe(cfg.Addr(), router); err != nil {
		log.Fatal(err)
	}
}
