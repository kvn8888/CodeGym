package environmentregistry

import (
	"context"
	"errors"
	"time"

	"github.com/kvn8888/codegym/backend/internal/environmentverify"
)

// NetworkPolicy names the network posture an environment boots under.
// Practice boots are isolated; the provider enforces the policy.
type NetworkPolicy string

// Supported practice network policies.
const (
	NetworkPolicyIsolated NetworkPolicy = "isolated"
)

// ValidationState records the registry's own validation standing for an
// artifact. It advances on promotion and on every gated boot check.
type ValidationState string

// Registry validation states.
const (
	ValidationPassed          ValidationState = "passed"
	ValidationBootCheckFailed ValidationState = "boot-check-failed"
)

// CompatibilityRules are the explicit reuse rules recorded with an
// artifact. Matching is exact: technology must equal, a requested version
// must equal the artifact version (never substituted), a non-empty
// requested objective must equal the recorded objective, and every
// requested reuse tag must be present. This strictness is a Track B policy
// proposed for #184 review.
type CompatibilityRules struct {
	Technology string
	Objective  string
	ReuseTags  []string
}

// Provenance records who produced the artifact and which backend runs
// verified it.
type Provenance struct {
	BuiltBy       string
	Owner         string
	BuilderRunID  string
	VerifierRunID string
}

// EvidenceRef associates the verifier evidence with the exact
// preparation/artifact promoted. It carries run identity plus a summary of
// what the backend ran, so downstream code never re-resolves evidence by
// artifact id/version alone.
type EvidenceRef struct {
	BuilderRunID  string
	VerifierRunID string
	Verdict       string
	CheckCount    int
}

// ArtifactRecord is the registry metadata for one promoted environment
// artifact: identity, environment, dependency references, commands,
// provenance, ownership, backend creation time, validation state, and
// explicit compatibility rules.
type ArtifactRecord struct {
	ArtifactID      string
	ArtifactVersion string
	Technology      string
	EnvironmentBase string
	DependencyFiles []string
	Lockfiles       []string
	SetupCommand    string
	BuildCommand    string
	TestCommand     string
	SnapshotRef     string
	Provenance      Provenance
	Owner           string
	CreatedAt       time.Time
	ValidationState ValidationState
	LastBootCheckAt time.Time
	Compatibility   CompatibilityRules
	Evidence        EvidenceRef
}

// ErrArtifactNotFound reports a registry lookup with no matching record.
var ErrArtifactNotFound = errors.New("environment artifact not found")

// Store persists promoted artifact records. The in-memory implementation
// below is the standing fake and test double; durable persistence is a
// production integration limit (see doc.go).
type Store interface {
	SaveArtifact(ctx context.Context, record ArtifactRecord) error
	ListByTechnology(ctx context.Context, technology string) ([]ArtifactRecord, error)
	GetArtifact(ctx context.Context, artifactID, version string) (ArtifactRecord, error)
	UpdateValidation(ctx context.Context, artifactID, version string, state ValidationState, at time.Time) error
}

// BootResult is the observed outcome of one command on a fresh boot.
type BootResult struct {
	ExitCode int
	Output   string
}

// Workspace is one isolated learner workspace with dependencies already
// available. Workspaces are always fresh: they carry no previous learner
// state.
type Workspace struct {
	ID              string
	ArtifactID      string
	ArtifactVersion string
}

// Boot is one fresh boot of a promoted snapshot under a network policy.
type Boot interface {
	// Run executes one representative check command (build or test —
	// never dependency installation) on the fresh boot.
	Run(ctx context.Context, command string) (BootResult, error)
	// NewWorkspace separates one clean learner workspace from the boot.
	NewWorkspace(ctx context.Context) (Workspace, error)
	Close(ctx context.Context) error
}

// SnapshotProvider boots clean environments from snapshot references
// under an explicit network policy.
type SnapshotProvider interface {
	BootFresh(ctx context.Context, snapshotRef string, policy NetworkPolicy) (Boot, error)
}

// Capabilities reports which technologies may be freshly built.
type Capabilities interface {
	SupportsTechnology(technology string) bool
}

// QuotaDenialReason names why a build was refused without invoking the
// builder.
type QuotaDenialReason string

// Build denial reasons, reported explicitly and never silently absorbed.
const (
	QuotaExhausted QuotaDenialReason = "quota-exhausted"
	RateLimited    QuotaDenialReason = "rate-limited"
)

// QuotaDenial is returned by Quota.CheckBuild when a build must not run.
type QuotaDenial struct {
	Reason QuotaDenialReason
	Owner  string
}

func (e *QuotaDenial) Error() string {
	return "environment build refused for owner " + e.Owner + ": " + string(e.Reason)
}

// Quota gates fresh builds on quota and rate limits. A non-nil *QuotaDenial
// means zero builder calls; any other error is a platform fault.
type Quota interface {
	CheckBuild(ctx context.Context, owner string) error
}

// BuildRequest asks the builder track for a fresh environment. Version
// is the exact requested pin; empty means any version is acceptable. The
// pin travels with the request so the builder can target it, and fresh
// output is still validated against it before promotion — the registry
// never silently substitutes versions.
type BuildRequest struct {
	Technology string
	Version    string
	Objective  string
	ReuseTags  []string
	Owner      string
}

// BuildOutput is what the builder track produced before verification:
// the builder's own contract result (run identity, runtime, proposed
// manifest path, diagnostics — owned by the builder track per the shared
// contract), plus the prepared snapshot reference and the parsed proposed
// manifest the registry validates. Output must still pass verification
// and promotion gates before becoming reusable; builder claims never
// establish readiness.
//
// Production wiring note (open #184 item): the prepared artifact lives
// behind SnapshotRef, while the #188 verifier loads its manifest from a
// filesystem artifact root via Builder.ProposedManifestPath. The
// production adapter must resolve SnapshotRef to that root and confirm
// the Manifest recorded here matches the file the verifier loads; the
// registry performs no such translation itself.
type BuildOutput struct {
	Builder     environmentverify.BuilderResult
	SnapshotRef string
	Manifest    environmentverify.Manifest
}

// Builder prepares fresh environments on a registry miss. Output must
// still pass verification and promotion gates before becoming reusable;
// builder claims never establish readiness.
type Builder interface {
	Build(ctx context.Context, req BuildRequest) (BuildOutput, error)
}

// Verifier reuses the #188 verification contract for builder fallback
// output. It receives the builder track's real handoff untouched —
// including the builder-owned BuilderResult — so a production adapter
// can map it onto the #188 verifier (runner, artifact root, verifier run
// ID, assessment inputs) instead of a registry-fabricated result. Tests
// inject fakes; production wires the real verifier with a sandboxed
// runner. The verdict it returns is gated, never trusted blindly.
type Verifier interface {
	Verify(ctx context.Context, build BuildOutput) environmentverify.VerifierResult
}
