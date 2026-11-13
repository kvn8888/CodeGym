// Command problem-quality-benchmark runs the local development
// problem-quality benchmark. It executes real artifact and assessment
// commands on the local machine, writes a schema-validated preliminary
// report, and makes no model calls or sandbox claims.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/kvn8888/codegym/backend/internal/problemquality/benchmark"
)

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }

func (s *stringList) Set(value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("case directory must not be blank")
	}
	*s = append(*s, value)
	return nil
}

func main() {
	var cases stringList
	execution := flag.String("execution", "", "execution mode; only 'local' is supported")
	corpus := flag.String("corpus", "", "problem-quality corpus directory")
	flag.Var(&cases, "case", "explicit case directory (repeatable)")
	output := flag.String("output", "problem-quality-report.json", "report output path")
	schema := flag.String("schema", "", "report schema path (default: repository benchmark schema)")
	timeout := flag.Duration("timeout", 2*time.Minute, "timeout for each setup/build/test/assessment command")
	baselineKind := flag.String("baseline-kind", "other", "independent, current, agent-built, or other")
	modelRevision := flag.String("model-revision", "", "baseline model revision, or blank for not_applicable")
	promptRevision := flag.String("prompt-revision", "", "baseline prompt revision, or blank for not_applicable")
	developmentFixture := flag.Bool("development-fixture", false, "explicitly allow running a development fixture")
	flag.Parse()

	if strings.TrimSpace(*execution) != "local" {
		if strings.TrimSpace(*execution) == "" {
			fmt.Fprintln(os.Stderr, "problem-quality-benchmark: local execution must be explicitly selected with -execution local")
		} else {
			fmt.Fprintf(os.Stderr, "problem-quality-benchmark: execution mode %q is unsupported; only local development execution is available (paid/model/sandbox modes are rejected before any work)\n", *execution)
		}
		os.Exit(2)
	}

	schemaPath := strings.TrimSpace(*schema)
	if schemaPath == "" {
		schemaPath = defaultSchemaPath()
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

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
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "problem-quality-benchmark: %v\n", err)
		os.Exit(2)
	}
	fmt.Printf("report: %s\ncases: %d passed=%d failed=%d error=%d not_run=%d\n",
		result.ReportPath,
		result.Report.Summary.CasesTotal,
		result.Report.Summary.CasesPassed,
		result.Report.Summary.CasesFailed,
		result.Report.Summary.CasesError,
		result.Report.Summary.CasesNotRun,
	)
	switch {
	case result.ExecutionFailure:
		fmt.Fprintln(os.Stderr, "problem-quality-benchmark: execution/configuration failure; report was still written")
		os.Exit(2)
	case result.QualityFailure:
		fmt.Fprintln(os.Stderr, "problem-quality-benchmark: benchmark completed but quality expectations were not met")
		os.Exit(1)
	}
}

func defaultSchemaPath() string {
	if root, err := exec.Command("git", "rev-parse", "--show-toplevel").Output(); err == nil {
		return filepath.Join(strings.TrimSpace(string(root)), "benchmarks", "problem-quality", "schema", "report.schema.json")
	}
	return filepath.Join("..", "benchmarks", "problem-quality", "schema", "report.schema.json")
}
