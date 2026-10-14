package environmentverify

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// stubRunner is the milestone 3 fake: generated commands are never executed
// on the host and no sandboxes are provisioned.
type stubOutcome struct {
	result CommandResult
	err    error
}

type stubRunner struct {
	calls         []CommandSpec
	outcomes      map[string]stubOutcome
	defaultResult CommandResult
}

func passingRunner() *stubRunner {
	return &stubRunner{defaultResult: CommandResult{ExitCode: 0}}
}

func (s *stubRunner) Run(_ context.Context, spec CommandSpec) (CommandResult, error) {
	s.calls = append(s.calls, spec)
	if outcome, ok := s.outcomes[spec.Stage]; ok {
		return outcome.result, outcome.err
	}
	return s.defaultResult, nil
}

func (s *stubRunner) stages() []string {
	stages := make([]string, 0, len(s.calls))
	for _, call := range s.calls {
		stages = append(stages, call.Stage)
	}
	return stages
}

func strptr(value string) *string { return &value }

// verifyFixture builds a temp artifact root holding every declared file of
// the valid example manifest plus a manifest.json, with a builder result
// pointing at it.
func verifyFixture(t *testing.T) (BuilderResult, string) {
	t.Helper()
	manifest := mustValidManifest(t)
	root := t.TempDir()
	materializeArtifact(t, root, manifest)
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), raw, 0o644); err != nil {
		t.Fatalf("write manifest.json: %v", err)
	}
	return BuilderResult{
		SchemaVersion:        BuilderResultSchemaVersion,
		BuilderRunID:         "buildrun_test",
		Runtime:              &RuntimeInfo{Name: "test-loop", Version: "0.0.1"},
		ProposedManifestPath: strptr("manifest.json"),
		Diagnostics:          []Diagnostic{},
		Telemetry:            &BuilderTelemetry{WallTimeMs: 1000},
	}, root
}

func requireRejected(t *testing.T, result VerifierResult, stage, code string) {
	t.Helper()
	if result.Verdict != VerdictRejected {
		t.Fatalf("verdict = %q, want rejected", result.Verdict)
	}
	if result.FailedStage == nil || *result.FailedStage != stage {
		t.Fatalf("failedStage = %v, want %q", result.FailedStage, stage)
	}
	if len(result.Diagnostics) == 0 || result.Diagnostics[0].Code != code {
		t.Fatalf("diagnostics = %+v, want code %q", result.Diagnostics, code)
	}
	if result.Promotion.Allowed {
		t.Fatalf("promotion.allowed = true, want false")
	}
	if result.SchemaVersion != VerifierResultSchemaVersion {
		t.Fatalf("schemaVersion = %q, want %q", result.SchemaVersion, VerifierResultSchemaVersion)
	}
}

