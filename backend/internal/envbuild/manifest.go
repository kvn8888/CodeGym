package envbuild

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// knownDependencyFiles and knownLockfiles are detection data, not technology
// branches: files present in the workspace are listed, whatever the
// technology. The uniform rule never names a technology; only these rows do.
var knownDependencyFiles = []string{
	"pom.xml",
	"package.json",
	"go.mod",
	"requirements.txt",
	"pyproject.toml",
	"Cargo.toml",
	"Gemfile",
}

var knownLockfiles = []string{
	"package-lock.json",
	"yarn.lock",
	"pnpm-lock.yaml",
	"go.sum",
	"Cargo.lock",
	"Gemfile.lock",
	"poetry.lock",
}

// skippedWorkspaceDirs are materialized or version-control directories that
// are never learner files, whatever the technology.
var skippedWorkspaceDirs = []string{
	".git",
	"node_modules",
	"target",
	"dist",
	"build",
	"__pycache__",
	".venv",
}

// fallbackCommands is last-resort data for agents that declare no commands.
// Rows are keyed by dependency filename, applied uniformly; an agent that
// declares commands never touches this table, so a new technology needs no
// entry to travel the generic path.
var fallbackCommands = map[string]ManifestCommands{
	"pom.xml": {
		Setup: "mvn -q dependency:go-offline",
		Build: "mvn -q test-compile",
		Test:  "mvn -q test",
	},
	"package.json": {
		Setup: "npm install",
		Build: "npm run build",
		Test:  "npm test",
		Run:   "npm start",
	},
	"go.mod": {
		Setup: "go mod download",
		Build: "go build ./...",
		Test:  "go test ./...",
	},
}

// declaredCommands is the agent's optional command declaration at
// .codegym/commands.json. Declared values always win over the fallback table.
type declaredCommands struct {
	Setup string `json:"setup"`
	Build string `json:"build"`
	Test  string `json:"test"`
	Run   string `json:"run"`
}

// isTestShaped reports the file-naming and directory conventions that mark
// assessment files. Conventions only; no language or framework is named.
func isTestShaped(relative string) bool {
	parts := strings.Split(filepath.ToSlash(relative), "/")
	for _, part := range parts[:len(parts)-1] {
		if part == "test" || part == "tests" || part == "__tests__" {
			return true
		}
	}
	base := parts[len(parts)-1]
	lower := strings.ToLower(base)
	ext := strings.ToLower(filepath.Ext(base))
	name := strings.TrimSuffix(lower, ext)
	return strings.Contains(lower, ".test.") ||
		strings.HasPrefix(name, "test_") ||
		strings.HasSuffix(name, "_test") ||
		strings.HasPrefix(base, "Test")
}

// splitWorkspace lists workspace files as learner-editable or protected.
// Protected covers test-shaped files and .codegym internals (hidden tests and
// reference material must never be learner-editable); everything else is
// editable. Directory walk order is lexical, so output is deterministic.
func splitWorkspace(root string) (editable, protected []string, err error) {
	editable = []string{}
	protected = []string{}
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		slash := filepath.ToSlash(relative)
		first := strings.Split(slash, "/")[0]
		for _, skipped := range skippedWorkspaceDirs {
			if first == skipped {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		if entry.IsDir() {
			return nil
		}
		if strings.HasPrefix(slash, ".codegym/") || slash == ".codegym" {
			if slash == ".codegym/commands.json" {
				return nil
			}
			protected = append(protected, slash)
			return nil
		}
		if isTestShaped(slash) {
			protected = append(protected, slash)
			return nil
		}
		editable = append(editable, slash)
		return nil
	})
	return editable, protected, err
}

