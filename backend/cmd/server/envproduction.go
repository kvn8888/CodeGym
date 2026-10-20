package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/kvn8888/codegym/backend/internal/agentrelay"
	"github.com/kvn8888/codegym/backend/internal/agentruntime"
	"github.com/kvn8888/codegym/backend/internal/config"
	"github.com/kvn8888/codegym/backend/internal/envintegration"
	"github.com/kvn8888/codegym/backend/internal/environmentregistry"
	"github.com/kvn8888/codegym/backend/internal/generation/openaicompat"
	"github.com/kvn8888/codegym/backend/internal/practice"
)

// This is the production environment-build corridor (#192). Every step runs
// off-host in a Daytona sandbox: the agent builds there, verification runs
// there, and practice sessions lease sandboxes from the promoted snapshot.
// The host never executes generated code, so the host-local corridor in
// envlocal.go exists only for development and CI.

// environmentCorridor is the assembled environment-build feature: the
// operation store the HTTP layer drives, plus the practice service that
// serves the sessions it opens.
type environmentCorridor struct {
	Operations *envintegration.OperationStore
	Practice   *practice.Service
}

// envBuildOperationPrefix marks operation IDs this server authorizes for
// environment builds. Only the server issues these, and only through
// envOperationGuard.allow, so the prefix alone never authorizes anything.
const envBuildOperationPrefix = "envbuild_"

// envOperationGuard is the relay's operation checker for a server that also
// runs environment builds. Environment-build operations are identified by
// their prefix and must be explicitly allowed by the build corridor; every
// other operation is decided by the workflow checker, exactly as before.
type envOperationGuard struct {
	workflow agentrelay.OperationChecker
	// ttl bounds how long an allowed build operation stays authorizable.
	ttl time.Duration

	mu     sync.Mutex
	active map[string]time.Time
}

func newEnvOperationGuard(workflow agentrelay.OperationChecker, ttl time.Duration) *envOperationGuard {
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	return &envOperationGuard{workflow: workflow, ttl: ttl, active: map[string]time.Time{}}
}