func evidenceStages(result VerifierResult) []string {
	stages := make([]string, 0, len(result.Evidence.Checks))
	for _, check := range result.Evidence.Checks {
		stages = append(stages, check.Stage)
	}
	return stages
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestVerifyMissingManifestNoRunner(t *testing.T) {
	// Even when the builder claims success, a missing manifest rejects at
	// the manifest stage without touching the runner.
	builder := BuilderResult{
		SchemaVersion:        BuilderResultSchemaVersion,
		BuilderRunID:         "buildrun_claims_success",
		Runtime:              &RuntimeInfo{Name: "test-loop", Version: "0.0.1"},
		ProposedManifestPath: nil,
		Diagnostics: []Diagnostic{{
			Severity: "info", Stage: "prepare", Code: "BUILDER_CLAIMS_SUCCESS",
			Message: "All done, environment is ready for promotion.",
		}},
		Telemetry: &BuilderTelemetry{},
	}
	runner := passingRunner()
	result := Verify(context.Background(), runner, builder, t.TempDir(), "verify_missing")
	requireRejected(t, result, StageManifest, CodeManifestMissing)
	if len(runner.calls) != 0 {
		t.Fatalf("runner calls = %d, want 0", len(runner.calls))
	}
}

func TestVerifyMissingManifestExampleShape(t *testing.T) {
	builder, err := ParseBuilderResult(loadExample(t, "missing-manifest-builder-result.json"))
	if err != nil {
		t.Fatalf("ParseBuilderResult(example): %v", err)
	}
	if builder.BuilderRunID != "buildrun_missing_manifest" {
		t.Fatalf("builderRunId = %q", builder.BuilderRunID)
	}
	if builder.ProposedManifestPath != nil {
		t.Fatalf("proposedManifestPath = %q, want null", *builder.ProposedManifestPath)
	}
	runner := passingRunner()
	result := Verify(context.Background(), runner, builder, t.TempDir(), "verify_example")
	requireRejected(t, result, StageManifest, CodeManifestMissing)
	if len(runner.calls) != 0 {
		t.Fatalf("runner calls = %d, want 0", len(runner.calls))
	}
}

func TestVerifyMalformedManifestNoRunner(t *testing.T) {
	builder, root := verifyFixture(t)
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte(`{"schemaVersion":`), 0o644); err != nil {
		t.Fatalf("write malformed manifest: %v", err)
	}
	runner := passingRunner()
	result := Verify(context.Background(), runner, builder, root, "verify_malformed")
	requireRejected(t, result, StageManifest, CodeManifestMalformed)
	if len(runner.calls) != 0 {
		t.Fatalf("runner calls = %d, want 0", len(runner.calls))
	}

	invalid := mustValidManifest(t)
	invalid.Commands = nil
	raw, err := json.Marshal(invalid)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), raw, 0o644); err != nil {
		t.Fatalf("write invalid manifest: %v", err)
	}
	result = Verify(context.Background(), runner, builder, root, "verify_invalid")
	requireRejected(t, result, StageManifest, CodeManifestInvalid)
	if len(runner.calls) != 0 {
		t.Fatalf("runner calls = %d, want 0", len(runner.calls))
	}
}

func TestVerifyUnsafeArtifactNoRunner(t *testing.T) {
	traversal := mustValidManifest(t)
	traversal.Workspace.Protected = append(traversal.Workspace.Protected, "../outside.txt")
	root := t.TempDir()
	materializeArtifact(t, root, mustValidManifest(t))
	raw, _ := json.Marshal(traversal)
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), raw, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	builder, _ := verifyFixture(t)
	runner := passingRunner()
	result := Verify(context.Background(), runner, builder, root, "verify_traversal")
	requireRejected(t, result, StageValidation, CodeArtifactPathUnsafe)
	if len(runner.calls) != 0 {
		t.Fatalf("runner calls = %d, want 0", len(runner.calls))
	}

	missingBuilder, missingRoot := verifyFixture(t)
	victim := mustValidManifest(t).Environment.DependencyFiles[0]
	if err := os.Remove(filepath.Join(missingRoot, filepath.FromSlash(victim))); err != nil {
		t.Fatalf("remove %s: %v", victim, err)
	}
	result = Verify(context.Background(), runner, missingBuilder, missingRoot, "verify_missing_file")
	requireRejected(t, result, StageValidation, CodeArtifactFileMissing)
	if len(runner.calls) != 0 {
		t.Fatalf("runner calls = %d, want 0", len(runner.calls))
	}
}

