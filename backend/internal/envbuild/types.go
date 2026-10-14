// Package envbuild implements cold preparation of coding practice
// environments: a requested technology and learning objective become project
// files plus a proposed versioned manifest, returned to the verifier
// boundary. It never verifies and never promotes; the backend verifier owns
// readiness and the registry owns promotion.
package envbuild

import "context"

// BuildRequest carries the technology and learning objective as data. No
// downstream code may branch on Technology; it is interpolated into fixed
// instructions and recorded on the manifest.
type BuildRequest struct {
	Technology string
	Objective  string
}

// Workspace is a temporary isolated working directory. The host
// implementation uses an isolated temp directory; a Daytona provisioner can
// sit behind WorkspaceProvisioner later without changing callers.
type Workspace struct {
	ID   string
	Root string
}

// WorkspaceProvisioner provisions and destroys workspaces. It owns temporary
// resources up to the verifier handoff.
type WorkspaceProvisioner interface {
	Provision(ctx context.Context, labels map[string]string) (Workspace, error)
	Destroy(ctx context.Context, workspace Workspace) error
}

// Prerequisite is one platform requirement for a technology.
type Prerequisite struct {
	ID        string
	Required  bool
	Satisfied bool
	Detail    string
}

// EnvironmentManifest is the Go representation of environment.manifest.v1.
// Field names and JSON keys match docs/environment-contract.md exactly.
type EnvironmentManifest struct {
	SchemaVersion string                `json:"schemaVersion"`
	Artifact      ManifestArtifact      `json:"artifact"`
	Environment   ManifestEnvironment   `json:"environment"`
	Workspace     ManifestWorkspace     `json:"workspace"`
	Commands      ManifestCommands      `json:"commands"`
	Assessment    ManifestAssessment    `json:"assessment"`
	Compatibility ManifestCompatibility `json:"compatibility"`
}

type ManifestArtifact struct {
	ID         string `json:"id"`
	Technology string `json:"technology"`
	Version    string `json:"version"`
}

type ManifestEnvironment struct {
	Base            string   `json:"base"`
	Network         string   `json:"network"`
	DependencyFiles []string `json:"dependencyFiles"`
	Lockfiles       []string `json:"lockfiles"`
}

type ManifestWorkspace struct {
	Root            string   `json:"root"`
	LearnerEditable []string `json:"learnerEditable"`
	Protected       []string `json:"protected"`
}

type ManifestCommands struct {
	Setup string `json:"setup"`
	Build string `json:"build"`
	Test  string `json:"test"`
	Run   string `json:"run"`
}

type ManifestAssessment struct {
	Type         string   `json:"type"`
	VisibleTests []string `json:"visibleTests"`
	HiddenTests  []string `json:"hiddenTests"`
}

type ManifestCompatibility struct {
	RequestedTechnology string   `json:"requestedTechnology"`
	RequestedObjective  string   `json:"requestedObjective"`
	ReuseTags           []string `json:"reuseTags"`
}

// Diagnostic matches the shared contract diagnostics format.
type Diagnostic struct {
	Severity string         `json:"severity"`
	Stage    string         `json:"stage"`
	Code     string         `json:"code"`
	Message  string         `json:"message"`
	Details  map[string]any `json:"details,omitempty"`
}

// BuilderTelemetry matches the shared contract builder telemetry.
type BuilderTelemetry struct {
	WallTimeMs       int64   `json:"wallTimeMs"`
	TokensIn         int64   `json:"tokensIn"`
	TokensOut        int64   `json:"tokensOut"`
	EstimatedCostUsd float64 `json:"estimatedCostUsd"`
	RepairIterations int     `json:"repairIterations"`
}

// BuildResult is what cold preparation returns to the verifier boundary.
// ProposedManifest is nil when preparation fails before drafting. Nothing
// here claims readiness; only a verifier result can do that.
type BuildResult struct {
	BuilderRunID     string
	Workspace        Workspace
	ProposedManifest *EnvironmentManifest
	Diagnostics      []Diagnostic
	Telemetry        BuilderTelemetry
}
