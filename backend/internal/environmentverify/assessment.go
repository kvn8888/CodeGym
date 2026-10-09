package environmentverify

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Solution roles assessed by the verifier.
const (
	RoleCorrectSolution = "correct-solution"
	RoleWrongSolution   = "wrong-solution"
)

// Structured test-outcome failure classes.
const (
	ClassAssertion = "assertion"
	ClassError     = "error"
	ClassCompile   = "compile"
)

// ExpectedFailure names the assertion that must reject the wrong solution:
// the test, a stable assertion code labeling the requirement, and the exact
// expected and actual values the backend requires. Reason is human-readable
// context only and is never matched. All three structured fields must equal
// the observed assertion record exactly; the values substantiate the
// behavior while the code labels which requirement failed.
type ExpectedFailure struct {
	TestID   string `json:"testId"`
	Code     string `json:"code"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
	Reason   string `json:"reason"`
}

// AssessmentSpec is the backend-controlled assessment input boundary. It
// identifies the correct and wrong solutions as file replacements, the
// learning objective, the expected wrong-solution failure, and the human
// review evidence for the reviewed sample. It is bound to one artifact id
// and version; builder "reviewed" or "passed" flags are never consulted.
// Whether production assessment inputs need per-artifact human approval is
// an unresolved shared-interface decision for Track A coordination under
// #184, not a rule established here.
//
// Proposed shared-contract addition (for both-track review, not silently
// adopted): an environment.assessment-input object carrying exactly these
// fields. The manifest schema is unchanged: assessment inputs travel beside
// VerifyOptions, never inside the manifest.
type AssessmentSpec struct {
	ArtifactID        string            `json:"artifactId"`
	ArtifactVersion   string            `json:"artifactVersion"`
	Objective         string            `json:"objective"`
	CorrectSolution   map[string]string `json:"correctSolution"`
	WrongSolution     map[string]string `json:"wrongSolution"`
	ExpectedFailure   ExpectedFailure   `json:"expectedFailure"`
	ReviewedBy        string            `json:"reviewedBy"`
	ReviewedReference string            `json:"reviewedReference,omitempty"`
}

// ObservedAssertion is the structured assertion record a test harness
// emitted for one failed test: which requirement it checked and the exact
// expected and actual values observed.
type ObservedAssertion struct {
	Code     string `json:"code"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
}

// TestOutcome is one structured test result with identity, failure
// classification, and the observed assertion record when the harness
// emitted one. Proposed shared-contract addition alongside
// AssessmentSpec: a harness outcome protocol the backend parses, not
// builder prose.
type TestOutcome struct {
	TestID       string             `json:"testId"`
	Passed       bool               `json:"passed"`
	FailureClass string             `json:"failureClass,omitempty"`
	Message      string             `json:"message,omitempty"`
	Assertion    *ObservedAssertion `json:"assertion,omitempty"`
}

// AssessmentReport is the structured outcome of evaluating one solution.
// CompileError is set when the solution could not compile or load and no
// tests ran.
type AssessmentReport struct {
	Tests        []TestOutcome `json:"tests"`
	CompileError *string       `json:"compileError,omitempty"`
}

// AssessmentWork is one isolated solution evaluation.
type AssessmentWork struct {
	Role        string
	WorkDir     string
	TestCommand string
}

// AssessmentRunner evaluates one candidate solution in a prepared clean
// workspace and returns structured test outcomes. Implementations must
// honor context cancellation, bound captured output, and create nothing
// outside the given workspace; workspace lifecycle belongs to the verifier.
// A returned error means timeout (context.DeadlineExceeded), cancellation,
// or a platform fault — never a test failure, which belongs in the report.
type AssessmentRunner interface {
	Assess(ctx context.Context, work AssessmentWork) (AssessmentReport, error)
}

