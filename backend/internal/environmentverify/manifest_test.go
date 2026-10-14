package environmentverify

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadExample(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("..", "..", "..", "docs", "examples", "environment-contract", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read example %s: %v", name, err)
	}
	return data
}

// mustValidManifest returns a freshly parsed valid manifest. Sections are
// pointers, so each caller gets its own decode and can mutate safely.
func mustValidManifest(t *testing.T) Manifest {
	t.Helper()
	manifest, err := ValidateBytes(loadExample(t, "valid-manifest.json"))
	if err != nil {
		t.Fatalf("ValidateBytes(valid-manifest): %v", err)
	}
	return manifest
}

func validExampleObject(t *testing.T) map[string]any {
	t.Helper()
	var object map[string]any
	if err := json.Unmarshal(loadExample(t, "valid-manifest.json"), &object); err != nil {
		t.Fatalf("unmarshal valid-manifest: %v", err)
	}
	return object
}

func marshalObject(t *testing.T, object map[string]any) []byte {
	t.Helper()
	raw, err := json.Marshal(object)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return raw
}

func sectionMap(t *testing.T, object map[string]any, section string) map[string]any {
	t.Helper()
	nested, ok := object[section].(map[string]any)
	if !ok {
		t.Fatalf("example section %q is not an object", section)
	}
	return nested
}

func requireValidationError(t *testing.T, payload []byte, wantField string) error {
	t.Helper()
	_, err := ValidateBytes(payload)
	if err == nil {
		t.Fatalf("ValidateBytes = nil, want error identifying %q", wantField)
	}
	if !strings.Contains(err.Error(), wantField) {
		t.Fatalf("ValidateBytes error = %q, want it to identify %q", err.Error(), wantField)
	}
	return err
}

func TestValidateValidManifestExample(t *testing.T) {
	manifest := mustValidManifest(t)
	if manifest.SchemaVersion != SupportedManifestSchemaVersion {
		t.Fatalf("schemaVersion = %q, want %q", manifest.SchemaVersion, SupportedManifestSchemaVersion)
	}
	if strings.TrimSpace(manifest.Commands.Run) == "" {
		t.Fatalf("valid example should include a run command")
	}
	// The valid example ships an explicit empty lockfiles array; empty
	// dependency sets must validate.
	if manifest.Environment.Lockfiles == nil {
		t.Fatalf("valid example lockfiles should decode as present")
	}
}

func TestValidateIncompleteManifestExample(t *testing.T) {
	data := loadExample(t, "malformed-manifest.json")
	if _, err := ValidateBytes(data); err == nil {
		t.Fatalf("ValidateBytes(malformed-manifest) = nil, want structural error")
	}
	// The incomplete example parses (supported version) but fails validation.
	parsed, err := ParseManifest(data)
	if err != nil {
		t.Fatalf("ParseManifest(malformed-manifest): %v", err)
	}
	if err := Validate(parsed); err == nil {
		t.Fatalf("Validate(malformed-manifest) = nil, want structural error")
	} else if !strings.Contains(err.Error(), "artifact.id") {
		t.Fatalf("Validate(malformed-manifest) = %q, want it to identify artifact.id", err.Error())
	}
}

func TestParseInvalidJSON(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		want    string
	}{
		{"empty", "", "environment manifest is empty"},
		{"whitespace", "  \n", "environment manifest is empty"},
		{"truncated", "{", "invalid JSON"},
		{"cut off", `{"schemaVersion":`, "invalid JSON"},
	}
	for _, tc := range cases {
		if _, err := ParseManifest([]byte(tc.payload)); err == nil {
			t.Fatalf("ParseManifest(%s) = nil, want error", tc.name)
		} else if !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("ParseManifest(%s) = %q, want %q", tc.name, err.Error(), tc.want)
		}
		if _, err := ValidateBytes([]byte(tc.payload)); err == nil {
			t.Fatalf("ValidateBytes(%s) = nil, want error", tc.name)
		}
	}
}

