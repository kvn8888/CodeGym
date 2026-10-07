package envbuild

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/kvn8888/codegym/backend/internal/agentruntime"
)

const (
	// prepareTurnCeiling and prepareTimeout are the shared structural runaway
	// guards. They bound runaway runs, not spending; spend ceilings stay off
	// per the recorded budget decision.
	prepareTurnCeiling = 16
	prepareTimeout     = 2 * time.Minute
)

// preparationInstructions is the single fixed instruction set for cold
// preparation. Technology and objective arrive only as interpolated data: a
// technology name in a branch, a prompt variant, or a template here is a
// defect against the generic path.
func preparationInstructions(request BuildRequest) string {
	return fmt.Sprintf(
		"Prepare a coding practice environment for %s: %s. Create project files, exercise files, and assessment tests in the working directory, then build and test the project there. Use only the supplied tools.",
		request.Technology, request.Objective,
	)
}

// Preparer runs cold preparation: prerequisites, then provisioning, then the
// agent runtime. It returns the temporary workspace to the caller for the
// verifier handoff and never verifies or promotes.
type Preparer struct {
	Runtime     agentruntime.AgentRuntime
	Provisioner WorkspaceProvisioner
}

func newBuilderRunID() string {
	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Sprintf("build_%d", time.Now().UnixNano())
	}
	return "build_" + hex.EncodeToString(raw)
}

func terminationDiagnostic(termination agentruntime.TerminationReason) Diagnostic {
	return Diagnostic{
		Severity: "error",
		Stage:    "prepare",
		Code:     strings.ToUpper(string(termination)),
		Message:  fmt.Sprintf("preparation ended before completion: %s", termination),
	}
}

func mapTelemetry(telemetry agentruntime.Telemetry, wallTime time.Duration) BuilderTelemetry {
	return BuilderTelemetry{
		WallTimeMs:       wallTime.Milliseconds(),
		TokensIn:         telemetry.Tokens.Input,
		TokensOut:        telemetry.Tokens.Output,
		EstimatedCostUsd: float64(telemetry.CostUSDMicros) / 1_000_000,
		RepairIterations: telemetry.RepairIterations,
	}
}

// Prepare runs one cold preparation and returns it to the verifier boundary.
// A missing prerequisite ends the run with diagnostics and zero agent spend.
// An agent failure records diagnostics, destroys the temporary workspace, and
// returns both the result and the error. A completed run returns the live
// workspace; the caller owns it past the verifier handoff.
func (p *Preparer) Prepare(ctx context.Context, request BuildRequest) (BuildResult, error) {
	startedAt := time.Now()
	result := BuildResult{BuilderRunID: newBuilderRunID()}
	if p == nil || p.Runtime == nil {
		return result, fmt.Errorf("envbuild: agent runtime is required")
	}
	if p.Provisioner == nil {
		return result, fmt.Errorf("envbuild: workspace provisioner is required")
	}
	prerequisites, err := CheckPrerequisites(ctx, request.Technology)
	if err != nil {
		return result, err
	}
	if missing := MissingRequired(prerequisites); missing.ID != "" {
		result.Diagnostics = append(result.Diagnostics, MissingPrerequisiteDiagnostic(missing))
		return result, nil
	}
	workspace, err := p.Provisioner.Provision(ctx, map[string]string{
		"technology": request.Technology,
		"objective":  request.Objective,
	})
	if err != nil {
		return result, err
	}
	result.Workspace = workspace
	completed := false
	defer func() {
		if !completed {
			_ = p.Provisioner.Destroy(ctx, workspace)
		}
	}()
	task := agentruntime.TaskSpec{
		Goal:             preparationInstructions(request),
		WorkingDirectory: workspace.Root,
		AllowedTools: []agentruntime.Tool{
			agentruntime.ToolReadFile,
			agentruntime.ToolWriteFile,
			agentruntime.ToolShell,
			agentruntime.ToolReportProgress,
		},
		TurnCeiling: prepareTurnCeiling,
		Deadline:    time.Now().Add(prepareTimeout),
	}
	runResult, err := p.Runtime.Run(ctx, task)
	result.Telemetry = mapTelemetry(runResult.Telemetry, time.Since(startedAt))
	if err != nil {
		result.Diagnostics = append(result.Diagnostics, terminationDiagnostic(runResult.Telemetry.Termination))
		return result, err
	}
	if runResult.Telemetry.Termination != agentruntime.TerminationCompleted {
		result.Diagnostics = append(result.Diagnostics, terminationDiagnostic(runResult.Telemetry.Termination))
		return result, fmt.Errorf("envbuild: preparation ended: %s", runResult.Telemetry.Termination)
	}
	completed = true
	return result, nil
}
