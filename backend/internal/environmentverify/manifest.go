package environmentverify

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// SupportedManifestSchemaVersion is the only environment manifest contract
// version this verifier accepts. See docs/environment-contract.md.
const SupportedManifestSchemaVersion = "environment.manifest.v1"

// Manifest is the builder-proposed description of a generated exercise
// environment. It carries no readiness or promotion state: a validated
// manifest is still unverified until backend-controlled checks run.
//
// Sections are pointers so that an omitted section (or an explicit null)
// decodes to nil and is reported as missing. This keeps required-field
// presence separate from nonempty-value rules: a nil slice means the field
// was omitted, while an explicit [] is present but empty and is accepted
// only where the contract permits it (see Validate).
type Manifest struct {
	SchemaVersion string         `json:"schemaVersion"`
	Artifact      *Artifact      `json:"artifact"`
	Environment   *Environment   `json:"environment"`
	Workspace     *Workspace     `json:"workspace"`
	Commands      *Commands      `json:"commands"`
	Assessment    *Assessment    `json:"assessment"`
	Compatibility *Compatibility `json:"compatibility"`
}

// Artifact identifies the prepared exercise and its technology.
type Artifact struct {
	ID         string `json:"id"`
	Technology string `json:"technology"`
	Version    string `json:"version"`
}

// Environment describes the runtime base, network mode, and dependency
// inputs the verifier must be able to reproduce.
type Environment struct {
	Base            string   `json:"base"`
	Network         string   `json:"network"`
	DependencyFiles []string `json:"dependencyFiles"`
	Lockfiles       []string `json:"lockfiles"`
}

// Workspace describes the learner-visible root, editable files, and files
// that must stay protected from learner edits.
type Workspace struct {
	Root            string   `json:"root"`
	LearnerEditable []string `json:"learnerEditable"`
	Protected       []string `json:"protected"`
}

// Commands are the builder-proposed setup/build/test steps the verifier
// executes. Run stays optional: the contract lists "Setup, build, test, and
// optional run commands", so its absence must not fail structural validation.
type Commands struct {
	Setup string `json:"setup"`
	Build string `json:"build"`
	Test  string `json:"test"`
	Run   string `json:"run,omitempty"`
}

// Assessment references the visible and hidden test artifacts.
type Assessment struct {
	Type         string   `json:"type"`
	VisibleTests []string `json:"visibleTests"`
	HiddenTests  []string `json:"hiddenTests"`
}

// Compatibility carries the original request metadata and reuse tags.
// Registry reuse rules are validated later, outside this package.
type Compatibility struct {
	RequestedTechnology string   `json:"requestedTechnology"`
	RequestedObjective  string   `json:"requestedObjective"`
	ReuseTags           []string `json:"reuseTags"`
}

