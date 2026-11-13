package problemquality

import (
	"fmt"
	"strings"

	"github.com/kvn8888/codegym/backend/internal/environmentverify"
)

// ReferenceInput is the explicit generated-reference input for benchmark
// assessment. Files maps declared learner-editable paths to replacement
// content. The reference is never inferred from the correct alternatives or
// read from the artifact filesystem.
type ReferenceInput struct {
	ID                string
	Files             map[string]string
	ReviewedBy        string
	ReviewedAt        string
	ReviewedReference string
}

type CompatibilityError struct {
	CaseID string
	Field  string
	Reason string
}

func (e *CompatibilityError) Error() string {
	prefix := "problem-quality case"
	if strings.TrimSpace(e.CaseID) != "" {
		prefix = fmt.Sprintf("problem-quality case %q", e.CaseID)
	}
	if strings.TrimSpace(e.Field) != "" {
		return fmt.Sprintf("%s cannot be represented by the environment verifier: %s: %s", prefix, e.Field, e.Reason)
	}
	return fmt.Sprintf("%s cannot be represented by the environment verifier: %s", prefix, e.Reason)
}

func (c *Case) AssessmentSpec(correctAlternativeID, wrongSolutionID string) (*environmentverify.AssessmentSpec, error) {
	if c == nil {
		return nil, &CompatibilityError{Reason: "case is nil"}
	}
	if c.EnvironmentManifestData == nil {
		return nil, &CompatibilityError{CaseID: c.CaseID, Field: "environment_manifest",
			Reason: "environment manifest was not loaded"}
	}

	_, correct, ok := findCorrectAlternative(c.CorrectAlternatives, correctAlternativeID)
	if !ok {
		return nil, &CompatibilityError{CaseID: c.CaseID, Field: "correct_alternatives",
			Reason: fmt.Sprintf("selected correct alternative %q was not found", correctAlternativeID)}
	}
	wrongIndex, wrong, ok := findWrongSolution(c.WrongSolutions, wrongSolutionID)
	if !ok {
		return nil, &CompatibilityError{CaseID: c.CaseID, Field: "wrong_solutions",
			Reason: fmt.Sprintf("selected wrong solution %q was not found", wrongSolutionID)}
	}
	if strings.TrimSpace(wrong.DenominatorDecision) != "meaningful" {
		return nil, &CompatibilityError{CaseID: c.CaseID,
			Field:  fmt.Sprintf("wrong_solutions[%d].denominator_decision", wrongIndex),
			Reason: fmt.Sprintf("the pair verifier requires a meaningful wrong solution, got %q", wrong.DenominatorDecision)}
	}
	if wrong.IntendedFailure.Expected == nil || strings.TrimSpace(*wrong.IntendedFailure.Expected) == "" {
		return nil, &CompatibilityError{CaseID: c.CaseID,
			Field:  fmt.Sprintf("wrong_solutions[%d].intended_failure.expected", wrongIndex),
			Reason: "the verifier requires non-empty assertion evidence; the schema-valid value cannot be passed through unchanged"}
	}
	if wrong.IntendedFailure.Actual == nil || strings.TrimSpace(*wrong.IntendedFailure.Actual) == "" {
		return nil, &CompatibilityError{CaseID: c.CaseID,
			Field:  fmt.Sprintf("wrong_solutions[%d].intended_failure.actual", wrongIndex),
			Reason: "the verifier requires non-empty assertion evidence; the schema-valid value cannot be passed through unchanged"}
	}
	if len(correct.ResolvedFiles) == 0 || len(wrong.ResolvedFiles) == 0 {
		return nil, &CompatibilityError{CaseID: c.CaseID, Field: "files",
			Reason: "replacement files were not resolved"}
	}

	return &environmentverify.AssessmentSpec{
		ArtifactID:        c.ArtifactID,
		ArtifactVersion:   c.ArtifactVersion,
		Objective:         c.Objective,
		CorrectSolution:   replacementContents(correct.ResolvedFiles),
		WrongSolution:     replacementContents(wrong.ResolvedFiles),
		ExpectedFailure:   expectedFailure(wrong.IntendedFailure),
		ReviewedBy:        strings.TrimSpace(c.Review.ReviewedBy),
		ReviewedReference: fmt.Sprintf("case:%s;correct:%s;wrong:%s", c.CaseID, correct.ID, wrong.ID),
	}, nil
}

