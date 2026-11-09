package practice

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"github.com/kvn8888/codegym/backend/internal/environmentregistry"
	"github.com/kvn8888/codegym/backend/internal/environmentverify"
)

// Sample exercise identity for the #191 development browser flow. The
// sample mirrors the checked-in #188 greet fixture but is served through
// dedicated dev-only wiring with learner-safe instructions.
const (
	SampleArtifactID      = "greet-basics-2026-10-01"
	SampleArtifactVersion = "0.1.0"
	SampleSnapshotRef     = "sample_greet_1"
	SampleBuilderRunID    = "sample-build_greet_1"
	SampleVerifierRunID   = "sample-verify_greet_1"
)

//go:embed sampledata/greet_manifest.json
var sampleManifestJSON []byte

//go:embed sampledata/greet_solution.py
var sampleSolutionPy string

//go:embed sampledata/greet_test.py
var sampleProtectedTestPy string

// SampleManifest returns the parsed development sample manifest.
func SampleManifest() (environmentverify.Manifest, error) {
	return environmentverify.ValidateBytes(sampleManifestJSON)
}

// SampleArtifact builds the trusted verified pair for the development
// sample: a registry-passed record mirroring the manifest plus the
// manifest itself. Test-only and development-only evidence.
func SampleArtifact() (VerifiedArtifact, error) {
	manifest, err := SampleManifest()
	if err != nil {
		return VerifiedArtifact{}, err
	}
	return VerifiedArtifact{
		Record: environmentregistry.ArtifactRecord{
			ArtifactID:      manifest.Artifact.ID,
			ArtifactVersion: manifest.Artifact.Version,
			Technology:      manifest.Artifact.Technology,
			EnvironmentBase: manifest.Environment.Base,
			SetupCommand:    manifest.Commands.Setup,
			BuildCommand:    manifest.Commands.Build,
			TestCommand:     manifest.Commands.Test,
			SnapshotRef:     SampleSnapshotRef,
			Provenance: environmentregistry.Provenance{
				BuiltBy:       "practice-sample",
				Owner:         "codegym-sample",
				BuilderRunID:  SampleBuilderRunID,
				VerifierRunID: SampleVerifierRunID,
			},
			Owner:           "codegym-sample",
			ValidationState: environmentregistry.ValidationPassed,
			Compatibility: environmentregistry.CompatibilityRules{
				Technology: manifest.Artifact.Technology,
				Objective:  manifest.Compatibility.RequestedObjective,
				ReuseTags:  append([]string(nil), manifest.Compatibility.ReuseTags...),
			},
		},
		Manifest: manifest,
	}, nil
}

// SampleArtifactSource serves the single development sample artifact.
// It resolves the exact artifact ID/version only: any other artifact ID,
// unknown version pin, or technology skew is refused explicitly and never
// substituted. Development/testing-only; production must not wire it.
type SampleArtifactSource struct{}

// LoadVerifiedArtifact serves the sample for the exact pin and refuses
// everything else explicitly.
func (SampleArtifactSource) LoadVerifiedArtifact(_ context.Context, _ string, ref ArtifactRef) (VerifiedArtifact, error) {
	artifact, err := SampleArtifact()
	if err != nil {
		return VerifiedArtifact{}, err
	}
	if pin := strings.TrimSpace(ref.ArtifactID); pin != "" && pin != artifact.Record.ArtifactID {
		return VerifiedArtifact{}, fmt.Errorf("%w: requested %q but the development sample is %q", environmentregistry.ErrArtifactNotFound, pin, artifact.Record.ArtifactID)
	}
	return artifact, nil
}

// SampleContentStore serves base file contents for the development
// sample snapshot. Development/testing-only.
type SampleContentStore struct{}

// ReadArtifactFile reads base files from the embedded sample snapshot.
func (SampleContentStore) ReadArtifactFile(_ context.Context, snapshotRef, filePath string) (string, error) {
	if snapshotRef != SampleSnapshotRef {
		return "", fmt.Errorf("practice sample snapshot %q not found", snapshotRef)
	}
	switch filePath {
	case "solution.py":
		return sampleSolutionPy, nil
	case "protected/test_greet.py":
		return sampleProtectedTestPy, nil
	default:
		return "", fmt.Errorf("practice sample has no file %q", filePath)
	}
}

// FakeSampleRunner is the explicitly simulated development/testing-only
// provider for the #191 browser sample. It never executes learner code,
// never materializes hidden/reference/verifier files, and never touches
// the host filesystem or subprocesses: it returns scripted stage outcomes
// from the learner overlay alone so the service can exercise its
// orchestration, gating, and safe-projection paths.
//
//   - It inspects only spec.Files (the isolated learner overlay). Hidden
//     test files are never read, regardless of spec.Visibility.
//   - Build stage: fails when solution.py does not define greet.
//   - Test stage: passes only when the overlay shows a strip-like
//     normalization and a stranger-like fallback; otherwise fails. This
//     is a static simulation heuristic, not verification.
//   - Run stage: returns a small canned program-output placeholder; the
//     service still projects and caps it like any run output.
//
// Production must not wire it. The service marks its results
// Simulated=true so clients label them, and a simulated "passed" is
// local behavior only, never verification evidence. One shared fake
// serves every stage of an operation; no per-stage directories exist, so
// there is no fresh-directory state-loss question.
type FakeSampleRunner struct{}

// RunCommand returns the scripted simulated outcome for one stage.
func (FakeSampleRunner) RunCommand(ctx context.Context, spec RunSpec) (RunOutcome, error) {
	if err := ctx.Err(); err != nil {
		return RunOutcome{}, err
	}
	overlay := map[string]string{}
	for _, file := range spec.Files {
		overlay[file.Path] = file.Content
	}
	solution := overlay["solution.py"]
	switch spec.Stage {
	case environmentverify.StageBuild:
		if !strings.Contains(solution, "def greet") {
			return RunOutcome{ExitCode: 1, Output: ""}, nil
		}
		return RunOutcome{ExitCode: 0, Output: ""}, nil
	case environmentverify.StageRun:
		// Canned placeholder, never learner execution. The service
		// projects and caps it before it becomes public.
		return RunOutcome{ExitCode: 0, Output: "Hello, Ada!\n"}, nil
	default:
		// Test stage (including the Run→test fallback): static
		// simulation heuristic, not verification.
		lowered := strings.ToLower(solution)
		if strings.Contains(solution, "def greet") &&
			strings.Contains(lowered, "strip") &&
			strings.Contains(lowered, "stranger") {
			return RunOutcome{ExitCode: 0, Output: ""}, nil
		}
		return RunOutcome{ExitCode: 1, Output: ""}, nil
	}
}