// assessSolutions runs the assessment phase: correct solution first, then
// the wrong solution. A nil return means both assessments satisfied their
// criteria and the caller may issue a verified verdict; the two evidence
// checks are already recorded. Any other return is the final rejected
// result. The correct solution runs first because its failure already
// invalidates the artifact; the wrong solution never runs after that.
func (v *verifier) assessSolutions(workspaceDir string, manifest Manifest) *VerifierResult {
	spec := v.assessment
	if err := validateAssessmentSpec(spec, manifest); err != nil {
		return v.rejectWithCheck(StageValidation, CodeAssessmentInputsMissing, false,
			fmt.Sprintf("Assessment inputs are missing or invalid: %v.", err),
			map[string]any{"error": err.Error()},
			"Assessment inputs missing.")
	}
	if strings.TrimSpace(spec.ReviewedBy) == "" {
		return v.rejectWithCheck(StageValidation, CodeReviewPending, false,
			"Assessment inputs lack human review evidence. Human review is pending; no verified verdict is issued.",
			map[string]any{"artifact": spec.ArtifactID, "reviewedBy": nil},
			"Human review pending.")
	}
	if v.assessor == nil {
		v.appendCheck(EvidenceCheck{Stage: StageValidation, ExitCode: -1,
			Summary: "Assessment could not run: no assessment runner is configured."})
		platform := v.platformFailure(nil, StageValidation,
			errors.New("assessment runner is not configured"),
			"No assessment runner is configured.")
		return &platform
	}

	correct, failure := v.assessOne(RoleCorrectSolution, manifest, workspaceDir, spec.CorrectSolution)
	if failure != nil {
		return failure
	}
	if correct.CompileError != nil {
		return v.rejectWithCheck(StageValidation, CodeCorrectSolutionFailed, true,
			fmt.Sprintf("The reviewed correct solution did not compile: %s.", truncateOutput(*correct.CompileError)),
			map[string]any{"assessment": RoleCorrectSolution, "compileError": truncateOutput(*correct.CompileError)},
			"Correct solution did not compile.")
	}
	if len(correct.Tests) == 0 {
		return v.rejectWithCheck(StageValidation, CodeAssessmentMalformed, true,
			"The correct-solution assessment reported no tests.",
			map[string]any{"assessment": RoleCorrectSolution},
			"Correct assessment reported no tests.")
	}
	if failed := failedTests(correct); len(failed) > 0 {
		return v.rejectWithCheck(StageValidation, CodeCorrectSolutionFailed, true,
			fmt.Sprintf("The reviewed correct solution failed %d of %d assessment tests: %s.",
				len(failed), len(correct.Tests), strings.Join(testIDs(failed), ", ")),
			map[string]any{"assessment": RoleCorrectSolution, "failedTests": testIDs(failed)},
			"Correct solution failed assessment.")
	}
	// The intended rejection is only meaningful if the correct solution
	// actually ran the expected test: a correct run that omits it proves
	// nothing about the wrong solution's later failure there.
	if _, found := findTest(correct.Tests, spec.ExpectedFailure.TestID); !found {
		return v.rejectWithCheck(StageValidation, CodeAssessmentMalformed, true,
			fmt.Sprintf("The correct-solution assessment did not run the expected test %q.",
				spec.ExpectedFailure.TestID),
			map[string]any{"assessment": RoleCorrectSolution, "expectedTest": spec.ExpectedFailure.TestID},
			"Expected test missing from assessment.")
	}
	v.appendCheck(EvidenceCheck{Stage: StageValidation,
		Summary: fmt.Sprintf("Correct solution passed %d/%d assessment tests.", len(correct.Tests), len(correct.Tests))})

	wrong, failure := v.assessOne(RoleWrongSolution, manifest, workspaceDir, spec.WrongSolution)
	if failure != nil {
		return failure
	}
	if wrong.CompileError != nil {
		return v.rejectWithCheck(StageValidation, CodeWrongSolutionUnexpected, true,
			fmt.Sprintf("The wrong solution could not compile, so it cannot fail for the intended reason: %s.",
				truncateOutput(*wrong.CompileError)),
			map[string]any{"assessment": RoleWrongSolution, "compileError": truncateOutput(*wrong.CompileError)},
			"Wrong solution did not compile.")
	}
	if len(wrong.Tests) == 0 {
		return v.rejectWithCheck(StageValidation, CodeAssessmentMalformed, true,
			"The wrong-solution assessment reported no tests.",
			map[string]any{"assessment": RoleWrongSolution},
			"Wrong assessment reported no tests.")
	}
	expected, found := findTest(wrong.Tests, spec.ExpectedFailure.TestID)
	if !found {
		return v.rejectWithCheck(StageValidation, CodeAssessmentMalformed, true,
			fmt.Sprintf("The wrong-solution assessment did not run the expected test %q.",
				spec.ExpectedFailure.TestID),
			map[string]any{"assessment": RoleWrongSolution, "expectedTest": spec.ExpectedFailure.TestID},
			"Expected test missing from assessment.")
	}
	if len(failedTests(wrong)) == 0 {
		return v.rejectWithCheck(StageValidation, CodeWrongSolutionPassed, true,
			fmt.Sprintf("The wrong solution passed all %d assessment tests, including the expected rejection assertion %q.",
				len(wrong.Tests), expected.TestID),
			map[string]any{"assessment": RoleWrongSolution, "expectedTest": expected.TestID,
				"reason": spec.ExpectedFailure.Reason},
			"Wrong solution unexpectedly passed.")
	}
	if expected.Passed {
		return v.rejectWithCheck(StageValidation, CodeWrongSolutionUnexpected, true,
			fmt.Sprintf("The wrong solution passed the expected rejection assertion %q; other failures: %s.",
				expected.TestID, describeOthers(wrong.Tests, expected.TestID)),
			map[string]any{"assessment": RoleWrongSolution, "expectedTest": expected.TestID,
				"reason": spec.ExpectedFailure.Reason},
			"Wrong solution unexpectedly passed.")
	}
	if expected.FailureClass != ClassAssertion {
		return v.rejectWithCheck(StageValidation, CodeWrongSolutionUnexpected, true,
			fmt.Sprintf("The wrong solution failed %q with %s, want an assertion failure for: %s.",
				expected.TestID, failureClassName(expected.FailureClass), spec.ExpectedFailure.Reason),
			map[string]any{"assessment": RoleWrongSolution, "expectedTest": expected.TestID,
				"failureClass": expected.FailureClass, "reason": spec.ExpectedFailure.Reason},
			"Wrong solution failed for an unrelated reason.")
	}
	if mismatch := matchAssertion(spec.ExpectedFailure, expected.Assertion); mismatch != "" {
		observed := map[string]any{"present": false}
		if expected.Assertion != nil {
			observed = map[string]any{
				"present":  true,
				"code":     expected.Assertion.Code,
				"expected": expected.Assertion.Expected,
				"actual":   expected.Assertion.Actual,
			}
		}
		return v.rejectWithCheck(StageValidation, CodeAssertionMismatch, true,
			fmt.Sprintf("The wrong solution failed %q but the assertion evidence does not match: %s.",
				expected.TestID, mismatch),
			map[string]any{"assessment": RoleWrongSolution, "expectedTest": expected.TestID,
				"mismatch": mismatch,
				"want": map[string]any{
					"code":     spec.ExpectedFailure.Code,
					"expected": spec.ExpectedFailure.Expected,
					"actual":   spec.ExpectedFailure.Actual,
				},
				"observed": observed},
			"Assertion evidence mismatch.")
	}
	// Additional assertion failures beyond the expected one are tolerated:
	// only the intended rejection is required. They are recorded for review.
	v.appendCheck(EvidenceCheck{Stage: StageValidation,
		Summary: fmt.Sprintf("Wrong solution failed as intended: %s (%s): expected %q, observed %q.",
			expected.TestID, spec.ExpectedFailure.Code,
			spec.ExpectedFailure.Expected, spec.ExpectedFailure.Actual)})
	return nil
}

