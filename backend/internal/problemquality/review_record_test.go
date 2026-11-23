package problemquality

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReviewRecordMissing(t *testing.T) {
	_, err := LoadReviewRecord(t.TempDir())
	if !errors.Is(err, ErrReviewRecordMissing) {
		t.Fatalf("error = %v, want ErrReviewRecordMissing", err)
	}
}

func TestLoadReviewRecordRejectsUnknownApprovalField(t *testing.T) {
	dir := t.TempDir()
	writeReviewRecordFile(t, dir, "reviewed_by: alice\nreviewer_authorty: board\n")
	if _, err := LoadReviewRecord(dir); err == nil {
		t.Fatal("error = nil, want unknown field rejection")
	}
}

func TestLoadReviewRecordReadsExistingCorpusRecord(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "benchmarks", "problem-quality", "cases", "python-sum-even-numbers")
	record, err := LoadReviewRecord(dir)
	if err != nil {
		t.Fatal(err)
	}
	if record.ReviewedBy != "Hunter and Cristina" || record.AmbiguousSpec == nil || *record.AmbiguousSpec {
		t.Fatalf("record = %+v, want existing reviewed record", record)
	}
	if len(record.DenominatorDecisions) != 2 {
		t.Fatalf("denominator decisions = %+v, want two", record.DenominatorDecisions)
	}
}

func writeReviewRecordFile(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ReviewRecordFileName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
