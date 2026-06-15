package main

import (
	"log"
	"net/http"

	"github.com/kvn8888/codegym/backend/internal/api"
	"github.com/kvn8888/codegym/backend/internal/auth"
	"github.com/kvn8888/codegym/backend/internal/config"
	"github.com/kvn8888/codegym/backend/internal/memory"
)

func main() {
	cfg := config.Load()

	authenticator := auth.NewDevAuthenticator(auth.DevAuthenticatorConfig{
		StaticToken:     cfg.DevAuthToken,
		DefaultUserID:   cfg.DevUserID,
		DefaultTenantID: cfg.DevTenantID,
	})

	memoryStore := memory.NewInMemoryStore()
	memoryService := memory.NewService(memoryStore, nil)

	router := api.NewRouter(api.Dependencies{
		Authenticator: authenticator,
		Memory:        memoryService,
	})

	log.Printf("CodeGym API listening on %s", cfg.Addr())
	if err := http.ListenAndServe(cfg.Addr(), router); err != nil {
		log.Fatal(err)
	}
}
