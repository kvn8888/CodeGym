package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/agentrelay"
	"github.com/kvn8888/codegym/backend/internal/config"
	"github.com/kvn8888/codegym/backend/internal/environmentregistry"
	"github.com/kvn8888/codegym/backend/internal/generation/openaicompat"
)

type stubChecker struct {
	active bool
	calls  []string
}

func (s *stubChecker) OperationActive(_ context.Context, workspaceID, userID, operationID string) (bool, error) {
	s.calls = append(s.calls, operationID)
	return s.active, nil
}

type stubRegistryStore struct{}

func (stubRegistryStore) SaveArtifact(context.Context, environmentregistry.ArtifactRecord) error {
	return nil
}
func (stubRegistryStore) ListByTechnology(context.Context, string) ([]environmentregistry.ArtifactRecord, error) {
	return nil, nil
}
func (stubRegistryStore) GetArtifact(context.Context, string, string) (environmentregistry.ArtifactRecord, error) {
	return environmentregistry.ArtifactRecord{}, nil
}
func (stubRegistryStore) UpdateValidation(context.Context, string, string, environmentregistry.ValidationState, time.Time) error {
	return nil
}

func tokenService(t *testing.T) *agentrelay.Service {
	t.Helper()
	service, err := agentrelay.NewService(agentrelay.NewInMemoryStore(), agentrelay.ServiceConfig{
		TokenSecret:             strings.Repeat("s", 32),
		TokenTTL:                time.Minute,
		DefaultMaxTotalTokens:   1,
		DefaultMaxCostUSDMicros: 1,
		DefaultMaxWallClock:     time.Minute,
		OperationChecker:        &stubChecker{active: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestEnvOperationGuardDefersWorkflowOperations(t *testing.T) {
	workflow := &stubChecker{active: true}
	guard := newEnvOperationGuard(workflow, time.Minute)
	if active, err := guard.OperationActive(t.Context(), "ws", "user", "operation-1"); err != nil || !active {
		t.Fatalf("workflow op = (%v, %v), want delegated active", active, err)
	}
	if len(workflow.calls) != 1 || workflow.calls[0] != "operation-1" {
		t.Fatalf("workflow calls = %#v", workflow.calls)
	}
}

func TestEnvOperationGuardAuthorizesOnlyAllowedBuilds(t *testing.T) {
	workflow := &stubChecker{active: true}
	guard := newEnvOperationGuard(workflow, time.Minute)
	op := envBuildOperationPrefix + "abcdef"

	if active, err := guard.OperationActive(t.Context(), "ws", "user", op); err != nil || active {
		t.Fatalf("unallowed build op = (%v, %v), want inactive", active, err)
	}
	guard.allow(op)
	if active, err := guard.OperationActive(t.Context(), "ws", "user", op); err != nil || !active {
		t.Fatalf("allowed build op = (%v, %v), want active", active, err)
	}
	if len(workflow.calls) != 0 {
		t.Fatalf("build ops must not reach the workflow checker: %#v", workflow.calls)
	}
}

func TestEnvOperationGuardExpiresAllowedBuilds(t *testing.T) {
	guard := newEnvOperationGuard(&stubChecker{}, time.Millisecond)
	op := envBuildOperationPrefix + "abcdef"
	guard.allow(op)
	time.Sleep(5 * time.Millisecond)
	if active, err := guard.OperationActive(t.Context(), "ws", "user", op); err != nil || active {
		t.Fatalf("expired build op = (%v, %v), want inactive", active, err)
	}
}

func TestBuildProductionEnvironmentRequiresEveryIngredient(t *testing.T) {
	base := config.Config{
		DaytonaAPIKey:       "key",
		DaytonaAPIURL:       "https://api.daytona.io",
		DaytonaBaseSnapshot: "codegym-base",
		EnvBuildTimeout:     time.Minute,
	}
	base.Relay.PublicBaseURL = "https://relay.example.com"

	cases := []struct {
		name     string
		mutate   func(*config.Config)
		upstream bool
		tokens   bool
		guard    bool
		want     string
	}{
		{name: "provider missing", mutate: func(*config.Config) {}, want: "GenAI provider"},
		{name: "relay missing", mutate: func(*config.Config) {}, upstream: true, want: "relay"},
		{name: "guard missing", mutate: func(*config.Config) {}, upstream: true, tokens: true, want: "operation guard"},
		{name: "daytona missing", mutate: func(c *config.Config) { c.DaytonaAPIKey = "" }, upstream: true, tokens: true, guard: true, want: "DAYTONA"},
		{name: "base snapshot missing", mutate: func(c *config.Config) { c.DaytonaBaseSnapshot = "" }, upstream: true, tokens: true, guard: true, want: "CODEGYM_BASE_SNAPSHOT"},
		{name: "relay url missing", mutate: func(c *config.Config) { c.Relay.PublicBaseURL = "" }, upstream: true, tokens: true, guard: true, want: "CODEGYM_RELAY_PUBLIC_URL"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base
			tc.mutate(&cfg)
			var upstream *openaicompat.Adapter
			if tc.upstream {
				upstream = &openaicompat.Adapter{}
			}
			var tokens *agentrelay.Service
			if tc.tokens {
				tokens = tokenService(t)
			}
			var guard *envOperationGuard
			if tc.guard {
				guard = newEnvOperationGuard(&stubChecker{}, time.Minute)
			}
			_, err := buildProductionEnvironment(cfg, upstream, tokens, guard, nil, stubRegistryStore{})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want mention of %q", err, tc.want)
			}
		})
	}
}