// ParseManifest decodes one JSON manifest object and checks that its
// schema version is supported. It does not check required sections or
// fields; use Validate for structural validation or ValidateBytes for both.
func ParseManifest(data []byte) (Manifest, error) {
	var manifest Manifest
	if len(bytes.TrimSpace(data)) == 0 {
		return Manifest{}, errors.New("environment manifest is empty")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("environment manifest: invalid JSON: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Manifest{}, errors.New("environment manifest: must contain exactly one JSON object")
	}
	if strings.TrimSpace(manifest.SchemaVersion) == "" {
		return Manifest{}, errors.New("environment manifest: schemaVersion is required")
	}
	if strings.TrimSpace(manifest.SchemaVersion) != SupportedManifestSchemaVersion {
		return Manifest{}, fmt.Errorf("environment manifest: unsupported schemaVersion %q", manifest.SchemaVersion)
	}
	return manifest, nil
}

// Validate checks the structural contract for an already-parsed manifest:
// supported schema version, presence of every required top-level section,
// and the required fields within each section.
//
// Presence versus nonempty: a nil section or nil array means the field was
// omitted (or explicitly null) and is rejected as required. An explicit []
// is accepted for dependencyFiles, lockfiles, reuseTags, and visibleTests:
// a project can have no external dependencies, reuse tags are validated by
// the registry later, and an assessment may be hidden-only. Entries must be
// non-blank when present. learnerEditable, protected, and hiddenTests must
// each hold at least one entry: without an editable file there is nothing
// to practice, and without protected or hidden references there is no
// hidden assessment boundary. That remaining cardinality is inferred from
// the valid example and the promotion rules, not stated verbatim by the
// contract, and stays a pending shared decision; see doc.go.
//
// The run command stays optional. A nil error means the manifest is
// well-formed, not that the environment is ready or promotable.
func Validate(manifest Manifest) error {
	if strings.TrimSpace(manifest.SchemaVersion) != SupportedManifestSchemaVersion {
		return fmt.Errorf("environment manifest: unsupported schemaVersion %q", manifest.SchemaVersion)
	}
	if manifest.Artifact == nil {
		return errors.New("environment manifest: artifact is required")
	}
	if strings.TrimSpace(manifest.Artifact.ID) == "" {
		return errors.New("environment manifest: artifact.id is required")
	}
	if strings.TrimSpace(manifest.Artifact.Technology) == "" {
		return errors.New("environment manifest: artifact.technology is required")
	}
	if strings.TrimSpace(manifest.Artifact.Version) == "" {
		return errors.New("environment manifest: artifact.version is required")
	}
	if manifest.Environment == nil {
		return errors.New("environment manifest: environment is required")
	}
	if strings.TrimSpace(manifest.Environment.Base) == "" {
		return errors.New("environment manifest: environment.base is required")
	}
	if strings.TrimSpace(manifest.Environment.Network) == "" {
		return errors.New("environment manifest: environment.network is required")
	}
	// AMBIGUITY: the contract names dependency files and lockfiles without
	// stating cardinality, and the promotion rule says "files or lockfiles".
	// Both may be explicitly empty (stdlib-only exercises exist); presence
	// plus non-blank entries is all milestone 1 enforces.
	if err := requirePresentStrings(manifest.Environment.DependencyFiles, "environment.dependencyFiles"); err != nil {
		return err
	}
	if err := requirePresentStrings(manifest.Environment.Lockfiles, "environment.lockfiles"); err != nil {
		return err
	}
	if manifest.Workspace == nil {
		return errors.New("environment manifest: workspace is required")
	}
	if strings.TrimSpace(manifest.Workspace.Root) == "" {
		return errors.New("environment manifest: workspace.root is required")
	}
	if err := requireNonEmptyStrings(manifest.Workspace.LearnerEditable, "workspace.learnerEditable"); err != nil {
		return err
	}
	if err := requireNonEmptyStrings(manifest.Workspace.Protected, "workspace.protected"); err != nil {
		return err
	}
	if overlap := editableProtectedOverlap(*manifest.Workspace); overlap != "" {
		return fmt.Errorf("environment manifest: workspace file %q must not be both learner-editable and protected", overlap)
	}
	if manifest.Commands == nil {
		return errors.New("environment manifest: commands is required")
	}
	if strings.TrimSpace(manifest.Commands.Setup) == "" {
		return errors.New("environment manifest: commands.setup is required")
	}
	if strings.TrimSpace(manifest.Commands.Build) == "" {
		return errors.New("environment manifest: commands.build is required")
	}
	if strings.TrimSpace(manifest.Commands.Test) == "" {
		return errors.New("environment manifest: commands.test is required")
	}
	if manifest.Assessment == nil {
		return errors.New("environment manifest: assessment is required")
	}
	if strings.TrimSpace(manifest.Assessment.Type) == "" {
		return errors.New("environment manifest: assessment.type is required")
	}
	if err := requirePresentStrings(manifest.Assessment.VisibleTests, "assessment.visibleTests"); err != nil {
		return err
	}
	if err := requireNonEmptyStrings(manifest.Assessment.HiddenTests, "assessment.hiddenTests"); err != nil {
		return err
	}
	if manifest.Compatibility == nil {
		return errors.New("environment manifest: compatibility is required")
	}
	if strings.TrimSpace(manifest.Compatibility.RequestedTechnology) == "" {
		return errors.New("environment manifest: compatibility.requestedTechnology is required")
	}
	if strings.TrimSpace(manifest.Compatibility.RequestedObjective) == "" {
		return errors.New("environment manifest: compatibility.requestedObjective is required")
	}
	// AMBIGUITY: the contract lists reuse tags for later registry
	// validation without stating whether they may be empty. Presence is
	// required; an explicit empty set is accepted for now.
	if err := requirePresentStrings(manifest.Compatibility.ReuseTags, "compatibility.reuseTags"); err != nil {
		return err
	}
	return nil
}

// ValidateBytes parses and structurally validates a raw manifest payload.
func ValidateBytes(data []byte) (Manifest, error) {
	manifest, err := ParseManifest(data)
	if err != nil {
		return Manifest{}, err
	}
	if err := Validate(manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

// requirePresentStrings rejects an omitted (nil) array but accepts an
// explicit empty array. Present entries must be non-blank.
func requirePresentStrings(values []string, field string) error {
	if values == nil {
		return fmt.Errorf("environment manifest: %s is required", field)
	}
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("environment manifest: %s must not contain empty entries", field)
		}
	}
	return nil
}

// requireNonEmptyStrings rejects an omitted or empty array and any blank
// entry. A missing key decodes to nil, so omission reports the same
// required-field error as an explicit null.
func requireNonEmptyStrings(values []string, field string) error {
	if values == nil {
		return fmt.Errorf("environment manifest: %s is required", field)
	}
	if len(values) == 0 {
		return fmt.Errorf("environment manifest: %s must contain at least one entry", field)
	}
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("environment manifest: %s must not contain empty entries", field)
		}
	}
	return nil
}

func editableProtectedOverlap(workspace Workspace) string {
	protected := make(map[string]struct{}, len(workspace.Protected))
	for _, file := range workspace.Protected {
		protected[strings.TrimSpace(file)] = struct{}{}
	}
	for _, file := range workspace.LearnerEditable {
		if _, ok := protected[strings.TrimSpace(file)]; ok {
			return strings.TrimSpace(file)
		}
	}
	return ""
}
