package harness

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

var identifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func Validate(spec Spec) error {
	if spec.Language != LanguagePython {
		return fmt.Errorf("unsupported language %q", spec.Language)
	}
	if !identifierPattern.MatchString(spec.Module) {
		return fmt.Errorf("module %q is not a valid identifier", spec.Module)
	}
	if !identifierPattern.MatchString(spec.EntryPoint) {
		return fmt.Errorf("entry point %q is not a valid identifier", spec.EntryPoint)
	}
	if len(spec.ParamNames) == 0 {
		return errors.New("parameter names must not be empty")
	}
	seenParams := make(map[string]struct{}, len(spec.ParamNames))
	for _, name := range spec.ParamNames {
		if !identifierPattern.MatchString(name) {
			return fmt.Errorf("parameter name %q is not a valid identifier", name)
		}
		if _, exists := seenParams[name]; exists {
			return fmt.Errorf("duplicate parameter name %q", name)
		}
		seenParams[name] = struct{}{}
	}
	if len(spec.Cases) == 0 {
		return errors.New("cases must not be empty")
	}
	seenCases := make(map[string]struct{}, len(spec.Cases))
	for index, testCase := range spec.Cases {
		name := strings.TrimSpace(testCase.Name)
		if name == "" {
			return fmt.Errorf("case %d has an empty name", index+1)
		}
		if _, exists := seenCases[name]; exists {
			return fmt.Errorf("duplicate case name %q", name)
		}
		seenCases[name] = struct{}{}
		if !knownComparator(testCase.Comparator) {
			return fmt.Errorf("case %q uses unknown comparator %q", name, testCase.Comparator)
		}
		if len(testCase.Args) != len(spec.ParamNames) {
			return fmt.Errorf("case %q has %d arguments, want %d", name, len(testCase.Args), len(spec.ParamNames))
		}
		for argumentIndex, argument := range testCase.Args {
			if len(argument) == 0 || !json.Valid(argument) {
				return fmt.Errorf("case %q argument %d is not valid JSON", name, argumentIndex+1)
			}
		}
		if len(testCase.Expected) == 0 || !json.Valid(testCase.Expected) {
			return fmt.Errorf("case %q expected value is not valid JSON", name)
		}
	}
	return nil
}

func BindArgs(paramNames []string, input json.RawMessage) ([]json.RawMessage, error) {
	if len(paramNames) == 0 {
		return nil, errors.New("parameter names must not be empty")
	}
	var values map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(input))
	if err := decoder.Decode(&values); err != nil {
		return nil, fmt.Errorf("input must be a JSON object: %w", err)
	}
	if values == nil {
		return nil, errors.New("input must be a JSON object")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("input must contain exactly one JSON object")
	}
	if len(values) != len(paramNames) {
		return nil, fmt.Errorf("input has %d fields, want %d", len(values), len(paramNames))
	}
	args := make([]json.RawMessage, 0, len(paramNames))
	seen := make(map[string]struct{}, len(paramNames))
	for _, name := range paramNames {
		if !identifierPattern.MatchString(name) {
			return nil, fmt.Errorf("parameter name %q is not a valid identifier", name)
		}
		if _, duplicate := seen[name]; duplicate {
			return nil, fmt.Errorf("duplicate parameter name %q", name)
		}
		seen[name] = struct{}{}
		value, exists := values[name]
		if !exists {
			return nil, fmt.Errorf("input is missing parameter %q", name)
		}
		args = append(args, append(json.RawMessage(nil), value...))
	}
	return args, nil
}

func knownComparator(comparator Comparator) bool {
	for _, supported := range Comparators() {
		if comparator == supported {
			return true
		}
	}
	return false
}