func TestVerifyManifestSymlinkEscapeNoRunner(t *testing.T) {
	// A manifest path that is lexically inside the root but resolves
	// outside it (symlink escape) must be rejected before reading, with
	// the runner untouched.
	outside := t.TempDir()
	manifest := mustValidManifest(t)
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outside, "manifest.json"), raw, 0o644); err != nil {
		t.Fatalf("write outside manifest: %v", err)
	}
	root := t.TempDir()
	if err := os.Symlink(filepath.Join(outside, "manifest.json"), filepath.Join(root, "manifest.json")); err != nil {
		t.Fatalf("symlink manifest: %v", err)
	}
	builder, _ := verifyFixture(t)
	runner := passingRunner()
	result := Verify(context.Background(), runner, builder, root, "verify_manifest_symlink")
	requireRejected(t, result, StageManifest, CodeManifestPathUnsafe)
	if len(runner.calls) != 0 {
		t.Fatalf("runner calls = %d, want 0", len(runner.calls))
	}

	// An internal symlink to a manifest inside the root stays allowed.
	inner := t.TempDir()
	materializeArtifact(t, inner, mustValidManifest(t))
	if err := os.WriteFile(filepath.Join(inner, "manifest.json"), raw, 0o644); err != nil {
		t.Fatalf("write inner manifest: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(inner, "nested"), 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	if err := os.Symlink(filepath.Join(inner, "manifest.json"), filepath.Join(inner, "nested", "manifest.json")); err != nil {
		t.Fatalf("symlink inner manifest: %v", err)
	}
	linked := builder
	linked.ProposedManifestPath = strptr("nested/manifest.json")
	linkedRunner := passingRunner()
	linkedResult := Verify(context.Background(), linkedRunner, linked, inner, "verify_manifest_symlink_inner")
	// Manifest loading succeeds, so verification proceeds past the
	// manifest stage (it later stops at assessment evidence, which is the
	// pre-existing milestone-3 behavior without assessment inputs).
	if linkedResult.FailedStage == nil || *linkedResult.FailedStage == StageManifest {
		t.Fatalf("failedStage = %v, want progress past manifest loading", linkedResult.FailedStage)
	}
	if len(linkedRunner.calls) == 0 {
		t.Fatalf("runner calls = %d, want stages to run after manifest load", len(linkedRunner.calls))
	}
}

func TestVerifySetupFailureStops(t *testing.T) {
	builder, root := verifyFixture(t)
	runner := &stubRunner{
		defaultResult: CommandResult{ExitCode: 0},
		outcomes: map[string]stubOutcome{
			StageSetup: {result: CommandResult{ExitCode: 1, Output: "dependency went missing"}},
		},
	}
	result := Verify(context.Background(), runner, builder, root, "verify_setup_fail")
	requireRejected(t, result, StageSetup, CodeSetupFailed)
	if !result.Repairable {
		t.Fatalf("repairable = false, want true for stage failures")
	}
	if got := runner.stages(); !equalStrings(got, []string{StageSetup}) {
		t.Fatalf("runner stages = %v, want [setup]", got)
	}
	if got := evidenceStages(result); !equalStrings(got, []string{StageManifest, StageValidation, StageSetup}) {
		t.Fatalf("evidence stages = %v, want manifest/validation/setup only", got)
	}
	setup := result.Evidence.Checks[2]
	if setup.ExitCode != 1 || setup.Command == nil || !strings.Contains(*setup.Command, "dependency:go-offline") {
		t.Fatalf("setup check = %+v, want exit 1 with command", setup)
	}
	details, ok := result.Diagnostics[0].Details.(map[string]any)
	if !ok || details["exitCode"] != 1 {
		t.Fatalf("diagnostic details = %+v, want exitCode", result.Diagnostics[0].Details)
	}
}

func TestVerifyBuildFailure(t *testing.T) {
	builder, root := verifyFixture(t)
	runner := &stubRunner{
		defaultResult: CommandResult{ExitCode: 0},
		outcomes: map[string]stubOutcome{
			StageBuild: {result: CommandResult{ExitCode: 1, Output: "compilation failed"}},
		},
	}
	result := Verify(context.Background(), runner, builder, root, "verify_build_fail")
	requireRejected(t, result, StageBuild, CodeBuildFailed)
	if got := runner.stages(); !equalStrings(got, []string{StageSetup, StageBuild}) {
		t.Fatalf("runner stages = %v, want [setup build]", got)
	}
	if got := evidenceStages(result); !equalStrings(got, []string{StageManifest, StageValidation, StageSetup, StageBuild}) {
		t.Fatalf("evidence stages = %v", got)
	}
}

