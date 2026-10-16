package envintegration

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAssessmentFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "assessments.json")
	payload := `{"technologies":{"demo":{"correctSolution":{"solution.py":"VALUE = 1\n"},"wrongSolution":{"solution.py":"VALUE = 2\n"},"expectedFailure":{"testId":"t1","code":"WRONG_VALUE","expected":"1","actual":"2","reason":"returns 2"},"reviewedBy":"test-review","reviewedReference":"file-fixture"}}}`
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
	source, err := LoadAssessmentFile(path)
	if err != nil {
		t.Fatal(err)
	}
	spec, assessor, err := source.Inputs(t.Context(), "Demo", "demo-1", "0.1.0", "learn demo")
	if err != nil {
		t.Fatal(err)
	}
	if spec.ArtifactID != "demo-1" || spec.Objective != "learn demo" || spec.ReviewedBy != "test-review" {
		t.Fatalf("spec = %#v", spec)
	}
	if assessor == nil {
		t.Fatal("no assessor attached")
	}
	if _, _, err := source.Inputs(t.Context(), "unknown", "x-1", "0.1.0", "learn"); err == nil {
		t.Fatal("expected review-pending error for unknown technology")
	}
	blank, err := LoadAssessmentFile("")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := blank.Inputs(t.Context(), "demo", "d-1", "0.1.0", "learn"); err == nil {
		t.Fatal("expected fail-closed error for empty source")
	}
}
