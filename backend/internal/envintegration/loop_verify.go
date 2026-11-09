package envintegration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kvn8888/codegym/backend/internal/envbuild"
	"github.com/kvn8888/codegym/backend/internal/environmentregistry"
	"github.com/kvn8888/codegym/backend/internal/environmentverify"
)

// convertManifest renders the builder's manifest through JSON into the
// verifier's manifest shape. JSON is the contract, so identical documents
// convert losslessly regardless of Go-shape differences.
func convertManifest(manifest *envbuild.EnvironmentManifest) environmentverify.Manifest {
	raw, err := json.Marshal(manifest)
	if err != nil {
		return environmentverify.Manifest{}
	}
	var converted environmentverify.Manifest
	if err := json.Unmarshal(raw, &converted); err != nil {
		return environmentverify.Manifest{}
	}
	return converted
}

// LoopVerifier adapts real backend verification to the repair loop's
// verifier contract. Each round materializes the round manifest, registers
// the workspace, confirms the recorded manifest matches the loaded file,
// and runs verification. The mapped verdict drives the loop; agent claims
// never appear in the verdict path.
type LoopVerifier struct {
	Snapshots   *SnapshotDirectories
	Runner      environmentverify.CommandRunner
	RuntimeName string
	Assessment  *environmentverify.AssessmentSpec
	Assessor    environmentverify.AssessmentRunner
	Timeouts    environmentverify.StageTimeouts
	RunIDs      func() string
}

// Verify judges one loop round. It returns the mapped verdict, never an
// error for verification outcomes; infrastructure failures of the adapter
// itself are the only errors.
func (v *LoopVerifier) Verify(ctx context.Context, result envbuild.BuildResult) (envbuild.Verification, error) {
	fail := func(stage, code, message string, repairable bool) (envbuild.Verification, error) {
		return envbuild.Verification{
			Verdict:     envbuild.VerdictRejected,
			FailedStage: stage,
			Repairable:  repairable,
			Diagnostics: []envbuild.Diagnostic{{
				Severity: "error",
				Stage:    stage,
				Code:     code,
				Message:  message,
			}},
		}, nil
	}
	if v == nil || v.Snapshots == nil {
		return fail("platform", "LOOP_VERIFY_MISCONFIGURED", "envintegration: snapshot directory is required", false)
	}
	if v.Runner == nil {
		return fail("platform", "LOOP_VERIFY_MISCONFIGURED", "envintegration: command runner is required", false)
	}
	if result.ProposedManifest == nil {
		return fail("manifest", "MANIFEST_MISSING", "envintegration: round produced no manifest", true)
	}
	manifestPath, err := WriteManifest(result.Workspace.Root, result.ProposedManifest)
	if err != nil {
		return fail("manifest", "MANIFEST_MISSING", fmt.Sprintf("envintegration: %v", err), true)
	}
	raw, err := os.ReadFile(filepath.Join(result.Workspace.Root, filepath.FromSlash(manifestPath)))
	if err != nil {
		return fail("manifest", "MANIFEST_MISSING", fmt.Sprintf("envintegration: %v", err), true)
	}
	parsed, err := environmentverify.ParseManifest(raw)
	if err != nil {
		return fail("manifest", "MANIFEST_MALFORMED", fmt.Sprintf("envintegration: %v", err), true)
	}
	if err := confirmRecordedManifest(convertManifest(result.ProposedManifest), parsed); err != nil {
		return fail("validation", "MANIFEST_MISMATCH", fmt.Sprintf("envintegration: %v", err), false)
	}
	reference := v.Snapshots.Register(result.Workspace.Root)
	runtimeName := strings.TrimSpace(v.RuntimeName)
	if runtimeName == "" {
		runtimeName = "opencode"
	}
	doc := ToBuilderResult(result.BuilderRunID, runtimeName, "", nil, manifestPath, result.Diagnostics, result.Telemetry)
	verifierRunID := newVerifierRunID()
	if v.RunIDs != nil {
		verifierRunID = v.RunIDs()
	}
	_ = verifierRunID
	inner := &RegistryVerifier{
		Roots:      v.Snapshots.Resolve,
		Runner:     v.Runner,
		Assessment: v.Assessment,
		Assessor:   v.Assessor,
		Timeouts:   v.Timeouts,
	}
	_ = verifierRunID
	res := inner.Verify(ctx, environmentregistry.BuildOutput{Builder: doc, SnapshotRef: reference, Manifest: parsed})
	mapped := envbuild.Verification{
		Verdict:    envbuild.Verdict(res.Verdict),
		Repairable: res.Repairable,
	}
	if res.FailedStage != nil {
		mapped.FailedStage = *res.FailedStage
	}
	for _, diagnostic := range res.Diagnostics {
		details := map[string]any{}
		if asMap, ok := diagnostic.Details.(map[string]any); ok {
			details = asMap
		}
		mapped.Diagnostics = append(mapped.Diagnostics, envbuild.Diagnostic{
			Severity: diagnostic.Severity,
			Stage:    diagnostic.Stage,
			Code:     diagnostic.Code,
			Message:  diagnostic.Message,
			Details:  details,
		})
	}
	return mapped, nil
}