func TestParseMultipleJSONValues(t *testing.T) {
	valid := loadExample(t, "valid-manifest.json")
	payloads := map[string][]byte{
		"trailing object": append(append([]byte{}, valid...), []byte("\n{}")...),
		"duplicated":      append(append([]byte{}, valid...), valid...),
	}
	for name, payload := range payloads {
		if _, err := ParseManifest(payload); err == nil {
			t.Fatalf("ParseManifest(%s) = nil, want multiple-value error", name)
		} else if !strings.Contains(err.Error(), "exactly one JSON object") {
			t.Fatalf("ParseManifest(%s) = %q, want single-object error", name, err.Error())
		}
		if _, err := ValidateBytes(payload); err == nil {
			t.Fatalf("ValidateBytes(%s) = nil, want multiple-value error", name)
		}
	}
}

func TestUnsupportedSchemaVersion(t *testing.T) {
	for _, version := range []string{"", "environment.manifest.v2", "environment.manifest.v0", "environment.builder-result.v1"} {
		manifest := mustValidManifest(t)
		manifest.SchemaVersion = version
		raw, err := json.Marshal(manifest)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if _, err := ParseManifest(raw); err == nil {
			t.Fatalf("ParseManifest(schemaVersion %q) = nil, want unsupported-version error", version)
		} else if !strings.Contains(err.Error(), "schemaVersion") {
			t.Fatalf("ParseManifest(schemaVersion %q) = %q, want it to identify schemaVersion", version, err.Error())
		}
		if err := Validate(manifest); err == nil {
			t.Fatalf("Validate(schemaVersion %q) = nil, want unsupported-version error", version)
		}
	}
}

func TestOmittedTopLevelSections(t *testing.T) {
	for _, section := range []string{"artifact", "environment", "workspace", "commands", "assessment", "compatibility"} {
		object := validExampleObject(t)
		delete(object, section)
		requireValidationError(t, marshalObject(t, object), section)

		nulled := validExampleObject(t)
		nulled[section] = nil
		requireValidationError(t, marshalObject(t, nulled), section)
	}
}

func TestOmittedOrBlankFields(t *testing.T) {
	cases := []struct {
		name    string
		section string
		field   string
	}{
		{"artifact id", "artifact", "id"},
		{"artifact technology", "artifact", "technology"},
		{"artifact version", "artifact", "version"},
		{"environment base", "environment", "base"},
		{"environment network", "environment", "network"},
		{"workspace root", "workspace", "root"},
		{"commands setup", "commands", "setup"},
		{"commands build", "commands", "build"},
		{"commands test", "commands", "test"},
		{"assessment type", "assessment", "type"},
		{"compatibility technology", "compatibility", "requestedTechnology"},
		{"compatibility objective", "compatibility", "requestedObjective"},
	}
	for _, tc := range cases {
		want := tc.section + "." + tc.field
		omitted := validExampleObject(t)
		delete(sectionMap(t, omitted, tc.section), tc.field)
		requireValidationError(t, marshalObject(t, omitted), want)

		blank := validExampleObject(t)
		sectionMap(t, blank, tc.section)[tc.field] = "   "
		requireValidationError(t, marshalObject(t, blank), want)
	}
}

func TestExplicitEmptyArraysPermitted(t *testing.T) {
	// Dependency files, lockfiles, reuse tags, and visible tests may be
	// explicitly empty: a project without external dependencies is
	// legitimate, reuse matching is a later registry concern, and an
	// assessment may be hidden-only.
	object := validExampleObject(t)
	sectionMap(t, object, "environment")["dependencyFiles"] = []any{}
	sectionMap(t, object, "environment")["lockfiles"] = []any{}
	sectionMap(t, object, "assessment")["visibleTests"] = []any{}
	sectionMap(t, object, "compatibility")["reuseTags"] = []any{}
	if _, err := ValidateBytes(marshalObject(t, object)); err != nil {
		t.Fatalf("ValidateBytes(empty dependencyFiles/lockfiles/visibleTests/reuseTags): %v", err)
	}
}

