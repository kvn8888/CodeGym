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
	ModelRevision  string         `json:"model_revision,omitempty"`
	PromptRevision string         `json:"prompt_revision,omitempty"`
	Config         map[string]any `json:"config,omitempty"`
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
	var config RunConfig
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("benchmark run config %q: %w", abs, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("benchmark run config %q: must contain exactly one JSON object", abs)
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
	return &config, nil
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
