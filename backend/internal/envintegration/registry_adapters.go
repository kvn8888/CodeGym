package envintegration

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/kvn8888/codegym/backend/internal/envbuild"
	"github.com/kvn8888/codegym/backend/internal/environmentregistry"
	"github.com/kvn8888/codegym/backend/internal/environmentverify"
)

// HostTempWorkspaceProvider names builder refresh sandboxes that are
// isolated host workspaces rather than Daytona sandboxes.
const HostTempWorkspaceProvider = "host-temp-workspace"

// SnapshotDirectories maps opaque snapshot references to workspace roots for
// development and CI. Production resolves references through real snapshots;
// this mapping keeps the handoff testable without one.
type SnapshotDirectories struct {
	roots map[string]string
}

// Register records a workspace root under a fresh opaque reference.
func (s *SnapshotDirectories) Register(root string) string {
	if s.roots == nil {
		s.roots = map[string]string{}
	}
	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		return "local-" + fmt.Sprintf("%d", time.Now().UnixNano())
	}
	reference := "local-" + hex.EncodeToString(raw)
	s.roots[reference] = root
	return reference
}

// Resolve returns the workspace root for a reference.
func (s *SnapshotDirectories) Resolve(reference string) (string, error) {
	root, ok := s.roots[reference]
	if !ok || strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("envintegration: unknown snapshot reference %q", reference)
	}
	return root, nil
}

// RegistryBuilder implements the registry Builder interface with the real
// cold-preparation path: prerequisites, provisioning, the agent runtime, and
// manifest drafting. Repair rounds belong to the outcome matrix, not to a
// single build.
type RegistryBuilder struct {
	Preparer    *envbuild.Preparer
	RuntimeName string
	Snapshots   *SnapshotDirectories
}

// Build prepares one environment and returns the registry handoff triple:
// the builder-result document, the snapshot reference, and the parsed
// manifest the verifier will load from the artifact root.
func (b *RegistryBuilder) Build(ctx context.Context, req environmentregistry.BuildRequest) (environmentregistry.BuildOutput, error) {
	if b == nil || b.Preparer == nil {
		return environmentregistry.BuildOutput{}, fmt.Errorf("envintegration: preparer is required")
	}
	if b.Snapshots == nil {
		return environmentregistry.BuildOutput{}, fmt.Errorf("envintegration: snapshot directory is required")
	}
	result, err := b.Preparer.Prepare(ctx, envbuild.BuildRequest{Technology: req.Technology, Objective: req.Objective})
	if err != nil {
		return environmentregistry.BuildOutput{}, err
	}
	if result.ProposedManifest == nil {
		return environmentregistry.BuildOutput{}, fmt.Errorf("envintegration: preparation produced no manifest")
	}
	manifestPath, err := WriteManifest(result.Workspace.Root, result.ProposedManifest)
	if err != nil {
		return environmentregistry.BuildOutput{}, err
	}
	raw, err := os.ReadFile(filepath.Join(result.Workspace.Root, manifestPath))
	if err != nil {
		return environmentregistry.BuildOutput{}, fmt.Errorf("envintegration: read back proposed manifest: %w", err)
	}
	parsed, err := environmentverify.ParseManifest(raw)
	if err != nil {
		return environmentregistry.BuildOutput{}, fmt.Errorf("envintegration: builder output rejected by manifest parser: %w", err)
	}
	runtimeName := strings.TrimSpace(b.RuntimeName)
	if runtimeName == "" {
		runtimeName = "opencode"
	}
	builder := ToBuilderResult(
		result.BuilderRunID, runtimeName, "",
		&environmentverify.SandboxRef{Provider: HostTempWorkspaceProvider, ID: result.Workspace.ID},
		manifestPath, result.Diagnostics, result.Telemetry,
	)
	return environmentregistry.BuildOutput{
		Builder:     builder,
		SnapshotRef: b.Snapshots.Register(result.Workspace.Root),
		Manifest:    parsed,
	}, nil
}

func newVerifierRunID() string {
	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Sprintf("verify_%d", time.Now().UnixNano())
	}
	return "verify_" + hex.EncodeToString(raw)
}

