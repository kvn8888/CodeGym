package environmentverify

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// maxDiagnosticOutput bounds command output quoted in diagnostics so
// failure details stay actionable without echoing unbounded logs.
const maxDiagnosticOutput = 2000

// StageTimeouts caps how long each command stage may run. Limits are owned
// by the backend, not the manifest: the verifier creates and cancels a
// context.WithTimeout for every stage with a positive limit, and the runner
// must honor cancellation and terminate its work (for example by killing
// the subprocess it started). A non-positive limit means no verifier
// deadline for that stage; the parent context still applies.
type StageTimeouts struct {
	Setup      time.Duration
	Build      time.Duration
	Test       time.Duration
	Assessment time.Duration
}

// TimeoutFor returns the configured limit for a setup, build, or test
// stage, or a non-positive duration when no verifier deadline applies.
func (t StageTimeouts) TimeoutFor(stage string) time.Duration {
	switch stage {
	case StageSetup:
		return t.Setup
	case StageBuild:
		return t.Build
	case StageTest:
		return t.Test
	default:
		return 0
	}
}

// DefaultStageTimeouts is the backend's standing stage policy. Callers with
// stricter budgets pass their own VerifyOptions.
func DefaultStageTimeouts() StageTimeouts {
	return StageTimeouts{Setup: 5 * time.Minute, Build: 10 * time.Minute, Test: 10 * time.Minute, Assessment: 10 * time.Minute}
}

// VerifyOptions carries backend-owned verification policy.
type VerifyOptions struct {
	Timeouts StageTimeouts
	// Assessment holds the backend-controlled assessment inputs. A nil
	// Assessment keeps the milestone 3 behavior: passing commands yield an
	// ASSESSMENT_EVIDENCE_PENDING rejection.
	Assessment *AssessmentSpec
	// Assessor evaluates one solution per isolated workspace. Required when
	// Assessment is set; tests use fakes or the sample-only local executor.
	Assessor AssessmentRunner
}

// DefaultVerifyOptions returns the standing backend policy.
func DefaultVerifyOptions() VerifyOptions {
	return VerifyOptions{Timeouts: DefaultStageTimeouts()}
}

// CommandSpec describes one backend-executed stage command. The command
// string is the builder-proposed text, but everything else — stage order,
// working directory, deadlines, and execution policy — is chosen by the
// backend and carried in (deadlines via context).
type CommandSpec struct {
	Stage   string
	Command string
	WorkDir string
}

// CommandResult is the runner-observed outcome of one stage command.
type CommandResult struct {
	ExitCode int
	Output   string
	TimedOut bool
}

// CommandRunner executes one stage command under backend policy. It must
// honor context cancellation and terminate its work (for example by killing
// the subprocess it started) instead of returning success after the
// deadline. A non-zero exit is a normal stage failure reported in the
// result; a returned error is a platform fault (except
// context.DeadlineExceeded, which is a timeout). Milestone 3 uses a fake
// runner in tests: generated commands are never executed on the host and no
// sandboxes are provisioned. Fake tests prove the verifier supplies and
// enforces real deadlines at the orchestration layer; they do not prove
// real subprocess termination, which is the runner implementation's job.
type CommandRunner interface {
	Run(ctx context.Context, spec CommandSpec) (CommandResult, error)
}

// Verify checks a builder result against a backend-provided artifact root
// and returns a contract-shaped verifier result. It always returns a
// result, never an error: every failure mode, including infrastructure
// faults, becomes a rejected verdict with failedStage, diagnostics,
// evidence of the checks actually attempted, and promotion disallowed.
//
// Order: builder result, artifact root, manifest load, structural
// validation, artifact filesystem validation, then setup/build/test stages
// in order, stopping at the first failure. Each command stage runs under
// its backend-owned deadline (see StageTimeouts); cancellation is checked
// before every stage, so no stage runs after its context dies. No stage
// runs before the earlier ones pass, and the runner is never called when
// loading or validation fails.
//
// Passing setup/build/test does not verify the environment by itself. With
// no assessment inputs, passing commands yield an ASSESSMENT_EVIDENCE_PENDING
// rejection; with backend-controlled assessment inputs and review evidence,
// the verifier additionally requires the reviewed correct solution to pass
// and the wrong solution to fail for the intended reason before issuing a
// verified verdict. Promotion always stays disallowed pending registry
// integration.
func Verify(ctx context.Context, runner CommandRunner, builder BuilderResult, artifactRoot, verifierRunID string) VerifierResult {
	return VerifyWithOptions(ctx, runner, builder, artifactRoot, verifierRunID, DefaultVerifyOptions())
}

