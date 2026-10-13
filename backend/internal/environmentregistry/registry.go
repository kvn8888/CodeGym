package environmentregistry

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kvn8888/codegym/backend/internal/environmentverify"
)

// Preparation is the backend-owned verification context for one
// promotion: the exact manifest verified, the prepared snapshot reference,
// and the builder run that produced them. A Preparation never stands
// alone as proof: only the sealed VerifiedPreparation created by
// VerifyPreparation carries verification authority, and Promote boots
// exactly the snapshot the seal names.
type Preparation struct {
	BuilderRunID string
	Manifest     environmentverify.Manifest
	SnapshotRef  string
}

// VerifiedPreparation is the backend-owned association of one preparation
// with the verifier result produced for it. Fields are unexported, so
// outside this package the only way to obtain one is VerifyPreparation,
// which runs the verifier against the preparation's own handoff and
// enforces run identity, snapshot identity, evidence-command binding,
// and promotion authority before sealing.
//
// Trusted construction boundary, stated accurately: the seal guarantees
// that one verification path saw one preparation — the same manifest,
// snapshot reference, and run ID presented for promotion — and issued
// the bound verdict under registry-pending authority. It does not
// establish byte-level content binding: evidence-command equality
// constrains the executed setup/build/test stages only, and the snapshot
// reference is an opaque handle whose content the registry never hashes.
// No fingerprints or hashes are used or required for this association.
type VerifiedPreparation struct {
	preparation Preparation
	verified    environmentverify.VerifierResult
}

// PromotionRequest asks the registry to promote one verified preparation.
// There is deliberately no caller-supplied Preparation: Promote derives
// the manifest, snapshot, and builder run exclusively from the sealed
// VerifiedPreparation, so no second assembly can diverge from what was
// verified — snapshot substitution is inexpressible, not merely checked.
type PromotionRequest struct {
	Verified VerifiedPreparation
	Owner    string
	BuiltBy  string
}

// VerifyPreparation creates the sealed verified-preparation association
// for one builder handoff. The preparation is derived directly from that
// handoff — never from a second, independently assembled manifest — and
// captured as an independent deep copy, so later caller mutations of the
// handoff cannot alter what was verified. The verifier result is gated
// exactly as promotion requires (verified verdict, registry-pending
// authority, valid manifest, preparation binding) before sealing, so a
// sealed value always carries a fully gated outcome. Sealing performs no
// boot and no storage.
func (s *Service) VerifyPreparation(ctx context.Context, build BuildOutput) (VerifiedPreparation, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := checkRegistryCtx(ctx, "verify preparation"); err != nil {
		return VerifiedPreparation{}, err
	}
	if s.Verifier == nil {
		return VerifiedPreparation{}, errors.New("environment verifier is not configured")
	}
	prep := Preparation{
		BuilderRunID: strings.TrimSpace(build.Builder.BuilderRunID),
		Manifest:     cloneManifest(build.Manifest),
		SnapshotRef:  strings.TrimSpace(build.SnapshotRef),
	}
	if prep.BuilderRunID == "" {
		return VerifiedPreparation{}, &PromotionRefusal{
			Reason:     "preparation has no builder run ID",
			Repairable: true,
			Details:    map[string]any{"field": "builderRunId"},
		}
	}
	if prep.SnapshotRef == "" {
		return VerifiedPreparation{}, &PromotionRefusal{
			Reason:     "preparation has no snapshot reference",
			Repairable: true,
			Details:    map[string]any{"field": "snapshotRef"},
		}
	}
	if err := environmentverify.Validate(prep.Manifest); err != nil {
		return VerifiedPreparation{}, &PromotionRefusal{
			Reason:     fmt.Sprintf("invalid manifest: %v", err),
			Repairable: true,
			Details:    map[string]any{"error": err.Error()},
		}
	}
	if err := checkRegistryCtx(ctx, "verification"); err != nil {
		return VerifiedPreparation{}, err
	}
	verified := s.Verifier.Verify(ctx, build)
	if err := checkVerified(verified); err != nil {
		return VerifiedPreparation{}, err
	}
	if err := checkPromotionAuthority(verified); err != nil {
		return VerifiedPreparation{}, err
	}
	if err := checkVerifierShape(prep, verified); err != nil {
		return VerifiedPreparation{}, err
	}
	if err := checkPreparationBinding(prep, verified); err != nil {
		return VerifiedPreparation{}, err
	}
	return VerifiedPreparation{preparation: prep, verified: cloneVerifierResult(verified)}, nil
}

