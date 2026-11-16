package benchmark

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kvn8888/codegym/backend/internal/problemquality"
)

// ContentIdentityAlgorithm labels the digest algorithm recorded in a report.
const ContentIdentityAlgorithm = "sha256"

// ContentIdentity pins the inputs a benchmark case ran against. Every path is
// repository- or case-relative; machine-specific absolute paths never enter
// the report. Digests are captured before assessment commands run so a test
// can never rewrite the evidence after the fact.
type ContentIdentity struct {
	Algorithm           string            `json:"algorithm"`
	CaseDefinition      ContentFile       `json:"case_definition"`
	Specification       ContentFile       `json:"specification"`
	RunConfig           ContentFile       `json:"run_config"`
	Reference           SolutionContent   `json:"reference"`
	CorrectAlternatives []SolutionContent `json:"correct_alternatives"`
	WrongSolutions      []SolutionContent `json:"wrong_solutions"`
	Artifact            ArtifactContent   `json:"artifact"`
}

// SolutionContent is one solution's file digests plus a group digest over its
// declared relative paths.
type SolutionContent struct {
	ID     string        `json:"id"`
	Files  []ContentFile `json:"files"`
	SHA256 string        `json:"sha256"`
}

// ArtifactContent is the artifact's manifest plus every manifest-declared
// input file. Generated build output and caches are outside the declared
// scope, so they never change this identity.
type ArtifactContent struct {
	Files  []ContentFile `json:"files"`
	SHA256 string        `json:"sha256"`
}

// ContentFile is one relative path and its SHA-256 digest.
type ContentFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// computeContentIdentity hashes the case, its specification, the run config,
// every solution replacement, and the artifact's manifest-declared inputs.
// Call it before execution: the case loader already read solution contents,
// but the case, spec, config, and artifact files are read here as they stood
// before any assessment command could mutate them.
func computeContentIdentity(c *problemquality.Case, config *RunConfig) (*ContentIdentity, error) {
	if c == nil {
		return nil, errors.New("content identity: case is nil")
	}
	if config == nil {
		return nil, errors.New("content identity: run config is nil")
	}
	caseFile, err := contentFileAt(c.Dir, filepath.Join(c.Dir, problemquality.CaseFileName))
	if err != nil {
		return nil, fmt.Errorf("content identity: case definition: %w", err)
	}
	specFile, err := specificationContent(c)
	if err != nil {
		return nil, fmt.Errorf("content identity: specification: %w", err)
	}
	configFile, err := contentFileAt(c.Dir, config.Path)
	if err != nil {
		return nil, fmt.Errorf("content identity: run config: %w", err)
	}
	reference := solutionContentFromContents(config.ReferenceData.ID, referenceRelativePaths(config), config.ReferenceData.Files)
	correct := make([]SolutionContent, 0, len(c.CorrectAlternatives))
	for _, solution := range c.CorrectAlternatives {
		correct = append(correct, solutionContentFromResolved(solution.ID, solution.ResolvedFiles))
	}
	wrong := make([]SolutionContent, 0, len(c.WrongSolutions))
	for _, solution := range c.WrongSolutions {
		wrong = append(wrong, solutionContentFromResolved(solution.ID, solution.ResolvedFiles))
	}
	artifact, err := artifactContent(config)
	if err != nil {
		return nil, fmt.Errorf("content identity: artifact: %w", err)
	}
	return &ContentIdentity{
		Algorithm:           ContentIdentityAlgorithm,
		CaseDefinition:      caseFile,
		Specification:       specFile,
		RunConfig:           configFile,
		Reference:           reference,
		CorrectAlternatives: correct,
		WrongSolutions:      wrong,
		Artifact:            artifact,
	}, nil
}

func specificationContent(c *problemquality.Case) (ContentFile, error) {
	if c.SpecInline {
		return contentFile("case.yaml#spec.path_or_inline", []byte(c.Spec.PathOrInline)), nil
	}
	if strings.TrimSpace(c.SpecPath) == "" {
		return ContentFile{}, errors.New("referenced specification path is empty")
	}
	return contentFileAt(c.Dir, c.SpecPath)
}

