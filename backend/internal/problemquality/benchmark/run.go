package benchmark

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kvn8888/codegym/backend/internal/agentruntime"
	"github.com/kvn8888/codegym/backend/internal/benchmarkbudget"
	"github.com/kvn8888/codegym/backend/internal/envintegration"
	"github.com/kvn8888/codegym/backend/internal/environmentverify"
	"github.com/kvn8888/codegym/backend/internal/problemquality"
)

// sandboxEstimateMicros is the bounded maximum reserved for one Daytona
// sandbox case. It is a conservative planning cap, not provider-reported cost.
const sandboxEstimateMicros int64 = 500_000

type GenerationUsageRecord struct {
	CaseID    string `json:"case_id"`
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	TokensIn  int    `json:"tokens_in"`
	TokensOut int    `json:"tokens_out"`
}

type Options struct {
	CorpusRoot          string
	CaseDirs            []string
	OutputPath          string
	SchemaPath          string
	Timeout             time.Duration
	BaselineKind        string
	ModelRevision       string
	PromptRevision      string
	DevelopmentFixture  bool
	ExecuteLocal        bool
	ExecuteSandbox      bool
	ExecutionMode       string
	SandboxExecutor     agentruntime.SandboxExecutor
	DaytonaAPIKey       string
	DaytonaAPIURL       string
	SandboxSnapshot     string
	SandboxWorkspaceDir string
	SandboxEphemeral    bool
	BudgetSnapshot      *benchmarkbudget.Snapshot
	BudgetStore         *benchmarkbudget.Store
	PaidOperations      bool
	ReviewAuthorities   []string
	GenerationUsage     []GenerationUsageRecord

	now func() time.Time
}

func (o Options) clock() time.Time {
	if o.now != nil {
		return o.now()
	}
	return time.Now()
}

type Result struct {
	Report           Report
	ReportPath       string
	QualityFailure   bool
	ExecutionFailure bool
}

