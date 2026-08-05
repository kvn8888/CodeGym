package generation

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"

	"github.com/kvn8888/codegym/backend/internal/problems"
)

func validateGeneratedHTTPProblem(output GeneratedProblem) (GeneratedProblem, error) {
	if output.Entrypoint != "main.go" {
		return output, errors.New(`http problem entrypoint must be "main.go"`)
	}
	if output.FunctionName != "" || len(output.Parameters) > 0 || output.ReturnType != "" || len(output.TestCases) > 0 || output.Checker != "" {
		return output, errors.New("http problems must not include unit function fields, test_cases, or checker")
	}
	if output.StarterCode == "" {
		return output, errors.New("http problem starter_code is required")
	}
	if len(output.StarterCode) > 12000 {
		return output, errors.New("starter_code exceeds allowed length")
	}
	if len(output.ReferenceSolution) > 12000 {
		return output, errors.New("reference_solution exceeds allowed length")
	}
	if err := validateGoHTTPServerSource(output.StarterCode, "starter_code"); err != nil {
		return output, err
	}
	if err := validateGoHTTPServerSource(output.ReferenceSolution, "reference_solution"); err != nil {
		return output, err
	}

	config, cases, err := problems.NormalizeHTTPCases(problems.TestConfig{
		Strategy:   problems.TestStrategyHTTP,
		Comparator: output.Comparator,
	}, output.HTTPTestCases)
	if err != nil {
		return output, err
	}
	output.Comparator = config.Comparator
	output.HTTPTestCases = cases
	if err := problems.ValidateCaseVisibilityMix(problems.CountHTTPCaseVisibility(cases), 2); err != nil {
		return output, err
	}
	return output, nil
}

func validateGoHTTPServerSource(source, field string) error {
	parsed, err := parser.ParseFile(token.NewFileSet(), field+".go", source, 0)
	if err != nil {
		return fmt.Errorf("%s is not valid Go: %w", field, err)
	}
	if parsed.Name.Name != "main" {
		return fmt.Errorf("%s must use package main", field)
	}
	hasMain := false
	for _, declaration := range parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Recv == nil && function.Name.Name == "main" && function.Type.Params.NumFields() == 0 {
			hasMain = true
			break
		}
	}
	if !hasMain {
		return fmt.Errorf("%s must define func main()", field)
	}

	importsHTTP := false
	for _, imported := range parsed.Imports {
		path, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			return fmt.Errorf("%s has an invalid import", field)
		}
		switch path {
		case "net/http":
			importsHTTP = true
		case "os/exec", "syscall", "unsafe":
			return fmt.Errorf("%s imports forbidden package %q", field, path)
		}
	}
	if !importsHTTP {
		return fmt.Errorf("%s must import net/http", field)
	}

	readsPort := false
	startsServer := false
	ast.Inspect(parsed, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if identifier, ok := selector.X.(*ast.Ident); ok && identifier.Name == "os" &&
			(selector.Sel.Name == "Getenv" || selector.Sel.Name == "LookupEnv") && len(call.Args) > 0 {
			if literal, ok := call.Args[0].(*ast.BasicLit); ok && literal.Kind == token.STRING {
				value, _ := strconv.Unquote(literal.Value)
				readsPort = value == "PORT" || readsPort
			}
		}
		switch selector.Sel.Name {
		case "ListenAndServe", "ListenAndServeTLS", "Serve":
			startsServer = true
		}
		return true
	})
	if !readsPort || !startsServer {
		return fmt.Errorf("%s entrypoint must read PORT and start an HTTP server", field)
	}
	lower := strings.ToLower(source)
	if strings.Contains(lower, "codegym_result") {
		return fmt.Errorf("%s contains forbidden token %q", field, "codegym_result")
	}
	return nil
}
