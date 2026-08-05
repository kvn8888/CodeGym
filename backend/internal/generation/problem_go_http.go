package generation

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kvn8888/codegym/backend/internal/execution"
	"github.com/kvn8888/codegym/backend/internal/problems"
)

const (
	goHTTPHarnessPath    = "codegym_http_harness.go"
	goHTTPComparatorPath = "codegym_http_comparator.go"
	goHTTPCasesPath      = "codegym_http_cases.json"
	goHTTPLauncherPath   = "codegym_http_compile.py"
)

func buildGoHTTPProblemDefinition(output GeneratedProblem) (problems.Definition, error) {
	config, cases, err := problems.NormalizeHTTPCases(problems.TestConfig{
		Strategy:   problems.TestStrategyHTTP,
		Comparator: output.Comparator,
	}, output.HTTPTestCases)
	if err != nil {
		return problems.Definition{}, err
	}
	hiddenFiles, err := buildGoHTTPTestFiles(config, cases)
	if err != nil {
		return problems.Definition{}, err
	}
	publicFiles, err := buildGoHTTPTestFiles(config, problems.SelectHTTPCases(cases, false))
	if err != nil {
		return problems.Definition{}, err
	}
	hints := make([]problems.Hint, 0, len(output.Hints))
	for index, hint := range output.Hints {
		hints = append(hints, problems.Hint{Cost: index, Text: strings.TrimSpace(hint)})
	}
	starter := strings.TrimRight(output.StarterCode, "\n") + "\n"
	reference := strings.TrimRight(output.ReferenceSolution, "\n") + "\n"
	return problems.Definition{
		Problem: problems.Problem{
			Summary: problems.Summary{
				Title: output.Title, Category: output.Category, Language: "go", Framework: "net/http",
				Difficulty: output.Difficulty, Tags: output.Tags,
				EstimatedMinutes: output.EstimatedMinutes, Type: "coding",
			},
			Version: "1.0.0", Description: output.Description, Subcategory: output.Subcategory,
			Runtime:     problems.Runtime{Image: "go1.25.4", TimeoutSeconds: 120, MemoryMB: 1024, NetworkMode: "block-all"},
			Files:       problems.FileManifest{Skeleton: []problems.FileRef{{Path: output.Entrypoint, Entry: true}}},
			TestConfig:  config,
			PublicCases: problems.ProjectPublicHTTPCases(cases),
			Hints:       hints,
		},
		SkeletonFiles:     []problems.File{{Path: output.Entrypoint, Content: starter}},
		PublicTestFiles:   publicFiles,
		HiddenTestFiles:   hiddenFiles,
		ReferenceSolution: reference,
		Entrypoint:        goHTTPLauncherPath,
	}, nil
}

func buildGoHTTPTestFiles(config problems.TestConfig, cases []problems.HTTPCase) ([]problems.File, error) {
	harnessConfig, err := json.Marshal(struct {
		ReadinessTimeoutSeconds int                 `json:"readiness_timeout_seconds"`
		Cases                   []problems.HTTPCase `json:"cases"`
	}{
		ReadinessTimeoutSeconds: config.ReadinessTimeoutSeconds,
		Cases:                   cases,
	})
	if err != nil {
		return nil, fmt.Errorf("encode HTTP harness cases: %w", err)
	}
	return []problems.File{
		{Path: goHTTPHarnessPath, Content: execution.GoHTTPHarnessSource},
		{Path: goHTTPComparatorPath, Content: execution.GoComparatorSource},
		{Path: goHTTPCasesPath, Content: string(harnessConfig)},
		{Path: goHTTPLauncherPath, Content: execution.GoHTTPCompileRunnerSource},
	}, nil
}
