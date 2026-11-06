package agentruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const BenchmarkReportVersion = 1

const PredeclaredDecisionRule = "Any policy violation disqualifies that runtime. Completion rate is primary. On equal completion, opencode wins only if it improves mean repair iterations or median wall time by at least 20 percent while using no more total tokens or cost; otherwise purpose-built wins. Inconclusive selects purpose-built."

type UsageSnapshot struct {
	Tokens        TokenUsage
	CostUSDMicros int64
}

type UsageMeter func(ctx context.Context) (UsageSnapshot, error)

type SandboxMeter func() (created, deleted int)

type BenchmarkConfig struct {
	PurposeBuilt          AgentRuntime
	OpenCode              AgentRuntime
	Fixtures              []BenchmarkFixture
	Repetitions           int
	TurnCeiling           int
	RunDeadline           time.Duration
	OutputCapBytes        int
	WorkingRoot           string
	ReportPath            string
	ModelDeployment       string
	ExecutionEnvironment  string
	Limitations           []string
	RelayMaxTokens        int64
	RelayMaxCostUSDMicros int64
	RelayWallClock        time.Duration
	RetryPolicy           string
	MaxSandboxes          int
	UsageMeter            UsageMeter
	SandboxMeter          SandboxMeter
}

type BenchmarkRun struct {
	Order            int               `json:"order"`
	Runtime          string            `json:"runtime"`
	Fixture          string            `json:"fixture"`
	Repetition       int               `json:"repetition"`
	Passed           bool              `json:"passed"`
	Detail           string            `json:"detail,omitempty"`
	Termination      TerminationReason `json:"termination"`
	ManifestStatus   ManifestStatus    `json:"manifest_status"`
	RepairIterations int               `json:"repair_iterations"`
	Tokens           TokenUsage        `json:"tokens"`
	CostUSDMicros    int64             `json:"cost_usd_micros"`
	WallTimeMS       int64             `json:"wall_time_ms"`
	SandboxSeconds   float64           `json:"sandbox_seconds"`
	PolicyViolations []string          `json:"policy_violations,omitempty"`
}

type RuntimeBenchmarkSummary struct {
	Runtime              string     `json:"runtime"`
	Runs                 int        `json:"runs"`
	Passes               int        `json:"passes"`
	CompletionRate       float64    `json:"completion_rate"`
	MeanRepairIterations float64    `json:"mean_repair_iterations"`
	MedianWallTimeMS     int64      `json:"median_wall_time_ms"`
	Tokens               TokenUsage `json:"tokens"`
	CostUSDMicros        int64      `json:"cost_usd_micros"`
	PolicyViolationCount int        `json:"policy_violation_count"`
	Disqualified         bool       `json:"disqualified"`
}

type BenchmarkDecision struct {
	Selected string `json:"selected"`
	Outcome  string `json:"outcome"`
	Reason   string `json:"reason"`
}

type BenchmarkReport struct {
	Version               int                       `json:"version"`
	StartedAt             time.Time                 `json:"started_at"`
	FinishedAt            time.Time                 `json:"finished_at"`
	Status                string                    `json:"status"`
	DecisionRule          string                    `json:"decision_rule"`
	ModelDeployment       string                    `json:"model_deployment"`
	ExecutionEnvironment  string                    `json:"execution_environment"`
	Limitations           []string                  `json:"limitations,omitempty"`
	RelayMaxTokens        int64                     `json:"relay_max_tokens"`
	RelayMaxCostUSDMicros int64                     `json:"relay_max_cost_usd_micros"`
	RelayWallClockMS      int64                     `json:"relay_wall_clock_ms"`
	TurnCeiling           int                       `json:"turn_ceiling"`
	RunDeadlineMS         int64                     `json:"run_deadline_ms"`
	OutputCapBytes        int                       `json:"output_cap_bytes"`
	RetryPolicy           string                    `json:"retry_policy"`
	FixtureFingerprints   map[string]string         `json:"fixture_fingerprints"`
	Runs                  []BenchmarkRun            `json:"runs"`
	Summaries             []RuntimeBenchmarkSummary `json:"summaries"`
	Decision              BenchmarkDecision         `json:"decision"`
	SandboxesCreated      int                       `json:"sandboxes_created"`
	SandboxesDeleted      int                       `json:"sandboxes_deleted"`
	WorkspacesCreated     int                       `json:"workspaces_created"`
	WorkspacesDeleted     int                       `json:"workspaces_deleted"`
	StoppedReason         string                    `json:"stopped_reason,omitempty"`
}

