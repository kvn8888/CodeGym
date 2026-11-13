package benchmark

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// ValidateReportJSON checks a serialized report against the shared report
// schema. The schema is a repository artifact, not an embedded copy, so the
// command and contract cannot drift silently.
func ValidateReportJSON(data []byte, schemaPath string) error {
	abs, err := filepath.Abs(schemaPath)
	if err != nil {
		return fmt.Errorf("resolve report schema: %w", err)
	}
	raw, err := os.ReadFile(abs)
	if err != nil {
		return fmt.Errorf("read report schema %q: %w", abs, err)
	}
	var schemaDoc any
	if err := json.Unmarshal(raw, &schemaDoc); err != nil {
		return fmt.Errorf("parse report schema %q: %w", abs, err)
	}
	var instance any
	if err := json.Unmarshal(data, &instance); err != nil {
		return fmt.Errorf("parse benchmark report: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	url := "file://" + filepath.ToSlash(abs)
	if err := compiler.AddResource(url, schemaDoc); err != nil {
		return fmt.Errorf("add report schema: %w", err)
	}
	schema, err := compiler.Compile(url)
	if err != nil {
		return fmt.Errorf("compile report schema: %w", err)
	}
	if err := schema.Validate(instance); err != nil {
		var buf bytes.Buffer
		fmt.Fprintf(&buf, "%v", err)
		return fmt.Errorf("%s", buf.String())
	}
	return nil
}