func contentFileAt(base, path string) (ContentFile, error) {
	relative, err := filepath.Rel(base, path)
	if err != nil {
		return ContentFile{}, fmt.Errorf("resolve %q relative to %q: %w", path, base, err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return ContentFile{}, fmt.Errorf("%q is outside %q", path, base)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ContentFile{}, fmt.Errorf("read %q: %w", relative, err)
	}
	return contentFile(relative, data), nil
}

func contentFile(path string, content []byte) ContentFile {
	return ContentFile{
		Path:   filepath.ToSlash(filepath.Clean(filepath.FromSlash(path))),
		SHA256: digestBytes(content),
	}
}

func digestBytes(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func groupDigest(files []ContentFile) string {
	sorted := append([]ContentFile(nil), files...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	hash := sha256.New()
	for _, file := range sorted {
		_, _ = hash.Write([]byte(file.Path))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(file.SHA256))
		_, _ = hash.Write([]byte{'\n'})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func solutionContentFromResolved(id string, files map[string]problemquality.ResolvedFile) SolutionContent {
	contents := make(map[string]string, len(files))
	paths := make(map[string]string, len(files))
	for target, file := range files {
		contents[target] = file.Content
		paths[target] = file.Source
	}
	return solutionContentFromContents(id, paths, contents)
}

func solutionContentFromContents(id string, paths, contents map[string]string) SolutionContent {
	files := make([]ContentFile, 0, len(contents))
	for target, content := range contents {
		path := paths[target]
		if strings.TrimSpace(path) == "" {
			path = target
		}
		files = append(files, contentFile(path, []byte(content)))
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return SolutionContent{ID: id, Files: files, SHA256: groupDigest(files)}
}

func referenceRelativePaths(config *RunConfig) map[string]string {
	paths := make(map[string]string, len(config.Reference.Files))
	for target, source := range config.Reference.Files {
		path := strings.TrimSpace(source)
		if filepath.IsAbs(path) {
			if relative, err := filepath.Rel(config.Dir, path); err == nil {
				path = relative
			} else {
				path = filepath.Base(path)
			}
		}
		paths[target] = filepath.ToSlash(path)
	}
	return paths
}

func artifactContent(config *RunConfig) (ArtifactContent, error) {
	declared := map[string]struct{}{
		config.ArtifactManifestPath(): {},
	}
	manifest := config.Manifest
	if manifest.Environment != nil {
		addArtifactPaths(declared, manifest.Environment.DependencyFiles)
		addArtifactPaths(declared, manifest.Environment.Lockfiles)
	}
	if manifest.Workspace != nil {
		addArtifactPaths(declared, manifest.Workspace.LearnerEditable)
		addArtifactPaths(declared, manifest.Workspace.Protected)
	}
	if manifest.Assessment != nil {
		addArtifactPaths(declared, manifest.Assessment.VisibleTests)
		addArtifactPaths(declared, manifest.Assessment.HiddenTests)
	}
	paths := make([]string, 0, len(declared))
	for path := range declared {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	files := make([]ContentFile, 0, len(paths))
	for _, path := range paths {
		absolute, err := resolveInside(config.ArtifactRoot, path)
		if err != nil {
			return ArtifactContent{}, fmt.Errorf("artifact input %q: %w", path, err)
		}
		info, err := os.Stat(absolute)
		if err != nil {
			return ArtifactContent{}, fmt.Errorf("artifact input %q: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			return ArtifactContent{}, fmt.Errorf("artifact input %q is not a regular file", path)
		}
		data, err := os.ReadFile(absolute)
		if err != nil {
			return ArtifactContent{}, fmt.Errorf("artifact input %q: %w", path, err)
		}
		files = append(files, contentFile(path, data))
	}
	return ArtifactContent{Files: files, SHA256: groupDigest(files)}, nil
}

func addArtifactPaths(paths map[string]struct{}, values []string) {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		paths[filepath.ToSlash(filepath.Clean(filepath.FromSlash(trimmed)))] = struct{}{}
	}
}
