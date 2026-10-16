package envintegration

import (
	"testing"

	"github.com/kvn8888/codegym/backend/internal/environmentverify"
)

func TestParseTestOutcomes(t *testing.T) {
	outcomes := ParseTestOutcomes("noise\nCODEGYM_TEST t1 PASS\nCODEGYM_TEST t2 FAIL code=WRONG_VALUE expected=ok actual=broken\nCODEGYM_TEST broken\n")
	if len(outcomes) != 2 {
		t.Fatalf("outcomes = %#v", outcomes)
	}
	if outcomes[0].TestID != "t1" || !outcomes[0].Passed {
		t.Fatalf("first = %#v", outcomes[0])
	}
	if outcomes[1].TestID != "t2" || outcomes[1].Passed || outcomes[1].FailureClass != environmentverify.ClassAssertion {
		t.Fatalf("second = %#v", outcomes[1])
	}
	assertion := outcomes[1].Assertion
	if assertion == nil || assertion.Code != "WRONG_VALUE" || assertion.Expected != "ok" || assertion.Actual != "broken" {
		t.Fatalf("assertion = %#v", assertion)
	}
	if len(ParseTestOutcomes("nothing structured here\n")) != 0 {
		t.Fatal("unstructured output must yield no outcomes, never a fabricated pass")
	}
}

func TestLocalAssessorParsesRealCommand(t *testing.T) {
	assessor := &LocalAssessor{}
	report, err := assessor.Assess(t.Context(), environmentverify.AssessmentWork{
		TestCommand: `printf 'CODEGYM_TEST t1 PASS\nCODEGYM_TEST t2 FAIL code=WRONG_VALUE expected=ok actual=broken\n'`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Tests) != 2 {
		t.Fatalf("report = %#v", report)
	}
}
