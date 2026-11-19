// Command problem-quality-benchmark runs the local development
// problem-quality benchmark. It executes real artifact and assessment
// commands on the local machine, writes a schema-validated preliminary
// report, and makes no model calls or sandbox claims.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/kvn8888/codegym/backend/internal/benchmarkbudget"
	"github.com/kvn8888/codegym/backend/internal/problemquality/benchmark"
)

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }

func (s *stringList) Set(value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("value must not be blank")
	}
	*s = append(*s, value)
	return nil
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("problem-quality-benchmark", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var cases stringList
	var reviewAuthorities stringList
	execution := flags.String("execution", "", "execution mode; only 'local' is supported")
	corpus := flags.String("corpus", "", "problem-quality corpus directory")
	flags.Var(&cases, "case", "explicit case directory (repeatable)")
	flags.Var(&reviewAuthorities, "review-authority", "authorized human review authority (repeatable)")
	output := flags.String("output", "problem-quality-report.json", "report output path")
	schema := flags.String("schema", "", "report schema path (default: repository benchmark schema)")
	timeout := flags.Duration("timeout", 2*time.Minute, "timeout for each setup/build/test/assessment command")
	baselineKind := flags.String("baseline-kind", "other", "independent, current, agent-built, or other")
	modelRevision := flags.String("model-revision", "", "baseline model revision, or blank for not_applicable")
	promptRevision := flags.String("prompt-revision", "", "baseline prompt revision, or blank for not_applicable")
	developmentFixture := flags.Bool("development-fixture", false, "explicitly allow running a development fixture")
	checkReadiness := flags.Bool("readiness", false, "validate saved input readiness without executing")
	budgetState := flags.String("budget-state", "problem-quality-budget.json", "persistent budget state path; blank disables budget accounting")
	priorSpendMicros := flags.Int64("prior-spend-micros", 0, "explicit prior spending in micro-USD recorded when budget state is first created")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if *checkReadiness {
		return runReadiness(cases, *baselineKind, stdout, stderr)
	}

	if strings.TrimSpace(*execution) != "local" {
		if strings.TrimSpace(*execution) == "" {
			fmt.Fprintln(stderr, "problem-quality-benchmark: local execution must be explicitly selected with -execution local")
		} else {
			fmt.Fprintf(stderr, "problem-quality-benchmark: execution mode %q is unsupported; only local development execution is available (paid/model/sandbox modes are rejected before any work)\n", *execution)
		}
		return 2
	}

	schemaPath := strings.TrimSpace(*schema)
	if schemaPath == "" {
		schemaPath = defaultSchemaPath()
	}

	var budgetSnapshot *benchmarkbudget.Snapshot
	if strings.TrimSpace(*budgetState) != "" {
		budgetStore, err := benchmarkbudget.Open(*budgetState, *priorSpendMicros)
		if err != nil {
			fmt.Fprintf(stderr, "problem-quality-benchmark: %v\n", err)
			return 2
		}
		snapshot, err := budgetStore.Snapshot()
		if err != nil {
			fmt.Fprintf(stderr, "problem-quality-benchmark: %v\n", err)
			return 2
		}
		budgetSnapshot = &snapshot
	}

	result, err := benchmark.Execute(ctx, benchmark.Options{
		CorpusRoot:         *corpus,
		CaseDirs:           cases,
		OutputPath:         *output,
		SchemaPath:         schemaPath,
		Timeout:            *timeout,
		BaselineKind:       *baselineKind,
		ModelRevision:      *modelRevision,
		PromptRevision:     *promptRevision,
		DevelopmentFixture: *developmentFixture,
		ExecuteLocal:       true,
		BudgetSnapshot:     budgetSnapshot,
		ReviewAuthorities:  reviewAuthorities,
	})
	if err != nil {
		fmt.Fprintf(stderr, "problem-quality-benchmark: %v\n", err)
		return 2
	}
	fmt.Fprintf(stdout, "report: %s\ncases: %d passed=%d failed=%d error=%d not_run=%d\n",
		result.ReportPath,
		result.Report.Summary.CasesTotal,
		result.Report.Summary.CasesPassed,
		result.Report.Summary.CasesFailed,
		result.Report.Summary.CasesError,
		result.Report.Summary.CasesNotRun,
	)
	if result.Report.Budget != nil {
		fmt.Fprintf(stdout, "budget: currency=%s limit_micros=%d prior_spend_micros=%d committed_micros=%d remaining_micros=%d reservations=%d\n",
			result.Report.Budget.Currency,
			result.Report.Budget.LimitMicros,
			result.Report.Budget.PriorSpendMicros,
			result.Report.Budget.CommittedMicros,
			result.Report.Budget.RemainingMicros,
			len(result.Report.Budget.Reservations),
		)
	}
	switch {
	case result.ExecutionFailure:
		fmt.Fprintln(stderr, "problem-quality-benchmark: execution/configuration failure; report was still written")
		return 2
	case result.QualityFailure:
		fmt.Fprintln(stderr, "problem-quality-benchmark: benchmark completed but quality expectations were not met")
		return 1
	}
	return 0
}

func runReadiness(cases []string, baselineKind string, stdout, stderr io.Writer) int {
	if len(cases) == 0 {
		fmt.Fprintln(stderr, "problem-quality-benchmark: -readiness requires at least one explicit -case")
		return 2
	}
	status := 0
	for i, caseDir := range cases {
		if i > 0 {
			fmt.Fprintln(stdout)
		}
		readiness, err := benchmark.ValidateSavedCaseReadiness(caseDir, baselineKind)
		if err != nil {
			fmt.Fprintf(stderr, "problem-quality-benchmark: %s: %v\n", caseDir, err)
			status = 2
			continue
		}
		fmt.Fprintf(stdout, "case: %s\n", readiness.CaseID)
		fmt.Fprintf(stdout, "runnable: %t\n", readiness.Runnable)
		fmt.Fprintf(stdout, "development_fixture: %t\n", readiness.DevelopmentFixture)
		fmt.Fprintf(stdout, "governed_content_sha256: %s\n", readiness.GovernedContentSHA256)
		fmt.Fprintf(stdout, "provenance_complete: %t\n", readiness.ProvenanceComplete)
		if len(readiness.MissingProvenance) == 0 {
			fmt.Fprintln(stdout, "missing_provenance: none")
		} else {
			fmt.Fprintf(stdout, "missing_provenance: %s\n", strings.Join(readiness.MissingProvenance, ", "))
		}
	}
	return status
}

func defaultSchemaPath() string {
	if root, err := exec.Command("git", "rev-parse", "--show-toplevel").Output(); err == nil {
		return filepath.Join(strings.TrimSpace(string(root)), "benchmarks", "problem-quality", "schema", "report.schema.json")
	}
	return filepath.Join("..", "benchmarks", "problem-quality", "schema", "report.schema.json")
}
