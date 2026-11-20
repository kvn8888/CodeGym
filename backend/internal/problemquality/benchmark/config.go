package benchmark

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kvn8888/codegym/backend/internal/environmentverify"
	"github.com/kvn8888/codegym/backend/internal/problemquality"
)

const RunConfigFileName = "benchmark-run.json"

// RunConfig binds a local environment artifact and an explicit reference to
// one problem-quality case. It exists because the shared case schema has no
// reference-input location; it is harness configuration, not corpus data.
type RunConfig struct {
	CaseID               string                      `json:"case_id"`
	ArtifactID           string                      `json:"artifact_id"`
	ArtifactVersion      string                      `json:"artifact_version"`
	ArtifactRoot         string                      `json:"artifact_root"`
	ManifestPath         string                      `json:"manifest_path,omitempty"`
	DevelopmentFixture   bool                        `json:"development_fixture,omitempty"`
	Reference            ReferenceConfig             `json:"reference"`
	GenerationProvenance *GenerationProvenanceConfig `json:"generation_provenance,omitempty"`

	Path          string                        `json:"-"`
	Dir           string                        `json:"-"`
	Manifest      environmentverify.Manifest    `json:"-"`
	ReferenceData problemquality.ReferenceInput `json:"-"`
}

// ReferenceConfig names the reference files. Files maps a learner-editable
// artifact path to a source file relative to the run-config file.
type ReferenceConfig struct {
	ID                string            `json:"id"`
	Files             map[string]string `json:"files"`
	ReviewedBy        string            `json:"reviewed_by,omitempty"`
	ReviewedAt        string            `json:"reviewed_at,omitempty"`
	ReviewedReference string            `json:"reviewed_reference,omitempty"`
}

// GenerationProvenanceConfig optionally supplies the saved exercise's known
// generation provenance. The harness never fabricates these values: when the
// field is absent, saved generated exercises report unavailable revisions.
type GenerationProvenanceConfig struct {
	ModelRevision  string                        `json:"model_revision,omitempty"`
	PromptRevision string                        `json:"prompt_revision,omitempty"`
	Config         map[string]any                `json:"config,omitempty"`
	Usage          *GenerationUsage              `json:"usage,omitempty"`
	Observations   *GenerationObservationsConfig `json:"observations,omitempty"`
}

// GenerationObservationsConfig is an optional historical record supplied with
// a saved independently generated exercise. It is evidence about how that
// exercise was produced, not a new model call. The artifact/input digests bind
// the record to the exact saved inputs; a mismatch rejects the run config.
//
// A failure that produced no saved artifact or reference cannot be represented
// here because both identity digests would have to be fabricated. Such evidence
// needs a separate identity path; this configuration records only failures with
// saved content identity.
type GenerationObservationsConfig struct {
	Source                   string `json:"source"`
	ArtifactSHA256           string `json:"artifact_sha256"`
	InputSHA256              string `json:"input_sha256"`
	FirstAttemptAgreement    *bool  `json:"first_attempt_agreement,omitempty"`
	ReconciliationCount      *int   `json:"reconciliation_count,omitempty"`
	ReconciliationDirection  string `json:"reconciliation_direction,omitempty"`
	NonConvergence           *bool  `json:"non_convergence,omitempty"`
	GenerationFailed         *bool  `json:"generation_failed,omitempty"`
	GenerationFailureDetails string `json:"generation_failure_details,omitempty"`
}

func (r *RunConfig) ArtifactManifestPath() string {
	manifestPath := strings.TrimSpace(r.ManifestPath)
	if manifestPath == "" {
		return "manifest.json"
	}
	return manifestPath
}

func (r *RunConfig) ReferenceTargets() []string {
	targets := make([]string, 0, len(r.ReferenceData.Files))
	for target := range r.ReferenceData.Files {
		targets = append(targets, target)
	}
	sort.Strings(targets)
	return targets
}

func decodeRunConfig(data []byte) (*RunConfig, error) {
	var config RunConfig
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("must contain exactly one JSON object")
	}
	if config.GenerationProvenance != nil {
		if err := validateGenerationUsage(config.GenerationProvenance.Usage); err != nil {
			return nil, fmt.Errorf("generation_provenance.usage: %w", err)
		}
		if err := validateGenerationObservations(config.GenerationProvenance.Observations); err != nil {
			return nil, fmt.Errorf("generation_provenance.observations: %w", err)
		}
	}
	if config.GenerationProvenance != nil && config.GenerationProvenance.Observations != nil && config.DevelopmentFixture {
		return nil, errors.New("development fixtures cannot supply generation observations")
	}
	return &config, nil
}