func RunBenchmark(ctx context.Context, config BenchmarkConfig) (BenchmarkReport, error) {
	if config.PurposeBuilt == nil || config.OpenCode == nil {
		return BenchmarkReport{}, errors.New("agentruntime: both benchmark runtimes are required")
	}
	if config.PurposeBuilt.Name() != "purpose-built" || config.OpenCode.Name() != "opencode" {
		return BenchmarkReport{}, errors.New("agentruntime: benchmark runtime names must be purpose-built and opencode")
	}
	if len(config.Fixtures) == 0 {
		config.Fixtures = BenchmarkFixtures()
	}
	if config.Repetitions <= 0 {
		config.Repetitions = 3
	}
	if config.TurnCeiling <= 1 || config.RunDeadline <= 0 || config.OutputCapBytes <= 0 {
		return BenchmarkReport{}, errors.New("agentruntime: benchmark turn, deadline, and output ceilings must be positive")
	}
	if config.WorkingRoot == "" {
		config.WorkingRoot = "/private/tmp"
	}
	if config.MaxSandboxes <= 0 {
		config.MaxSandboxes = 40
	}
	if config.RetryPolicy == "" {
		config.RetryPolicy = "zero retries; every failure counts"
	}
	if config.ExecutionEnvironment == "" {
		config.ExecutionEnvironment = "unspecified"
	}
	report := BenchmarkReport{
		Version: BenchmarkReportVersion, StartedAt: time.Now().UTC(), Status: "running",
		DecisionRule: PredeclaredDecisionRule, ModelDeployment: config.ModelDeployment,
		ExecutionEnvironment: config.ExecutionEnvironment, Limitations: append([]string(nil), config.Limitations...),
		RelayMaxTokens: config.RelayMaxTokens, RelayMaxCostUSDMicros: config.RelayMaxCostUSDMicros,
		RelayWallClockMS: config.RelayWallClock.Milliseconds(), TurnCeiling: config.TurnCeiling,
		RunDeadlineMS: config.RunDeadline.Milliseconds(), OutputCapBytes: config.OutputCapBytes,
		RetryPolicy: config.RetryPolicy, FixtureFingerprints: map[string]string{}, Runs: []BenchmarkRun{},
	}
	for _, fixture := range config.Fixtures {
		report.FixtureFingerprints[fixture.ID] = fixtureFingerprint(fixture)
	}

	runtimes := map[string]AgentRuntime{"purpose-built": config.PurposeBuilt, "opencode": config.OpenCode}
	order := 0
	for repetition := 1; repetition <= config.Repetitions; repetition++ {
		for fixtureIndex, fixture := range config.Fixtures {
			runtimeOrder := []string{"purpose-built", "opencode"}
			pairIndex := (repetition-1)*len(config.Fixtures) + fixtureIndex
			if pairIndex%2 == 1 {
				runtimeOrder = []string{"opencode", "purpose-built"}
			}
			for _, runtimeName := range runtimeOrder {
				if reason := benchmarkStopReason(ctx, config, report); reason != "" {
					report.Status = "stopped"
					report.StoppedReason = reason
					return finishBenchmarkReport(report, config)
				}
				order++
				workingDirectory, err := os.MkdirTemp(config.WorkingRoot, "codegym-agent-benchmark-")
				if err != nil {
					return report, fmt.Errorf("agentruntime: create benchmark workspace: %w", err)
				}
				report.WorkspacesCreated++
				if err := fixture.MaterializeBase(workingDirectory); err != nil {
					_ = os.RemoveAll(workingDirectory)
					report.WorkspacesDeleted++
					return report, err
				}
				before, meterErr := readUsageMeter(ctx, config.UsageMeter)
				if meterErr != nil {
					_ = os.RemoveAll(workingDirectory)
					report.WorkspacesDeleted++
					return report, meterErr
				}
				task := TaskSpec{
					Goal: fixture.TaskPrompt, WorkingDirectory: workingDirectory,
					AllowedTools: []Tool{ToolReadFile, ToolWriteFile, ToolShell, ToolReportProgress},
					TurnCeiling:  config.TurnCeiling, Deadline: time.Now().Add(config.RunDeadline),
					OutputCapBytes: config.OutputCapBytes,
				}
				runResult, runErr := runtimes[runtimeName].Run(ctx, task)
				after, meterErr := readUsageMeter(context.WithoutCancel(ctx), config.UsageMeter)
				if meterErr != nil {
					_ = os.RemoveAll(workingDirectory)
					report.WorkspacesDeleted++
					return report, meterErr
				}
				if config.UsageMeter != nil {
					runResult.Telemetry.Tokens = subtractTokens(after.Tokens, before.Tokens)
					runResult.Telemetry.CostUSDMicros = maxInt64(0, after.CostUSDMicros-before.CostUSDMicros)
				}
				benchmarkRun := BenchmarkRun{
					Order: order, Runtime: runtimeName, Fixture: fixture.ID, Repetition: repetition,
					Termination: runResult.Telemetry.Termination, ManifestStatus: runResult.Manifest.Status,
					RepairIterations: runResult.Telemetry.RepairIterations, Tokens: runResult.Telemetry.Tokens,
					CostUSDMicros: runResult.Telemetry.CostUSDMicros,
					WallTimeMS:    runResult.Telemetry.WallTime.Milliseconds(), SandboxSeconds: runResult.Telemetry.SandboxSeconds,
					PolicyViolations: append([]string(nil), runResult.Telemetry.PolicyViolations...),
				}
				if runErr != nil {
					benchmarkRun.Detail = runErr.Error()
				} else {
					verification, verifyErr := Evaluate(ctx, FixtureVerifier{Fixture: fixture}, task, runResult)
					if verifyErr != nil {
						benchmarkRun.Detail = verifyErr.Error()
					} else {
						benchmarkRun.Passed = verification.Passed
						benchmarkRun.Detail = verification.Detail
						benchmarkRun.PolicyViolations = append(benchmarkRun.PolicyViolations, verification.PolicyViolations...)
					}
				}
				report.Runs = append(report.Runs, benchmarkRun)
				if err := os.RemoveAll(workingDirectory); err != nil {
					return report, fmt.Errorf("agentruntime: delete benchmark workspace: %w", err)
				}
				report.WorkspacesDeleted++
			}
		}
	}
	report.Status = "complete"
	return finishBenchmarkReport(report, config)
}