// Execute runs the local development benchmark. It always attempts to write a
// report for cases that were selected, including quality failures and
// per-case configuration/execution errors. An error return means no report
// could be produced (invalid global configuration or report write/validation
// failure); CLI exit codes distinguish quality from execution failures in the
// returned Result.
func Execute(ctx context.Context, opts Options) (Result, error) {
	executionMode := strings.TrimSpace(opts.ExecutionMode)
	if executionMode == "" {
		if opts.ExecuteSandbox {
			executionMode = "sandbox"
		} else if opts.ExecuteLocal {
			executionMode = "local"
		}
	}
	switch executionMode {
	case "local", "sandbox":
	default:
		if executionMode == "" {
			return Result{}, errors.New("execution mode must be explicitly selected with -execution local or -execution sandbox")
		}
		return Result{}, fmt.Errorf("execution mode %q is unsupported; only local and sandbox development execution are available", executionMode)
	}
	opts.ExecutionMode = executionMode
	if opts.Timeout <= 0 {
		return Result{}, errors.New("timeout must be greater than zero")
	}
	if err := validateBaselineKind(opts.BaselineKind); err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(opts.OutputPath) == "" {
		return Result{}, errors.New("report output path is required")
	}
	if strings.TrimSpace(opts.SchemaPath) == "" {
		return Result{}, errors.New("report schema path is required")
	}
	if strings.TrimSpace(opts.CorpusRoot) != "" && len(opts.CaseDirs) > 0 {
		return Result{}, errors.New("select either a corpus root or explicit case directories, not both")
	}
	if strings.TrimSpace(opts.CorpusRoot) == "" && len(opts.CaseDirs) == 0 {
		return Result{}, errors.New("select a corpus root or at least one case directory")
	}

	started := opts.clock().UTC()
	commit, branch, dirty, err := gitIdentity(ctx)
	if err != nil {
		return Result{}, err
	}
	runID, err := newRunID(started)
	if err != nil {
		return Result{}, err
	}

	loaded := make([]problemquality.CorpusCase, 0)
	selection := "explicit_cases"
	corpusRoot := ""
	if strings.TrimSpace(opts.CorpusRoot) != "" {
		selection = "corpus"
		abs, err := filepath.Abs(opts.CorpusRoot)
		if err != nil {
			return Result{}, fmt.Errorf("resolve corpus root: %w", err)
		}
		corpusRoot = abs
		cases, err := problemquality.LoadCorpus(abs)
		if err != nil {
			return Result{}, err
		}
		loaded = append(loaded, cases...)
	} else {
		for _, dir := range opts.CaseDirs {
			abs, err := filepath.Abs(dir)
			if err != nil {
				return Result{}, fmt.Errorf("resolve case directory %q: %w", dir, err)
			}
			c, err := problemquality.LoadCase(abs)
			loaded = append(loaded, problemquality.CorpusCase{Dir: abs, Case: c, Err: err})
		}
	}
	if len(loaded) == 0 {
		return Result{}, errors.New("no problem-quality cases were selected")
	}

	selectedIDs := make([]string, 0, len(loaded))
	runs := make([]CaseRun, 0, len(loaded))
	for _, loadedCase := range loaded {
		caseID := filepath.Base(loadedCase.Dir)
		if loadedCase.Case != nil && strings.TrimSpace(loadedCase.Case.CaseID) != "" {
			caseID = loadedCase.Case.CaseID
		}
		selectedIDs = append(selectedIDs, caseID)
		if err := ctx.Err(); err != nil {
			runs = append(runs, CaseRun{
				Dir:          loadedCase.Dir,
				Case:         loadedCase.Case,
				NotRunReason: "not run: benchmark cancelled before this case",
			})
			continue
		}
		runs = append(runs, runCase(ctx, runID, opts, loadedCase))
	}

	completed := opts.clock().UTC()
	report := BuildReport(Meta{
		RunID:              runID,
		StartedAt:          started,
		CompletedAt:        completed,
		TestedCommit:       commit,
		Branch:             branch,
		Dirty:              dirty,
		BaselineKind:       opts.BaselineKind,
		ExecutionMode:      opts.ExecutionMode,
		GenerationUsage:    opts.GenerationUsage,
		PaidOperations:     opts.PaidOperations,
		ModelRevision:      opts.ModelRevision,
		PromptRevision:     opts.PromptRevision,
		CorpusRoot:         corpusRoot,
		Selection:          selection,
		SelectedCaseIDs:    selectedIDs,
		Timeout:            opts.Timeout,
		DevelopmentFixture: opts.DevelopmentFixture,
		BudgetSnapshot:     opts.BudgetSnapshot,
	}, runs)
	if err := WriteReport(opts.OutputPath, report, opts.SchemaPath); err != nil {
		return Result{Report: report, ReportPath: opts.OutputPath}, err
	}
	result := Result{Report: report, ReportPath: opts.OutputPath}
	for _, caseReport := range report.Cases {
		switch caseReport.Status {
		case CaseStatusFailed:
			result.QualityFailure = true
		case CaseStatusError, CaseStatusNotRun:
			result.ExecutionFailure = true
		}
	}
	return result, nil
}