// VerifyWithOptions is Verify with explicit backend policy. Timeouts caps
// each command stage; use DefaultVerifyOptions for standing policy.
func VerifyWithOptions(ctx context.Context, runner CommandRunner, builder BuilderResult, artifactRoot, verifierRunID string, opts VerifyOptions) VerifierResult {
	if ctx == nil {
		ctx = context.Background()
	}
	v := &verifier{
		ctx:         ctx,
		runner:      runner,
		builder:     builder,
		verifierRun: strings.TrimSpace(verifierRunID),
		timeouts:    opts.Timeouts,
		assessment:  opts.Assessment,
		assessor:    opts.Assessor,
	}
	// The entry point enforces the builder contract itself, so a directly
	// constructed BuilderResult that bypasses ParseBuilderResult gets the
	// same validation as a parsed one.
	if strings.TrimSpace(v.builder.SchemaVersion) != BuilderResultSchemaVersion {
		return v.reject(StageManifest, CodeBuilderResultInvalid, true,
			fmt.Sprintf("The builder result has unsupported schema version %q.", v.builder.SchemaVersion),
			map[string]any{"field": "schemaVersion"})
	}
	if strings.TrimSpace(v.builder.BuilderRunID) == "" {
		return v.reject(StageManifest, CodeBuilderResultInvalid, true,
			"The builder result is missing its run ID.",
			map[string]any{"field": "builderRunId"})
	}
	if v.verifierRun == "" {
		return v.reject(StagePlatform, CodePlatformFault, false,
			"The backend did not supply a verifier run ID.",
			map[string]any{"field": "verifierRunId"})
	}
	v.builderRun = strings.TrimSpace(v.builder.BuilderRunID)
	root, err := resolveArtifactRoot(artifactRoot)
	if err != nil {
		return v.platformFailure(nil, "", err, "The artifact root could not be resolved.")
	}
	manifest, manifestPath, failure := v.loadManifest(root, builder)
	if failure != nil {
		return *failure
	}
	if err := Validate(manifest); err != nil {
		v.appendCheck(EvidenceCheck{Stage: StageManifest, ExitCode: 1, Summary: "Manifest structural validation failed."})
		return v.reject(StageManifest, CodeManifestInvalid, true,
			fmt.Sprintf("The proposed manifest failed structural validation: %v.", err),
			map[string]any{"manifest": manifestPath, "error": err.Error()})
	}
	v.appendCheck(EvidenceCheck{Stage: StageManifest, Summary: "Manifest parsed and passed structural validation."})

	workspaceDir, err := validateArtifactContents(root, manifest)
	if err != nil {
		v.appendCheck(EvidenceCheck{Stage: StageValidation, ExitCode: 1, Summary: "Artifact filesystem validation failed."})
		var platform *PlatformError
		if errors.As(err, &platform) {
			return v.platformFailure(&manifestPath, "", platform, "Artifact filesystem validation hit an infrastructure fault.")
		}
		var artifactErr *ArtifactError
		if errors.As(err, &artifactErr) {
			return v.reject(StageValidation, artifactErr.Code, true,
				artifactErr.Error()+".",
				map[string]any{"field": artifactErr.Field, "path": artifactErr.Entry})
		}
		if strings.HasPrefix(err.Error(), "environment manifest:") {
			return v.reject(StageManifest, CodeManifestInvalid, true,
				fmt.Sprintf("The proposed manifest failed structural validation: %v.", err),
				map[string]any{"manifest": manifestPath, "error": err.Error()})
		}
		return v.reject(StageValidation, CodeArtifactInvalid, true,
			fmt.Sprintf("Artifact validation failed: %v.", err),
			map[string]any{"error": err.Error()})
	}
	v.appendCheck(EvidenceCheck{Stage: StageValidation, Summary: "Artifact files and protected boundaries passed."})

	stages := []struct {
		stage   string
		command string
		code    string
	}{
		{StageSetup, strings.TrimSpace(manifest.Commands.Setup), CodeSetupFailed},
		{StageBuild, strings.TrimSpace(manifest.Commands.Build), CodeBuildFailed},
		{StageTest, strings.TrimSpace(manifest.Commands.Test), CodeTestFailed},
	}
	for _, s := range stages {
		if outcome := v.runStage(s.stage, s.command, s.code, workspaceDir); outcome != nil {
			return *outcome
		}
	}

	if opts.Assessment == nil {
		pending := v.reject(StageValidation, CodeAssessmentEvidencePending, false,
			"Setup, build, and test checks passed, but reviewed-correct and plausible-wrong "+
				"solution verification is pending, so no verified verdict is issued.",
			map[string]any{"pending": []string{"reviewed-correct solution", "plausible-wrong solution"}})
		pending.Promotion.Reason = "No promotion: passing commands alone do not authorize promotion while assessment verification is pending."
		return pending
	}
	if outcome := v.assessSolutions(workspaceDir, manifest); outcome != nil {
		return *outcome
	}
	// Every required check passed: manifest, artifact, setup, build, test,
	// correct solution, and intended wrong-solution failure. Promotion
	// stays disallowed: registry promotion prerequisites (promotion
	// metadata, owner/provenance, compatibility rules) are evaluated by
	// later registry integration, which is pending. Verified never means
	// promotable here.
	checks := v.checks
	if checks == nil {
		checks = []EvidenceCheck{}
	}
	return VerifierResult{
		SchemaVersion: VerifierResultSchemaVersion,
		VerifierRunID: v.verifierRun,
		BuilderRunID:  v.builderRun,
		Verdict:       VerdictVerified,
		FailedStage:   nil,
		Repairable:    false,
		Diagnostics:   []Diagnostic{},
		Evidence:      Evidence{Checks: checks},
		Promotion: Promotion{Allowed: false, Reason: "No promotion: registry promotion prerequisites " +
			"(promotion metadata, owner/provenance, compatibility rules) are evaluated by later " +
			"registry integration, which is pending."},
	}
}