// matchAssertion compares backend-controlled expectations against the
// observed assertion record. It returns "" on an exact match of code,
// expected, and actual values, or a structured description of the first
// difference. A missing record, however the test failed, never counts as
// the intended behavioral failure.
func matchAssertion(want ExpectedFailure, observed *ObservedAssertion) string {
	if observed == nil {
		return "no assertion record was emitted for the expected test"
	}
	if observed.Code != want.Code {
		return fmt.Sprintf("assertion code is %q, want %q", observed.Code, want.Code)
	}
	if observed.Expected != want.Expected {
		return fmt.Sprintf("expected value is %q, want %q", observed.Expected, want.Expected)
	}
	if observed.Actual != want.Actual {
		return fmt.Sprintf("actual value is %q, want %q", observed.Actual, want.Actual)
	}
	return ""
}

// assessOne evaluates one solution in a separate clean workspace: the source
// artifact is copied, only declared learner-editable files are replaced,
// and the copy is removed on every path afterward. Backend-owned deadlines
// apply; cancellation is checked before starting.
func (v *verifier) assessOne(role string, manifest Manifest, workspaceDir string, files map[string]string) (AssessmentReport, *VerifierResult) {
	if err := v.ctx.Err(); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			v.appendCheck(EvidenceCheck{Stage: StageValidation, ExitCode: 1,
				Summary: roleTitle(role) + " assessment did not start: parent deadline exceeded."})
			result := v.reject(StageTimeout, CodeStageTimeout, false,
				fmt.Sprintf("Verification stopped before the %s assessment: the parent context deadline was exceeded.", role),
				map[string]any{"stage": StageValidation, "assessment": role, "timeout": true, "started": false})
			return AssessmentReport{}, &result
		}
		v.appendCheck(EvidenceCheck{Stage: StageValidation, ExitCode: -1,
			Summary: roleTitle(role) + " assessment did not start: parent context cancelled."})
		platform := v.platformFailure(nil, StageValidation, err,
			fmt.Sprintf("Verification stopped before the %s assessment: the parent context was cancelled.", role))
		platform.Diagnostics[0].Details = map[string]any{"stage": StageValidation, "assessment": role, "error": err.Error()}
		return AssessmentReport{}, &platform
	}
	workDir, err := prepareSolutionWorkspace(workspaceDir, files)
	if err != nil {
		v.appendCheck(EvidenceCheck{Stage: StageValidation, ExitCode: -1,
			Summary: roleTitle(role) + " workspace could not be prepared."})
		platform := v.platformFailure(nil, StageValidation, err,
			fmt.Sprintf("The %s workspace could not be prepared.", role))
		platform.Diagnostics[0].Details = map[string]any{"stage": StageValidation, "assessment": role, "error": err.Error()}
		return AssessmentReport{}, &platform
	}
	defer os.RemoveAll(workDir)

	assessCtx := v.ctx
	cancel := func() {}
	if timeout := v.timeouts.Assessment; timeout > 0 {
		assessCtx, cancel = context.WithTimeout(v.ctx, timeout)
	}
	defer cancel()
	report, err := v.assessor.Assess(assessCtx, AssessmentWork{
		Role:        role,
		WorkDir:     workDir,
		TestCommand: strings.TrimSpace(manifest.Commands.Test),
	})
	// As with stage commands, a report returned after the backend context
	// died is untrustworthy: cancellation supersedes it, so a cancelled
	// final assessment can never issue verified.
	if err == nil {
		if ctxErr := v.ctx.Err(); ctxErr != nil {
			err = ctxErr
		}
	}
	timedOut := errors.Is(err, context.DeadlineExceeded) || errors.Is(assessCtx.Err(), context.DeadlineExceeded)
	switch {
	case timedOut:
		v.appendCheck(EvidenceCheck{Stage: StageValidation, ExitCode: 1,
			Summary: roleTitle(role) + " assessment timed out."})
		result := v.reject(StageTimeout, CodeStageTimeout, false,
			fmt.Sprintf("The %s assessment exceeded its deadline.", role),
			map[string]any{"stage": StageValidation, "assessment": role, "timeout": true})
		return AssessmentReport{}, &result
	case err != nil:
		v.appendCheck(EvidenceCheck{Stage: StageValidation, ExitCode: -1,
			Summary: roleTitle(role) + " assessment hit a platform fault."})
		platform := v.platformFailure(nil, StageValidation, err,
			fmt.Sprintf("The %s assessment could not complete.", role))
		platform.Diagnostics[0].Details = map[string]any{"stage": StageValidation, "assessment": role, "error": err.Error()}
		return AssessmentReport{}, &platform
	default:
		return report, nil
	}
}