func runCase(ctx context.Context, runID string, opts Options, loaded problemquality.CorpusCase) (result CaseRun) {
	started := opts.clock()
	result = CaseRun{Dir: loaded.Dir, Case: loaded.Case}
	defer func() { result.ElapsedMS = opts.clock().Sub(started).Milliseconds() }()
	if loaded.Err != nil {
		result.Err = loaded.Err
		return result
	}
	if loaded.Case == nil {
		result.Err = errors.New("case loader returned no case")
		return result
	}
	configPath := filepath.Join(loaded.Case.Dir, RunConfigFileName)
	config, err := LoadRunConfig(configPath, loaded.Case)
	if err != nil {
		result.Err = err
		return result
	}
	result.Config = config
	if config.DevelopmentFixture != opts.DevelopmentFixture {
		if config.DevelopmentFixture {
			result.Err = fmt.Errorf("case %q is a development fixture; rerun with -development-fixture", loaded.Case.CaseID)
		} else {
			result.Err = fmt.Errorf("case %q is not marked as a development fixture; remove -development-fixture", loaded.Case.CaseID)
		}
		return result
	}
	plan, err := loaded.Case.AssessmentPlan(&config.ReferenceData)
	if err != nil {
		result.Err = err
		return result
	}
	identity, err := computeContentIdentity(loaded.Case, config)
	if err != nil {
		result.Err = err
		return result
	}
	result.Identity = identity
	if err := validateGenerationObservationIdentity(config, identity, opts.BaselineKind); err != nil {
		result.Err = err
		return result
	}
	manifestPath := config.ArtifactManifestPath()
	builder := environmentverify.BuilderResult{
		SchemaVersion:        environmentverify.BuilderResultSchemaVersion,
		BuilderRunID:         "local-artifact:" + config.ArtifactID + ":" + config.ArtifactVersion,
		Runtime:              &environmentverify.RuntimeInfo{Name: "local-shell", Version: "development-only"},
		ProposedManifestPath: &manifestPath,
		Diagnostics:          []environmentverify.Diagnostic{},
	}
	var commandRunner environmentverify.CommandRunner
	var assessor environmentverify.AssessmentRunner
	var cleanupSandbox func()
	cleanupSandbox = func() {}
	if strings.TrimSpace(opts.ExecutionMode) == "sandbox" {
		networkBlockAll := false
		if limits := loaded.Case.BuildTestRun.Limits; limits != nil {
			mode := strings.ToLower(strings.TrimSpace(limits.NetworkMode))
			networkBlockAll = mode == "none" || mode == "block-all"
		}
		sandbox, cleanup, err := createBenchmarkSandbox(ctx, opts, networkBlockAll, "sandbox:"+runID+":"+loaded.Case.CaseID)
		if err != nil {
			result.Err = fmt.Errorf("create sandbox: %w", err)
			return result
		}
		cleanupSandbox = cleanup
		defer cleanupSandbox()
		remoteRoot := strings.TrimRight(strings.TrimSpace(opts.SandboxWorkspaceDir), "/")
		if remoteRoot == "" {
			remoteRoot = envintegration.DefaultSandboxWorkspaceDir
		}
		if err := envintegration.UploadTree(ctx, sandbox, config.ArtifactRoot, remoteRoot); err != nil {
			result.Err = fmt.Errorf("upload artifact to sandbox: %w", err)
			return result
		}
		commandRunner = envintegration.SandboxCommandRunner{
			Executor: sandbox,
			Paths: envintegration.SandboxPathMapper{
				LocalRoot:  config.ArtifactRoot,
				RemoteRoot: remoteRoot,
			},
			Timeout: opts.Timeout,
		}
		assessor = &envintegration.SandboxProtocolAssessor{
			Executor:   sandbox,
			ScratchDir: "/tmp/codegym-assess",
			Timeout:    opts.Timeout,
		}
		builder.Runtime = &environmentverify.RuntimeInfo{Name: "daytona-sandbox", Version: "development-only"}
	} else {
		commandRunner = &envintegration.LocalCommandRunner{}
		assessor = &environmentverify.LocalProtocolAssessor{}
	}
	verifyOptions := environmentverify.DefaultVerifyOptions()
	verifyOptions.Timeouts = environmentverify.StageTimeouts{
		Setup:      opts.Timeout,
		Build:      opts.Timeout,
		Test:       opts.Timeout,
		Assessment: opts.Timeout,
	}
	verifyOptions.Assessor = assessor
	verifierResult, collection := environmentverify.VerifyAssessmentPlanWithOptions(
		ctx, commandRunner, builder, config.ArtifactRoot, runID+":"+loaded.Case.CaseID, verifyOptions, *plan)
	if !collection.Blocked {
		review := EvaluateReviewEligibility(loaded.Case, identity, opts.ReviewAuthorities, config.DevelopmentFixture)
		collection.BenchmarkEligible = review.Eligible()
		collection.BenchmarkEligibility = review.Status
		collection.BenchmarkEligibilityReason = review.Reason
	}
	result.Verifier = verifierResult
	result.Collection = collection
	return result
}