type verifier struct {
	ctx         context.Context
	runner      CommandRunner
	builder     BuilderResult
	builderRun  string
	verifierRun string
	timeouts    StageTimeouts
	assessment  *AssessmentSpec
	assessor    AssessmentRunner
	checks      []EvidenceCheck
}

func (v *verifier) appendCheck(check EvidenceCheck) {
	v.checks = append(v.checks, check)
}

// loadManifest resolves the builder-proposed manifest path safely inside the
// backend-provided root and reads it. The builder path is untrusted input:
// absolute paths and escapes are rejected as manifest errors.
func (v *verifier) loadManifest(root string, builder BuilderResult) (Manifest, string, *VerifierResult) {
	fail := func(code, message string, details map[string]any, summary string) *VerifierResult {
		v.appendCheck(EvidenceCheck{Stage: StageManifest, ExitCode: 1, Summary: summary})
		result := v.reject(StageManifest, code, true, message, details)
		return &result
	}
	if builder.ProposedManifestPath == nil || strings.TrimSpace(*builder.ProposedManifestPath) == "" {
		result := fail(CodeManifestMissing,
			"The builder completed without a proposed manifest path.",
			map[string]any{"proposedManifestPath": nil},
			"No manifest was proposed.")
		return Manifest{}, "", result
	}
	raw := strings.TrimSpace(*builder.ProposedManifestPath)
	rel := filepath.FromSlash(raw)
	if filepath.IsAbs(rel) {
		result := fail(CodeManifestPathUnsafe,
			fmt.Sprintf("The proposed manifest path %q is absolute; only artifact-relative paths are allowed.", raw),
			map[string]any{"proposedManifestPath": raw},
			"Manifest path is unsafe.")
		return Manifest{}, "", result
	}
	joined := filepath.Join(root, rel)
	if escapesRoot(root, joined) {
		result := fail(CodeManifestPathUnsafe,
			fmt.Sprintf("The proposed manifest path %q escapes the artifact root.", raw),
			map[string]any{"proposedManifestPath": raw},
			"Manifest path is unsafe.")
		return Manifest{}, "", result
	}
	// The lexical check is not enough: a symlink inside the root may
	// resolve outside it. Resolve before reading so the verifier never
	// opens a file outside the backend-provided artifact root.
	canonical, err := filepath.EvalSymlinks(joined)
	if err != nil {
		if pathMissing(joined) {
			result := fail(CodeManifestMissing,
				fmt.Sprintf("The proposed manifest %q was not found under the artifact root.", raw),
				map[string]any{"proposedManifestPath": raw},
				"No manifest was found.")
			return Manifest{}, "", result
		}
		platform := v.platformFailure(&raw, "", err, "The proposed manifest could not be read.")
		return Manifest{}, "", &platform
	}
	if escapesRoot(root, canonical) {
		result := fail(CodeManifestPathUnsafe,
			fmt.Sprintf("The proposed manifest path %q resolves outside the artifact root.", raw),
			map[string]any{"proposedManifestPath": raw},
			"Manifest path is unsafe.")
		return Manifest{}, "", result
	}
	data, err := os.ReadFile(canonical)
	if err != nil {
		if os.IsNotExist(err) {
			result := fail(CodeManifestMissing,
				fmt.Sprintf("The proposed manifest %q was not found under the artifact root.", raw),
				map[string]any{"proposedManifestPath": raw},
				"No manifest was found.")
			return Manifest{}, "", result
		}
		v.appendCheck(EvidenceCheck{Stage: StageManifest, ExitCode: -1, Summary: "Manifest could not be read."})
		platform := v.platformFailure(&raw, "", err, "The proposed manifest could not be read.")
		return Manifest{}, "", &platform
	}
	manifest, err := ParseManifest(data)
	if err != nil {
		result := fail(CodeManifestMalformed,
			fmt.Sprintf("The proposed manifest %q could not be parsed: %v.", raw, err),
			map[string]any{"proposedManifestPath": raw, "error": err.Error()},
			"Manifest is malformed.")
		return Manifest{}, "", result
	}
	return manifest, raw, nil
}

