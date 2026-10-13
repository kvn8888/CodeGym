package environmentregistry

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kvn8888/codegym/backend/internal/environmentverify"
)

// ResolutionStatus explicitly reports how a resolve request concluded.
// Expected policy outcomes are statuses, never silent substitutions;
// returned errors are reserved for platform faults.
type ResolutionStatus string

// Resolution statuses.
const (
	// ResolutionHit reuses a compatible promoted artifact.
	ResolutionHit ResolutionStatus = "hit"
	// ResolutionBuilt records a miss that was freshly built, verified,
	// promoted, and then served.
	ResolutionBuilt ResolutionStatus = "built"
	// ResolutionIncompatibleVersion reports an exact-version request with
	// no matching artifact version. The registry never substitutes.
	ResolutionIncompatibleVersion ResolutionStatus = "incompatible-version"
	// ResolutionUnsupportedCapability reports a miss for a technology no
	// builder supports.
	ResolutionUnsupportedCapability ResolutionStatus = "unsupported-capability"
	// ResolutionQuotaExhausted reports a quota denial with zero builder calls.
	ResolutionQuotaExhausted ResolutionStatus = "quota-exhausted"
	// ResolutionRateLimited reports a rate-limit denial with zero builder calls.
	ResolutionRateLimited ResolutionStatus = "rate-limited"
	// ResolutionBootCheckFailed reports fresh-boot or representative
	// build/test failure: the artifact is not offered as ready and its
	// record is marked failed.
	ResolutionBootCheckFailed ResolutionStatus = "boot-check-failed"
	// ResolutionBuildFailed reports builder-track failure on a miss.
	ResolutionBuildFailed ResolutionStatus = "build-failed"
	// ResolutionVerificationFailed reports a miss whose fresh output did
	// not verify and therefore never became reusable.
	ResolutionVerificationFailed ResolutionStatus = "verification-failed"
	// ResolutionOutputMismatch reports fresh builder output that does not
	// match what was requested (technology, pinned version, objective, or
	// tags). Mismatched output is rejected before promotion and before any
	// workspace is issued.
	ResolutionOutputMismatch ResolutionStatus = "output-mismatch"
)

// ResolveRequest asks for a ready practice environment.
type ResolveRequest struct {
	Technology string
	// Version is an exact version pin. Empty means any compatible version
	// is acceptable, and the served version is always reported explicitly.
	Version   string
	ReuseTags []string
	Owner     string
	// Objective describes the learning goal for fresh builds. Compatibility
	// recorded on promoted artifacts comes from the verified manifest.
	Objective string
}

// Resolution is the explicit outcome of a resolve request. Workspace is
// set only for hit and built outcomes.
type Resolution struct {
	Status    ResolutionStatus
	Reason    string
	Record    ArtifactRecord
	Workspace Workspace
}

// Resolve serves a ready practice environment: compatible hits boot clean
// and separate a fresh learner workspace with dependencies already
// available (no setup rerun); ordinary misses invoke the builder only when
// capability and quota/rate checks permit, and fresh output must pass
// verification and promotion gates before becoming reusable.
func (s *Service) Resolve(ctx context.Context, req ResolveRequest) (Resolution, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := checkRegistryCtx(ctx, "resolve"); err != nil {
		return Resolution{}, err
	}
	technology := strings.TrimSpace(req.Technology)
	if technology == "" {
		return Resolution{}, errors.New("environment resolve requires a technology")
	}
	if s.Store == nil {
		return Resolution{}, errors.New("environment registry store is not configured")
	}
	candidates, err := s.Store.ListByTechnology(ctx, technology)
	if err != nil {
		return Resolution{}, fmt.Errorf("environment registry lookup: %w", err)
	}
	compatible := matchCompatible(candidates, strings.TrimSpace(req.Version), strings.TrimSpace(req.Objective), req.ReuseTags)
	if len(compatible) > 0 {
		return s.serveHit(ctx, compatible[0])
	}
	// A pinned version genuinely differing from every promoted artifact is
	// a true version mismatch: reported explicitly, never built, never
	// substituted. But a version that exists with no compatible exercise
	// (objective, tag, or validation mismatch) is an ordinary miss:
	// capability/quota-gated builder fallback still applies, and the pin
	// is re-enforced on fresh output before promotion. An empty store is
	// likewise a miss — fallback may produce the pinned version.
	if pin := strings.TrimSpace(req.Version); pin != "" && len(candidates) > 0 && !hasVersion(candidates, pin) {
		return Resolution{
			Status: ResolutionIncompatibleVersion,
			Reason: fmt.Sprintf("No promoted %s artifact at version %q; versions are never substituted.",
				technology, pin),
		}, nil
	}
	return s.serveMiss(ctx, req, technology)
}