func createBenchmarkSandbox(ctx context.Context, opts Options, networkBlockAll bool, operationID string) (agentruntime.SandboxExecutor, func(), error) {
	if opts.SandboxExecutor != nil {
		return opts.SandboxExecutor, func() {}, nil
	}
	reserved := false
	if opts.BudgetStore != nil {
		if _, err := opts.BudgetStore.Reserve(operationID, sandboxEstimateMicros); err != nil {
			return nil, nil, fmt.Errorf("reserve sandbox budget: %w", err)
		}
		reserved = true
	}
	settleOnFailure := func() {
		if reserved {
			_, _ = opts.BudgetStore.Settle(operationID, 0)
		}
	}
	apiKey := strings.TrimSpace(opts.DaytonaAPIKey)
	apiURL := strings.TrimSpace(opts.DaytonaAPIURL)
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("DAYTONA_API_KEY"))
	}
	if apiURL == "" {
		apiURL = strings.TrimSpace(os.Getenv("DAYTONA_API_URL"))
	}
	if apiKey == "" || apiURL == "" {
		settleOnFailure()
		return nil, nil, errors.New("Daytona credentials are required for sandbox execution")
	}
	snapshot := strings.TrimSpace(opts.SandboxSnapshot)
	if snapshot == "" {
		snapshot = strings.TrimSpace(os.Getenv("CODEGYM_BASE_SNAPSHOT"))
	}
	if snapshot == "" {
		settleOnFailure()
		return nil, nil, errors.New("a Daytona base snapshot is required for sandbox execution")
	}
	client, err := envintegration.NewDaytonaClient(apiKey, apiURL)
	if err != nil {
		settleOnFailure()
		return nil, nil, err
	}
	labels := map[string]string{"codegym": "problem-quality-benchmark"}
	sandbox, err := client.Create(ctx, envintegration.SandboxParams(snapshot, labels, networkBlockAll, opts.SandboxEphemeral))
	if err != nil {
		settleOnFailure()
		return nil, nil, fmt.Errorf("create Daytona sandbox: %w", err)
	}
	cleanup := func() {
		if reserved {
			_, _ = opts.BudgetStore.Settle(operationID, sandboxEstimateMicros)
			reserved = false
		}
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		_ = sandbox.Delete(cleanupCtx)
	}
	return sandbox, cleanup, nil
}

func validateBaselineKind(kind string) error {
	switch strings.TrimSpace(kind) {
	case "independent", "current", "agent-built", "other":
		return nil
	default:
		return fmt.Errorf("baseline kind %q must be independent, current, agent-built, or other", kind)
	}
}

func newRunID(started time.Time) (string, error) {
	var random [4]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("generate benchmark run ID: %w", err)
	}
	return "pqb-" + started.Format("20060102T150405Z") + "-" + hex.EncodeToString(random[:]), nil
}

func gitIdentity(ctx context.Context) (commit, branch string, dirty bool, err error) {
	output := func(args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, "git", args...)
		data, err := cmd.Output()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(data)), nil
	}
	commit, err = output("rev-parse", "HEAD")
	if err != nil {
		return "", "", false, fmt.Errorf("determine tested commit: %w", err)
	}
	if commit == "" {
		return "", "", false, errors.New("determine tested commit: git returned an empty commit")
	}
	branch, err = output("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", "", false, fmt.Errorf("determine tested branch: %w", err)
	}
	status, err := output("status", "--porcelain")
	if err != nil {
		return "", "", false, fmt.Errorf("determine working-tree state: %w", err)
	}
	return commit, branch, status != "", nil
}
