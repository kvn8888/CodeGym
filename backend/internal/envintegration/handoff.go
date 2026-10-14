// Package envintegration connects the environment builder to the backend
// verifier and registry. It owns no runtime, no verdicts, and no promotion;
// it only translates between the tracks' contract shapes at the handoff
// points the shared contract defines.
package envintegration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kvn8888/codegym/backend/internal/envbuild"
	"github.com/kvn8888/codegym/backend/internal/environmentverify"
)

// ManifestFileName is the workspace-relative path the verifier loads the
// proposed manifest from. The verifier rejects absolute and escaping paths,
// so the name stays a single flat filename.
const ManifestFileName = "environment.manifest.json"

// WriteManifest serializes a builder-drafted manifest into the workspace
// root. JSON is the contract: both tracks' Go shapes decode the same
// document, so the file is the handoff, not either struct.
func WriteManifest(workspaceRoot string, manifest *envbuild.EnvironmentManifest) (string, error) {
	if manifest == nil {
		return "", fmt.Errorf("envintegration: no proposed manifest to write")
	}
	payload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", fmt.Errorf("envintegration: encode proposed manifest: %w", err)
	}
	path := filepath.Join(workspaceRoot, ManifestFileName)
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return "", fmt.Errorf("envintegration: write proposed manifest: %w", err)
	}
	return ManifestFileName, nil
}

// ToBuilderResult translates a builder outcome into the verifier's
// builder-result document. Field mapping is mechanical; nothing here judges
// readiness, and the forbidden readiness-claiming fields are never set.
func ToBuilderResult(runID, runtimeName, runtimeVersion string, sandbox *environmentverify.SandboxRef, manifestPath string, diagnostics []envbuild.Diagnostic, telemetry envbuild.BuilderTelemetry) environmentverify.BuilderResult {
	converted := make([]environmentverify.Diagnostic, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		converted = append(converted, environmentverify.Diagnostic{
			Severity: diagnostic.Severity,
			Stage:    diagnostic.Stage,
			Code:     diagnostic.Code,
			Message:  diagnostic.Message,
			Details:  diagnostic.Details,
		})
	}
	return environmentverify.BuilderResult{
		SchemaVersion:        environmentverify.BuilderResultSchemaVersion,
		BuilderRunID:         runID,
		Runtime:              &environmentverify.RuntimeInfo{Name: runtimeName, Version: runtimeVersion},
		SandboxRef:           sandbox,
		ProposedManifestPath: &manifestPath,
		Diagnostics:          converted,
		Telemetry: &environmentverify.BuilderTelemetry{
			WallTimeMs:       telemetry.WallTimeMs,
			TokensIn:         int(telemetry.TokensIn),
			TokensOut:        int(telemetry.TokensOut),
			EstimatedCostUSD: telemetry.EstimatedCostUsd,
			RepairIterations: telemetry.RepairIterations,
		},
	}
}

// VerifyWorkspace runs backend verification over a materialized workspace.
// The manifest file must already sit at manifestPath under artifactRoot; the
// verifier loads and validates it before executing anything.
func VerifyWorkspace(ctx context.Context, runner environmentverify.CommandRunner, artifactRoot, manifestPath, builderRunID, verifierRunID string, builder environmentverify.BuilderResult) environmentverify.VerifierResult {
	if strings.TrimSpace(builder.BuilderRunID) == "" {
		builder.BuilderRunID = builderRunID
	}
	return environmentverify.Verify(ctx, runner, builder, artifactRoot, verifierRunID)
}
