package benchmark

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/kvn8888/codegym/backend/internal/environmentverify"
	"github.com/kvn8888/codegym/backend/internal/problemquality"
)

const ReviewApprovalScopeCase = "case"

// ReviewEvaluation is the benchmark-layer eligibility decision. Eligibility
// requires an explicit review record whose approved content digest matches
// the governed case identity and whose reviewer authority is configured by
// the caller. Missing policy, missing evidence, stale evidence, revocation,
// and fixtures remain unresolved and excluded.
type ReviewEvaluation struct {
	Status string
	Reason string
	Record *problemquality.ReviewRecord
}

func (e ReviewEvaluation) Eligible() bool {
	return e.Status == environmentverify.BenchmarkEligibilityEligible
}

func EvaluateReviewEligibility(c *problemquality.Case, identity *ContentIdentity, authorities []string, fixture bool) ReviewEvaluation {
	unresolved := func(format string, args ...any) ReviewEvaluation {
		return ReviewEvaluation{
			Status: environmentverify.BenchmarkEligibilityUnresolved,
			Reason: fmt.Sprintf(format, args...),
		}
	}
	if c == nil {
		return unresolved("case is missing")
	}
	if fixture {
		return unresolved("development fixtures are always excluded from benchmark eligibility")
	}
	allowed := configuredReviewAuthorities(authorities)
	if len(allowed) == 0 {
		return unresolved("review policy is not configured; supply at least one -review-authority")
	}
	record, err := problemquality.LoadReviewRecord(c.Dir)
	if errors.Is(err, problemquality.ErrReviewRecordMissing) {
		return unresolved("review record %s is missing", problemquality.ReviewRecordFileName)
	}
	if err != nil {
		return unresolved("review record is invalid: %v", err)
	}
	if record.RevokedAt != "" {
		reason := fmt.Sprintf("approval was revoked at %s", record.RevokedAt)
		if record.RevokedReason != "" {
			reason += ": " + record.RevokedReason
		}
		return ReviewEvaluation{Status: environmentverify.BenchmarkEligibilityUnresolved, Reason: reason, Record: record}
	}
	if missing := missingApprovalFields(record); len(missing) > 0 {
		return ReviewEvaluation{
			Status: environmentverify.BenchmarkEligibilityUnresolved,
			Reason: "approval evidence is incomplete: missing " + strings.Join(missing, ", "),
			Record: record,
		}
	}
	if isPlaceholderReviewer(record.ReviewedBy) {
		return ReviewEvaluation{
			Status: environmentverify.BenchmarkEligibilityUnresolved,
			Reason: fmt.Sprintf("reviewed_by %q is not a human reviewer identity", record.ReviewedBy),
			Record: record,
		}
	}
	if _, ok := allowed[normalizeAuthority(record.ReviewerAuthority)]; !ok {
		return ReviewEvaluation{
			Status: environmentverify.BenchmarkEligibilityUnresolved,
			Reason: fmt.Sprintf("reviewer authority %q is not configured as authorized", record.ReviewerAuthority),
			Record: record,
		}
	}
	if !validReviewReference(record.ReviewReference) {
		return ReviewEvaluation{
			Status: environmentverify.BenchmarkEligibilityUnresolved,
			Reason: fmt.Sprintf("review_reference %q is not a resolvable review record", record.ReviewReference),
			Record: record,
		}
	}
	if record.ApprovalScope != ReviewApprovalScopeCase {
		return ReviewEvaluation{
			Status: environmentverify.BenchmarkEligibilityUnresolved,
			Reason: fmt.Sprintf("approval_scope %q is unsupported; expected %q", record.ApprovalScope, ReviewApprovalScopeCase),
			Record: record,
		}
	}
	if err := validateCaseReviewConsistency(c, record); err != nil {
		return ReviewEvaluation{
			Status: environmentverify.BenchmarkEligibilityUnresolved,
			Reason: err.Error(),
			Record: record,
		}
	}
	if err := validateDenominatorReview(c, record); err != nil {
		return ReviewEvaluation{
			Status: environmentverify.BenchmarkEligibilityUnresolved,
			Reason: err.Error(),
			Record: record,
		}
	}
	if identity == nil {
		return unresolved("content identity is unavailable")
	}
	digest, err := identity.SHA256()
	if err != nil {
		return unresolved("compute governed content identity: %v", err)
	}
	if _, err := normalizeSHA256Digest("approved_content_sha256", record.ApprovedContentSHA256); err != nil {
		return ReviewEvaluation{
			Status: environmentverify.BenchmarkEligibilityUnresolved,
			Reason: "approval is invalid: " + err.Error(),
			Record: record,
		}
	}
	if !strings.EqualFold(record.ApprovedContentSHA256, digest) {
		return ReviewEvaluation{
			Status: environmentverify.BenchmarkEligibilityUnresolved,
			Reason: "approval is stale: governed content identity has changed since approval",
			Record: record,
		}
	}
	return ReviewEvaluation{
		Status: environmentverify.BenchmarkEligibilityEligible,
		Reason: "review approval matches the governed content identity and an authorized reviewer",
		Record: record,
	}
}

