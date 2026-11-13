package benchmark

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kvn8888/codegym/backend/internal/envintegration"
	"github.com/kvn8888/codegym/backend/internal/environmentverify"
	"github.com/kvn8888/codegym/backend/internal/problemquality"
)

type Options struct {
	CorpusRoot         string
	CaseDirs           []string
	OutputPath         string
	SchemaPath         string
	Timeout            time.Duration
	BaselineKind       string
	ModelRevision      string
	PromptRevision     string
	DevelopmentFixture bool
	ExecuteLocal       bool

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
	if !opts.ExecuteLocal {
		return Result{}, errors.New("local execution must be explicitly selected with -execution local")
	}
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
		ModelRevision:      opts.ModelRevision,
		PromptRevision:     opts.PromptRevision,
		CorpusRoot:         corpusRoot,
		Selection:          selection,
		SelectedCaseIDs:    selectedIDs,
		Timeout:            opts.Timeout,
		DevelopmentFixture: opts.DevelopmentFixture,
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
	manifestPath := config.ArtifactManifestPath()
	builder := environmentverify.BuilderResult{
		SchemaVersion:        environmentverify.BuilderResultSchemaVersion,
		BuilderRunID:         "local-artifact:" + config.ArtifactID + ":" + config.ArtifactVersion,
		Runtime:              &environmentverify.RuntimeInfo{Name: "local-shell", Version: "development-only"},
		ProposedManifestPath: &manifestPath,
		Diagnostics:          []environmentverify.Diagnostic{},
	}
	commandRunner := &envintegration.LocalCommandRunner{}
	assessor := &environmentverify.LocalProtocolAssessor{}
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
	result.Verifier = verifierResult
	result.Collection = collection
	return result
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
