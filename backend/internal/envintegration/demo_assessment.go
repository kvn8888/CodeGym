package envintegration

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kvn8888/codegym/backend/internal/environmentregistry"
	"github.com/kvn8888/codegym/backend/internal/environmentverify"
)

// StarterDirectory holds the learner starter the builder writes beside the
// complete reference project. Each file mirrors a learner-editable path.
const StarterDirectory = ".codegym/starter"

// Demo-mode assessment identity. The whole test command is the single
// assessed "test": it passes when the command exits zero.
const (
	DemoSuiteTestID        = "assessment-suite"
	DemoSuiteFailureCode   = "TEST_COMMAND_FAILED"
	DemoUnreviewedReviewer = "unreviewed: agent reference solution (demo mode)"
	DemoReviewedReference  = "demo-mode"
)

// BuildAssessmentSource resolves assessment inputs from the built artifact
// itself. AssessmentResolvingVerifier prefers it over AssessmentSource when
// the configured source implements it.
type BuildAssessmentSource interface {
	InputsForBuild(ctx context.Context, build environmentregistry.BuildOutput) (*environmentverify.AssessmentSpec, environmentverify.AssessmentRunner, error)
}

// StarterAssessmentSource is the demo-mode assessment source for cold
// builds, where no human-reviewed solution can exist ahead of time. The
// artifact is the agent's complete reference project; the correct solution
// is that reference and the wrong solution is the agent's starter, which
// must fail the test command. Verdicts carry DemoUnreviewedReviewer as their
// review evidence so they are never mistaken for human-reviewed ones.
type StarterAssessmentSource struct {
	Roots func(reference string) (string, error)
	// Assessor runs each solution's test command. Nil keeps the host
	// assessor; the sandbox corridor supplies a sandbox assessor so
	// assessments execute off-host like the rest of verification.
	Assessor environmentverify.AssessmentRunner
}

// Inputs satisfies AssessmentSource; demo mode needs the artifact contents,
// so only InputsForBuild can serve it.
func (s *StarterAssessmentSource) Inputs(context.Context, string, string, string, string) (*environmentverify.AssessmentSpec, environmentverify.AssessmentRunner, error) {
	return nil, nil, fmt.Errorf("envintegration: demo-mode assessment needs the built artifact")
}

// InputsForBuild derives the assessment from the artifact's reference files
// and the starter under StarterDirectory.
func (s *StarterAssessmentSource) InputsForBuild(_ context.Context, build environmentregistry.BuildOutput) (*environmentverify.AssessmentSpec, environmentverify.AssessmentRunner, error) {
	if s == nil || s.Roots == nil {
		return nil, nil, fmt.Errorf("envintegration: snapshot resolver is required")
	}
	if build.Manifest.Artifact == nil || build.Manifest.Workspace == nil {
		return nil, nil, fmt.Errorf("envintegration: builder output names no artifact")
	}
	root, err := s.Roots(build.SnapshotRef)
	if err != nil {
		return nil, nil, err
	}
	correct := map[string]string{}
	wrong := map[string]string{}
	for _, path := range build.Manifest.Workspace.LearnerEditable {
		rel := filepath.FromSlash(strings.TrimSpace(path))
		starter, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(StarterDirectory), rel))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, nil, fmt.Errorf("envintegration: read starter %q: %w", path, err)
		}
		reference, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return nil, nil, fmt.Errorf("envintegration: read reference %q: %w", path, err)
		}
		correct[path] = string(reference)
		wrong[path] = string(starter)
	}
	if len(wrong) == 0 {
		return nil, nil, fmt.Errorf("envintegration: preparation wrote no learner starter under %s", StarterDirectory)
	}
	objective := ""
	if build.Manifest.Compatibility != nil {
		objective = strings.TrimSpace(build.Manifest.Compatibility.RequestedObjective)
	}
	if objective == "" {
		objective = "practice exercise"
	}
	return &environmentverify.AssessmentSpec{
		ArtifactID:      build.Manifest.Artifact.ID,
		ArtifactVersion: build.Manifest.Artifact.Version,
		Objective:       objective,
		CorrectSolution: correct,
		WrongSolution:   wrong,
		ExpectedFailure: environmentverify.ExpectedFailure{
			TestID:   DemoSuiteTestID,
			Code:     DemoSuiteFailureCode,
			Expected: "pass",
			Actual:   "fail",
			Reason:   "the starter leaves the exercise unimplemented",
		},
		ReviewedBy:        DemoUnreviewedReviewer,
		ReviewedReference: DemoReviewedReference,
	}, s.assessor(), nil
}