func configuredReviewAuthorities(values []string) map[string]struct{} {
	allowed := make(map[string]struct{}, len(values))
	for _, value := range values {
		normalized := normalizeAuthority(value)
		if normalized == "" {
			continue
		}
		allowed[normalized] = struct{}{}
	}
	return allowed
}

func normalizeAuthority(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func missingApprovalFields(record *problemquality.ReviewRecord) []string {
	var missing []string
	if record.ReviewedBy == "" {
		missing = append(missing, "reviewed_by")
	}
	if record.ReviewerAuthority == "" {
		missing = append(missing, "reviewer_authority")
	}
	if record.ReviewedAt == "" {
		missing = append(missing, "reviewed_at")
	}
	if record.ReviewReference == "" {
		missing = append(missing, "review_reference")
	}
	if record.ApprovalScope == "" {
		missing = append(missing, "approval_scope")
	}
	if record.ApprovedContentSHA256 == "" {
		missing = append(missing, "approved_content_sha256")
	}
	if record.AmbiguousSpec == nil {
		missing = append(missing, "ambiguous_spec")
	}
	if len(record.DenominatorDecisions) == 0 {
		missing = append(missing, "denominator_decisions")
	}
	return missing
}

func isPlaceholderReviewer(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "reviewer", "review", "fixture", "development-fixture", "development fixture",
		"test", "synthetic", "todo", "pending", "not human approval":
		return true
	default:
		return false
	}
}

func validReviewReference(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || strings.ContainsAny(trimmed, " \t\r\n") {
		return false
	}
	if strings.Contains(strings.ToLower(trimmed), "placeholder") ||
		strings.Contains(strings.ToLower(trimmed), "not human approval") {
		return false
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "https", "review":
		return parsed.Host != ""
	default:
		return false
	}
}

func validateCaseReviewConsistency(c *problemquality.Case, record *problemquality.ReviewRecord) error {
	reviewedBy := strings.TrimSpace(c.Review.ReviewedBy)
	if reviewedBy == "" {
		return errors.New("case review is missing reviewed_by")
	}
	if reviewedBy != record.ReviewedBy {
		return fmt.Errorf("review record reviewer %q does not match case reviewer %q", record.ReviewedBy, reviewedBy)
	}
	reviewedAt := strings.TrimSpace(c.Review.ReviewedAt)
	if reviewedAt == "" {
		return errors.New("case review is missing reviewed_at")
	}
	if reviewedAt != record.ReviewedAt {
		return fmt.Errorf("review record date %q does not match case review date %q", record.ReviewedAt, reviewedAt)
	}
	if c.Review.AmbiguousSpec == nil {
		return errors.New("case review is missing ambiguous_spec")
	}
	if record.AmbiguousSpec == nil || *c.Review.AmbiguousSpec != *record.AmbiguousSpec {
		return errors.New("review record ambiguous_spec does not match the case review decision")
	}
	return nil
}

func validateDenominatorReview(c *problemquality.Case, record *problemquality.ReviewRecord) error {
	decisions := make(map[string]string, len(record.DenominatorDecisions))
	for _, decision := range record.DenominatorDecisions {
		if decision.ID == "" {
			return errors.New("review record contains a denominator decision with no solution id")
		}
		if _, exists := decisions[decision.ID]; exists {
			return fmt.Errorf("review record contains duplicate denominator decision for %q", decision.ID)
		}
		switch decision.Decision {
		case "meaningful", "equivalent", "invalid":
		default:
			return fmt.Errorf("review record has unsupported denominator decision %q for %q", decision.Decision, decision.ID)
		}
		decisions[decision.ID] = decision.Decision
	}
	expected := make(map[string]struct{}, len(c.WrongSolutions))
	for _, solution := range c.WrongSolutions {
		id := strings.TrimSpace(solution.ID)
		expected[id] = struct{}{}
		reviewed, ok := decisions[id]
		if !ok {
			return fmt.Errorf("review record is missing denominator decision for %q", id)
		}
		if reviewed != strings.TrimSpace(solution.DenominatorDecision) {
			return fmt.Errorf("review record denominator decision for %q is %q, case says %q",
				id, reviewed, strings.TrimSpace(solution.DenominatorDecision))
		}
	}
	for id := range decisions {
		if _, ok := expected[id]; !ok {
			return fmt.Errorf("review record has denominator decision for unknown wrong solution %q", id)
		}
	}
	return nil
}