// hasVersion reports whether any candidate record — compatible or not,
// passed or failed — carries the requested version.
func hasVersion(candidates []ArtifactRecord, version string) bool {
	for _, record := range candidates {
		if record.ArtifactVersion == version {
			return true
		}
	}
	return false
}

// matchCompatible keeps passed records whose version (when pinned) equals,
// whose recorded objective equals a non-empty requested objective, and
// whose reuse tags cover every requested tag. Input order is newest first.
// An empty requested objective matches any recorded objective; matching
// semantics stay a Track B proposal for #184 review.
func matchCompatible(candidates []ArtifactRecord, version, objective string, tags []string) []ArtifactRecord {
	var out []ArtifactRecord
	for _, record := range candidates {
		if record.ValidationState != ValidationPassed {
			continue
		}
		if version != "" && record.ArtifactVersion != version {
			continue
		}
		if objective != "" && record.Compatibility.Objective != objective {
			continue
		}
		if !coversTags(record.Compatibility.ReuseTags, tags) {
			continue
		}
		out = append(out, record)
	}
	return out
}

func coversTags(have, want []string) bool {
	set := make(map[string]struct{}, len(have))
	for _, tag := range have {
		set[strings.TrimSpace(tag)] = struct{}{}
	}
	for _, tag := range want {
		if _, ok := set[strings.TrimSpace(tag)]; !ok {
			return false
		}
	}
	return true
}

// serveHit boots a compatible record clean under the practice policy,
// re-runs representative build/test checks, and separates a fresh learner
// workspace from the boot that passed the checks. Setup never runs:
// dependencies are already available. Check failure marks the record failed
// and refuses to offer it. The boot is closed before returning; the issued
// workspace outlives it, so production Close must never destroy separated
// workspaces, and a workspace is only ever returned from a successfully
// closed boot — never a destroyed one.
func (s *Service) serveHit(ctx context.Context, record ArtifactRecord) (Resolution, error) {
	policy := s.practicePolicy()
	boot, err := s.openCheckedBoot(ctx, record.SnapshotRef, policy, record.BuildCommand, record.TestCommand)
	if err != nil {
		markErr := s.Store.UpdateValidation(ctx, record.ArtifactID, record.ArtifactVersion, ValidationBootCheckFailed, time.Now().UTC())
		if markErr != nil {
			return Resolution{}, fmt.Errorf("environment registry validation update: %w", markErr)
		}
		refusal, _ := err.(*PromotionRefusal)
		reason := "fresh boot checks failed"
		if refusal != nil {
			reason = refusal.Reason
		}
		return Resolution{Status: ResolutionBootCheckFailed, Reason: reason, Record: record}, nil
	}
	workspace, err := s.separateWorkspace(ctx, boot, record)
	if err != nil {
		if closeErr := s.closeRegistryBoot(ctx, boot); closeErr != nil {
			return Resolution{}, errors.Join(err, closeErr)
		}
		return Resolution{}, err
	}
	if err := s.closeRegistryBoot(ctx, boot); err != nil {
		return Resolution{}, err
	}
	// Cleanup runs cancellation-independent and may report success on a
	// dead caller context: recheck before issuing. A workspace separated
	// and cleaned up after cancellation is never returned; provider-side
	// reclamation of the unissued workspace stays a documented
	// production lifetime item.
	if err := checkRegistryCtx(ctx, "workspace issuance"); err != nil {
		return Resolution{}, err
	}
	if err := s.Store.UpdateValidation(ctx, record.ArtifactID, record.ArtifactVersion, ValidationPassed, time.Now().UTC()); err != nil {
		return Resolution{}, fmt.Errorf("environment registry validation update: %w", err)
	}
	record.LastBootCheckAt = time.Now().UTC()
	return Resolution{
		Status:    ResolutionHit,
		Reason:    fmt.Sprintf("Reusing promoted artifact %s version %s with dependencies already available; setup not rerun.", record.ArtifactID, record.ArtifactVersion),
		Record:    record,
		Workspace: workspace,
	}, nil
}

