package environmentverify

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Contract schema versions shared with docs/environment-contract.md.
const (
	BuilderResultSchemaVersion  = "environment.builder-result.v1"
	VerifierResultSchemaVersion = "environment.verifier-result.v1"
)

// Verifier verdicts. Only VerdictVerified establishes readiness, and only
// the backend verifier can issue it.
const (
	VerdictVerified = "verified"
	VerdictRejected = "rejected"
)

// Failure stages from the shared contract.
const (
	StageManifest   = "manifest"
	StageSetup      = "setup"
	StageBuild      = "build"
	StageTest       = "test"
	StageRun        = "run"
	StageValidation = "validation"
	StageTimeout    = "timeout"
	StagePlatform   = "platform"
)

// Stable machine-readable diagnostic codes produced by this package.
const (
	CodeBuilderResultInvalid      = "BUILDER_RESULT_INVALID"
	CodeManifestMissing           = "MANIFEST_MISSING"
	CodeManifestPathUnsafe        = "MANIFEST_PATH_UNSAFE"
	CodeManifestMalformed         = "MANIFEST_MALFORMED"
	CodeManifestInvalid           = "MANIFEST_INVALID"
	CodeArtifactFileMissing       = "ARTIFACT_FILE_MISSING"
	CodeArtifactPathUnsafe        = "ARTIFACT_PATH_UNSAFE"
	CodeArtifactTypeMismatch      = "ARTIFACT_TYPE_MISMATCH"
	CodeBoundaryViolation         = "BOUNDARY_VIOLATION"
	CodeHiddenTestUnprotected     = "HIDDEN_TEST_UNPROTECTED"
	CodeHiddenTestEditable        = "HIDDEN_TEST_EDITABLE"
	CodeArtifactInvalid           = "ARTIFACT_INVALID"
	CodeSetupFailed               = "SETUP_FAILED"
	CodeBuildFailed               = "BUILD_FAILED"
	CodeTestFailed                = "TEST_FAILED"
	CodeStageTimeout              = "STAGE_TIMEOUT"
	CodePlatformFault             = "PLATFORM_FAULT"
	CodeAssessmentInputsMissing   = "ASSESSMENT_INPUTS_MISSING"
	CodeReviewPending             = "REVIEW_PENDING"
	CodeCorrectSolutionFailed     = "CORRECT_SOLUTION_FAILED"
	CodeWrongSolutionPassed       = "WRONG_SOLUTION_PASSED"
	CodeWrongSolutionUnexpected   = "WRONG_SOLUTION_UNEXPECTED"
	CodeAssertionMismatch         = "ASSERTION_EVIDENCE_MISMATCH"
	CodeAssessmentMalformed       = "ASSESSMENT_MALFORMED"
	CodeAssessmentEvidencePending = "ASSESSMENT_EVIDENCE_PENDING"
)