func finishBenchmarkReport(report BenchmarkReport, config BenchmarkConfig) (BenchmarkReport, error) {
	report.FinishedAt = time.Now().UTC()
	if config.SandboxMeter != nil {
		report.SandboxesCreated, report.SandboxesDeleted = config.SandboxMeter()
	}
	report.Summaries = summarizeBenchmarkRuns(report.Runs)
	report.Decision = decideBenchmark(report.Summaries)
	if report.WorkspacesCreated != report.WorkspacesDeleted {
		return report, fmt.Errorf("agentruntime: benchmark leaked %d workspaces", report.WorkspacesCreated-report.WorkspacesDeleted)
	}
	if report.SandboxesCreated != report.SandboxesDeleted {
		return report, fmt.Errorf("agentruntime: benchmark leaked %d sandboxes", report.SandboxesCreated-report.SandboxesDeleted)
	}
	if config.ReportPath != "" {
		if err := writeBenchmarkReport(config.ReportPath, report); err != nil {
			return report, err
		}
	}
	return report, nil
}

func benchmarkStopReason(ctx context.Context, config BenchmarkConfig, report BenchmarkReport) string {
	if ctx.Err() != nil {
		return ctx.Err().Error()
	}
	created, deleted := 0, 0
	if config.SandboxMeter != nil {
		created, deleted = config.SandboxMeter()
	}
	if created >= config.MaxSandboxes {
		return fmt.Sprintf("sandbox budget reached: created=%d deleted=%d max=%d", created, deleted, config.MaxSandboxes)
	}
	var tokens int64
	var cost int64
	for _, run := range report.Runs {
		tokens += run.Tokens.Total
		cost += run.CostUSDMicros
	}
	if config.RelayMaxTokens > 0 && tokens >= config.RelayMaxTokens {
		return fmt.Sprintf("relay token budget reached: used=%d max=%d", tokens, config.RelayMaxTokens)
	}
	if config.RelayMaxCostUSDMicros > 0 && cost >= config.RelayMaxCostUSDMicros {
		return fmt.Sprintf("relay cost budget reached: used_micros=%d max_micros=%d", cost, config.RelayMaxCostUSDMicros)
	}
	return ""
}

func summarizeBenchmarkRuns(runs []BenchmarkRun) []RuntimeBenchmarkSummary {
	names := []string{"purpose-built", "opencode"}
	summaries := make([]RuntimeBenchmarkSummary, 0, len(names))
	for _, name := range names {
		summary := RuntimeBenchmarkSummary{Runtime: name}
		wallTimes := make([]int64, 0)
		for _, run := range runs {
			if run.Runtime != name {
				continue
			}
			summary.Runs++
			if run.Passed {
				summary.Passes++
			}
			summary.MeanRepairIterations += float64(run.RepairIterations)
			wallTimes = append(wallTimes, run.WallTimeMS)
			addTokenUsage(&summary.Tokens, run.Tokens)
			summary.CostUSDMicros += run.CostUSDMicros
			summary.PolicyViolationCount += len(run.PolicyViolations)
		}
		if summary.Runs > 0 {
			summary.CompletionRate = float64(summary.Passes) / float64(summary.Runs)
			summary.MeanRepairIterations /= float64(summary.Runs)
		}
		sort.Slice(wallTimes, func(i, j int) bool { return wallTimes[i] < wallTimes[j] })
		if len(wallTimes) > 0 {
			summary.MedianWallTimeMS = wallTimes[len(wallTimes)/2]
		}
		summary.Disqualified = summary.PolicyViolationCount > 0
		summaries = append(summaries, summary)
	}
	return summaries
}

