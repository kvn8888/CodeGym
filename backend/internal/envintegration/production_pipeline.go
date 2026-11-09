package envintegration

import (
	"context"
	"fmt"
	"strings"

	"github.com/kvn8888/codegym/backend/internal/envbuild"
	"github.com/kvn8888/codegym/backend/internal/environmentregistry"
	"github.com/kvn8888/codegym/backend/internal/environmentverify"
	"github.com/kvn8888/codegym/backend/internal/practice"
)

// ReviewedSolutions is one technology's human-reviewed assessment material:
// the correct and wrong solutions plus the exact expected failure, with the
// reviewer named. The artifact identity is minted per build because IDs are
// only known after preparation; the reviewed content is what carries human
// approval, never agent output.
type ReviewedSolutions struct {
	CorrectSolution   map[string]string
	WrongSolution     map[string]string
	ExpectedFailure   environmentverify.ExpectedFailure
	ReviewedBy        string
	ReviewedReference string
	Assessor          environmentverify.AssessmentRunner
}

// AssessmentSource provides reviewed assessment inputs per technology. A
// missing entry fails closed with an explicit message: unverified material
// never reaches the verifier by default, and no agent output substitutes
// for human review.
type AssessmentSource interface {
	Inputs(ctx context.Context, technology, artifactID, version, objective string) (*environmentverify.AssessmentSpec, environmentverify.AssessmentRunner, error)
}

// MapAssessmentSource serves reviewed inputs from a static table keyed by
// normalized technology. Demo and test configurations populate it with
// reviewed exercises; production loads it from durable configuration.
type MapAssessmentSource struct {
	Entries map[string]ReviewedSolutions
}

// Inputs mints the assessment spec for one built artifact from the
// technology's reviewed solutions.
func (s *MapAssessmentSource) Inputs(_ context.Context, technology, artifactID, version, objective string) (*environmentverify.AssessmentSpec, environmentverify.AssessmentRunner, error) {
	if s == nil {
		return nil, nil, fmt.Errorf("envintegration: no reviewed assessment inputs are configured")
	}
	entry, ok := s.Entries[strings.ToLower(strings.TrimSpace(technology))]
	if !ok || entry.Assessor == nil || len(entry.CorrectSolution) == 0 {
		return nil, nil, fmt.Errorf("envintegration: assessment inputs for %q are not reviewed yet", technology)
	}
	return &environmentverify.AssessmentSpec{
		ArtifactID:        artifactID,
		ArtifactVersion:   version,
		Objective:         objective,
		CorrectSolution:   entry.CorrectSolution,
		WrongSolution:     entry.WrongSolution,
		ExpectedFailure:   entry.ExpectedFailure,
		ReviewedBy:        entry.ReviewedBy,
		ReviewedReference: entry.ReviewedReference,
	}, entry.Assessor, nil
}

// AssessmentResolvingVerifier wraps registry verification with per-build
// assessment lookup. The artifact ID exists only after the build, so static
// assessment configuration cannot work; this resolves reviewed inputs from
// the built manifest and fails closed when review is missing.
type AssessmentResolvingVerifier struct {
	Inner       *RegistryVerifier
	Assessments AssessmentSource
}

// Verify judges one builder output with its own reviewed inputs.
func (v *AssessmentResolvingVerifier) Verify(ctx context.Context, build environmentregistry.BuildOutput) environmentverify.VerifierResult {
	inner := v.Inner
	if inner == nil {
		inner = &RegistryVerifier{}
	}
	if v == nil || v.Assessments == nil {
		return inner.withRejection(build, environmentverify.StageValidation, environmentverify.CodeReviewPending, "envintegration: no reviewed assessment inputs are configured", false)
	}
	technology := ""
	objective := ""
	if build.Manifest.Artifact != nil {
		technology = build.Manifest.Artifact.Technology
	}
	if build.Manifest.Compatibility != nil {
		objective = build.Manifest.Compatibility.RequestedObjective
	}
	if build.Manifest.Artifact == nil {
		return inner.withRejection(build, environmentverify.StageManifest, environmentverify.CodeManifestInvalid, "envintegration: builder output names no artifact", true)
	}
	var (
		spec     *environmentverify.AssessmentSpec
		assessor environmentverify.AssessmentRunner
		err      error
	)
	if fromBuild, ok := v.Assessments.(BuildAssessmentSource); ok {
		spec, assessor, err = fromBuild.InputsForBuild(ctx, build)
	} else {
		spec, assessor, err = v.Assessments.Inputs(ctx, technology, build.Manifest.Artifact.ID, build.Manifest.Artifact.Version, objective)
	}
	if err != nil {
		return inner.withRejection(build, environmentverify.StageValidation, environmentverify.CodeReviewPending, err.Error(), false)
	}
	configured := *inner
	configured.Assessment = spec
	configured.Assessor = assessor
	return configured.Verify(ctx, build)
}

// ProductionPipeline is the corridor behind the environment request
// endpoints: resolve (building on miss), then open the browser practice
// session on the served record. Every stage is reported for progress
// polling across minutes-long builds.
type ProductionPipeline struct {
	Registry *environmentregistry.Service
	Practice *practice.Service
}

// Run resolves the request to a ready record and opens its practice
// session, returning the session ID the frontend navigates to.
func (p *ProductionPipeline) Run(ctx context.Context, operation *EnvironmentOperation, report func(stage string)) (string, error) {
	if p == nil || p.Registry == nil {
		return "", fmt.Errorf("envintegration: registry service is required")
	}
	if p.Practice == nil {
		return "", fmt.Errorf("envintegration: practice service is required")
	}
	report("looking for a verified environment")
	resolution, err := p.Registry.Resolve(withStageReporter(ctx, report), environmentregistry.ResolveRequest{
		// The builder records the canonical key; resolving by the display
		// name made every fresh build an output mismatch.
		Technology:  envbuild.NormalizeTechnology(operation.Technology),
		Objective:   operation.Objective,
		Owner:       operation.Owner,
		WorkspaceID: operation.WorkspaceID,
	})
	if err != nil {
		return "", err
	}
	switch resolution.Status {
	case environmentregistry.ResolutionHit, environmentregistry.ResolutionBuilt:
	default:
		return "", fmt.Errorf("envintegration: environment not ready: %s (%s)", resolution.Status, resolution.Reason)
	}
	report("opening practice session")
	session, err := p.Practice.OpenBrowserSession(ctx, operation.Owner, operation.WorkspaceID, practice.ArtifactRef{
		ArtifactID: resolution.Record.ArtifactID,
		Technology: resolution.Record.Technology,
		Version:    resolution.Record.ArtifactVersion,
		Objective:  resolution.Record.Compatibility.Objective,
	})
	if err != nil {
		return "", fmt.Errorf("envintegration: open practice session: %w", err)
	}
	return session.SessionID, nil
}

// Pipeline adapts Run to the operation store's pipeline signature.
func (p *ProductionPipeline) Pipeline(ctx context.Context, operation *EnvironmentOperation, report func(stage string)) (string, error) {
	return p.Run(ctx, operation, report)
}