// rejectWithCheck records a failed validation check, then rejects. Used for
// assessment-phase outcomes where the check itself ran.
func (v *verifier) rejectWithCheck(stage, code string, repairable bool, message string, details map[string]any, summary string) *VerifierResult {
	v.appendCheck(EvidenceCheck{Stage: stage, ExitCode: 1, Summary: summary})
	result := v.reject(stage, code, repairable, message, details)
	return &result
}

// validateAssessmentSpec enforces the assessment input boundary: binding to
// the artifact under verification, nonempty solutions keyed only by
// declared learner-editable files, and a fully specified expected failure.
func validateAssessmentSpec(spec *AssessmentSpec, manifest Manifest) error {
	if spec == nil {
		return errors.New("assessment inputs are missing")
	}
	if strings.TrimSpace(spec.ArtifactID) != strings.TrimSpace(manifest.Artifact.ID) ||
		strings.TrimSpace(spec.ArtifactVersion) != strings.TrimSpace(manifest.Artifact.Version) {
		return fmt.Errorf("assessment inputs target artifact %q version %q, want manifest artifact %q version %q",
			strings.TrimSpace(spec.ArtifactID), strings.TrimSpace(spec.ArtifactVersion),
			strings.TrimSpace(manifest.Artifact.ID), strings.TrimSpace(manifest.Artifact.Version))
	}
	if strings.TrimSpace(spec.Objective) == "" {
		return errors.New("assessment inputs: objective is required")
	}
	if len(spec.CorrectSolution) == 0 {
		return errors.New("assessment inputs: correctSolution must include at least one file")
	}
	if len(spec.WrongSolution) == 0 {
		return errors.New("assessment inputs: wrongSolution must include at least one file")
	}
	editable := make(map[string]struct{}, len(manifest.Workspace.LearnerEditable))
	for _, path := range manifest.Workspace.LearnerEditable {
		editable[strings.TrimSpace(path)] = struct{}{}
	}
	for role, files := range map[string]map[string]string{
		RoleCorrectSolution: spec.CorrectSolution,
		RoleWrongSolution:   spec.WrongSolution,
	} {
		for path := range files {
			if strings.TrimSpace(path) == "" {
				return fmt.Errorf("assessment inputs: %s has a blank file path", role)
			}
			if _, ok := editable[strings.TrimSpace(path)]; !ok {
				return fmt.Errorf("assessment inputs: %s file %q is not a declared learner-editable file", role, path)
			}
		}
	}
	if strings.TrimSpace(spec.ExpectedFailure.TestID) == "" {
		return errors.New("assessment inputs: expectedFailure.testId is required")
	}
	if strings.TrimSpace(spec.ExpectedFailure.Code) == "" {
		return errors.New("assessment inputs: expectedFailure.code is required")
	}
	if strings.TrimSpace(spec.ExpectedFailure.Expected) == "" {
		return errors.New("assessment inputs: expectedFailure.expected is required")
	}
	if strings.TrimSpace(spec.ExpectedFailure.Actual) == "" {
		return errors.New("assessment inputs: expectedFailure.actual is required")
	}
	if strings.TrimSpace(spec.ExpectedFailure.Reason) == "" {
		return errors.New("assessment inputs: expectedFailure.reason is required")
	}
	return nil
}