func decideBenchmark(summaries []RuntimeBenchmarkSummary) BenchmarkDecision {
	byName := make(map[string]RuntimeBenchmarkSummary, len(summaries))
	for _, summary := range summaries {
		byName[summary.Runtime] = summary
	}
	purpose := byName["purpose-built"]
	openCode := byName["opencode"]
	if purpose.Disqualified && openCode.Disqualified {
		return BenchmarkDecision{Outcome: "both-disqualified", Reason: "both runtimes had policy violations; no runtime can be selected"}
	}
	if purpose.Disqualified {
		return BenchmarkDecision{Selected: "opencode", Outcome: "policy-disqualification", Reason: "purpose-built had a policy violation"}
	}
	if openCode.Disqualified {
		return BenchmarkDecision{Selected: "purpose-built", Outcome: "policy-disqualification", Reason: "opencode had a policy violation"}
	}
	if purpose.CompletionRate > openCode.CompletionRate {
		return BenchmarkDecision{Selected: "purpose-built", Outcome: "completion-rate", Reason: "purpose-built completed more fixtures"}
	}
	if openCode.CompletionRate > purpose.CompletionRate {
		return BenchmarkDecision{Selected: "opencode", Outcome: "completion-rate", Reason: "opencode completed more fixtures"}
	}
	repairImprovement := purpose.MeanRepairIterations > 0 && openCode.MeanRepairIterations <= purpose.MeanRepairIterations*0.8
	wallImprovement := purpose.MedianWallTimeMS > 0 && float64(openCode.MedianWallTimeMS) <= float64(purpose.MedianWallTimeMS)*0.8
	noRegression := openCode.Tokens.Total <= purpose.Tokens.Total && openCode.CostUSDMicros <= purpose.CostUSDMicros
	if (repairImprovement || wallImprovement) && noRegression {
		return BenchmarkDecision{Selected: "opencode", Outcome: "material-tie-break", Reason: "opencode materially improved repair iterations or wall time without token/cost regression"}
	}
	return BenchmarkDecision{Selected: "purpose-built", Outcome: "effective-tie-or-inconclusive", Reason: "the predeclared default selects purpose-built"}
}

func fixtureFingerprint(fixture BenchmarkFixture) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(fixture.ID + "\x00" + fixture.TaskPrompt + "\x00" + fixture.EndpointPath + "\x00" + fixture.ExpectedPayload))
	files := append([]FixtureFile(nil), fixture.BaseFiles...)
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	for _, file := range files {
		_, _ = hash.Write([]byte("\x00" + file.Path + "\x00" + file.Content))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func readUsageMeter(ctx context.Context, meter UsageMeter) (UsageSnapshot, error) {
	if meter == nil {
		return UsageSnapshot{}, nil
	}
	snapshot, err := meter(ctx)
	if err != nil {
		return UsageSnapshot{}, fmt.Errorf("agentruntime: read benchmark usage meter: %w", err)
	}
	return snapshot, nil
}

func subtractTokens(after, before TokenUsage) TokenUsage {
	return TokenUsage{
		Total: maxInt64(0, after.Total-before.Total), Input: maxInt64(0, after.Input-before.Input),
		Output: maxInt64(0, after.Output-before.Output), Reasoning: maxInt64(0, after.Reasoning-before.Reasoning),
		CacheRead: maxInt64(0, after.CacheRead-before.CacheRead), CacheWrite: maxInt64(0, after.CacheWrite-before.CacheWrite),
	}
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func writeBenchmarkReport(path string, report BenchmarkReport) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("agentruntime: create benchmark report directory: %w", err)
	}
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("agentruntime: encode benchmark report: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".benchmark-report-*.json")
	if err != nil {
		return fmt.Errorf("agentruntime: create temporary benchmark report: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if _, err := temporary.Write(append(payload, '\n')); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Chmod(temporaryPath, 0o600); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("agentruntime: publish benchmark report: %w", err)
	}
	return nil
}