func TestVerifyTestFailure(t *testing.T) {
	builder, root := verifyFixture(t)
	runner := &stubRunner{
		defaultResult: CommandResult{ExitCode: 0},
		outcomes: map[string]stubOutcome{
			StageTest: {result: CommandResult{ExitCode: 1, Output: "2 tests failed"}},
		},
	}
	result := Verify(context.Background(), runner, builder, root, "verify_test_fail")
	requireRejected(t, result, StageTest, CodeTestFailed)
	if got := runner.stages(); !equalStrings(got, []string{StageSetup, StageBuild, StageTest}) {
		t.Fatalf("runner stages = %v, want all three stages", got)
	}
}

func TestVerifyTimeoutDistinguishable(t *testing.T) {
	builder, root := verifyFixture(t)
	runner := &stubRunner{
		defaultResult: CommandResult{ExitCode: 0},
		outcomes: map[string]stubOutcome{
			StageBuild: {result: CommandResult{TimedOut: true}},
		},
	}
	result := Verify(context.Background(), runner, builder, root, "verify_timeout")
	requireRejected(t, result, StageTimeout, CodeStageTimeout)
	if result.Repairable {
		t.Fatalf("repairable = true, want false for timeouts")
	}
	details, ok := result.Diagnostics[0].Details.(map[string]any)
	if !ok || details["stage"] != StageBuild {
		t.Fatalf("diagnostic details = %+v, want original stage preserved", result.Diagnostics[0].Details)
	}

	deadlineRunner := &stubRunner{
		defaultResult: CommandResult{ExitCode: 0},
		outcomes: map[string]stubOutcome{
			StageTest: {err: context.DeadlineExceeded},
		},
	}
	result = Verify(context.Background(), deadlineRunner, builder, root, "verify_deadline")
	requireRejected(t, result, StageTimeout, CodeStageTimeout)
	details, ok = result.Diagnostics[0].Details.(map[string]any)
	if !ok || details["stage"] != StageTest {
		t.Fatalf("diagnostic details = %+v, want original stage preserved", result.Diagnostics[0].Details)
	}
}

func TestVerifyPlatformFaultDistinguishable(t *testing.T) {
	builder, root := verifyFixture(t)
	runner := &stubRunner{
		defaultResult: CommandResult{ExitCode: 0},
		outcomes: map[string]stubOutcome{
			StageSetup: {err: errors.New("sandbox dial failed: connection refused")},
		},
	}
	result := Verify(context.Background(), runner, builder, root, "verify_platform")
	requireRejected(t, result, StagePlatform, CodePlatformFault)
	if result.Repairable {
		t.Fatalf("repairable = true, want false for platform faults")
	}
	details, ok := result.Diagnostics[0].Details.(map[string]any)
	if !ok || details["stage"] != StageSetup {
		t.Fatalf("diagnostic details = %+v, want original stage preserved", result.Diagnostics[0].Details)
	}
	if !strings.Contains(result.Diagnostics[0].Message, "connection refused") {
		t.Fatalf("diagnostic message = %q, want underlying fault preserved", result.Diagnostics[0].Message)
	}
	if got := evidenceStages(result); !equalStrings(got, []string{StageManifest, StageValidation, StageSetup}) {
		t.Fatalf("evidence stages = %v", got)
	}
}

