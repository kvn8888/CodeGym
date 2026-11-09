package environmentverify

import (
	"testing"
)

func TestParseTestOutcomesVectors(t *testing.T) {
	outcomes := ParseTestOutcomes("noise\nCODEGYM_TEST t1 PASS\nCODEGYM_TEST t2 FAIL code=WRONG_VALUE expected=ok actual=broken\nCODEGYM_TEST broken\n")
	if len(outcomes) != 2 {
		t.Fatalf("outcomes = %d, want 2", len(outcomes))
	}
	if !outcomes[0].Passed || outcomes[0].TestID != "t1" {
		t.Fatalf("first outcome = %#v, want t1 PASS", outcomes[0])
	}
	second := outcomes[1]
	if second.Passed || second.TestID != "t2" || second.FailureClass != ClassAssertion {
		t.Fatalf("second outcome = %#v, want t2 assertion failure", second)
	}
	if second.Assertion == nil || second.Assertion.Code != "WRONG_VALUE" ||
		second.Assertion.Expected != "ok" || second.Assertion.Actual != "broken" {
		t.Fatalf("second assertion = %#v, want coded expected/actual", second.Assertion)
	}
	if len(ParseTestOutcomes("nothing structured here\n")) != 0 {
		t.Fatal("expected no outcomes from unstructured output")
	}
	if failed := FailedOutcomes(outcomes); len(failed) != 1 || failed[0].TestID != "t2" {
		t.Fatalf("failed = %#v, want only t2", failed)
	}
}