func validateGenerationObservations(observations *GenerationObservationsConfig) error {
	if observations == nil {
		return nil
	}
	observations.Source = strings.TrimSpace(observations.Source)
	if observations.Source == "" {
		return errors.New("source is required")
	}
	var err error
	observations.ArtifactSHA256, err = normalizeSHA256Digest("artifact_sha256", observations.ArtifactSHA256)
	if err != nil {
		return err
	}
	observations.InputSHA256, err = normalizeSHA256Digest("input_sha256", observations.InputSHA256)
	if err != nil {
		return err
	}
	observations.GenerationFailureDetails = strings.TrimSpace(observations.GenerationFailureDetails)
	if observations.GenerationFailed == nil && observations.GenerationFailureDetails != "" {
		return errors.New("generation_failure_details requires generation_failed to be recorded")
	}
	if observations.FirstAttemptAgreement == nil && observations.ReconciliationCount == nil &&
		strings.TrimSpace(observations.ReconciliationDirection) == "" && observations.NonConvergence == nil &&
		observations.GenerationFailed == nil {
		return errors.New("at least one of first_attempt_agreement, reconciliation_count, reconciliation_direction, non_convergence, or generation_failed is required")
	}
	if observations.GenerationFailed != nil {
		if *observations.GenerationFailed && observations.GenerationFailureDetails == "" {
			return errors.New("generation_failure_details is required when generation_failed is true")
		}
		if !*observations.GenerationFailed && observations.GenerationFailureDetails != "" {
			return errors.New("generation_failure_details must be empty when generation_failed is false")
		}
	}
	if observations.ReconciliationCount != nil && *observations.ReconciliationCount < 0 {
		return errors.New("reconciliation_count must be non-negative")
	}
	direction := strings.TrimSpace(observations.ReconciliationDirection)
	if direction != "" {
		switch direction {
		case "reference", "tests", "both":
		default:
			return fmt.Errorf("reconciliation_direction must be reference, tests, or both, got %q", observations.ReconciliationDirection)
		}
		if observations.ReconciliationCount == nil || *observations.ReconciliationCount == 0 {
			return errors.New("reconciliation_direction requires a positive reconciliation_count")
		}
	} else if observations.ReconciliationCount != nil && *observations.ReconciliationCount > 0 {
		return errors.New("reconciliation_direction is required when reconciliation_count is positive")
	}
	observations.ReconciliationDirection = direction
	return nil
}

func normalizeSHA256Digest(field, value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if len(normalized) != 64 {
		return "", fmt.Errorf("%s must be a 64-character SHA-256 hex digest", field)
	}
	for _, character := range normalized {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return "", fmt.Errorf("%s must be a SHA-256 hex digest", field)
		}
	}
	return normalized, nil
}

// LoadRunConfig reads and validates the per-case run configuration. The
// artifact's own manifest remains the identity authority; the config only
// states where that artifact and its explicit reference inputs live.
func LoadRunConfig(path string, c *problemquality.Case) (*RunConfig, error) {
	if c == nil {
		return nil, errors.New("benchmark run config: case is nil")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("benchmark run config: resolve path: %w", err)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("benchmark run config %q: %w", abs, err)
	}
	config, err := decodeRunConfig(data)
	if err != nil {
		return nil, fmt.Errorf("benchmark run config %q: %w", abs, err)
	}
	config.Path = abs
	config.Dir = filepath.Dir(abs)
	if strings.TrimSpace(config.CaseID) != c.CaseID {
		return nil, fmt.Errorf("benchmark run config %q: case_id %q does not match loaded case %q",
			abs, config.CaseID, c.CaseID)
	}
	if strings.TrimSpace(config.ArtifactID) == "" ||
		strings.TrimSpace(config.ArtifactVersion) == "" {
		return nil, fmt.Errorf("benchmark run config %q: artifact_id and artifact_version are required", abs)
	}
	if strings.TrimSpace(config.ArtifactRoot) == "" {
		return nil, fmt.Errorf("benchmark run config %q: artifact_root is required", abs)
	}
	config.ArtifactRoot = resolveRelative(config.Dir, config.ArtifactRoot)
	info, err := os.Stat(config.ArtifactRoot)
	if err != nil {
		return nil, fmt.Errorf("benchmark run config %q: artifact_root: %w", abs, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("benchmark run config %q: artifact_root is not a directory", abs)
	}
	manifestPath, err := resolveInside(config.ArtifactRoot, config.ArtifactManifestPath())
	if err != nil {
		return nil, fmt.Errorf("benchmark run config %q: manifest_path: %w", abs, err)
	}
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("benchmark run config %q: read artifact manifest: %w", abs, err)
	}
	manifest, err := environmentverify.ValidateBytes(manifestData)
	if err != nil {
		return nil, fmt.Errorf("benchmark run config %q: artifact manifest: %w", abs, err)
	}
	config.Manifest = manifest
	if manifest.Artifact == nil {
		return nil, fmt.Errorf("benchmark run config %q: artifact manifest has no artifact identity", abs)
	}
	manifestID := strings.TrimSpace(manifest.Artifact.ID)
	manifestVersion := strings.TrimSpace(manifest.Artifact.Version)
	if manifestID != strings.TrimSpace(config.ArtifactID) ||
		manifestVersion != strings.TrimSpace(config.ArtifactVersion) {
		return nil, fmt.Errorf("benchmark run config %q: artifact manifest is %q version %q, config says %q version %q",
			abs, manifestID, manifestVersion, config.ArtifactID, config.ArtifactVersion)
	}
	if manifestID != strings.TrimSpace(c.ArtifactID) || manifestVersion != strings.TrimSpace(c.ArtifactVersion) {
		return nil, fmt.Errorf("benchmark run config %q: artifact manifest is %q version %q, case binds %q version %q",
			abs, manifestID, manifestVersion, c.ArtifactID, c.ArtifactVersion)
	}
	if !strings.EqualFold(strings.TrimSpace(manifest.Artifact.Technology), strings.TrimSpace(c.Technology)) {
		return nil, fmt.Errorf("benchmark run config %q: artifact technology %q does not match case technology %q",
			abs, manifest.Artifact.Technology, c.Technology)
	}
	if err := commandsMatchCase(manifest, c); err != nil {
		return nil, fmt.Errorf("benchmark run config %q: %w", abs, err)
	}
	if strings.TrimSpace(config.Reference.ID) == "" {
		return nil, fmt.Errorf("benchmark run config %q: reference.id is required", abs)
	}
	if len(config.Reference.Files) == 0 {
		return nil, fmt.Errorf("benchmark run config %q: reference.files must contain at least one file", abs)
	}
	editable := map[string]struct{}{}
	for _, target := range manifest.Workspace.LearnerEditable {
		editable[strings.TrimSpace(target)] = struct{}{}
	}
	resolved := make(map[string]string, len(config.Reference.Files))
	for target, source := range config.Reference.Files {
		target = strings.TrimSpace(target)
		if target == "" {
			return nil, fmt.Errorf("benchmark run config %q: reference file target is blank", abs)
		}
		if _, ok := editable[target]; !ok {
			return nil, fmt.Errorf("benchmark run config %q: reference target %q is not learner-editable", abs, target)
		}
		source = strings.TrimSpace(source)
		if source == "" {
			return nil, fmt.Errorf("benchmark run config %q: reference source for %q is blank", abs, target)
		}
		sourcePath := resolveRelative(config.Dir, source)
		content, err := os.ReadFile(sourcePath)
		if err != nil {
			return nil, fmt.Errorf("benchmark run config %q: read reference %q: %w", abs, sourcePath, err)
		}
		resolved[target] = string(content)
	}
	config.ReferenceData = problemquality.ReferenceInput{
		ID:                strings.TrimSpace(config.Reference.ID),
		Files:             resolved,
		ReviewedBy:        strings.TrimSpace(config.Reference.ReviewedBy),
		ReviewedAt:        strings.TrimSpace(config.Reference.ReviewedAt),
		ReviewedReference: strings.TrimSpace(config.Reference.ReviewedReference),
	}
	return config, nil
}