// separateWorkspace carves one fresh learner workspace from the boot that
// passed checks. Learner state lives only in the returned workspace, which
// starts empty; the caller closes the boot afterwards.
func (s *Service) separateWorkspace(ctx context.Context, boot Boot, record ArtifactRecord) (Workspace, error) {
	if err := checkRegistryCtx(ctx, "workspace separation"); err != nil {
		return Workspace{}, err
	}
	workspace, err := boot.NewWorkspace(ctx)
	if err != nil {
		return Workspace{}, fmt.Errorf("environment workspace separation: %w", err)
	}
	// Cancellation supersedes even a successful separation: a workspace
	// produced on a dead context is never issued. The caller still closes
	// the boot (on a decoupled cleanup context), so provider teardown is
	// not skipped.
	if err := checkRegistryCtx(ctx, "workspace separation"); err != nil {
		return Workspace{}, err
	}
	workspace.ArtifactID = record.ArtifactID
	workspace.ArtifactVersion = record.ArtifactVersion
	return workspace, nil
}

// serveMiss handles a registry miss: capability and quota/rate gates run
// before the builder is ever invoked, denials report explicitly with zero
// builder calls, fresh output is validated against the request and must
// verify and promote before reuse. A missing quota policy fails closed:
// fresh builds are refused without invoking the builder.
func (s *Service) serveMiss(ctx context.Context, req ResolveRequest, technology string) (Resolution, error) {
	if s.Capabilities == nil || !s.Capabilities.SupportsTechnology(technology) {
		return Resolution{
			Status: ResolutionUnsupportedCapability,
			Reason: fmt.Sprintf("Technology %q is not supported for fresh builds.", technology),
		}, nil
	}
	if strings.TrimSpace(req.Owner) == "" {
		return Resolution{}, errors.New("environment resolve requires an owner for fresh builds")
	}
	if s.Quota == nil {
		return Resolution{
			Status: ResolutionQuotaExhausted,
			Reason: fmt.Sprintf("No quota policy is configured for owner %q; fresh builds are refused without invoking the builder.", strings.TrimSpace(req.Owner)),
		}, nil
	}
	if err := s.Quota.CheckBuild(ctx, req.Owner); err != nil {
		var denial *QuotaDenial
		if errors.As(err, &denial) {
			status := ResolutionQuotaExhausted
			if denial.Reason == RateLimited {
				status = ResolutionRateLimited
			}
			return Resolution{Status: status, Reason: denial.Error()}, nil
		}
		return Resolution{}, fmt.Errorf("environment quota check: %w", err)
	}
	if s.Builder == nil {
		return Resolution{}, errors.New("environment builder is not configured")
	}
	// Fail fast before any side effect: a miss that cannot be verified
	// must never invoke the builder.
	if s.Verifier == nil {
		return Resolution{}, errors.New("environment verifier is not configured")
	}
	if err := checkRegistryCtx(ctx, "fresh build"); err != nil {
		return Resolution{}, err
	}
	output, err := s.Builder.Build(ctx, BuildRequest{
		Technology: technology,
		Version:    strings.TrimSpace(req.Version),
		Objective:  strings.TrimSpace(req.Objective),
		ReuseTags:  req.ReuseTags,
		Owner:      req.Owner,
	})
	if err != nil {
		return Resolution{Status: ResolutionBuildFailed, Reason: fmt.Sprintf("Fresh build failed: %v.", err)}, nil
	}
	if status, reason := checkBuildOutput(req, technology, output); status != "" {
		return Resolution{Status: status, Reason: reason}, nil
	}
	if err := checkRegistryCtx(ctx, "fresh build verification"); err != nil {
		return Resolution{}, err
	}
	// The builder track's real handoff is sealed by the verification path:
	// the preparation is derived exclusively from the output being
	// verified and captured as an independent copy, so the seal binds one
	// manifest, snapshot, run, and verdict together. The registry never
	// fabricates runtime metadata or manifest paths.
	sealed, err := s.VerifyPreparation(ctx, output)
	if err != nil {
		var refusal *PromotionRefusal
		if errors.As(err, &refusal) {
			return Resolution{Status: ResolutionVerificationFailed, Reason: fmt.Sprintf("Fresh build output refused verification: %s.", refusal.Reason)}, nil
		}
		return Resolution{}, err
	}
	receipt, err := s.Promote(ctx, PromotionRequest{
		Verified: sealed,
		Owner:    req.Owner,
		BuiltBy:  builderDisplayName(output.Builder),
	})
	if err != nil {
		var refusal *PromotionRefusal
		if errors.As(err, &refusal) {
			return Resolution{Status: ResolutionVerificationFailed, Reason: fmt.Sprintf("Fresh build output refused promotion: %s.", refusal.Reason)}, nil
		}
		return Resolution{}, err
	}
	boot, err := s.openCheckedBoot(ctx, receipt.Record.SnapshotRef, s.practicePolicy(), receipt.Record.BuildCommand, receipt.Record.TestCommand)
	if err != nil {
		return Resolution{Status: ResolutionBootCheckFailed, Reason: fmt.Sprintf("Freshly promoted artifact failed reuse boot checks: %v.", bootCheckReason(err))}, nil
	}
	workspace, err := s.separateWorkspace(ctx, boot, receipt.Record)
	if err != nil {
		if closeErr := s.closeRegistryBoot(ctx, boot); closeErr != nil {
			return Resolution{}, errors.Join(err, closeErr)
		}
		return Resolution{}, err
	}
	if err := s.closeRegistryBoot(ctx, boot); err != nil {
		return Resolution{}, err
	}
	// Same issuance recheck as the hit path: cancellation during
	// cancellation-independent cleanup must not yield a workspace.
	if err := checkRegistryCtx(ctx, "workspace issuance"); err != nil {
		return Resolution{}, err
	}
	return Resolution{
		Status:    ResolutionBuilt,
		Reason:    fmt.Sprintf("Built, verified, and promoted %s version %s, then separated a fresh workspace.", receipt.Record.ArtifactID, receipt.Record.ArtifactVersion),
		Record:    receipt.Record,
		Workspace: workspace,
	}, nil
}

