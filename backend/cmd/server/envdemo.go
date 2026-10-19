package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/kvn8888/codegym/backend/internal/agentrelay"
	"github.com/kvn8888/codegym/backend/internal/agentruntime"
	"github.com/kvn8888/codegym/backend/internal/config"
	"github.com/kvn8888/codegym/backend/internal/envintegration"
	"github.com/kvn8888/codegym/backend/internal/environmentregistry"
	"github.com/kvn8888/codegym/backend/internal/generation/openaicompat"
	"github.com/kvn8888/codegym/backend/internal/practice"
	"github.com/kvn8888/codegym/backend/internal/usage"
)

// Budgets for one cold environment build. A Spring Boot build used ~75k
// tokens; these leave room for repair-heavy runs while still bounding them.
const (
	envDemoMaxTotalTokens   = 2_000_000
	envDemoMaxCostUSDMicros = 20_000_000
	envDemoTurnCeiling      = 32
)

// envDemoOperations reports every relay operation active: each token is
// issued for exactly one build and expires with it.
type envDemoOperations struct{}

func (envDemoOperations) OperationActive(context.Context, string, string, string) (bool, error) {
	return true, nil
}

// buildEnvDemo assembles the host-local agent-built environment corridor
// (#192). The agent reaches the model through a loopback-only relay with
// per-build tokens, so provider keys never enter the agent's environment.
func buildEnvDemo(ctx context.Context, cfg config.Config, upstream *openaicompat.Adapter, saved practice.SavedExerciseStore, store environmentregistry.Store) (*envintegration.LocalDemo, error) {
	if upstream == nil {
		return nil, fmt.Errorf("a GenAI provider is required for agent builds")
	}
	// opencode runs from inside each build workspace, so relative paths
	// must be resolved against the server's working directory first.
	binary, err := filepath.Abs(cfg.OpenCodeBinary)
	if err != nil {
		return nil, err
	}
	template, err := filepath.Abs(cfg.OpenCodeTemplate)
	if err != nil {
		return nil, err
	}
	for _, path := range []string{binary, template} {
		if _, err := os.Stat(path); err != nil {
			return nil, fmt.Errorf("opencode runtime missing at %s (run spikes/opencode/download-opencode.sh): %w", path, err)
		}
	}
	secret := cfg.Relay.TokenSecret
	if secret == "" {
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			return nil, err
		}
		secret = hex.EncodeToString(raw)
	}
	tokens, err := agentrelay.NewService(agentrelay.NewInMemoryStore(), agentrelay.ServiceConfig{
		TokenSecret:             secret,
		TokenTTL:                cfg.EnvDemoTimeout + 5*time.Minute,
		DefaultMaxTotalTokens:   envDemoMaxTotalTokens,
		DefaultMaxCostUSDMicros: envDemoMaxCostUSDMicros,
		DefaultMaxWallClock:     cfg.EnvDemoTimeout,
		OperationChecker:        envDemoOperations{},
	})
	if err != nil {
		return nil, err
	}
	handler, err := agentrelay.NewHTTPHandler(tokens, upstream, usage.NewService(usage.NewInMemoryStore(), nil), cfg.Relay.PublicModel)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", handler.Models)
	mux.HandleFunc("POST /v1/chat/completions", handler.ChatCompletions)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	relayServer := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := relayServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("CodeGym environment demo relay stopped: %v", err)
		}
	}()
	go func() {
		<-ctx.Done()
		_ = relayServer.Close()
	}()
	relayURL := "http://" + listener.Addr().String() + "/v1"

	newRuntime := func(ctx context.Context) (agentruntime.AgentRuntime, func(), error) {
		raw := make([]byte, 8)
		if _, err := rand.Read(raw); err != nil {
			return nil, nil, err
		}
		issued, err := tokens.Issue(ctx, agentrelay.IssueInput{
			OperationID: "envbuild_" + hex.EncodeToString(raw),
			WorkspaceID: "env-demo",
			UserID:      "env-demo",
		})
		if err != nil {
			return nil, nil, err
		}
		runtime, err := agentruntime.NewOpenCodeRuntime(agentruntime.OpenCodeConfig{
			BinaryPath:      binary,
			RuntimeTemplate: template,
			RelayBaseURL:    relayURL,
			RelayToken:      issued.Token,
			Model:           cfg.Relay.PublicModel,
		})
		if err != nil {
			return nil, nil, err
		}
		return runtime, nil, nil
	}
	return envintegration.NewLocalDemo(envintegration.LocalDemoConfig{
		NewRuntime:  newRuntime,
		RuntimeName: "opencode",
		TurnCeiling: envDemoTurnCeiling,
		Timeout:     cfg.EnvDemoTimeout,
		Saved:       saved,
		Store:       store,
	})
}
