package envbuild

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kvn8888/codegym/backend/internal/agentruntime"
)

// repairDirective renders verifier diagnostics as an appended repair section
// for the fixed instruction template. Failure types never earn their own
// prompt variant; every repair speaks this one shape.
func repairDirective(diagnostics []Diagnostic) string {
	known := make([]Diagnostic, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		if strings.TrimSpace(diagnostic.Code) == "" && strings.TrimSpace(diagnostic.Message) == "" {
			continue
		}
		known = append(known, diagnostic)
	}
	if len(known) == 0 {
		return ""
	}
	var text strings.Builder
	text.WriteString("\n\nThe previous attempt failed verification. Repair the following and re-verify:\n")
	for _, diagnostic := range known {
		fmt.Fprintf(&text, "- [%s/%s] %s\n", diagnostic.Stage, diagnostic.Code, diagnostic.Message)
	}
	text.WriteString("Fix every item, rebuild, rerun the checks, and update the workspace files.")
	return text.String()
}

// RepairAttempt binds the repair loop's AttemptFunc to the real preparation
// path. Round zero is standard preparation; later rounds reuse the same
// workspace by identity, with prior verifier diagnostics appended to the
// instructions and the manifest re-drafted from the repaired files.
type RepairAttempt struct {
	Request     BuildRequest
	Runtime     agentruntime.AgentRuntime
	Provisioner WorkspaceProvisioner

	workspace    Workspace
	hasWorkspace bool
	runID        string
	history      []Diagnostic
}

func (a *RepairAttempt) freshPrepare(ctx context.Context) (BuildResult, error) {
	preparer := &Preparer{Runtime: a.Runtime, Provisioner: a.Provisioner}
	return preparer.Prepare(ctx, a.Request)
}

// Attempt performs one loop round. It never verifies and never promotes; it
// only prepares or repairs, and records the round on the shared history the
// final result carries.
func (a *RepairAttempt) Attempt(ctx context.Context, round int, feedback []Diagnostic) (BuildResult, error) {
	if a.Runtime == nil {
		return BuildResult{}, fmt.Errorf("envbuild: agent runtime is required")
	}
	if a.Provisioner == nil {
		return BuildResult{}, fmt.Errorf("envbuild: workspace provisioner is required")
	}
	if round == 0 || !a.hasWorkspace {
		result, err := a.freshPrepare(ctx)
		a.record(&result, feedback)
		if result.Workspace.Root != "" {
			a.workspace = result.Workspace
			a.hasWorkspace = true
		}
		if a.runID == "" {
			a.runID = result.BuilderRunID
		}
		result.BuilderRunID = a.runID
		return result, err
	}
	startedAt := time.Now()
	task := agentruntime.TaskSpec{
		Goal:             preparationInstructions(a.Request) + repairDirective(feedback),
		WorkingDirectory: a.workspace.Root,
		AllowedTools: []agentruntime.Tool{
			agentruntime.ToolReadFile,
			agentruntime.ToolWriteFile,
			agentruntime.ToolShell,
			agentruntime.ToolReportProgress,
		},
		TurnCeiling: prepareTurnCeiling,
		Deadline:    time.Now().Add(prepareTimeout),
	}
	runResult, err := a.Runtime.Run(ctx, task)
	result := BuildResult{
		BuilderRunID: a.runID,
		Workspace:    a.workspace,
		Telemetry:    mapTelemetry(runResult.Telemetry, time.Since(startedAt)),
	}
	if err != nil {
		result.Diagnostics = append(result.Diagnostics, terminationDiagnostic(runResult.Telemetry.Termination))
		a.record(&result, feedback)
		return result, err
	}
	if runResult.Telemetry.Termination != agentruntime.TerminationCompleted {
		result.Diagnostics = append(result.Diagnostics, terminationDiagnostic(runResult.Telemetry.Termination))
		a.record(&result, feedback)
		return result, fmt.Errorf("envbuild: repair ended: %s", runResult.Telemetry.Termination)
	}
	manifest, manifestDiagnostics, err := DraftManifest(a.Request, a.workspace.Root)
	result.Diagnostics = append(result.Diagnostics, manifestDiagnostics...)
	if err != nil {
		a.record(&result, feedback)
		return result, err
	}
	result.ProposedManifest = manifest
	a.record(&result, feedback)
	return result, nil
}

// record folds the round into the shared history: prior rounds, then the
// verifier feedback that caused this round, then this round's own
// diagnostics. The final result tells the whole story, not just the ending.
func (a *RepairAttempt) record(result *BuildResult, feedback []Diagnostic) {
	combined := append(append(append([]Diagnostic{}, a.history...), feedback...), result.Diagnostics...)
	result.Diagnostics = combined
	a.history = combined
}
