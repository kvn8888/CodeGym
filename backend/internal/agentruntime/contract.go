// Package agentruntime defines the runtime-neutral boundary for coding agents.
package agentruntime

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const (
	ManifestVersion      = 1
	ManifestRelativePath = ".codegym/agent-result.json"
	DefaultOutputCap     = 64 * 1024
)

type Tool string

const (
	ToolReadFile       Tool = "read_file"
	ToolWriteFile      Tool = "write_file"
	ToolShell          Tool = "shell"
	ToolReportProgress Tool = "report_progress"
)

var supportedTools = []Tool{ToolReadFile, ToolWriteFile, ToolShell, ToolReportProgress}

type TaskSpec struct {
	Goal             string
	WorkingDirectory string
	AllowedTools     []Tool
	TurnCeiling      int
	Deadline         time.Time
	OutputCapBytes   int
}

func (s TaskSpec) Validate(now time.Time) error {
	if strings.TrimSpace(s.Goal) == "" {
		return errors.New("agentruntime: goal is required")
	}
	workingDirectory := strings.TrimSpace(s.WorkingDirectory)
	if workingDirectory == "" || !filepath.IsAbs(workingDirectory) {
		return errors.New("agentruntime: working directory must be absolute")
	}
	if s.TurnCeiling <= 0 {
		return errors.New("agentruntime: turn ceiling must be positive")
	}
	if s.Deadline.IsZero() || !s.Deadline.After(now) {
		return errors.New("agentruntime: deadline must be in the future")
	}
	if s.OutputCapBytes < 0 {
		return errors.New("agentruntime: output cap must not be negative")
	}
	seen := make(map[Tool]struct{}, len(s.AllowedTools))
	for _, tool := range s.AllowedTools {
		if !slices.Contains(supportedTools, tool) {
			return fmt.Errorf("agentruntime: unsupported tool %q", tool)
		}
		if _, duplicate := seen[tool]; duplicate {
			return fmt.Errorf("agentruntime: duplicate tool %q", tool)
		}
		seen[tool] = struct{}{}
	}
	return nil
}

func (s TaskSpec) EffectiveOutputCap() int {
	if s.OutputCapBytes > 0 {
		return s.OutputCapBytes
	}
	return DefaultOutputCap
}

type AgentRuntime interface {
	Name() string
	Run(ctx context.Context, task TaskSpec) (RunResult, error)
}

type TerminationReason string

const (
	TerminationCompleted      TerminationReason = "completed"
	TerminationCancelled      TerminationReason = "cancelled"
	TerminationDeadline       TerminationReason = "deadline_exhausted"
	TerminationTurnCeiling    TerminationReason = "turn_ceiling_exhausted"
	TerminationTokenBudget    TerminationReason = "token_budget_exhausted"
	TerminationCostBudget     TerminationReason = "cost_budget_exhausted"
	TerminationOutputCeiling  TerminationReason = "output_ceiling_exhausted"
	TerminationRuntimeFailure TerminationReason = "runtime_failure"
)

type TokenUsage struct {
	Total      int64 `json:"total"`
	Input      int64 `json:"input"`
	Output     int64 `json:"output"`
	Reasoning  int64 `json:"reasoning"`
	CacheRead  int64 `json:"cache_read"`
	CacheWrite int64 `json:"cache_write"`
}

type ProgressEvent struct {
	StepID   string         `json:"step_id"`
	Label    string         `json:"label"`
	Metadata map[string]any `json:"metadata,omitempty"`
	At       time.Time      `json:"at"`
}

type ToolInvocation struct {
	Tool      Tool          `json:"tool"`
	StartedAt time.Time     `json:"started_at"`
	Duration  time.Duration `json:"duration"`
	ExitCode  *int          `json:"exit_code,omitempty"`
	Truncated bool          `json:"truncated,omitempty"`
	Error     string        `json:"error,omitempty"`
}

type Telemetry struct {
	Runtime          string            `json:"runtime"`
	RuntimeVersion   string            `json:"runtime_version,omitempty"`
	Turns            int               `json:"turns"`
	RepairIterations int               `json:"repair_iterations"`
	Tokens           TokenUsage        `json:"tokens"`
	CostUSDMicros    int64             `json:"cost_usd_micros"`
	WallTime         time.Duration     `json:"wall_time"`
	SandboxSeconds   float64           `json:"sandbox_seconds"`
	ExitCode         *int              `json:"exit_code,omitempty"`
	Termination      TerminationReason `json:"termination"`
	ToolInvocations  []ToolInvocation  `json:"tool_invocations,omitempty"`
	Progress         []ProgressEvent   `json:"progress,omitempty"`
	PolicyViolations []string          `json:"policy_violations,omitempty"`
	OutputTruncated  bool              `json:"output_truncated,omitempty"`
}

type ManifestCheck struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail,omitempty"`
}

// ResultManifest is an untrusted claim made by an agent. Completed and Checks
// are telemetry only; neither field authorizes promotion or marks a run passed.
type ResultManifest struct {
	Version   int             `json:"version"`
	Completed bool            `json:"completed"`
	Summary   string          `json:"summary,omitempty"`
	Artifacts []string        `json:"artifacts,omitempty"`
	Checks    []ManifestCheck `json:"checks,omitempty"`
}

type ManifestStatus string

const (
	ManifestPresent   ManifestStatus = "present"
	ManifestAbsent    ManifestStatus = "absent"
	ManifestMalformed ManifestStatus = "malformed"
)

type ManifestClaim struct {
	Status   ManifestStatus  `json:"status"`
	Manifest *ResultManifest `json:"manifest,omitempty"`
	Error    string          `json:"error,omitempty"`
}

type RunResult struct {
	Telemetry Telemetry     `json:"telemetry"`
	Manifest  ManifestClaim `json:"manifest_claim"`
	Prose     string        `json:"prose,omitempty"`
}

type Verification struct {
	Passed           bool     `json:"passed"`
	Detail           string   `json:"detail,omitempty"`
	PolicyViolations []string `json:"policy_violations,omitempty"`
}

type Verifier interface {
	Verify(ctx context.Context, task TaskSpec, result RunResult) (Verification, error)
}

type VerifierFunc func(ctx context.Context, task TaskSpec, result RunResult) (Verification, error)

func (f VerifierFunc) Verify(ctx context.Context, task TaskSpec, result RunResult) (Verification, error) {
	return f(ctx, task, result)
}

// Evaluate delegates the verdict exclusively to backend verifier code. It does
// not inspect the runtime exit code, agent prose, or the manifest's Completed
// claim when deciding success.
func Evaluate(ctx context.Context, verifier Verifier, task TaskSpec, result RunResult) (Verification, error) {
	if verifier == nil {
		return Verification{}, errors.New("agentruntime: verifier is required")
	}
	return verifier.Verify(ctx, task, result)
}
