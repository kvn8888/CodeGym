package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/kvn8888/codegym/backend/internal/agentrelay"
	"github.com/kvn8888/codegym/backend/internal/agentruntime"
	"github.com/kvn8888/codegym/backend/internal/config"
	"github.com/kvn8888/codegym/backend/internal/envbuild"
	"github.com/kvn8888/codegym/backend/internal/envintegration"
	"github.com/kvn8888/codegym/backend/internal/environmentregistry"
	"github.com/kvn8888/codegym/backend/internal/practice"
	"github.com/kvn8888/codegym/backend/internal/workflow"
)

// demoTokenIssuer issues per-run demo tokens from a dedicated relay service
// with env-aware liveness checks and demo budgets.
type demoTokenIssuer struct {
	service *agentrelay.Service
}

func (i *demoTokenIssuer) IssueToken(ctx context.Context, operationID, workspaceID, userID string) (string, error) {
	issued, err := i.service.Issue(ctx, agentrelay.IssueInput{
		OperationID: operationID,
		WorkspaceID: workspaceID,
		UserID:      userID,
	})
	if err != nil {
		return "", err
	}
	return issued.Token, nil
}

type demoCapabilities struct{}

func (demoCapabilities) SupportsTechnology(string) bool { return true }

type demoQuota struct{}

func (demoQuota) CheckBuild(context.Context, string) error { return nil }

// configureEnvDemo assembles the demonstration environment pipeline: live
// agent builds through verification into browser practice. Every seam is
// real except Daytona snapshots, which retained workspace directories stand
// in for, documented as demo-grade. A nil store and nil service mean
// disabled; an error fails server startup closed with the missing piece
// named. Callers must not fall back to samples when this fails.
func configureEnvDemo(cfg config.Config, relayStore agentrelay.Store, workflowService *workflow.Service, saved practice.SavedExerciseStore) (*envintegration.OperationStore, *practice.Service, error) {
	if !cfg.EnvDemo {
		return nil, nil, nil
	}
	runtime, err := demoAgentRuntime(cfg, relayStore, workflowService)
	if err != nil {
		return nil, nil, err
	}
	assessments, err := envintegration.LoadAssessmentFile(cfg.AssessmentInputsFile)
	if err != nil {
		return nil, nil, err
	}
	preparer := &envbuild.Preparer{
		Runtime:     runtime,
		Provisioner: envbuild.HostProvisioner{},
		TurnCeiling: 32,
		Timeout:     40 * time.Minute,
	}
	dirs := &envintegration.SnapshotDirectories{}
	localRunner := &envintegration.LocalCommandRunner{}
	verifier := &envintegration.AssessmentResolvingVerifier{
		Inner: &envintegration.RegistryVerifier{
			Roots:  dirs.Resolve,
			Runner: localRunner,
		},
		Assessments: assessments,
	}
	registryService := &environmentregistry.Service{
		Store:          environmentregistry.NewInMemoryStore(),
		Snapshots:      &envintegration.LocalSnapshotProvider{Roots: dirs.Resolve, Runner: localRunner},
		Builder:        &envintegration.RegistryBuilder{Preparer: preparer, RuntimeName: "opencode", Snapshots: dirs},
		Capabilities:   demoCapabilities{},
		Quota:          demoQuota{},
		Verifier:       verifier,
		PracticePolicy: environmentregistry.NetworkPolicyIsolated,
	}
	contents := &envintegration.SnapshotContents{Roots: dirs.Resolve}
	practiceService := envintegration.ProductionPracticeService(
		&envintegration.RegistryArtifactSource{Store: registryService.Store, Contents: contents},
		contents,
		// The local runner materializes the sealed snapshot into each
		// execution space (multi-file projects need pom.xml and friends,
		// not just the learner overlay); production swaps in the Daytona
		// runner whose snapshots bake the tree into the sandbox image.
		&envintegration.PracticeLocalRunner{Snapshots: contents},
		practice.NewInMemoryRecordStore(),
		saved,
		0,
	)
	operations := &envintegration.OperationStore{}
	operations.Pipeline = (&envintegration.ProductionPipeline{Registry: registryService, Practice: practiceService}).Pipeline
	log.Print("CodeGym environment demo enabled: live agent builds through verification into browser practice (demo-grade local snapshots and open capabilities, never production)")
	return operations, practiceService, nil
}

func demoAgentRuntime(cfg config.Config, relayStore agentrelay.Store, workflowService *workflow.Service) (agentruntime.AgentRuntime, error) {
	if cfg.EnvDemoFixture {
		log.Print("CodeGym environment demo fixture: using pinned deterministic Spring Boot exercise (no model credentials required)")
		return envintegration.NewSpringBootFixtureRuntime(), nil
	}
	if strings.TrimSpace(cfg.Relay.TokenSecret) == "" {
		return nil, fmt.Errorf("CODEGYM_ENV_DEMO requires CODEGYM_RELAY_TOKEN_SECRET")
	}
	if strings.TrimSpace(cfg.OpenCodeBinary) == "" {
		return nil, fmt.Errorf("CODEGYM_ENV_DEMO requires CODEGYM_OPENCODE_BINARY")
	}
	if info, err := os.Stat(cfg.OpenCodeBinary); err != nil || info.Mode()&0o111 == 0 {
		return nil, fmt.Errorf("CODEGYM_ENV_DEMO: opencode binary is not executable: %s", cfg.OpenCodeBinary)
	}
	demoRelay, err := agentrelay.NewService(relayStore, agentrelay.ServiceConfig{
		TokenSecret:             cfg.Relay.TokenSecret,
		TokenTTL:                90 * time.Minute,
		DefaultMaxTotalTokens:   10_000_000,
		DefaultMaxCostUSDMicros: 25_000_000,
		DefaultMaxWallClock:     90 * time.Minute,
		OperationChecker:        &envintegration.EnvOperationChecker{Fallback: workflowService},
	})
	if err != nil {
		return nil, fmt.Errorf("configure demo relay tokens: %w", err)
	}
	relayURL := "http://127.0.0.1:" + strings.TrimSpace(cfg.Port) + "/api/v1/agent-relay/v1"
	runtime := &envintegration.DemoAgentRuntime{
		Issue:      &demoTokenIssuer{service: demoRelay},
		BinaryPath: cfg.OpenCodeBinary,
		RelayURL:   relayURL,
		Model:      cfg.Relay.PublicModel,
		Owner:      "demo",
	}
	return runtime, nil
}