// builderDisplayName reports the builder track's own runtime name for
// provenance. It is a recorded claim, never an authorization, and stays
// empty when the builder reports no runtime rather than gaining a
// registry placeholder.
func builderDisplayName(builder environmentverify.BuilderResult) string {
	if builder.Runtime == nil {
		return ""
	}
	return strings.TrimSpace(builder.Runtime.Name)
}

// checkBuildOutput validates fresh builder output against the request
// before verification: exact technology, pinned version, requested
// objective, and requested tags, plus a non-empty builder run identity
// so later promotion binding cannot match on emptiness. Mismatches reject
// the output before promotion and before any workspace is issued.
// Semantics stay a Track B proposal for #184 review.
func checkBuildOutput(req ResolveRequest, technology string, output BuildOutput) (ResolutionStatus, string) {
	if strings.TrimSpace(output.Builder.BuilderRunID) == "" {
		return ResolutionOutputMismatch, "Fresh build is missing its builder run ID; refusing before promotion."
	}
	manifest := output.Manifest
	if manifest.Artifact == nil || manifest.Environment == nil || manifest.Commands == nil || manifest.Compatibility == nil {
		return ResolutionOutputMismatch, "Fresh build manifest is missing required sections; refusing before promotion."
	}
	manifestTech := strings.TrimSpace(manifest.Artifact.Technology)
	if manifestTech != technology {
		return ResolutionOutputMismatch, fmt.Sprintf("Fresh build produced technology %q, want %q; refusing before promotion.", manifestTech, technology)
	}
	if pin := strings.TrimSpace(req.Version); pin != "" {
		if got := strings.TrimSpace(output.Manifest.Artifact.Version); got != pin {
			return ResolutionOutputMismatch, fmt.Sprintf("Fresh build produced version %q, want pinned version %q; refusing before promotion.", got, pin)
		}
	}
	if want := strings.TrimSpace(req.Objective); want != "" {
		if got := strings.TrimSpace(output.Manifest.Compatibility.RequestedObjective); got != want {
			return ResolutionOutputMismatch, fmt.Sprintf("Fresh build targets objective %q, want %q; refusing before promotion.", got, want)
		}
	}
	if !coversTags(output.Manifest.Compatibility.ReuseTags, req.ReuseTags) {
		return ResolutionOutputMismatch, "Fresh build does not cover the requested reuse tags; refusing before promotion."
	}
	return "", ""
}

// bootCheckReason summarizes a boot-check failure for reuse reporting.
func bootCheckReason(err error) string {
	var refusal *PromotionRefusal
	if errors.As(err, &refusal) {
		return refusal.Reason
	}
	return err.Error()
}