func commandsMatchCase(manifest environmentverify.Manifest, c *problemquality.Case) error {
	if c.EnvironmentManifestData == nil || manifest.Commands == nil || c.EnvironmentManifestData.Commands == nil {
		return errors.New("artifact manifest commands are missing")
	}
	pairs := []struct {
		name   string
		config string
		cased  string
	}{
		{"setup", manifest.Commands.Setup, c.EnvironmentManifestData.Commands.Setup},
		{"build", manifest.Commands.Build, c.EnvironmentManifestData.Commands.Build},
		{"test", manifest.Commands.Test, c.EnvironmentManifestData.Commands.Test},
		{"run", manifest.Commands.Run, c.EnvironmentManifestData.Commands.Run},
	}
	for _, pair := range pairs {
		if strings.TrimSpace(pair.config) != strings.TrimSpace(pair.cased) {
			return fmt.Errorf("artifact manifest %s command %q does not match case manifest command %q",
				pair.name, pair.config, pair.cased)
		}
	}
	return nil
}

func resolveRelative(base, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(base, filepath.FromSlash(path)))
}

func resolveInside(root, rel string) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	candidate := resolveRelative(root, rel)
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return "", err
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%q escapes the artifact root", rel)
	}
	return candidate, nil
}

func validateGenerationObservationIdentity(config *RunConfig, identity *ContentIdentity, baselineKind string) error {
	if config == nil || config.GenerationProvenance == nil || config.GenerationProvenance.Observations == nil {
		return nil
	}
	if strings.TrimSpace(baselineKind) != "independent" {
		return errors.New("generation observations are supported only for baseline_kind independent")
	}
	if config.DevelopmentFixture {
		return errors.New("development fixtures cannot supply generation observations")
	}
	if identity == nil {
		return errors.New("generation observations require saved input identity")
	}
	observations := config.GenerationProvenance.Observations
	if observations.ArtifactSHA256 != identity.Artifact.SHA256 {
		return fmt.Errorf("generation observations artifact_sha256 %q does not match saved artifact identity %q",
			observations.ArtifactSHA256, identity.Artifact.SHA256)
	}
	if observations.InputSHA256 != identity.Reference.SHA256 {
		return fmt.Errorf("generation observations input_sha256 %q does not match saved reference input identity %q",
			observations.InputSHA256, identity.Reference.SHA256)
	}
	return nil
}