// DraftManifest assembles the proposed versioned manifest from a prepared
// workspace. It fabricates nothing: an empty workspace yields diagnostics and
// an error, never a manifest.
func DraftManifest(request BuildRequest, root string) (*EnvironmentManifest, []Diagnostic, error) {
	editable, protected, err := splitWorkspace(root)
	if err != nil {
		return nil, nil, err
	}
	dependencyFiles := []string{}
	lockfiles := []string{}
	all := append(append([]string{}, editable...), protected...)
	for _, file := range all {
		base := filepath.Base(file)
		for _, known := range knownDependencyFiles {
			if base == known {
				dependencyFiles = append(dependencyFiles, file)
			}
		}
		for _, known := range knownLockfiles {
			if base == known {
				lockfiles = append(lockfiles, file)
			}
		}
	}
	declared := declaredCommands{}
	declaredPath := filepath.Join(root, ".codegym", "commands.json")
	if payload, err := os.ReadFile(declaredPath); err == nil {
		_ = json.Unmarshal(payload, &declared)
	}
	// Dependency files and lockfiles are build-owned: listed for the
	// verifier, editable by nobody. This mirrors the contract example, where
	// the dependency file appears in neither the editable nor the protected
	// list.
	buildOwned := map[string]bool{}
	for _, file := range dependencyFiles {
		buildOwned[file] = true
	}
	for _, file := range lockfiles {
		buildOwned[file] = true
	}
	learnerEditable := []string{}
	for _, file := range editable {
		if !buildOwned[file] {
			learnerEditable = append(learnerEditable, file)
		}
	}
	if len(learnerEditable) == 0 && len(protected) == 0 {
		return nil, []Diagnostic{{
			Severity: "error",
			Stage:    "prepare",
			Code:     "NO_PROJECT_FILES",
			Message:  "preparation produced no exercise files; refusing to draft a manifest",
		}}, errors.New("envbuild: preparation produced no exercise files")
	}
	commands := ManifestCommands{
		Setup: declared.Setup,
		Build: declared.Build,
		Test:  declared.Test,
		Run:   declared.Run,
	}
	diagnostics := []Diagnostic{}
	if commands.Setup == "" || commands.Build == "" || commands.Test == "" {
		for _, file := range dependencyFiles {
			if fallback, ok := fallbackCommands[filepath.Base(file)]; ok {
				if commands.Setup == "" {
					commands.Setup = fallback.Setup
				}
				if commands.Build == "" {
					commands.Build = fallback.Build
				}
				if commands.Test == "" {
					commands.Test = fallback.Test
				}
				if commands.Run == "" {
					commands.Run = fallback.Run
				}
			}
		}
		if commands.Setup != "" || commands.Build != "" || commands.Test != "" {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: "warning",
				Stage:    "prepare",
				Code:     "COMMANDS_DETECTED",
				Message:  "the agent declared no complete command set; commands were inferred from detected dependency files",
			})
		}
	}
	technology := normalizeTechnology(request.Technology)
	manifest := &EnvironmentManifest{
		SchemaVersion: "environment.manifest.v1",
		Artifact: ManifestArtifact{
			ID:         technology + "-" + time.Now().Format("20060102"),
			Technology: technology,
			Version:    "0.1.0",
		},
		Environment: ManifestEnvironment{
			Base:            "",
			Network:         "setup-only",
			DependencyFiles: dependencyFiles,
			Lockfiles:       lockfiles,
		},
		Workspace: ManifestWorkspace{
			Root:            ".",
			LearnerEditable: learnerEditable,
			Protected:       protected,
		},
		Commands: commands,
		Assessment: ManifestAssessment{
			Type:         "unit-tests",
			VisibleTests: protectedTests(protected),
			HiddenTests:  hiddenTests(protected),
		},
		Compatibility: ManifestCompatibility{
			RequestedTechnology: request.Technology,
			RequestedObjective:  request.Objective,
			ReuseTags:           []string{technology},
		},
	}
	return manifest, diagnostics, nil
}

func protectedTests(protected []string) []string {
	visible := []string{}
	for _, file := range protected {
		if strings.HasPrefix(file, ".codegym/") {
			continue
		}
		visible = append(visible, file)
	}
	return visible
}

func hiddenTests(protected []string) []string {
	hidden := []string{}
	for _, file := range protected {
		if strings.HasPrefix(file, ".codegym/hidden/") {
			hidden = append(hidden, file)
		}
	}
	return hidden
}
