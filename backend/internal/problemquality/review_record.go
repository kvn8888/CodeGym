package problemquality

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const ReviewRecordFileName = "review.yaml"

// ErrReviewRecordMissing means the case has no explicit review record. It is
// separate from a malformed record so callers can keep missing approval
// evidence unresolved rather than treating it as an invalid case.
var ErrReviewRecordMissing = errors.New("problem-quality review record is missing")

// ReviewRecord is the explicit human-review evidence stored beside a case.
// Existing corpus files contain reviewed_by, reviewed_at, ambiguous_spec,
// notes, and denominator_decisions; the remaining fields are the approval
// evidence required for benchmark eligibility. Unknown fields are rejected so
// a misspelled approval field cannot be ignored.
type ReviewRecord struct {
	ReviewedBy            string                      `yaml:"reviewed_by"`
	ReviewerAuthority     string                      `yaml:"reviewer_authority"`
	ReviewedAt            string                      `yaml:"reviewed_at"`
	ReviewReference       string                      `yaml:"review_reference"`
	ApprovalScope         string                      `yaml:"approval_scope"`
	ApprovedContentSHA256 string                      `yaml:"approved_content_sha256"`
	RevokedAt             string                      `yaml:"revoked_at"`
	RevokedReason         string                      `yaml:"revoked_reason"`
	AmbiguousSpec         *bool                       `yaml:"ambiguous_spec,omitempty"`
	Notes                 string                      `yaml:"notes,omitempty"`
	DenominatorDecisions  []ReviewDenominatorDecision `yaml:"denominator_decisions,omitempty"`
}

type ReviewDenominatorDecision struct {
	ID       string `yaml:"id"`
	Decision string `yaml:"decision"`
}

// LoadReviewRecord reads the optional review record. A missing file is
// reported with ErrReviewRecordMissing; malformed YAML or unknown fields are
// validation errors. Approval completeness is evaluated by the benchmark
// layer so missing approval evidence does not make the case unrunnable.
func LoadReviewRecord(caseDir string) (*ReviewRecord, error) {
	dir, err := filepath.Abs(caseDir)
	if err != nil {
		return nil, fmt.Errorf("resolve case directory: %w", err)
	}
	path := filepath.Join(dir, ReviewRecordFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrReviewRecordMissing
		}
		return nil, fmt.Errorf("read %s: %w", ReviewRecordFileName, err)
	}

	var record ReviewRecord
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&record); err != nil {
		return nil, fmt.Errorf("parse %s: %w", ReviewRecordFileName, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s must contain exactly one document", ReviewRecordFileName)
	}

	record.ReviewedBy = strings.TrimSpace(record.ReviewedBy)
	record.ReviewerAuthority = strings.TrimSpace(record.ReviewerAuthority)
	record.ReviewedAt = strings.TrimSpace(record.ReviewedAt)
	record.ReviewReference = strings.TrimSpace(record.ReviewReference)
	record.ApprovalScope = strings.ToLower(strings.TrimSpace(record.ApprovalScope))
	record.ApprovedContentSHA256 = strings.ToLower(strings.TrimSpace(record.ApprovedContentSHA256))
	record.RevokedAt = strings.TrimSpace(record.RevokedAt)
	record.RevokedReason = strings.TrimSpace(record.RevokedReason)
	record.Notes = strings.TrimSpace(record.Notes)
	for i := range record.DenominatorDecisions {
		record.DenominatorDecisions[i].ID = strings.TrimSpace(record.DenominatorDecisions[i].ID)
		record.DenominatorDecisions[i].Decision = strings.TrimSpace(record.DenominatorDecisions[i].Decision)
	}

	if record.ReviewedAt != "" {
		if err := validateDateTime(record.ReviewedAt); err != nil {
			return nil, fmt.Errorf("%s: reviewed_at: %w", ReviewRecordFileName, err)
		}
	}
	if record.RevokedAt != "" {
		if err := validateDateTime(record.RevokedAt); err != nil {
			return nil, fmt.Errorf("%s: revoked_at: %w", ReviewRecordFileName, err)
		}
	}
	return &record, nil
}
