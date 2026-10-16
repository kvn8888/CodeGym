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
	// prepareOutputCap bounds the runtime's captured output for one
	// preparation. The runtime default (64 KiB) is sized for benchmark tasks;
	// opencode streams every tool result through it, so a single dependency
	// download (Maven, npm) exhausted it within seconds.
	prepareOutputCap = 16 << 20
)

// preparationInstructions is the single fixed instruction set for cold
// preparation. Technology and objective arrive only as interpolated data: a
// technology name in a branch, a prompt variant, or a template here is a
// defect against the generic path.
func preparationInstructions(request BuildRequest) string {
	return fmt.Sprintf(
		"Prepare a coding practice exercise for %s: %s. "+
			"In the working directory, create a complete, working project with source files, assessment tests, and a short learner-facing exercise statement in EXERCISE.md. "+
			"Build and test the project there until every assessment test passes. "+
			"Write the setup, build, test, and run commands you used as JSON object fields setup, build, test, and run in .codegym/commands.json, including the runtime base as field base; the run command must finish on its own rather than start a long-running server. "+
			"The run command must succeed in a fresh copy of the project with no prior command having run: if launching needs built artifacts, the run command must build them first instead of assuming they exist. "+
			"EXERCISE.md must state the complete contract the hidden tests grade: every class, interface, and method signature the learner must implement or preserve, and the exact expected behavior, so a learner who implements exactly what is written passes. "+
			"Then write the learner starter: for each source file the learner must complete, write a copy under .codegym/starter/ at the same relative path, with the code the exercise asks the learner to write replaced by minimal stubs that still compile, so the assessment tests fail on the starter. "+
			"Never edit the working project files while writing the starter. "+
			"Place hidden assessment tests that learners must never see under .codegym/hidden/. Each hidden test must print one verdict line per assertion to stdout in the form CODEGYM_TEST <id> PASS on success or CODEGYM_TEST <id> FAIL code=<code> expected=<expected> actual=<actual> on failure, with single whitespace-free tokens, in addition to failing the test command on a wrong solution so the verdict is visible both in the exit code and in the output. Use only the supplied tools.",
		request.Technology, request.Objective,
	)
}

// Preparer runs cold preparation: prerequisites, then provisioning, then the
// agent runtime. It returns the temporary workspace to the caller for the
// verifier handoff and never verifies or promotes.
type Preparer struct {
	Runtime     agentruntime.AgentRuntime
	Provisioner WorkspaceProvisioner
	// TurnCeiling and Timeout override the standing runaway guards for live
	// demonstrations with heavyweight toolchains. Zero values keep the
	// defaults; tests and fast stacks never set them.
	TurnCeiling int
	Timeout     time.Duration
	// OutputCapBytes overrides prepareOutputCap when positive.
	OutputCapBytes int
}

func (p *Preparer) outputCap() int {
	if p.OutputCapBytes > 0 {
		return p.OutputCapBytes
	}
	return prepareOutputCap
}

func (p *Preparer) turnCeiling() int {
	if p.TurnCeiling > 0 {
		return p.TurnCeiling
	}
	return prepareTurnCeiling
}

func (p *Preparer) timeout() time.Duration {
	if p.Timeout > 0 {
		return p.Timeout
	}
	return prepareTimeout
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
		TurnCeiling:    p.turnCeiling(),
		Deadline:       time.Now().Add(p.timeout()),
		OutputCapBytes: p.outputCap(),
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
	manifest, manifestDiagnostics, err := DraftManifest(request, workspace.Root)
	result.Diagnostics = append(result.Diagnostics, manifestDiagnostics...)
	if err != nil {
		return result, err
	}
	result.ProposedManifest = manifest
	completed = true
	return result, nil
}
