package harness

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

func TestRenderPythonEmitsResultPrefixAndLoadsCases(t *testing.T) {
	rendered, err := Render(validSpec())
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if rendered.Path != "test_solution.py" {
		t.Fatalf("Path = %q", rendered.Path)
	}
	for _, expected := range []string{
		`PREFIX = "CODEGYM_RESULT "`,
		"CASES = json.loads(",
		`importlib.import_module("solution")`,
		`target = getattr(solution, "two_sum")`,
	} {
		if !strings.Contains(rendered.Content, expected) {
			t.Fatalf("rendered harness is missing %q", expected)
		}
	}
}

func TestRenderEncodesCaseDataAsEmbeddedJSONNotPythonLiterals(t *testing.T) {
	spec := validSpec()
	spec.Cases[0].Args = []json.RawMessage{
		json.RawMessage(`["quote\"","line\n","<&>","unicode \u2603",true,null]`),
		json.RawMessage(`9`),
	}
	spec.Cases[0].Expected = json.RawMessage(`{"ok":false}`)

	rendered, err := Render(spec)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	match := regexp.MustCompile(`(?m)^CASES = json\.loads\((.+)\)$`).FindStringSubmatch(rendered.Content)
	if len(match) != 2 {
		t.Fatal("could not find embedded case literal")
	}
	if strings.Contains(match[1], "True") || strings.Contains(match[1], "None") {
		t.Fatalf("case data was rendered as Python literals: %s", match[1])
	}
	var document string
	if err := json.Unmarshal([]byte(match[1]), &document); err != nil {
		t.Fatalf("case literal is not a JSON string: %v", err)
	}
	var decoded []templateCase
	if err := json.Unmarshal([]byte(document), &decoded); err != nil {
		t.Fatalf("embedded document is not JSON: %v", err)
	}
	if len(decoded) != 1 || string(decoded[0].Expected) != `{"ok":false}` {
		t.Fatalf("decoded cases = %#v", decoded)
	}
}

func TestRenderIsDeterministic(t *testing.T) {
	first, err := Render(validSpec())
	if err != nil {
		t.Fatalf("first Render: %v", err)
	}
	second, err := Render(validSpec())
	if err != nil {
		t.Fatalf("second Render: %v", err)
	}
	if first != second {
		t.Fatal("identical specs produced different harnesses")
	}
}

func TestRenderRejectsInvalidSpec(t *testing.T) {
	spec := validSpec()
	spec.Cases[0].Comparator = Comparator("lambda actual, expected: True")
	if _, err := Render(spec); err == nil {
		t.Fatal("Render accepted model-supplied comparator code")
	}
}