// assessor returns the configured assessor, defaulting to the host one so the
// host corridor is unchanged.
func (s *StarterAssessmentSource) assessor() environmentverify.AssessmentRunner {
	if s != nil && s.Assessor != nil {
		return s.Assessor
	}
	return &CommandAssessor{}
}

// CommandAssessor runs the manifest test command and reports it as one
// assessed test. It is technology-neutral: no test-report format is parsed.
type CommandAssessor struct {
	MaxOutputBytes int
}

// Assess runs the test command in the prepared solution workspace.
func (a *CommandAssessor) Assess(ctx context.Context, work environmentverify.AssessmentWork) (environmentverify.AssessmentReport, error) {
	if strings.TrimSpace(work.TestCommand) == "" {
		return environmentverify.AssessmentReport{}, fmt.Errorf("envintegration: manifest declares no test command")
	}
	capBytes := a.MaxOutputBytes
	if capBytes <= 0 {
		capBytes = DefaultLocalOutputCap
	}
	exitCode, output, timedOut, err := runLocalStageCommand(ctx, work.TestCommand, work.WorkDir, capBytes)
	if err != nil {
		return environmentverify.AssessmentReport{}, err
	}
	if timedOut {
		return environmentverify.AssessmentReport{}, context.DeadlineExceeded
	}
	return ReportFromCommandResult(exitCode, output), nil
}

// ReportFromCommandResult turns one test-command outcome into the single
// assessed test this corridor reports, whatever ran it. It is
// technology-neutral: no test-report format is parsed, and exit code alone
// decides the outcome. Both the host assessor and the sandbox assessor use
// it so a solution verifies identically on either.
func ReportFromCommandResult(exitCode int, output string) environmentverify.AssessmentReport {
	outcome := environmentverify.TestOutcome{TestID: DemoSuiteTestID, Passed: exitCode == 0}
	if !outcome.Passed {
		outcome.FailureClass = environmentverify.ClassAssertion
		outcome.Message = tailOutput(output, 2000)
		outcome.Assertion = &environmentverify.ObservedAssertion{Code: DemoSuiteFailureCode, Expected: "pass", Actual: "fail"}
	}
	return environmentverify.AssessmentReport{Tests: []environmentverify.TestOutcome{outcome}}
}

func tailOutput(output string, limit int) string {
	if len(output) <= limit {
		return output
	}
	return output[len(output)-limit:]
}

// StarterContents serves the learner starter in place of the reference for
// every file that has one, so practice sessions open on the exercise rather
// than its answer. Other files pass through unchanged.
type StarterContents struct {
	Inner *SnapshotContents
}

// ReadArtifactFile returns the starter for path when present.
func (s *StarterContents) ReadArtifactFile(ctx context.Context, snapshotRef, filePath string) (string, error) {
	if s == nil || s.Inner == nil {
		return "", fmt.Errorf("envintegration: snapshot contents are required")
	}
	if starter, err := s.Inner.ReadArtifactFile(ctx, snapshotRef, StarterDirectory+"/"+strings.TrimSpace(filePath)); err == nil {
		return starter, nil
	}
	return s.Inner.ReadArtifactFile(ctx, snapshotRef, filePath)
}
