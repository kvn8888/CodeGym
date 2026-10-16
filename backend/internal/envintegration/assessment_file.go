package envintegration

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/kvn8888/codegym/backend/internal/environmentverify"
)

// assessmentFileEntry is one technology's reviewed material as stored in the
// demo and production configuration file. It reuses the verifier's own
// assessment shapes, so the file cannot drift from what verification
// enforces. The assessor is always the generic local assessor: reviewed
// content varies per technology, the runner does not.
type assessmentFileEntry struct {
	CorrectSolution   map[string]string                 `json:"correctSolution"`
	WrongSolution     map[string]string                 `json:"wrongSolution"`
	ExpectedFailure   environmentverify.ExpectedFailure `json:"expectedFailure"`
	ReviewedBy        string                            `json:"reviewedBy"`
	ReviewedReference string                            `json:"reviewedReference"`
}

// LoadAssessmentFile reads reviewed assessment inputs from a JSON
// configuration file of the form {"technologies": {"<name>": {...}}}. A
// blank path is not an error: it yields an empty source that fails closed,
// so unreviewed technologies stay unverifiable by default. Incomplete
// entries are errors, never silent gaps.
func LoadAssessmentFile(path string) (*MapAssessmentSource, error) {
	source := &MapAssessmentSource{Entries: map[string]ReviewedSolutions{}}
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return source, nil
	}
	payload, err := os.ReadFile(trimmed)
	if err != nil {
		return nil, fmt.Errorf("envintegration: read assessment inputs file: %w", err)
	}
	var document struct {
		Technologies map[string]assessmentFileEntry `json:"technologies"`
	}
	if err := json.Unmarshal(payload, &document); err != nil {
		return nil, fmt.Errorf("envintegration: parse assessment inputs file: %w", err)
	}
	for technology, entry := range document.Technologies {
		if len(entry.CorrectSolution) == 0 || len(entry.WrongSolution) == 0 || strings.TrimSpace(entry.ReviewedBy) == "" {
			return nil, fmt.Errorf("envintegration: assessment entry for %q is incomplete", technology)
		}
		source.Entries[strings.ToLower(strings.TrimSpace(technology))] = ReviewedSolutions{
			CorrectSolution:   entry.CorrectSolution,
			WrongSolution:     entry.WrongSolution,
			ExpectedFailure:   entry.ExpectedFailure,
			ReviewedBy:        entry.ReviewedBy,
			ReviewedReference: entry.ReviewedReference,
			Assessor:          &LocalAssessor{},
		}
	}
	return source, nil
}