// checkVerifierShape rejects malformed or self-contradictory verifier
// results before sealing: the contract schema version must match, the
// verifier run ID must be nonblank, and the evidence must show the
// required setup, build, and test checks actually ran successfully — each
// present, each exit-clean, each recording the exact command the sealed
// manifest declares for its stage. Manifest-only evidence, an omitted
// required stage, a required check with no recorded command, or a
// duplicate check contradicting the manifest or the verified verdict all
// refuse here. An empty list, a nonzero exit anywhere else, or a command
// on an unexpected stage likewise authorizes nothing merely because the
// verdict says verified. A malformed result is a backend fault, never a
// builder repair signal.
//
// The manifest was validated before this gate, so Commands is present;
// the nil guard below is defense in depth, never the normal path.
func checkVerifierShape(prep Preparation, verified environmentverify.VerifierResult) error {
	if strings.TrimSpace(verified.SchemaVersion) != environmentverify.VerifierResultSchemaVersion {
		return &PromotionRefusal{
			Reason:     fmt.Sprintf("verifier result has unsupported schema version %q", verified.SchemaVersion),
			Repairable: false,
			Details:    map[string]any{"schemaVersion": verified.SchemaVersion},
		}
	}
	if strings.TrimSpace(verified.VerifierRunID) == "" {
		return &PromotionRefusal{
			Reason:     "verifier result is missing its verifier run ID",
			Repairable: false,
			Details:    map[string]any{"field": "verifierRunId"},
		}
	}
	if len(verified.Evidence.Checks) == 0 {
		return &PromotionRefusal{
			Reason:     "verifier result carries no check evidence",
			Repairable: false,
			Details:    map[string]any{"field": "evidence.checks"},
		}
	}
	expected := map[string]string{}
	if prep.Manifest.Commands != nil {
		expected = map[string]string{
			environmentverify.StageSetup: strings.TrimSpace(prep.Manifest.Commands.Setup),
			environmentverify.StageBuild: strings.TrimSpace(prep.Manifest.Commands.Build),
			environmentverify.StageTest:  strings.TrimSpace(prep.Manifest.Commands.Test),
		}
	}
	for _, step := range []struct {
		stage   string
		command string
	}{
		{environmentverify.StageSetup, expected[environmentverify.StageSetup]},
		{environmentverify.StageBuild, expected[environmentverify.StageBuild]},
		{environmentverify.StageTest, expected[environmentverify.StageTest]},
	} {
		found := false
		for _, check := range verified.Evidence.Checks {
			if check.Stage != step.stage {
				continue
			}
			found = true
			if check.ExitCode != 0 {
				return &PromotionRefusal{
					Reason:     fmt.Sprintf("verifier evidence records exit code %d at required stage %q against a verified verdict", check.ExitCode, check.Stage),
					Repairable: false,
					Details:    map[string]any{"stage": check.Stage, "exitCode": check.ExitCode},
				}
			}
			if check.Command == nil {
				return &PromotionRefusal{
					Reason:     fmt.Sprintf("verifier evidence %q check records no command", check.Stage),
					Repairable: false,
					Details:    map[string]any{"stage": check.Stage},
				}
			}
			if step.command == "" || strings.TrimSpace(*check.Command) != step.command {
				return &PromotionRefusal{
					Reason:     fmt.Sprintf("verifier evidence %q command does not match the sealed manifest", check.Stage),
					Repairable: false,
					Details:    map[string]any{"stage": check.Stage, "want": step.command, "observed": strings.TrimSpace(*check.Command)},
				}
			}
		}
		if !found {
			return &PromotionRefusal{
				Reason:     fmt.Sprintf("verifier evidence omits required %q check", step.stage),
				Repairable: false,
				Details:    map[string]any{"stage": step.stage},
			}
		}
	}
	for _, check := range verified.Evidence.Checks {
		if check.ExitCode != 0 {
			return &PromotionRefusal{
				Reason:     fmt.Sprintf("verifier evidence records exit code %d at stage %q against a verified verdict", check.ExitCode, check.Stage),
				Repairable: false,
				Details:    map[string]any{"stage": check.Stage, "exitCode": check.ExitCode},
			}
		}
	}
	return nil
}