// AssessmentPlan converts the whole case into a benchmark assessment plan.
// It retains every correct alternative and wrong solution, including
// equivalent and invalid wrong entries, so exclusions stay visible in the
// collection. A nil reference is represented as a missing reference and is
// rejected by the verifier with a blocked collection.
func (c *Case) AssessmentPlan(reference *ReferenceInput) (*environmentverify.AssessmentPlan, error) {
	if c == nil {
		return nil, &CompatibilityError{Reason: "case is nil"}
	}
	if c.EnvironmentManifestData == nil {
		return nil, &CompatibilityError{CaseID: c.CaseID, Field: "environment_manifest",
			Reason: "environment manifest was not loaded"}
	}
	if err := c.validateCommandConsistency(); err != nil {
		return nil, err
	}
	plan := &environmentverify.AssessmentPlan{
		ArtifactID:        c.ArtifactID,
		ArtifactVersion:   c.ArtifactVersion,
		Objective:         c.Objective,
		ReviewedBy:        strings.TrimSpace(c.Review.ReviewedBy),
		ReviewedAt:        strings.TrimSpace(c.Review.ReviewedAt),
		ReviewedReference: fmt.Sprintf("case:%s", c.CaseID),
	}
	if reference != nil {
		plan.Reference = &environmentverify.AssessmentSolution{
			ID:                strings.TrimSpace(reference.ID),
			Role:              environmentverify.RoleGeneratedReference,
			Files:             copyContents(reference.Files),
			ReviewedBy:        strings.TrimSpace(reference.ReviewedBy),
			ReviewedAt:        strings.TrimSpace(reference.ReviewedAt),
			ReviewedReference: strings.TrimSpace(reference.ReviewedReference),
		}
	}
	for _, correct := range c.CorrectAlternatives {
		plan.CorrectAlternatives = append(plan.CorrectAlternatives, environmentverify.AssessmentSolution{
			ID:                correct.ID,
			Role:              environmentverify.RoleCorrectAlternative,
			Files:             replacementContents(correct.ResolvedFiles),
			ReviewedBy:        strings.TrimSpace(correct.ReviewedBy),
			ReviewedAt:        strings.TrimSpace(correct.ReviewedAt),
			ReviewedReference: fmt.Sprintf("case:%s;correct:%s", c.CaseID, correct.ID),
		})
	}
	for _, wrong := range c.WrongSolutions {
		solution := environmentverify.AssessmentSolution{
			ID:                  wrong.ID,
			Role:                environmentverify.RoleWrongSolution,
			Files:               replacementContents(wrong.ResolvedFiles),
			DenominatorDecision: strings.TrimSpace(wrong.DenominatorDecision),
			ReviewedBy:          strings.TrimSpace(wrong.ReviewedBy),
			ReviewedAt:          strings.TrimSpace(wrong.ReviewedAt),
			ReviewedReference:   fmt.Sprintf("case:%s;wrong:%s", c.CaseID, wrong.ID),
		}
		if solution.DenominatorDecision == environmentverify.DenominatorMeaningful {
			solution.ExpectedFailure = planExpectedFailure(wrong.IntendedFailure)
		}
		plan.WrongSolutions = append(plan.WrongSolutions, solution)
	}
	return plan, nil
}

func (c *Case) validateCommandConsistency() error {
	manifest := c.EnvironmentManifestData
	commands := []struct {
		field    string
		declared string
		manifest string
	}{
		{"setup", c.BuildTestRun.Commands.Setup, manifest.Commands.Setup},
		{"build", c.BuildTestRun.Commands.Build, manifest.Commands.Build},
		{"test", c.BuildTestRun.Commands.Test, manifest.Commands.Test},
		{"run", c.BuildTestRun.Commands.Run, manifest.Commands.Run},
	}
	for _, command := range commands {
		declared := strings.TrimSpace(command.declared)
		if declared == "" {
			continue
		}
		fromManifest := strings.TrimSpace(command.manifest)
		if declared != fromManifest {
			return &CompatibilityError{CaseID: c.CaseID,
				Field: fmt.Sprintf("build_test_run.commands.%s", command.field),
				Reason: fmt.Sprintf("case command %q does not match environment manifest command %q; the manifest command is authoritative",
					declared, fromManifest)}
		}
	}
	return nil
}

func copyContents(files map[string]string) map[string]string {
	contents := make(map[string]string, len(files))
	for target, content := range files {
		contents[target] = content
	}
	return contents
}

func planExpectedFailure(failure IntendedFailure) *environmentverify.ExpectedFailure {
	return &environmentverify.ExpectedFailure{
		TestID:   failure.TestID,
		Code:     failure.Code,
		Expected: stringValue(failure.Expected),
		Actual:   stringValue(failure.Actual),
		Reason:   failure.Reason,
	}
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func findCorrectAlternative(solutions []Solution, id string) (int, *Solution, bool) {
	for index := range solutions {
		if solutions[index].ID == strings.TrimSpace(id) {
			return index, &solutions[index], true
		}
	}
	return -1, nil, false
}

func findWrongSolution(solutions []WrongSolution, id string) (int, *WrongSolution, bool) {
	for index := range solutions {
		if solutions[index].ID == strings.TrimSpace(id) {
			return index, &solutions[index], true
		}
	}
	return -1, nil, false
}

func replacementContents(files map[string]ResolvedFile) map[string]string {
	contents := make(map[string]string, len(files))
	for target, file := range files {
		contents[target] = file.Content
	}
	return contents
}

func expectedFailure(failure IntendedFailure) environmentverify.ExpectedFailure {
	return environmentverify.ExpectedFailure{
		TestID:   failure.TestID,
		Code:     failure.Code,
		Expected: *failure.Expected,
		Actual:   *failure.Actual,
		Reason:   failure.Reason,
	}
}
