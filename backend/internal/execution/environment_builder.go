package execution

// EnvironmentBuilder is the agent-scaffold track (milestone 2+, not wired):
// for runtimes the deterministic language registry doesn't cover (Spring
// Boot, Rails, …), a coding-agent harness works inside a NETWORK-ENABLED
// sandbox — installing dependencies, writing skeleton + tests — and the
// result is optionally promoted to a snapshot that languages.go can then
// reference for deterministic reuse. The validated recipe is
// spikes/daytona/main.go phaseB (build + preview) and phaseC (promote +
// network-blocked reuse).
//
// Nothing implements this interface yet; it exists so the service layer and
// future handlers can be designed against it.

import "context"

// EnvironmentSpec describes the environment an agent should produce.
type EnvironmentSpec struct {
	// BaseSnapshot to boot from; empty means the platform default.
	BaseSnapshot string
	// Setup commands the agent harness runs (npm install, curl, …).
	// TODO: this will likely become an agent prompt/transcript rather than
	// a flat command list once the harness design lands.
	Setup  []string
	Labels map[string]string
}

// Environment is a live, network-enabled sandbox being built by an agent.
type Environment struct {
	ID string
	// PreviewURL exposes a port to the browser. GOTCHA: private sandboxes
	// require the x-daytona-preview-token header (spike B6);
	// sb.GetPreviewLink(ctx, port) returns both URL and token.
	PreviewURL string
	// SnapshotName is set once the environment has been promoted.
	SnapshotName string
}

type EnvironmentBuilder interface {
	// Build creates a network-enabled sandbox and runs the setup inside it
	// (spike B1–B5). Unlike submission runs, these sandboxes are NOT
	// ephemeral — they live until Promote/Destroy.
	Build(ctx context.Context, spec EnvironmentSpec) (Environment, error)

	// Promote turns the built environment into a reusable snapshot (spike
	// C1: sandbox.ExperimentalCreateSnapshotWithTimeout — promotion took
	// ~20s in the spike; boot-from-snapshot ~4s). The returned snapshot
	// name is what languages.go registers for the deterministic track.
	Promote(ctx context.Context, envID, snapshotName string) (string, error)

	Destroy(ctx context.Context, envID string) error
}