// cloneManifest captures an independent deep copy of a manifest: every
// section pointer is re-pointed and every string slice is re-backed, with
// nil slices staying nil (Validate distinguishes omitted from explicitly
// empty). Later mutations of the source — reassigned scalars, mutated
// slice elements, or nilled sections — cannot reach the copy.
func cloneManifest(manifest environmentverify.Manifest) environmentverify.Manifest {
	out := manifest
	if manifest.Artifact != nil {
		artifact := *manifest.Artifact
		out.Artifact = &artifact
	}
	if manifest.Environment != nil {
		environment := *manifest.Environment
		environment.DependencyFiles = cloneStrings(manifest.Environment.DependencyFiles)
		environment.Lockfiles = cloneStrings(manifest.Environment.Lockfiles)
		out.Environment = &environment
	}
	if manifest.Workspace != nil {
		workspace := *manifest.Workspace
		workspace.LearnerEditable = cloneStrings(manifest.Workspace.LearnerEditable)
		workspace.Protected = cloneStrings(manifest.Workspace.Protected)
		out.Workspace = &workspace
	}
	if manifest.Commands != nil {
		commands := *manifest.Commands
		out.Commands = &commands
	}
	if manifest.Assessment != nil {
		assessment := *manifest.Assessment
		assessment.VisibleTests = cloneStrings(manifest.Assessment.VisibleTests)
		assessment.HiddenTests = cloneStrings(manifest.Assessment.HiddenTests)
		out.Assessment = &assessment
	}
	if manifest.Compatibility != nil {
		compatibility := *manifest.Compatibility
		compatibility.ReuseTags = cloneStrings(manifest.Compatibility.ReuseTags)
		out.Compatibility = &compatibility
	}
	return out
}

// cloneStrings re-backs a string slice, preserving the nil versus
// explicitly-empty distinction exactly (Validate rejects an omitted
// array but accepts an explicit empty one, so collapsing empty to nil
// would corrupt the copy).
func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}

// cloneVerifierResult captures an independent deep copy of the mutable
// verdict and evidence data the seal retains: evidence checks (including
// their command pointers), diagnostics (including structured details),
// and the failed-stage pointer. Strings are immutable and safely shared.
func cloneVerifierResult(verified environmentverify.VerifierResult) environmentverify.VerifierResult {
	out := verified
	if verified.FailedStage != nil {
		stage := *verified.FailedStage
		out.FailedStage = &stage
	}
	if verified.Diagnostics != nil {
		diagnostics := make([]environmentverify.Diagnostic, len(verified.Diagnostics))
		for i, diagnostic := range verified.Diagnostics {
			diagnostics[i] = diagnostic
			diagnostics[i].Details = cloneDetail(diagnostic.Details)
		}
		out.Diagnostics = diagnostics
	}
	if verified.Evidence.Checks != nil {
		checks := make([]environmentverify.EvidenceCheck, len(verified.Evidence.Checks))
		for i, check := range verified.Evidence.Checks {
			checks[i] = check
			if check.Command != nil {
				command := *check.Command
				checks[i].Command = &command
			}
		}
		out.Evidence.Checks = checks
	}
	return out
}

// cloneDetail copies the mutable JSON-like values a diagnostic may carry
// in Details. Maps and slices recurse; scalars and all other values are
// immutable or opaque and shared, which is documented as the residual
// limit of this copy.
func cloneDetail(details any) any {
	switch typed := details.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, value := range typed {
			out[key] = cloneDetail(value)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, value := range typed {
			out[i] = cloneDetail(value)
		}
		return out
	default:
		return details
	}
}