// prepareSolutionWorkspace copies the verified workspace tree into a fresh
// temporary directory and overlays solution content onto declared
// learner-editable files only. The source artifact is only read, never
// written. Callers remove the returned directory on every path.
func prepareSolutionWorkspace(workspaceDir string, files map[string]string) (string, error) {
	dir, err := os.MkdirTemp("", "codegym-assess-*")
	if err != nil {
		return "", fmt.Errorf("create assessment workspace: %w", err)
	}
	if err := copyWorkspaceTree(workspaceDir, dir); err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	for path, content := range files {
		rel := filepath.FromSlash(strings.TrimSpace(path))
		full := filepath.Join(dir, rel)
		if escapesRoot(dir, full) {
			os.RemoveAll(dir)
			return "", fmt.Errorf("solution file %q escapes the assessment workspace", path)
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			os.RemoveAll(dir)
			return "", fmt.Errorf("prepare solution file %q: %w", path, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			os.RemoveAll(dir)
			return "", fmt.Errorf("prepare solution file %q: %w", path, err)
		}
	}
	return dir, nil
}

// copyWorkspaceTree replicates regular files and directories into a fresh
// assessment workspace. The source tree passed artifact validation only for
// its declared files, so every copied entry — including unlisted symlinks —
// is resolved and validated here: entries resolving outside the approved
// source boundary are rejected, directory cycles are rejected instead of
// recursed forever, and unsupported file types are rejected. Internal
// symlinks still materialize as content copies. Failures propagate to the
// caller as platform faults: they are never builder-repairable and never
// verified.
func copyWorkspaceTree(srcDir, dstDir string) error {
	boundary, err := filepath.EvalSymlinks(srcDir)
	if err != nil {
		return fmt.Errorf("resolve workspace: %w", err)
	}
	return copyWorkspaceTreeInto(boundary, boundary, dstDir, map[string]struct{}{boundary: {}})
}

func copyWorkspaceTreeInto(boundary, srcDir, dstDir string, visited map[string]struct{}) error {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return fmt.Errorf("read workspace: %w", err)
	}
	for _, entry := range entries {
		canonical, err := filepath.EvalSymlinks(filepath.Join(srcDir, entry.Name()))
		if err != nil {
			return fmt.Errorf("resolve %q: %w", entry.Name(), err)
		}
		if escapesRoot(boundary, canonical) {
			return fmt.Errorf("workspace entry %q resolves outside the approved source boundary", entry.Name())
		}
		info, err := os.Stat(canonical)
		if err != nil {
			return fmt.Errorf("stat %q: %w", entry.Name(), err)
		}
		dst := filepath.Join(dstDir, entry.Name())
		switch {
		case info.IsDir():
			if _, ok := visited[canonical]; ok {
				return fmt.Errorf("workspace directory cycle at %q", entry.Name())
			}
			visited[canonical] = struct{}{}
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return fmt.Errorf("create directory %q: %w", entry.Name(), err)
			}
			if err := copyWorkspaceTreeInto(boundary, canonical, dst, visited); err != nil {
				return err
			}
		case info.Mode().IsRegular():
			data, err := os.ReadFile(canonical)
			if err != nil {
				return fmt.Errorf("read file %q: %w", entry.Name(), err)
			}
			if err := os.WriteFile(dst, data, 0o644); err != nil {
				return fmt.Errorf("write file %q: %w", entry.Name(), err)
			}
		default:
			return fmt.Errorf("unsupported file type %q", entry.Name())
		}
	}
	return nil
}

func failedTests(report AssessmentReport) []TestOutcome {
	var failed []TestOutcome
	for _, test := range report.Tests {
		if !test.Passed {
			failed = append(failed, test)
		}
	}
	return failed
}

func testIDs(tests []TestOutcome) []string {
	ids := make([]string, 0, len(tests))
	for _, test := range tests {
		ids = append(ids, test.TestID)
	}
	return ids
}

func findTest(tests []TestOutcome, id string) (TestOutcome, bool) {
	for _, test := range tests {
		if test.TestID == id {
			return test, true
		}
	}
	return TestOutcome{}, false
}

func describeOthers(tests []TestOutcome, except string) string {
	var others []string
	for _, test := range tests {
		if test.TestID == except || test.Passed {
			continue
		}
		others = append(others, test.TestID)
	}
	if len(others) == 0 {
		return "none"
	}
	return strings.Join(others, ", ")
}

func failureClassName(class string) string {
	if strings.TrimSpace(class) == "" {
		return "an unclassified failure"
	}
	return "a " + class + " failure"
}

func roleTitle(role string) string {
	switch role {
	case RoleCorrectSolution:
		return "Correct solution"
	case RoleWrongSolution:
		return "Wrong solution"
	default:
		return role
	}
}
