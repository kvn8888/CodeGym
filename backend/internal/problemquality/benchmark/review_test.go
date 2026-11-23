package benchmark

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kvn8888/codegym/backend/internal/problemquality"
)

const testReviewAuthority = "problem-quality-review-board"

func TestEvaluateReviewEligibilityAcceptsMatchingApproval(t *testing.T) {
	dir, c, identity := eligibleReviewCase(t)
	writeReviewRecord(t, dir, reviewRecordYAML(c, mustIdentityDigest(t, identity)))

	evaluation := EvaluateReviewEligibility(c, identity, []string{testReviewAuthority}, false)
	if !evaluation.Eligible() {
		t.Fatalf("eligibility = %s (%s), want eligible", evaluation.Status, evaluation.Reason)
	}
}

func TestEvaluateReviewEligibilityRequiresPolicy(t *testing.T) {
	dir, c, identity := eligibleReviewCase(t)
	writeReviewRecord(t, dir, reviewRecordYAML(c, mustIdentityDigest(t, identity)))

	evaluation := EvaluateReviewEligibility(c, identity, nil, false)
	if evaluation.Eligible() {
		t.Fatal("eligibility = eligible, want unresolved without configured review policy")
	}
	if !strings.Contains(evaluation.Reason, "review policy is not configured") {
		t.Fatalf("reason = %q, want missing policy explanation", evaluation.Reason)
	}
}

func TestEvaluateReviewEligibilityRequiresReviewRecord(t *testing.T) {
	_, c, identity := eligibleReviewCase(t)
	evaluation := EvaluateReviewEligibility(c, identity, []string{testReviewAuthority}, false)
	if evaluation.Eligible() {
		t.Fatal("eligibility = eligible, want unresolved without review.yaml")
	}
	if !strings.Contains(evaluation.Reason, "review.yaml is missing") {
		t.Fatalf("reason = %q, want missing review record explanation", evaluation.Reason)
	}
}

func TestEvaluateReviewEligibilityRejectsStaleApproval(t *testing.T) {
	dir, c, identity := eligibleReviewCase(t)
	writeReviewRecord(t, dir, reviewRecordYAML(c, mustIdentityDigest(t, identity)))

	appendFile(t, filepath.Join(dir, "spec.json"), "\n")
	updated, _ := loadFixture(t, dir)
	updatedIdentity := loadIdentity(t, dir)

	evaluation := EvaluateReviewEligibility(updated, updatedIdentity, []string{testReviewAuthority}, false)
	if evaluation.Eligible() {
		t.Fatal("eligibility = eligible, want unresolved after governed content changed")
	}
	if !strings.Contains(evaluation.Reason, "stale") {
		t.Fatalf("reason = %q, want stale approval explanation", evaluation.Reason)
	}
}

func TestEvaluateReviewEligibilityRejectsRevocation(t *testing.T) {
	dir, c, identity := eligibleReviewCase(t)
	record := reviewRecordYAML(c, mustIdentityDigest(t, identity)) +
		"revoked_at: \"2026-11-19T00:00:00Z\"\nrevoked_reason: superseded by corrected case\n"
	writeReviewRecord(t, dir, record)

	evaluation := EvaluateReviewEligibility(c, identity, []string{testReviewAuthority}, false)
	if evaluation.Eligible() {
		t.Fatal("eligibility = eligible, want unresolved for a revoked approval")
	}
	if !strings.Contains(evaluation.Reason, "revoked") {
		t.Fatalf("reason = %q, want revocation explanation", evaluation.Reason)
	}
}

func TestEvaluateReviewEligibilityRejectsUnauthorizedReviewer(t *testing.T) {
	dir, c, identity := eligibleReviewCase(t)
	writeReviewRecord(t, dir, reviewRecordYAML(c, mustIdentityDigest(t, identity)))

	evaluation := EvaluateReviewEligibility(c, identity, []string{"some-other-board"}, false)
	if evaluation.Eligible() {
		t.Fatal("eligibility = eligible, want unresolved for an unauthorized reviewer")
	}
	if !strings.Contains(evaluation.Reason, "not configured as authorized") {
		t.Fatalf("reason = %q, want authority explanation", evaluation.Reason)
	}
}

func TestEvaluateReviewEligibilityRejectsDenominatorMismatch(t *testing.T) {
	dir, c, identity := eligibleReviewCase(t)
	record := strings.Replace(reviewRecordYAML(c, mustIdentityDigest(t, identity)),
		"decision: meaningful", "decision: equivalent", 1)
	writeReviewRecord(t, dir, record)

	evaluation := EvaluateReviewEligibility(c, identity, []string{testReviewAuthority}, false)
	if evaluation.Eligible() {
		t.Fatal("eligibility = eligible, want unresolved for a denominator mismatch")
	}
	if !strings.Contains(evaluation.Reason, "denominator decision") {
		t.Fatalf("reason = %q, want denominator explanation", evaluation.Reason)
	}
}

func TestEvaluateReviewEligibilityExcludesFixtures(t *testing.T) {
	dir, c, identity := eligibleReviewCase(t)
	writeReviewRecord(t, dir, reviewRecordYAML(c, mustIdentityDigest(t, identity)))

	evaluation := EvaluateReviewEligibility(c, identity, []string{testReviewAuthority}, true)
	if evaluation.Eligible() {
		t.Fatal("eligibility = eligible, want fixtures excluded")
	}
	if !strings.Contains(evaluation.Reason, "fixtures") {
		t.Fatalf("reason = %q, want fixture exclusion explanation", evaluation.Reason)
	}
}

func eligibleReviewCase(t *testing.T) (string, *problemquality.Case, *ContentIdentity) {
	t.Helper()
	dir := copyFixture(t)
	path := filepath.Join(dir, "case.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.ReplaceAll(data, []byte("reviewed_by: development-fixture"), []byte("reviewed_by: alice"))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := problemquality.LoadCase(dir)
	if err != nil {
		t.Fatal(err)
	}
	identity := loadIdentity(t, dir)
	return dir, c, identity
}

func reviewRecordYAML(c *problemquality.Case, digest string) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, `reviewed_by: alice
reviewer_authority: %s
reviewed_at: "2026-11-13T00:00:00Z"
review_reference: "review://quality/approvals/207"
approval_scope: case
approved_content_sha256: %q
ambiguous_spec: false
denominator_decisions:
`, testReviewAuthority, digest)
	for _, solution := range c.WrongSolutions {
		fmt.Fprintf(&builder, "  - id: %s\n    decision: %s\n", solution.ID, solution.DenominatorDecision)
	}
	return builder.String()
}

func writeReviewRecord(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, problemquality.ReviewRecordFileName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustIdentityDigest(t *testing.T, identity *ContentIdentity) string {
	t.Helper()
	digest, err := identity.SHA256()
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