// PromotionReceipt is the registry's own promotion decision. Allowed is
// true only after every registry gate passed: verified verdict, backend
// promotion authority (registry-pending, never an explicit denial),
// valid manifest, exact preparation binding, and fresh boot checks under
// the practice network policy. VerifierPromotion preserves the verifier
// result's promotion stance verbatim. A future allowed=true is never
// trusted as authorization — registry gates still decide independently —
// but an explicit backend denial (disallowed for any reason other than
// #188's standing registry-pending stance) can never be overridden to
// allowed=true here. That split is flagged for #184 (see doc.go).
type PromotionReceipt struct {
	Allowed           bool
	Reason            string
	Record            ArtifactRecord
	Policy            NetworkPolicy
	CheckedAt         time.Time
	VerifierPromotion environmentverify.Promotion
}

// PromotionRefusal explains why an artifact was not promoted. Refusals are
// explicit and never silent; builder-repairable shape problems stay
// repairable so the builder track can retry with corrected output.
type PromotionRefusal struct {
	Reason     string
	Repairable bool
	Details    map[string]any
}

func (e *PromotionRefusal) Error() string {
	return "environment promotion refused: " + e.Reason
}

// Service wires registry storage, boot checks, and guarded builder
// fallback behind small interfaces. All providers are injected; tests use
// fakes, production wires durable implementations.
type Service struct {
	Store        Store
	Snapshots    SnapshotProvider
	Builder      Builder
	Capabilities Capabilities
	Quota        Quota
	Verifier     Verifier
	// PracticePolicy is the network policy every promotion and reuse boot
	// check runs under. Zero value means isolated.
	PracticePolicy NetworkPolicy
	// CleanupTimeout bounds boot teardown on a cancellation-decoupled
	// context. Zero value means DefaultCleanupTimeout. This is a
	// backend-owned limit: callers never supply cleanup deadlines.
	CleanupTimeout time.Duration
}

// DefaultCleanupTimeout bounds one boot teardown when the service sets no
// explicit CleanupTimeout.
const DefaultCleanupTimeout = 30 * time.Second

func (s *Service) practicePolicy() NetworkPolicy {
	if s.PracticePolicy == "" {
		return NetworkPolicyIsolated
	}
	return s.PracticePolicy
}

func (s *Service) cleanupTimeout() time.Duration {
	if s == nil || s.CleanupTimeout <= 0 {
		return DefaultCleanupTimeout
	}
	return s.CleanupTimeout
}