// runStage executes one setup/build/test stage under its backend-owned
// deadline. A nil return means the stage passed and the caller continues; a
// non-nil return is the final rejected result. Cancellation is checked
// before the stage starts, so a dead parent context never launches another
// command, and later stages never run after a rejection.
func (v *verifier) runStage(stage, command, code, workDir string) *VerifierResult {
	if err := v.ctx.Err(); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			rejected := v.reject(StageTimeout, CodeStageTimeout, false,
				fmt.Sprintf("Verification stopped before %s: the parent context deadline was exceeded.", stage),
				stageDetails(stage, command, map[string]any{"timeout": true, "started": false}))
			return &rejected
		}
		platform := v.platformFailure(&command, stage, err,
			fmt.Sprintf("Verification stopped before %s: the parent context was cancelled.", stage))
		return &platform
	}
	if v.runner == nil {
		result := v.platformFailure(&command, stage, errors.New("command runner is not configured"), "No command runner is configured.")
		return &result
	}
	stageCtx := v.ctx
	cancel := func() {}
	if timeout := v.timeouts.TimeoutFor(stage); timeout > 0 {
		stageCtx, cancel = context.WithTimeout(v.ctx, timeout)
	}
	defer cancel()
	cmd := command
	result, err := v.runner.Run(stageCtx, CommandSpec{Stage: stage, Command: command, WorkDir: workDir})
	// A result returned after the backend context died is untrustworthy:
	// cancellation supersedes whatever the runner reports, so a runner
	// that returns success on a dead context can never advance
	// verification. Without this, a cancelled run could read as a pass.
	if err == nil {
		if ctxErr := v.ctx.Err(); ctxErr != nil {
			err = ctxErr
		}
	}
	check := EvidenceCheck{Stage: stage, Command: &cmd}
	// The backend deadline is authoritative: a stage whose context expired
	// is a timeout even if the runner reported another outcome.
	timedOut := result.TimedOut || errors.Is(err, context.DeadlineExceeded) || errors.Is(stageCtx.Err(), context.DeadlineExceeded)
	switch {
	case err == nil && !timedOut && result.ExitCode == 0:
		check.Summary = fmt.Sprintf("%s completed successfully.", stageTitle(stage))
		v.appendCheck(check)
		return nil
	case err == nil && !timedOut:
		check.ExitCode = result.ExitCode
		check.Summary = fmt.Sprintf("%s failed with exit code %d.", stageTitle(stage), result.ExitCode)
		v.appendCheck(check)
		rejected := v.reject(stage, code, true,
			fmt.Sprintf("The %s command failed with exit code %d.", stage, result.ExitCode),
			stageDetails(stage, command, map[string]any{"exitCode": result.ExitCode, "output": truncateOutput(result.Output)}))
		return &rejected
	case timedOut:
		// The original stage is preserved in the diagnostic details; the
		// failed stage is timeout per the shared contract.
		check.ExitCode = result.ExitCode
		check.Summary = fmt.Sprintf("%s timed out.", stageTitle(stage))
		v.appendCheck(check)
		rejected := v.reject(StageTimeout, CodeStageTimeout, false,
			fmt.Sprintf("The %s command exceeded its deadline.", stage),
			stageDetails(stage, command, map[string]any{"timeout": true}))
		return &rejected
	default:
		check.ExitCode = -1
		check.Summary = fmt.Sprintf("%s hit a platform fault before completing.", stageTitle(stage))
		v.appendCheck(check)
		platform := v.platformFailure(&command, stage, err, fmt.Sprintf("The %s command could not complete.", stage))
		return &platform
	}
}

