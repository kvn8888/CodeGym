package generation

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/kvn8888/codegym/backend/internal/execution"
	"github.com/kvn8888/codegym/backend/internal/problems"
)

func buildGoProblemDefinition(output GeneratedProblem) (problems.Definition, error) {
	if output.Strategy == problems.TestStrategyHTTP {
		return buildGoHTTPProblemDefinition(output)
	}
	comparator, err := problems.NormalizeComparator(output.Comparator)
	if err != nil {
		return problems.Definition{}, err
	}
	output.Comparator = comparator
	hiddenFiles, err := buildGoUnitTestFiles(output, output.TestCases)
	if err != nil {
		return problems.Definition{}, err
	}
	publicFiles, err := buildGoUnitTestFiles(output, problems.SelectUnitCases(output.TestCases, false))
	if err != nil {
		return problems.Definition{}, err
	}
	parameters := make([]string, 0, len(output.Parameters))
	for _, parameter := range output.Parameters {
		parameters = append(parameters, parameter.Name+" "+parameter.Type)
	}
	skeleton := fmt.Sprintf("package main\n\nfunc %s(%s) %s {\n\tpanic(\"TODO: implement this function\")\n}\n",
		output.FunctionName, strings.Join(parameters, ", "), output.ReturnType)
	hints := make([]problems.Hint, 0, len(output.Hints))
	for index, hint := range output.Hints {
		hints = append(hints, problems.Hint{Cost: index, Text: strings.TrimSpace(hint)})
	}
	return problems.Definition{
		Problem: problems.Problem{
			Summary: problems.Summary{
				Title: output.Title, Category: output.Category, Language: "go",
				Difficulty: output.Difficulty, Tags: output.Tags,
				EstimatedMinutes: output.EstimatedMinutes, Type: "coding",
			},
			Version: "1.0.0", Description: output.Description, Subcategory: output.Subcategory,
			Runtime:     problems.Runtime{Image: "go1.25.4", TimeoutSeconds: 30, MemoryMB: 1024, NetworkMode: "block-all"},
			Files:       problems.FileManifest{Skeleton: []problems.FileRef{{Path: "solution.go", Entry: true}}},
			TestConfig:  problems.TestConfig{Strategy: "unit", Comparator: output.Comparator},
			PublicCases: problems.ProjectPublicUnitCases(output.TestCases),
			Hints:       hints,
		},
		SkeletonFiles:     []problems.File{{Path: "solution.go", Content: skeleton}},
		PublicTestFiles:   publicFiles,
		HiddenTestFiles:   hiddenFiles,
		ReferenceSolution: strings.TrimRight(output.ReferenceSolution, "\n") + "\n",
		Entrypoint:        ".codegym/compile_and_run.py",
	}, nil
}

