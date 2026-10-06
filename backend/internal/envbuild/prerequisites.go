package envbuild

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// prerequisiteSpec is one row in the prerequisite table. Technologies are
// rows, never code branches: supporting a new technology adds a table entry.
type prerequisiteSpec struct {
	id       string
	required bool
	check    func(ctx context.Context) (satisfied bool, detail string)
}

// prerequisiteTable maps normalized technology names to their platform
// requirements. Technologies without an entry have no requirements and
// proceed directly to preparation.
var prerequisiteTable = map[string][]prerequisiteSpec{
	"spring-boot": {
		{
			id:       "spring-boot-dependency-cache",
			required: true,
			check:    checkSpringBootDependencyCache,
		},
	},
}

// normalizeTechnology folds display variants onto table keys: "Spring Boot"
// and "spring-boot" address the same row.
func normalizeTechnology(technology string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(technology)), " ", "-")
}

// mavenRepoRoot is the offline repository probed for cached artifacts. It
// reads the environment on every call so tests can redirect it.
func mavenRepoRoot() string {
	if root := strings.TrimSpace(os.Getenv("CODEGYM_MAVEN_REPO")); root != "" {
		return root
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".m2", "repository")
}

// checkSpringBootDependencyCache probes for the pinned parent POM the
// benchmark record establishes as the Spring Boot prerequisite. It performs
// no downloads and contacts no network.
func checkSpringBootDependencyCache(context.Context) (bool, string) {
	const groupPath = "org/springframework/boot"
	const artifact = "spring-boot-starter-parent"
	const version = "3.3.5"
	root := mavenRepoRoot()
	pom := filepath.Join(root, groupPath, artifact, version, artifact+"-"+version+".pom")
	if root == "" {
		return false, "spring-boot-starter-parent:3.3.5 is unavailable: no Maven repository is configured (set CODEGYM_MAVEN_REPO)"
	}
	if _, err := os.Stat(pom); err != nil {
		return false, fmt.Sprintf("spring-boot-starter-parent:3.3.5 is missing from the dependency cache at %s; refusing setup before budget is spent", pom)
	}
	return true, fmt.Sprintf("spring-boot-starter-parent:3.3.5 is cached at %s", pom)
}

// CheckPrerequisites evaluates the table rows for a technology. Unknown
// technologies carry no requirements. It spends no agent budget and starts no
// runtime; it only reports what platform pieces exist.
func CheckPrerequisites(ctx context.Context, technology string) ([]Prerequisite, error) {
	specs, ok := prerequisiteTable[normalizeTechnology(technology)]
	if !ok {
		return nil, nil
	}
	prerequisites := make([]Prerequisite, 0, len(specs))
	for _, spec := range specs {
		satisfied, detail := spec.check(ctx)
		prerequisites = append(prerequisites, Prerequisite{
			ID:        spec.id,
			Required:  spec.required,
			Satisfied: satisfied,
			Detail:    detail,
		})
	}
	return prerequisites, nil
}

// MissingRequired returns the first unsatisfied required prerequisite, or an
// empty ID when preparation may proceed.
func MissingRequired(prerequisites []Prerequisite) Prerequisite {
	for _, prerequisite := range prerequisites {
		if prerequisite.Required && !prerequisite.Satisfied {
			return prerequisite
		}
	}
	return Prerequisite{}
}

// MissingPrerequisiteDiagnostic renders a fail-fast diagnostic in the shared
// contract shape. Stage "prepare" marks it as builder-side, before any
// verifier stage runs.
func MissingPrerequisiteDiagnostic(prerequisite Prerequisite) Diagnostic {
	return Diagnostic{
		Severity: "error",
		Stage:    "prepare",
		Code:     "PREREQUISITE_MISSING",
		Message:  prerequisite.Detail,
		Details:  map[string]any{"prerequisite": prerequisite.ID},
	}
}