// Promote records one prepared artifact after passing every registry gate.
// The manifest, snapshot, and builder run derive exclusively from the
// sealed verified preparation — there is no caller-supplied preparation
// to diverge from what was verified. The sealed verdict must be verified
// under registry-pending authority, the sealed manifest must be valid,
// and fresh boot plus representative build/test checks under the practice
// network policy must pass against the sealed snapshot. Any gate failure
// refuses promotion; failed boot checks never offer the artifact as ready.
func (s *Service) Promote(ctx context.Context, req PromotionRequest) (PromotionReceipt, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := checkRegistryCtx(ctx, "promote"); err != nil {
		return PromotionReceipt{}, err
	}
	sealed := req.Verified
	prep := sealed.preparation
	if err := checkVerified(sealed.verified); err != nil {
		return PromotionReceipt{}, err
	}
	if err := checkPromotionAuthority(sealed.verified); err != nil {
		return PromotionReceipt{}, err
	}
	if err := environmentverify.Validate(prep.Manifest); err != nil {
		return PromotionReceipt{}, &PromotionRefusal{
			Reason:     fmt.Sprintf("invalid manifest: %v", err),
			Repairable: true,
			Details:    map[string]any{"error": err.Error()},
		}
	}
	if err := checkPreparationBinding(prep, sealed.verified); err != nil {
		return PromotionReceipt{}, err
	}
	if strings.TrimSpace(req.Owner) == "" {
		return PromotionReceipt{}, &PromotionRefusal{
			Reason:     "promotion requires an owner",
			Repairable: true,
			Details:    map[string]any{"artifact": strings.TrimSpace(prep.Manifest.Artifact.ID)},
		}
	}
	now := time.Now().UTC()
	record := ArtifactRecord{
		ArtifactID:      strings.TrimSpace(prep.Manifest.Artifact.ID),
		ArtifactVersion: strings.TrimSpace(prep.Manifest.Artifact.Version),
		Technology:      strings.TrimSpace(prep.Manifest.Artifact.Technology),
		EnvironmentBase: strings.TrimSpace(prep.Manifest.Environment.Base),
		DependencyFiles: append([]string(nil), prep.Manifest.Environment.DependencyFiles...),
		Lockfiles:       append([]string(nil), prep.Manifest.Environment.Lockfiles...),
		SetupCommand:    strings.TrimSpace(prep.Manifest.Commands.Setup),
		BuildCommand:    strings.TrimSpace(prep.Manifest.Commands.Build),
		TestCommand:     strings.TrimSpace(prep.Manifest.Commands.Test),
		SnapshotRef:     strings.TrimSpace(prep.SnapshotRef),
		Provenance:      Provenance{BuiltBy: req.BuiltBy, Owner: req.Owner, BuilderRunID: prep.BuilderRunID, VerifierRunID: sealed.verified.VerifierRunID},
		Owner:           req.Owner,
		CreatedAt:       now,
		ValidationState: ValidationPassed,
		LastBootCheckAt: now,
		Compatibility:   CompatibilityRules{Technology: strings.TrimSpace(prep.Manifest.Artifact.Technology), Objective: strings.TrimSpace(prep.Manifest.Compatibility.RequestedObjective), ReuseTags: append([]string(nil), prep.Manifest.Compatibility.ReuseTags...)},
		Evidence:        EvidenceRef{BuilderRunID: prep.BuilderRunID, VerifierRunID: sealed.verified.VerifierRunID, Verdict: sealed.verified.Verdict, CheckCount: len(sealed.verified.Evidence.Checks)},
	}
	if record.SnapshotRef == "" {
		return PromotionReceipt{}, &PromotionRefusal{
			Reason:     "no prepared snapshot reference to boot for checks",
			Repairable: true,
			Details:    map[string]any{"artifact": record.ArtifactID},
		}
	}
	policy := s.practicePolicy()
	boot, err := s.openCheckedBoot(ctx, record.SnapshotRef, policy, record.BuildCommand, record.TestCommand)
	if err != nil {
		return PromotionReceipt{}, err
	}
	if err := s.closeRegistryBoot(ctx, boot); err != nil {
		return PromotionReceipt{}, err
	}
	if s.Store == nil {
		return PromotionReceipt{}, errors.New("environment registry store is not configured")
	}
	if err := checkRegistryCtx(ctx, "promote save"); err != nil {
		return PromotionReceipt{}, err
	}
	if err := s.Store.SaveArtifact(ctx, record); err != nil {
		return PromotionReceipt{}, fmt.Errorf("environment registry save: %w", err)
	}
	return PromotionReceipt{
		Allowed:           true,
		Reason:            "Sealed verified preparation under registry-pending promotion authority: manifest, snapshot, and run derived exclusively from the verified handoff, with exact run and evidence-command binding, plus fresh boot and build/test checks under the practice network policy. An explicit backend denial can never yield this receipt; see the #184 shared-interface flag.",
		Record:            record,
		Policy:            policy,
		CheckedAt:         now,
		VerifierPromotion: sealed.verified.Promotion,
	}, nil
}

// checkVerified refuses rejected or unverified results. It does not consult
// the promotion field: authority over that field belongs to
// checkPromotionAuthority, and every verified result must still pass the
// remaining registry gates (see doc.go).
func checkVerified(verified environmentverify.VerifierResult) error {
	if strings.TrimSpace(verified.Verdict) != environmentverify.VerdictVerified || verified.FailedStage != nil {
		stage := ""
		if verified.FailedStage != nil {
			stage = *verified.FailedStage
		}
		return &PromotionRefusal{
			Reason:     "verifier result is not verified",
			Repairable: true,
			Details:    map[string]any{"verdict": verified.Verdict, "failedStage": stage, "builderRunId": verified.BuilderRunID},
		}
	}
	return nil
}

// checkRegistryCtx fails registry work on a dead backend context.
// Cancellation and deadlines are faults, never silent progress.
func checkRegistryCtx(ctx context.Context, op string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("environment %s: %w", op, err)
	}
	return nil
}