func buildGoUnitTestFiles(output GeneratedProblem, cases []problems.UnitCase) ([]problems.File, error) {
	caseSources := make([]string, 0, len(cases))
	usesChecker := false
	for index, test := range cases {
		name := strings.TrimSpace(test.Name)
		if name == "" {
			name = fmt.Sprintf("case-%d", index+1)
		}
		resolved, err := problems.ResolveComparator(output.Comparator, test.Comparator)
		if err != nil {
			return nil, fmt.Errorf("case %q comparator: %w", name, err)
		}
		usesChecker = usesChecker || resolved.Kind == problems.ComparatorChecker
		caseSource, err := buildGoCaseSource(output, test, name, resolved)
		if err != nil {
			return nil, err
		}
		caseSources = append(caseSources, caseSource)
	}

	runner := fmt.Sprintf(`package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type codegymCase struct {
	Name string
	Run  func() (bool, string)
}

func appendEvent(event map[string]any) {
	stream, err := os.OpenFile(".codegym/cases.jsonl", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}
	if err := json.NewEncoder(stream).Encode(event); err != nil {
		panic(err)
	}
	if err := stream.Sync(); err != nil {
		panic(err)
	}
	if err := stream.Close(); err != nil {
		panic(err)
	}
}

func writeVerdict(status string, cases []map[string]any) {
	payload := map[string]any{"schema": 1, "status": status, "compile_error": nil, "cases": cases}
	data, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(".codegym/verdict.json.tmp", data, 0600); err != nil {
		panic(err)
	}
	if err := os.Rename(".codegym/verdict.json.tmp", ".codegym/verdict.json"); err != nil {
		panic(err)
	}
}

func decodeValue[T any](raw string) (T, error) {
	var value T
	err := json.Unmarshal([]byte(raw), &value)
	return value, err
}

func codegymJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%%v", value)
	}
	return string(data)
}

func codegymEpsilon(value float64) *float64 { return &value }

func safeRun(run func() (bool, string)) (equal bool, reason string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			equal = false
			reason = fmt.Sprintf("panic: %%v", recovered)
		}
	}()
	return run()
}

func main() {
	cases := []codegymCase{
%s
	}
	results := make([]map[string]any, 0, len(cases))
	allPassed := true
	for _, testCase := range cases {
		appendEvent(map[string]any{"event": "case_start", "name": testCase.Name})
		started := time.Now()
		equal, reason := safeRun(testCase.Run)
		status := "pass"
		var errorText any
		if !equal {
			status = "fail"
			allPassed = false
			errorText = reason
		}
		result := map[string]any{
			"name": testCase.Name, "status": status,
			"duration_ms": max(int64(0), time.Since(started).Milliseconds()), "error": errorText,
		}
		results = append(results, result)
		appendEvent(map[string]any{
			"event": "case_result", "name": testCase.Name, "status": status,
			"duration_ms": result["duration_ms"], "error": errorText,
		})
	}
	status := "failed"
	if allPassed {
		status = "passed"
	}
	writeVerdict(status, results)
}
`, strings.Join(caseSources, "\n"))

	hiddenFiles := []problems.File{
		{Path: "test_solution.go", Content: runner},
		{Path: "codegym_comparator.go", Content: execution.GoComparatorSource},
		{Path: ".codegym/compile_and_run.py", Content: execution.GoCompileRunnerSource},
	}
	if usesChecker {
		hiddenFiles = append(hiddenFiles, problems.File{Path: "checker.go", Content: strings.TrimRight(output.Checker, "\n") + "\n"})
	}
	return hiddenFiles, nil
}

func buildGoCaseSource(output GeneratedProblem, test ProblemTestCase, name string, comparator problems.Comparator) (string, error) {
	lines := []string{
		fmt.Sprintf("\t\t{Name: %s, Run: func() (bool, string) {", strconv.Quote(name)),
	}
	argumentNames := make([]string, 0, len(output.Parameters))
	for index, parameter := range output.Parameters {
		argumentName := fmt.Sprintf("argument%d", index)
		argumentNames = append(argumentNames, argumentName)
		lines = append(lines,
			fmt.Sprintf("\t\t\t%s, err := decodeValue[%s](%s)", argumentName, parameter.Type, strconv.Quote(string(test.Args[index]))),
			fmt.Sprintf("\t\t\tif err != nil { return false, fmt.Sprintf(\"decode argument %d: %%v\", err) }", index+1),
		)
	}
	lines = append(lines,
		fmt.Sprintf("\t\t\texpected, err := decodeValue[%s](%s)", output.ReturnType, strconv.Quote(string(test.Expected))),
		"\t\t\tif err != nil { return false, fmt.Sprintf(\"decode expected value: %v\", err) }",
		fmt.Sprintf("\t\t\tactual := %s(%s)", output.FunctionName, strings.Join(argumentNames, ", ")),
	)
	defaultReason := `fmt.Sprintf("expected %s, got %s", codegymJSON(expected), codegymJSON(actual))`
	if comparator.Kind == problems.ComparatorChecker {
		lines = append(lines,
			fmt.Sprintf("\t\t\tequal, reason := check([]any{%s}, actual, expected)", strings.Join(argumentNames, ", ")),
			fmt.Sprintf("\t\t\tif !equal && reason == \"\" { reason = %s }", defaultReason),
			"\t\t\treturn equal, reason",
		)
	} else {
		epsilon := "nil"
		if comparator.Epsilon != nil {
			epsilon = "codegymEpsilon(" + strconv.FormatFloat(*comparator.Epsilon, 'g', -1, 64) + ")"
		}
		lines = append(lines,
			fmt.Sprintf("\t\t\tequal, compareErr := compareValues(%s, expected, actual, %s)", strconv.Quote(string(comparator.Kind)), epsilon),
			"\t\t\tif compareErr != nil { return false, compareErr.Error() }",
			fmt.Sprintf("\t\t\tif !equal { return false, %s }", defaultReason),
			"\t\t\treturn true, \"\"",
		)
	}
	lines = append(lines, "\t\t}},")
	return strings.Join(lines, "\n"), nil
}
