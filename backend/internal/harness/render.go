package harness

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"text/template"
)

//go:embed templates/*.tmpl
var templateFiles embed.FS

var pythonTemplate = template.Must(template.ParseFS(templateFiles, "templates/python_unit.py.tmpl"))

type templateCase struct {
	Name       string            `json:"name"`
	Args       []json.RawMessage `json:"args"`
	Expected   json.RawMessage   `json:"expected"`
	Comparator Comparator        `json:"compare"`
}

type templateData struct {
	CasesJSON  string
	Module     string
	EntryPoint string
}

func Render(spec Spec) (Rendered, error) {
	if err := Validate(spec); err != nil {
		return Rendered{}, err
	}

	cases := make([]templateCase, 0, len(spec.Cases))
	for _, testCase := range spec.Cases {
		cases = append(cases, templateCase{
			Name:       testCase.Name,
			Args:       testCase.Args,
			Expected:   testCase.Expected,
			Comparator: testCase.Comparator,
		})
	}
	payload, err := json.Marshal(cases)
	if err != nil {
		return Rendered{}, fmt.Errorf("encode harness cases: %w", err)
	}
	literal, err := json.Marshal(string(payload))
	if err != nil {
		return Rendered{}, fmt.Errorf("encode harness case literal: %w", err)
	}

	data := templateData{
		CasesJSON:  string(literal),
		Module:     spec.Module,
		EntryPoint: spec.EntryPoint,
	}
	var content bytes.Buffer
	switch spec.Language {
	case LanguagePython:
		if err := pythonTemplate.Execute(&content, data); err != nil {
			return Rendered{}, fmt.Errorf("render Python harness: %w", err)
		}
	default:
		return Rendered{}, fmt.Errorf("unsupported language %q", spec.Language)
	}
	return Rendered{Path: "test_solution.py", Content: content.String()}, nil
}