// pendingRegistryPromotionReason quotes #188's standing promotion stance
// on a verified verdict: promotion prerequisites belong to later registry
// integration, which is pending. It is the only disallowed-promotion
// stance the registry proceeds past. Quoting reason text is a stopgap: a
// machine-readable pending-vs-denied signal is an open #184 item, and any
// rewording on the verifier side fails closed here (refused as a denial)
// until the quote is reconciled under the shared contract.
const pendingRegistryPromotionReason = "No promotion: registry promotion prerequisites " +
	"(promotion metadata, owner/provenance, compatibility rules) are evaluated by later " +
	"registry integration, which is pending."

// checkPromotionAuthority enforces promotion authority against the shared
// contract (docs/environment-contract.md). #188's registry-pending state
// — verified verdict, promotion disallowed with the standing pending
// reason — means "not yet decided, registry prerequisites still to
// evaluate", and the registry proceeds to its own gates. Any other
// disallowed stance is an explicit backend denial, which registry gates
// cannot override: the refusal is terminal for this pairing
// (Repairable=false; the builder cannot repair a backend decision). A
// future allowed=true is never trusted as authorization either —
// registry gates still decide independently.
func checkPromotionAuthority(verified environmentverify.VerifierResult) error {
	if verified.Promotion.Allowed {
		return nil
	}
	if strings.TrimSpace(verified.Promotion.Reason) == pendingRegistryPromotionReason {
		return nil
	}
	return &PromotionRefusal{
		Reason:     "backend explicitly denies promotion; registry gates cannot override a denial",
		Repairable: false,
		Details:    map[string]any{"verdict": verified.Verdict, "builderRunId": verified.BuilderRunID, "promotionReason": verified.Promotion.Reason},
	}
}

// checkPreparationBinding associates verifier evidence with the sealed
// preparation. Two checks, both required:
//
//  1. Run identity: the run IDs must match within the bound context, and
//     neither side may be empty (empty-equals-empty is a missing
//     association, not a match).
//  2. Evidence commands: every verifier check that records a command must
//     name the exact command the sealed manifest declares for that
//     stage. #188 records the executed command only on setup/build/test
//     checks, so evidence for preparation A (different commands) cannot
//     promote preparation B merely by reusing A's builder run ID.
//
// Id/version equality alone is never content integrity, but mismatched
// identity or commands are always a refusal. Command comparison constrains
// the executed stages only; it does not establish exact byte-level
// binding of the manifest, which remains an open #184 item with no
// prescribed mechanism.
func checkPreparationBinding(prep Preparation, verified environmentverify.VerifierResult) error {
	want := strings.TrimSpace(prep.BuilderRunID)
	observed := strings.TrimSpace(verified.BuilderRunID)
	if want == "" || observed == "" || observed != want {
		return &PromotionRefusal{
			Reason:     "verifier evidence does not match the preparation being promoted",
			Repairable: true,
			Details:    map[string]any{"field": "builderRunId", "want": want, "observed": observed, "builderRunId": want},
		}
	}
	expected := map[string]string{}
	if prep.Manifest.Commands != nil {
		expected = map[string]string{
			environmentverify.StageSetup: strings.TrimSpace(prep.Manifest.Commands.Setup),
			environmentverify.StageBuild: strings.TrimSpace(prep.Manifest.Commands.Build),
			environmentverify.StageTest:  strings.TrimSpace(prep.Manifest.Commands.Test),
		}
	}
	for _, check := range verified.Evidence.Checks {
		if check.Command == nil {
			continue
		}
		wantCommand, ok := expected[check.Stage]
		if !ok || wantCommand == "" || strings.TrimSpace(*check.Command) != wantCommand {
			return &PromotionRefusal{
				Reason:     "verifier evidence commands do not match the preparation being promoted",
				Repairable: true,
				Details:    map[string]any{"field": "evidence.command", "stage": check.Stage, "want": wantCommand, "observed": strings.TrimSpace(*check.Command), "builderRunId": want},
			}
		}
	}
	return nil
}