// Diagnostic is the shared builder/verifier diagnostic shape. Details carry
// optional structured context and are omitted when absent.
type Diagnostic struct {
	Severity string `json:"severity"`
	Stage    string `json:"stage"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	Details  any    `json:"details,omitempty"`
}

// RuntimeInfo identifies the agent runtime behind a builder result.
type RuntimeInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// SandboxRef is the builder's temporary sandbox reference, if one exists.
type SandboxRef struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
}

// BuilderTelemetry carries runtime, token, cost, and repair metrics. Fields
// stay optional: the verifier never makes decisions from telemetry.
type BuilderTelemetry struct {
	WallTimeMs       int64   `json:"wallTimeMs,omitempty"`
	TokensIn         int     `json:"tokensIn,omitempty"`
	TokensOut        int     `json:"tokensOut,omitempty"`
	EstimatedCostUSD float64 `json:"estimatedCostUsd,omitempty"`
	RepairIterations int     `json:"repairIterations,omitempty"`
}

// BuilderResult describes what the agent/runtime produced before
// verification. It carries no readiness state: the contract forbids ready,
// verified, promotable, and verdict fields here, and ParseBuilderResult
// rejects payloads that include them.
type BuilderResult struct {
	SchemaVersion        string            `json:"schemaVersion"`
	BuilderRunID         string            `json:"builderRunId"`
	Runtime              *RuntimeInfo      `json:"runtime"`
	SandboxRef           *SandboxRef       `json:"sandboxRef,omitempty"`
	ArtifactRef          json.RawMessage   `json:"artifactRef,omitempty"`
	ProposedManifestPath *string           `json:"proposedManifestPath"`
	Diagnostics          []Diagnostic      `json:"diagnostics"`
	Telemetry            *BuilderTelemetry `json:"telemetry,omitempty"`
}

// forbiddenBuilderFields must never appear in builder output. Their presence
// means the builder is claiming readiness, which only the verifier can
// establish.
var forbiddenBuilderFields = []string{"ready", "verified", "promotable", "verdict"}

// ParseBuilderResult decodes one JSON builder-result object, checks the
// schema version, rejects readiness-claiming fields, and enforces the
// required contract fields. Builder prose, exit claims, and telemetry never
// influence the outcome; they are carried through untouched.
func ParseBuilderResult(data []byte) (BuilderResult, error) {
	var result BuilderResult
	if len(bytes.TrimSpace(data)) == 0 {
		return BuilderResult{}, errors.New("environment builder result is empty")
	}
	var raw map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&raw); err != nil {
		return BuilderResult{}, fmt.Errorf("environment builder result: invalid JSON: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return BuilderResult{}, errors.New("environment builder result: must contain exactly one JSON object")
	}
	for _, field := range forbiddenBuilderFields {
		if _, ok := raw[field]; ok {
			return BuilderResult{}, fmt.Errorf("environment builder result: field %q must not appear in builder output", field)
		}
	}
	remarshaled, err := json.Marshal(raw)
	if err != nil {
		return BuilderResult{}, fmt.Errorf("environment builder result: invalid JSON: %w", err)
	}
	if err := json.Unmarshal(remarshaled, &result); err != nil {
		return BuilderResult{}, fmt.Errorf("environment builder result: invalid JSON: %w", err)
	}
	if strings.TrimSpace(result.SchemaVersion) == "" {
		return BuilderResult{}, errors.New("environment builder result: schemaVersion is required")
	}
	if strings.TrimSpace(result.SchemaVersion) != BuilderResultSchemaVersion {
		return BuilderResult{}, fmt.Errorf("environment builder result: unsupported schemaVersion %q", result.SchemaVersion)
	}
	if strings.TrimSpace(result.BuilderRunID) == "" {
		return BuilderResult{}, errors.New("environment builder result: builderRunId is required")
	}
	if result.Runtime == nil || strings.TrimSpace(result.Runtime.Name) == "" {
		return BuilderResult{}, errors.New("environment builder result: runtime.name is required")
	}
	if result.Diagnostics == nil {
		return BuilderResult{}, errors.New("environment builder result: diagnostics is required")
	}
	return result, nil
}

// EvidenceCheck records one backend-executed check and its outcome. Command
// is null for checks that run no shell command (manifest and validation).
type EvidenceCheck struct {
	Stage    string  `json:"stage"`
	Command  *string `json:"command"`
	ExitCode int     `json:"exitCode"`
	Summary  string  `json:"summary"`
}

// Evidence lists exactly the checks the backend attempted, in order.
type Evidence struct {
	Checks []EvidenceCheck `json:"checks"`
}

// Promotion states whether registry promotion is allowed and why.
type Promotion struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason"`
}

// VerifierResult is the only contract object that can establish readiness.
// FailedStage is null when the verdict is verified.
type VerifierResult struct {
	SchemaVersion string       `json:"schemaVersion"`
	VerifierRunID string       `json:"verifierRunId"`
	BuilderRunID  string       `json:"builderRunId"`
	Verdict       string       `json:"verdict"`
	FailedStage   *string      `json:"failedStage"`
	Repairable    bool         `json:"repairable"`
	Diagnostics   []Diagnostic `json:"diagnostics"`
	Evidence      Evidence     `json:"evidence"`
	Promotion     Promotion    `json:"promotion"`
}