func TestVerifyArtifactRootFaultIsPlatform(t *testing.T) {
	// A backend-provided root that is not a directory is infrastructure,
	// not a builder failure; the underlying error is preserved.
	builder, _ := verifyFixture(t)
	notDir := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(notDir, []byte("test"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	runner := passingRunner()
	result := Verify(context.Background(), runner, builder, notDir, "verify_root")
	requireRejected(t, result, StagePlatform, CodePlatformFault)
	if len(runner.calls) != 0 {
		t.Fatalf("runner calls = %d, want 0", len(runner.calls))
	}
}

func TestVerifyPassingCommandsPendingAssessment(t *testing.T) {
	builder, root := verifyFixture(t)
	runner := passingRunner()
	result := Verify(context.Background(), runner, builder, root, "verify_pending")
	requireRejected(t, result, StageValidation, CodeAssessmentEvidencePending)
	if result.Repairable {
		t.Fatalf("repairable = true, want false while verifier capability is pending")
	}
	if got := runner.stages(); !equalStrings(got, []string{StageSetup, StageBuild, StageTest}) {
		t.Fatalf("runner stages = %v, want all three stages", got)
	}
	if got := evidenceStages(result); !equalStrings(got, []string{StageManifest, StageValidation, StageSetup, StageBuild, StageTest}) {
		t.Fatalf("evidence stages = %v", got)
	}
	for _, check := range result.Evidence.Checks {
		if check.ExitCode != 0 {
			t.Fatalf("check %+v has nonzero exit", check)
		}
	}
	if !strings.Contains(result.Promotion.Reason, "assessment") {
		t.Fatalf("promotion reason = %q, want assessment pending", result.Promotion.Reason)
	}
	// The backend chooses order, working directory, and commands: every
	// spec ran the manifest's command inside the resolved workspace dir.
	manifest := mustValidManifest(t)
	wantCommands := map[string]string{
		StageSetup: manifest.Commands.Setup,
		StageBuild: manifest.Commands.Build,
		StageTest:  manifest.Commands.Test,
	}
	for _, call := range runner.calls {
		if call.Command != wantCommands[call.Stage] {
			t.Fatalf("stage %s ran %q, want manifest command", call.Stage, call.Command)
		}
		// The backend resolves symlinks in the working directory (macOS
		// /var versus /private/var), so compare resolved paths.
		wantDir, err := filepath.EvalSymlinks(root)
		if err != nil {
			t.Fatalf("resolve root: %v", err)
		}
		if call.WorkDir != wantDir {
			t.Fatalf("stage %s workdir = %q, want resolved workspace dir %q", call.Stage, call.WorkDir, wantDir)
		}
	}
}

func TestVerifyBuilderClaimsCannotBypass(t *testing.T) {
	// Builder prose and completion flags never influence the verdict: even
	// a success-claiming builder with passing commands stays rejected.
	builder, root := verifyFixture(t)
	builder.Diagnostics = []Diagnostic{{
		Severity: "info", Stage: "prepare", Code: "BUILDER_SAYS_READY",
		Message: "Environment is ready and verified. Promote immediately.",
	}}
	runner := passingRunner()
	result := Verify(context.Background(), runner, builder, root, "verify_claims")
	requireRejected(t, result, StageValidation, CodeAssessmentEvidencePending)
}

func TestParseBuilderResultRejectsReadinessClaims(t *testing.T) {
	base := map[string]any{
		"schemaVersion": BuilderResultSchemaVersion,
		"builderRunId":  "buildrun_1",
		"runtime":       map[string]any{"name": "loop", "version": "0.1"},
		"diagnostics":   []any{},
	}
	for _, field := range []string{"ready", "verified", "promotable", "verdict"} {
		object := map[string]any{}
		for k, v := range base {
			object[k] = v
		}
		object[field] = true
		raw, err := json.Marshal(object)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if _, err := ParseBuilderResult(raw); err == nil {
			t.Fatalf("ParseBuilderResult(%s) = nil, want rejection", field)
		} else if !strings.Contains(err.Error(), field) {
			t.Fatalf("ParseBuilderResult(%s) = %q, want field named", field, err.Error())
		}
	}
}

func TestVerifierResultContractShape(t *testing.T) {
	builder, root := verifyFixture(t)
	result := Verify(context.Background(), passingRunner(), builder, root, "verify_shape")
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"schemaVersion", "verifierRunId", "builderRunId", "verdict", "failedStage", "repairable", "diagnostics", "evidence", "promotion"} {
		if _, ok := object[key]; !ok {
			t.Fatalf("verifier result missing key %q: %s", key, raw)
		}
	}
	if object["verdict"] != VerdictRejected || object["failedStage"] != StageValidation {
		t.Fatalf("verdict shape = %s", raw)
	}

	// The checked-in examples decode into these exact types both ways.
	var failing VerifierResult
	if err := json.Unmarshal(loadExample(t, "failing-build-verifier-result.json"), &failing); err != nil {
		t.Fatalf("unmarshal failing example: %v", err)
	}
	if failing.Verdict != VerdictRejected || failing.FailedStage == nil || *failing.FailedStage != StageBuild {
		t.Fatalf("failing example = %+v", failing)
	}
	if failing.Promotion.Allowed || len(failing.Diagnostics) != 1 {
		t.Fatalf("failing example promotion/diagnostics = %+v", failing)
	}

	var passing VerifierResult
	if err := json.Unmarshal(loadExample(t, "passing-build-verifier-result.json"), &passing); err != nil {
		t.Fatalf("unmarshal passing example: %v", err)
	}
	if passing.Verdict != VerdictVerified || passing.FailedStage != nil {
		t.Fatalf("passing example = %+v", passing)
	}

	// Builder output serializes without any readiness-claiming field.
	builderRaw, err := json.Marshal(builder)
	if err != nil {
		t.Fatalf("marshal builder: %v", err)
	}
	var builderObject map[string]any
	if err := json.Unmarshal(builderRaw, &builderObject); err != nil {
		t.Fatalf("unmarshal builder: %v", err)
	}
	for _, forbidden := range []string{"ready", "verified", "promotable", "verdict", "promotion"} {
		if _, ok := builderObject[forbidden]; ok {
			t.Fatalf("builder result must not include %q", forbidden)
		}
	}
}

// blockingRunner waits for context cancellation before returning, the way a
// well-behaved subprocess runner must (terminate work on Done). It records
// whether the verifier supplied a real deadline per stage. These tests prove
// deadline propagation and enforcement at the orchestration layer; they do
// not prove real subprocess termination, which is the future runner
// implementation's job.
type blockingRunner struct {
	calls       []CommandSpec
	deadlines   map[string]time.Time
	hasDeadline map[string]bool
	passStages  map[string]bool
}

func newBlockingRunner() *blockingRunner {
	return &blockingRunner{
		deadlines:   map[string]time.Time{},
		hasDeadline: map[string]bool{},
		passStages:  map[string]bool{},
	}
}

func (r *blockingRunner) Run(ctx context.Context, spec CommandSpec) (CommandResult, error) {
	r.calls = append(r.calls, spec)
	if r.passStages[spec.Stage] {
		return CommandResult{ExitCode: 0}, nil
	}
	deadline, ok := ctx.Deadline()
	r.deadlines[spec.Stage] = deadline
	r.hasDeadline[spec.Stage] = ok
	<-ctx.Done()
	return CommandResult{}, ctx.Err()
}

func (r *blockingRunner) stages() []string {
	stages := make([]string, 0, len(r.calls))
	for _, call := range r.calls {
		stages = append(stages, call.Stage)
	}
	return stages
}

func TestVerifyStageDeadlineEnforced(t *testing.T) {
	builder, root := verifyFixture(t)
	runner := newBlockingRunner()
	started := time.Now()
	opts := VerifyOptions{Timeouts: StageTimeouts{Setup: 40 * time.Millisecond}}
	result := VerifyWithOptions(context.Background(), runner, builder, root, "verify_stage_deadline", opts)
	requireRejected(t, result, StageTimeout, CodeStageTimeout)
	// The verifier supplied an actual deadline near the configured limit.
	if !runner.hasDeadline[StageSetup] {
		t.Fatalf("setup context had no deadline, want verifier-owned timeout")
	}
	deadline := runner.deadlines[StageSetup]
	if deadline.Before(started) || deadline.After(started.Add(5*time.Second)) {
		t.Fatalf("setup deadline = %v, want near the 40ms configured limit", deadline)
	}
	// Timeout diagnostics stay tied to the original command stage, and
	// later stages never run.
	details, ok := result.Diagnostics[0].Details.(map[string]any)
	if !ok || details["stage"] != StageSetup {
		t.Fatalf("diagnostic details = %+v, want original stage setup", result.Diagnostics[0].Details)
	}
	if got := runner.stages(); !equalStrings(got, []string{StageSetup}) {
		t.Fatalf("runner stages = %v, want setup only", got)
	}
	if got := evidenceStages(result); !equalStrings(got, []string{StageManifest, StageValidation, StageSetup}) {
		t.Fatalf("evidence stages = %v", got)
	}
}

func TestVerifyPerStageDeadlineBuildOnly(t *testing.T) {
	// Only the build stage carries a verifier deadline; setup passes
	// without one, the build times out, and test never starts.
	builder, root := verifyFixture(t)
	runner := newBlockingRunner()
	runner.passStages[StageSetup] = true
	opts := VerifyOptions{Timeouts: StageTimeouts{Build: 40 * time.Millisecond}}
	result := VerifyWithOptions(context.Background(), runner, builder, root, "verify_build_deadline", opts)
	requireRejected(t, result, StageTimeout, CodeStageTimeout)
	if runner.hasDeadline[StageSetup] {
		t.Fatalf("setup context had a deadline, want none configured")
	}
	if !runner.hasDeadline[StageBuild] {
		t.Fatalf("build context had no deadline, want verifier-owned timeout")
	}
	details, ok := result.Diagnostics[0].Details.(map[string]any)
	if !ok || details["stage"] != StageBuild {
		t.Fatalf("diagnostic details = %+v, want original stage build", result.Diagnostics[0].Details)
	}
	if got := runner.stages(); !equalStrings(got, []string{StageSetup, StageBuild}) {
		t.Fatalf("runner stages = %v, want setup then build", got)
	}
	if got := evidenceStages(result); !equalStrings(got, []string{StageManifest, StageValidation, StageSetup, StageBuild}) {
		t.Fatalf("evidence stages = %v", got)
	}
}

func TestVerifyParentCancellationPreventsStages(t *testing.T) {
	builder, root := verifyFixture(t)
	runner := passingRunner()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := VerifyWithOptions(ctx, runner, builder, root, "verify_parent_cancel", DefaultVerifyOptions())
	requireRejected(t, result, StagePlatform, CodePlatformFault)
	if len(runner.calls) != 0 {
		t.Fatalf("runner calls = %d, want 0: cancelled context must not start stages", len(runner.calls))
	}
	details, ok := result.Diagnostics[0].Details.(map[string]any)
	if !ok || details["stage"] != StageSetup {
		t.Fatalf("diagnostic details = %+v, want upcoming stage setup", result.Diagnostics[0].Details)
	}
	if got := evidenceStages(result); !equalStrings(got, []string{StageManifest, StageValidation}) {
		t.Fatalf("evidence stages = %v, want no stage check attempted", got)
	}

	// An already-expired parent deadline is a timeout, not a stage failure.
	expired, stop := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer stop()
	result = VerifyWithOptions(expired, runner, builder, root, "verify_parent_deadline", DefaultVerifyOptions())
	requireRejected(t, result, StageTimeout, CodeStageTimeout)
	if len(runner.calls) != 0 {
		t.Fatalf("runner calls = %d, want 0", len(runner.calls))
	}
}

func TestVerifyMidRunCancellationStopsLaterStages(t *testing.T) {
	builder, root := verifyFixture(t)
	runner := newBlockingRunner()
	runner.passStages[StageSetup] = true
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(30*time.Millisecond, cancel)
	result := VerifyWithOptions(ctx, runner, builder, root, "verify_midrun_cancel", DefaultVerifyOptions())
	requireRejected(t, result, StagePlatform, CodePlatformFault)
	details, ok := result.Diagnostics[0].Details.(map[string]any)
	if !ok || details["stage"] != StageBuild {
		t.Fatalf("diagnostic details = %+v, want original stage build", result.Diagnostics[0].Details)
	}
	if got := runner.stages(); !equalStrings(got, []string{StageSetup, StageBuild}) {
		t.Fatalf("runner stages = %v, want test prevented after cancellation", got)
	}
}

// cancelThenSucceedRunner cancels the backend context and then reports
// success, simulating a runner that outlives cancellation.
type cancelThenSucceedRunner struct {
	cancel context.CancelFunc
	calls  []CommandSpec
}

func (r *cancelThenSucceedRunner) Run(_ context.Context, spec CommandSpec) (CommandResult, error) {
	r.calls = append(r.calls, spec)
	r.cancel()
	return CommandResult{ExitCode: 0}, nil
}

func (r *cancelThenSucceedRunner) stages() []string {
	stages := make([]string, 0, len(r.calls))
	for _, call := range r.calls {
		stages = append(stages, call.Stage)
	}
	return stages
}

func TestVerifyCancelledSuccessNeverPasses(t *testing.T) {
	// Success reported after the backend context died must not read as a
	// pass: cancellation supersedes the result and later stages never run.
	builder, root := verifyFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	runner := &cancelThenSucceedRunner{cancel: cancel}
	result := VerifyWithOptions(ctx, runner, builder, root, "verify_cancel_success", DefaultVerifyOptions())
	requireRejected(t, result, StagePlatform, CodePlatformFault)
	if got := runner.stages(); !equalStrings(got, []string{StageSetup}) {
		t.Fatalf("runner stages = %v, want setup only", got)
	}
	if got := evidenceStages(result); !equalStrings(got, []string{StageManifest, StageValidation, StageSetup}) {
		t.Fatalf("evidence stages = %v, want the cancelled setup recorded", got)
	}
}

func TestVerifyDirectBuilderResultValidation(t *testing.T) {
	// The public entry point enforces the builder contract itself, so a
	// directly constructed BuilderResult that bypasses ParseBuilderResult
	// gets the same validation as a parsed one.
	cases := []struct {
		name      string
		mutate    func(*BuilderResult)
		runID     string
		stage     string
		code      string
		wantField string
	}{
		{"wrong schema version", func(b *BuilderResult) {
			b.SchemaVersion = "environment.builder-result.v0"
		}, "verify_direct_version", StageManifest, CodeBuilderResultInvalid, "schemaVersion"},
		{"blank schema version", func(b *BuilderResult) {
			b.SchemaVersion = "  "
		}, "verify_direct_blank_version", StageManifest, CodeBuilderResultInvalid, "schemaVersion"},
		{"blank builder run", func(b *BuilderResult) {
			b.BuilderRunID = "  "
		}, "verify_direct_run", StageManifest, CodeBuilderResultInvalid, "builderRunId"},
		{"blank verifier run", func(_ *BuilderResult) {}, "", StagePlatform, CodePlatformFault, "verifierRunId"},
		{"whitespace verifier run", func(_ *BuilderResult) {}, "  ", StagePlatform, CodePlatformFault, "verifierRunId"},
	}
	for _, tc := range cases {
		builder, root := verifyFixture(t)
		tc.mutate(&builder)
		runner := passingRunner()
		result := Verify(context.Background(), runner, builder, root, tc.runID)
		requireRejected(t, result, tc.stage, tc.code)
		if len(runner.calls) != 0 {
			t.Fatalf("%s: runner calls = %d, want 0", tc.name, len(runner.calls))
		}
		details, ok := result.Diagnostics[0].Details.(map[string]any)
		if !ok || details["field"] != tc.wantField {
			t.Fatalf("%s: diagnostic details = %+v, want field %q", tc.name, result.Diagnostics[0].Details, tc.wantField)
		}
	}
}