// openCheckedBoot boots the snapshot fresh under the practice network
// policy and runs the representative build and test commands, returning the
// still-open boot the checks passed on. Dependency installation (setup)
// never runs here: the prepared artifact already carries installed
// dependencies, and reuse must not repeat installation. Check failures
// close the boot and refuse promotion or reuse. Callers close the returned
// boot explicitly and handle cleanup errors via closeRegistryBoot.
func (s *Service) openCheckedBoot(ctx context.Context, snapshotRef string, policy NetworkPolicy, buildCommand, testCommand string) (Boot, error) {
	if s.Snapshots == nil {
		return nil, errors.New("environment snapshot provider is not configured")
	}
	if err := checkRegistryCtx(ctx, "boot checks"); err != nil {
		return nil, err
	}
	boot, err := s.Snapshots.BootFresh(ctx, snapshotRef, policy)
	if err != nil {
		return nil, &PromotionRefusal{
			Reason:     fmt.Sprintf("fresh boot failed under practice network policy: %v", err),
			Repairable: false,
			Details:    map[string]any{"snapshot": snapshotRef, "policy": string(policy), "error": err.Error()},
		}
	}
	checkErr := func() error {
		for _, step := range []struct {
			name    string
			command string
		}{
			{"build", strings.TrimSpace(buildCommand)},
			{"test", strings.TrimSpace(testCommand)},
		} {
			if step.command == "" {
				return &PromotionRefusal{
					Reason:     fmt.Sprintf("representative %s check has no command to run", step.name),
					Repairable: true,
					Details:    map[string]any{"snapshot": snapshotRef, "check": step.name},
				}
			}
			if err := checkRegistryCtx(ctx, step.name+" check"); err != nil {
				return err
			}
			outcome, err := boot.Run(ctx, step.command)
			// A result returned after the backend context died is
			// untrustworthy: cancellation supersedes whatever the boot
			// reports, so success on a dead context can never advance
			// promotion or reuse (same rule as the #188 verifier).
			if err == nil {
				if ctxErr := ctx.Err(); ctxErr != nil {
					return ctxErr
				}
			}
			if err != nil {
				return &PromotionRefusal{
					Reason:     fmt.Sprintf("representative %s check failed: %v", step.name, err),
					Repairable: false,
					Details:    map[string]any{"snapshot": snapshotRef, "check": step.name, "policy": string(policy), "error": err.Error()},
				}
			}
			if outcome.ExitCode != 0 {
				return &PromotionRefusal{
					Reason:     fmt.Sprintf("representative %s check exited %d", step.name, outcome.ExitCode),
					Repairable: false,
					Details:    map[string]any{"snapshot": snapshotRef, "check": step.name, "policy": string(policy), "exitCode": outcome.ExitCode, "output": truncateRegistryOutput(outcome.Output)},
				}
			}
		}
		return nil
	}()
	if checkErr != nil {
		if closeErr := s.closeRegistryBoot(ctx, boot); closeErr != nil {
			return nil, errors.Join(checkErr, closeErr)
		}
		return nil, checkErr
	}
	return boot, nil
}

// closeRegistryBoot releases a boot explicitly so cleanup failures are
// reported, never silently dropped. The registry owns boot lifetime: every
// boot opened by openCheckedBoot is closed by its caller on all paths —
// success, check failure, separation failure, and cancellation. Cleanup
// runs on a context decoupled from caller cancellation and bounded by the
// backend-owned cleanup timeout: a dead or deadline-less caller context
// must neither skip provider teardown nor leave it unbounded. Cleanup
// failures fail the operation loudly; no workspace is ever issued from a
// boot whose teardown failed. Production Close must not invalidate learner
// workspaces already separated from the boot (see separateWorkspace).
func (s *Service) closeRegistryBoot(ctx context.Context, boot Boot) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.cleanupTimeout())
	defer cancel()
	if err := boot.Close(cleanupCtx); err != nil {
		return fmt.Errorf("environment boot cleanup: %w", err)
	}
	return nil
}

// truncateRegistryOutput bounds quoted boot output in refusal details.
func truncateRegistryOutput(output string) string {
	const limit = 2000
	if len(output) <= limit {
		return output
	}
	return output[:limit] + "…[truncated]"
}