func TestOmittedArraysRejected(t *testing.T) {
	// Even where an empty set is permitted, the field itself must be
	// present: omission (nil) is a different claim from "none".
	cases := []struct {
		section string
		field   string
	}{
		{"environment", "dependencyFiles"},
		{"environment", "lockfiles"},
		{"workspace", "learnerEditable"},
		{"workspace", "protected"},
		{"assessment", "visibleTests"},
		{"assessment", "hiddenTests"},
		{"compatibility", "reuseTags"},
	}
	for _, tc := range cases {
		object := validExampleObject(t)
		delete(sectionMap(t, object, tc.section), tc.field)
		requireValidationError(t, marshalObject(t, object), tc.section+"."+tc.field)
	}
}

func TestNonEmptyArraysRequired(t *testing.T) {
	// learnerEditable, protected, and hiddenTests carry the exercise and
	// its hidden boundary, so an explicit empty set is rejected. Each entry
	// must also be non-blank. (visibleTests is intentionally absent here:
	// hidden-only assessments are accepted.)
	for _, tc := range []struct {
		section string
		field   string
	}{
		{"workspace", "learnerEditable"},
		{"workspace", "protected"},
		{"assessment", "hiddenTests"},
	} {
		want := tc.section + "." + tc.field
		empty := validExampleObject(t)
		sectionMap(t, empty, tc.section)[tc.field] = []any{}
		requireValidationError(t, marshalObject(t, empty), want)

		blank := validExampleObject(t)
		sectionMap(t, blank, tc.section)[tc.field] = []any{"  "}
		requireValidationError(t, marshalObject(t, blank), want)
	}

	blankDep := validExampleObject(t)
	sectionMap(t, blankDep, "environment")["dependencyFiles"] = []any{"pom.xml", ""}
	requireValidationError(t, marshalObject(t, blankDep), "environment.dependencyFiles")
}

func TestMissingRequiredCommands(t *testing.T) {
	for _, field := range []string{"setup", "build", "test"} {
		manifest := mustValidManifest(t)
		switch field {
		case "setup":
			manifest.Commands.Setup = "  "
		case "build":
			manifest.Commands.Build = ""
		case "test":
			manifest.Commands.Test = ""
		}
		if err := Validate(manifest); err == nil {
			t.Fatalf("Validate(missing %s command) = nil, want error", field)
		} else if !strings.Contains(err.Error(), "commands."+field) {
			t.Fatalf("Validate(missing %s command) = %q, want field path", field, err.Error())
		}
	}
}

func TestRunCommandIsOptional(t *testing.T) {
	manifest := mustValidManifest(t)
	manifest.Commands.Run = ""
	if err := Validate(manifest); err != nil {
		t.Fatalf("Validate(empty run command): %v", err)
	}
	// A manifest without the run key must still validate.
	object := validExampleObject(t)
	delete(sectionMap(t, object, "commands"), "run")
	if _, err := ValidateBytes(marshalObject(t, object)); err != nil {
		t.Fatalf("ValidateBytes(missing run command): %v", err)
	}
}

func TestEditableProtectedOverlap(t *testing.T) {
	manifest := mustValidManifest(t)
	overlap := "src/test/java/com/codegym/exercise/GreetingServiceTest.java"
	manifest.Workspace.LearnerEditable = append(manifest.Workspace.LearnerEditable, overlap)
	err := Validate(manifest)
	if err == nil {
		t.Fatalf("Validate(editable/protected overlap) = nil, want error")
	}
	if !strings.Contains(err.Error(), overlap) || !strings.Contains(err.Error(), "learner-editable and protected") {
		t.Fatalf("Validate(editable/protected overlap) = %q, want file and boundary", err.Error())
	}
}

// Successful structural validation must not establish readiness or allow
// promotion. The manifest carries no verdict/ready/promotable state, and this
// package exposes no API that grants either.
func TestValidationDoesNotGrantReadinessOrPromotion(t *testing.T) {
	manifest := mustValidManifest(t)
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, forbidden := range []string{"ready", "verified", "promotable", "verdict", "promotion"} {
		if _, ok := object[forbidden]; ok {
			t.Fatalf("validated manifest must not include %q", forbidden)
		}
	}
}
