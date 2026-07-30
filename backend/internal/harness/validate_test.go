package harness

import (
	"encoding/json"
	"strings"
	"testing"
)

func validSpec() Spec {
	return Spec{
		Language:   LanguagePython,
		Module:     "solution",
		EntryPoint: "two_sum",
		ParamNames: []string{"nums", "target"},
		Cases: []Case{{
			Name:       "basic",
			Args:       []json.RawMessage{json.RawMessage(`[2,7,11,15]`), json.RawMessage(`9`)},
			Expected:   json.RawMessage(`[0,1]`),
			Comparator: ComparatorUnorderedList,
		}},
	}
}

func TestValidateRejectsUnknownComparator(t *testing.T) {
	spec := validSpec()
	spec.Cases[0].Comparator = Comparator(`__import__('os').system('x')`)

	if err := Validate(spec); err == nil || !strings.Contains(err.Error(), "unknown comparator") {
		t.Fatalf("Validate error = %v", err)
	}
}

func TestValidateRejectsNonIdentifierEntryPoint(t *testing.T) {
	for _, entryPoint := range []string{"two_sum; import os", "solution.two_sum", "", "2sum"} {
		t.Run(entryPoint, func(t *testing.T) {
			spec := validSpec()
			spec.EntryPoint = entryPoint
			if err := Validate(spec); err == nil {
				t.Fatalf("Validate accepted entry point %q", entryPoint)
			}
		})
	}
}

func TestValidateRejectsUnknownLanguage(t *testing.T) {
	spec := validSpec()
	spec.Language = Language("javascript")

	if err := Validate(spec); err == nil || !strings.Contains(err.Error(), "unsupported language") {
		t.Fatalf("Validate error = %v", err)
	}
	if got := SupportedLanguages(); len(got) != 1 || got[0] != LanguagePython {
		t.Fatalf("SupportedLanguages = %#v", got)
	}
}

func TestValidateRejectsEmptyAndDuplicateCases(t *testing.T) {
	spec := validSpec()
	spec.Cases = nil
	if err := Validate(spec); err == nil {
		t.Fatal("Validate accepted empty cases")
	}

	spec = validSpec()
	spec.Cases = append(spec.Cases, spec.Cases[0])
	if err := Validate(spec); err == nil || !strings.Contains(err.Error(), "duplicate case") {
		t.Fatalf("Validate duplicate error = %v", err)
	}
}

func TestBindArgs(t *testing.T) {
	args, err := BindArgs(
		[]string{"nums", "target"},
		json.RawMessage(`{"target":9,"nums":[2,7,11,15]}`),
	)
	if err != nil {
		t.Fatalf("BindArgs: %v", err)
	}
	if string(args[0]) != `[2,7,11,15]` || string(args[1]) != `9` {
		t.Fatalf("args = %q, %q", args[0], args[1])
	}

	for _, input := range []string{
		`{"nums":[2,7],"target":9,"extra":true}`,
		`{"nums":[2,7]}`,
		`["not","an","object"]`,
	} {
		if _, bindErr := BindArgs([]string{"nums", "target"}, json.RawMessage(input)); bindErr == nil {
			t.Fatalf("BindArgs accepted %s", input)
		}
	}
}