// OperationActive authorizes an environment-build operation only while the
// build corridor holds it open, and defers everything else to the workflow.
func (g *envOperationGuard) OperationActive(ctx context.Context, workspaceID, userID, operationID string) (bool, error) {
	if !isEnvBuildOperation(operationID) {
		if g.workflow == nil {
			return false, nil
		}
		return g.workflow.OperationActive(ctx, workspaceID, userID, operationID)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	until, ok := g.active[operationID]
	if !ok {
		return false, nil
	}
	if time.Now().After(until) {
		delete(g.active, operationID)
		return false, nil
	}
	return true, nil
}

// allow opens one environment-build operation for the guard's TTL.
func (g *envOperationGuard) allow(operationID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.active[operationID] = time.Now().Add(g.ttl)
}

func isEnvBuildOperation(operationID string) bool {
	return len(operationID) > len(envBuildOperationPrefix) &&
		operationID[:len(envBuildOperationPrefix)] == envBuildOperationPrefix
}

// buildProductionEnvironment assembles the Daytona-backed corridor. Every
// ingredient is required: without sandbox credentials, a base snapshot, a
// sandbox-reachable relay, and a durable store there is no way to build,
// judge, or record an environment off-host.
func buildProductionEnvironment(
	cfg config.Config,
	upstream *openaicompat.Adapter,
	tokens *agentrelay.Service,
	guard *envOperationGuard,
	saved practice.SavedExerciseStore,
	store environmentregistry.Store,
) (*environmentCorridor, error) {
	switch {
	case upstream == nil:
		return nil, fmt.Errorf("a GenAI provider is required for agent builds")
	case tokens == nil:
		return nil, fmt.Errorf("the agent relay is required for agent builds")
	case guard == nil:
		return nil, fmt.Errorf("the operation guard is required for agent builds")
	case store == nil:
		return nil, fmt.Errorf("the artifact store is required for agent builds")
	case cfg.DaytonaAPIKey == "" || cfg.DaytonaAPIURL == "":
		return nil, fmt.Errorf("DAYTONA_API_KEY and DAYTONA_API_URL are required")
	case cfg.DaytonaBaseSnapshot == "":
		return nil, fmt.Errorf("CODEGYM_BASE_SNAPSHOT is required (run spikes/daytona/base-snapshot)")
	case cfg.Relay.SandboxBaseURL() == "":
		return nil, fmt.Errorf("CODEGYM_RELAY_PUBLIC_URL is required so build sandboxes can reach the relay")
	}

	client, err := envintegration.NewDaytonaClient(cfg.DaytonaAPIKey, cfg.DaytonaAPIURL)
	if err != nil {
		return nil, err
	}
	workspaceDir := cfg.DaytonaWorkspaceDir
	if workspaceDir == "" {
		workspaceDir = envintegration.DefaultSandboxWorkspaceDir
	}
	builds := &envintegration.DaytonaBuilds{}

	// Each build gets its own runtime and its own relay token, so provider
	// keys never enter a sandbox and one build can never spend another's
	// budget.
	newRuntime := func(ctx context.Context, sandbox agentruntime.SandboxExecutor) (agentruntime.AgentRuntime, func(), error) {
		raw := make([]byte, 8)
		if _, err := rand.Read(raw); err != nil {
			return nil, nil, err
		}
		operationID := envBuildOperationPrefix + hex.EncodeToString(raw)
		issued, err := tokens.Issue(ctx, agentrelay.IssueInput{
			OperationID: operationID,
			WorkspaceID: "environment-build",
			UserID:      "environment-build",
		})
		if err != nil {
			return nil, nil, err
		}
		guard.allow(operationID)
		runtime, err := agentruntime.NewSandboxRuntime(agentruntime.SandboxConfig{
			Executor:     sandbox,
			BinaryPath:   "/opt/codegym/bin/opencode",
			ConfigDir:    "/home/agent/.config/opencode",
			HomeDir:      "/home/agent",
			ScratchDir:   "/home/agent/.codegym-scratch",
			RelayBaseURL: cfg.Relay.SandboxBaseURL(),
			RelayToken:   issued.Token,
			Model:        cfg.Relay.PublicModel,
			ManifestDir:  os.TempDir(),
		})
		if err != nil {
			return nil, nil, err
		}
		return runtime, nil, nil
	}

	builder := &envintegration.DaytonaBuilder{
		Client:       client,
		BaseSnapshot: cfg.DaytonaBaseSnapshot,
		WorkspaceDir: workspaceDir,
		NewRuntime:   newRuntime,
		TurnCeiling:  envBuildTurnCeiling,
		Timeout:      cfg.EnvBuildTimeout,
		Builds:       builds,
	}
	verifier := &envintegration.DaytonaVerifier{
		Client:      client,
		Builds:      builds,
		Assessments: &envintegration.StarterAssessmentSource{},
	}

	registryService := &environmentregistry.Service{
		Store:          store,
		Snapshots:      &envintegration.DaytonaSnapshotProvider{Client: client},
		Builder:        builder,
		Capabilities:   envintegration.AllTechnologies{},
		Quota:          envintegration.NoBuildQuota{},
		Verifier:       verifier,
		PracticePolicy: environmentregistry.NetworkPolicyIsolated,
	}

	// Sealed snapshot contents are read by booting the artifact, and learner
	// practice commands run in sandboxes leased from the same snapshot, so
	// both halves of practice are off-host.
	sandboxFactory := &envintegration.DaytonaSandboxFactory{Client: client, Ephemeral: true}
	remoteContents := &envintegration.SandboxContents{
		Factory:      sandboxFactory,
		WorkspaceDir: workspaceDir,
	}
	practiceService := envintegration.ProductionPracticeService(
		&envintegration.RegistryArtifactSource{Store: store, Contents: remoteContents},
		&envintegration.StarterContents{Inner: remoteContents},
		&envintegration.DaytonaPracticeRunner{Factory: sandboxFactory},
		practice.NewInMemoryRecordStore(),
		saved,
		cfg.EnvBuildTimeout,
	)

	pipeline := &envintegration.ProductionPipeline{Registry: registryService, Practice: practiceService}
	return &environmentCorridor{
		Operations: &envintegration.OperationStore{Pipeline: pipeline.Pipeline},
		Practice:   practiceService,
	}, nil
}

// logProductionEnvironmentState records why the production corridor is or is
// not active, so a misconfigured deployment is diagnosable from one line.
func logProductionEnvironmentState(cfg config.Config, active bool, err error) {
	if active {
		log.Printf("CodeGym environment builds: Daytona corridor active (base snapshot %s, build timeout %s)",
			cfg.DaytonaBaseSnapshot, cfg.EnvBuildTimeout)
		return
	}
	if cfg.ProductionEnvironment() {
		log.Fatalf("CodeGym environment builds unavailable in production: %v", err)
	}
	log.Printf("CodeGym environment builds disabled: %v", err)
}