// RegistryVerifier implements the registry Verifier interface with real
// backend verification. It resolves the snapshot reference to an artifact
// root, confirms the recorded manifest matches the file the verifier loads,
// and runs verification with the injected runner.
type RegistryVerifier struct {
	Roots      func(reference string) (string, error)
	Runner     environmentverify.CommandRunner
	Assessment *environmentverify.AssessmentSpec
	Assessor   environmentverify.AssessmentRunner
}

// Verify judges one builder output. It returns a verifier result, never an
// error: every failure mode becomes a rejected verdict the registry gates on.
func (v *RegistryVerifier) Verify(ctx context.Context, build environmentregistry.BuildOutput) environmentverify.VerifierResult {
	verifierRunID := newVerifierRunID()
	reject := func(stage, code, message string, repairable bool) environmentverify.VerifierResult {
		failed := stage
		return environmentverify.VerifierResult{
			SchemaVersion: environmentverify.VerifierResultSchemaVersion,
			VerifierRunID: verifierRunID,
			BuilderRunID:  build.Builder.BuilderRunID,
			Verdict:       environmentverify.VerdictRejected,
			FailedStage:   &failed,
			Repairable:    repairable,
			Diagnostics: []environmentverify.Diagnostic{{
				Severity: "error",
				Stage:    stage,
				Code:     code,
				Message:  message,
			}},
			Promotion: environmentverify.Promotion{Allowed: false, Reason: message},
		}
	}
	if v == nil || v.Roots == nil {
		return reject(environmentverify.StagePlatform, environmentverify.CodePlatformFault, "envintegration: snapshot resolver is required", false)
	}
	if v.Runner == nil {
		return reject(environmentverify.StagePlatform, environmentverify.CodePlatformFault, "envintegration: command runner is required", false)
	}
	root, err := v.Roots(build.SnapshotRef)
	if err != nil {
		return reject(environmentverify.StagePlatform, environmentverify.CodePlatformFault, fmt.Sprintf("envintegration: resolve snapshot: %v", err), false)
	}
	manifestPath := ""
	if build.Builder.ProposedManifestPath != nil {
		manifestPath = strings.TrimSpace(*build.Builder.ProposedManifestPath)
	}
	if manifestPath == "" {
		return reject(environmentverify.StageManifest, environmentverify.CodeManifestMissing, "envintegration: builder output names no manifest", true)
	}
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(manifestPath)))
	if err != nil {
		return reject(environmentverify.StageManifest, environmentverify.CodeManifestMissing, fmt.Sprintf("envintegration: read manifest: %v", err), true)
	}
	parsed, err := environmentverify.ParseManifest(raw)
	if err != nil {
		return reject(environmentverify.StageManifest, environmentverify.CodeManifestMalformed, fmt.Sprintf("envintegration: parse manifest: %v", err), true)
	}
	canonicalRecorded, err := json.Marshal(build.Manifest)
	if err != nil {
		return reject(environmentverify.StageValidation, environmentverify.CodeManifestInvalid, fmt.Sprintf("envintegration: encode recorded manifest: %v", err), false)
	}
	canonicalLoaded, err := json.Marshal(parsed)
	if err != nil {
		return reject(environmentverify.StageValidation, environmentverify.CodeManifestInvalid, fmt.Sprintf("envintegration: encode loaded manifest: %v", err), false)
	}
	var recordedAny, loadedAny any
	if err := json.Unmarshal(canonicalRecorded, &recordedAny); err != nil {
		return reject(environmentverify.StageValidation, environmentverify.CodeManifestInvalid, fmt.Sprintf("envintegration: decode recorded manifest: %v", err), false)
	}
	if err := json.Unmarshal(canonicalLoaded, &loadedAny); err != nil {
		return reject(environmentverify.StageValidation, environmentverify.CodeManifestInvalid, fmt.Sprintf("envintegration: decode loaded manifest: %v", err), false)
	}
	if !reflect.DeepEqual(recordedAny, loadedAny) {
		return reject(environmentverify.StageValidation, environmentverify.CodeManifestInvalid, "envintegration: recorded manifest differs from the file the verifier loads", false)
	}
	opts := environmentverify.DefaultVerifyOptions()
	if v.Assessment != nil {
		opts.Assessment = v.Assessment
		opts.Assessor = v.Assessor
	}
	return environmentverify.VerifyWithOptions(ctx, v.Runner, build.Builder, root, verifierRunID, opts)
}