// platformFailure reports an infrastructure fault, preserving the underlying
// error and the original stage in the diagnostic details. Platform faults
// are never repairable by the builder.
func (v *verifier) platformFailure(command *string, stage string, err error, message string) VerifierResult {
	details := map[string]any{"error": err.Error()}
	if stage != "" {
		details["stage"] = stage
	}
	if command != nil {
		details["command"] = *command
	}
	return v.reject(StagePlatform, CodePlatformFault, false,
		message+" Underlying fault: "+err.Error()+".", details)
}

// reject builds a rejected result over the checks attempted so far.
// Repairability policy (Track B decision, pending shared review):
// manifest, validation, setup, build, and test failures go back to the
// builder for bounded repair; timeouts, platform faults, and the pending
// assessment gate do not.
func (v *verifier) reject(stage, code string, repairable bool, message string, details map[string]any) VerifierResult {
	diagnostic := Diagnostic{Severity: "error", Stage: stage, Code: code, Message: message}
	if details != nil {
		diagnostic.Details = details
	}
	checks := v.checks
	if checks == nil {
		checks = []EvidenceCheck{}
	}
	failed := stage
	return VerifierResult{
		SchemaVersion: VerifierResultSchemaVersion,
		VerifierRunID: v.verifierRun,
		BuilderRunID:  v.builderRun,
		Verdict:       VerdictRejected,
		FailedStage:   &failed,
		Repairable:    repairable,
		Diagnostics:   []Diagnostic{diagnostic},
		Evidence:      Evidence{Checks: checks},
		Promotion:     Promotion{Allowed: false, Reason: "No promotion: verification rejected at stage " + stage + "."},
	}
}

func stageDetails(stage, command string, extra map[string]any) map[string]any {
	details := map[string]any{"stage": stage, "command": command}
	for key, value := range extra {
		details[key] = value
	}
	return details
}

func truncateOutput(output string) string {
	if len(output) <= maxDiagnosticOutput {
		return output
	}
	return output[:maxDiagnosticOutput] + "…[truncated]"
}

func stageTitle(stage string) string {
	if stage == "" {
		return stage
	}
	return strings.ToUpper(stage[:1]) + stage[1:]
}
