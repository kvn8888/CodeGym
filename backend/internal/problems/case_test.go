package problems

import (
	"encoding/json"
	"testing"
)

func TestCaseVisibilityUsesHiddenAsAuthority(t *testing.T) {
	cases := []UnitCase{
		{CaseMetadata: CaseMetadata{Kind: CaseKindHidden, Hidden: false, Rationale: "Visible despite its descriptive kind."}, Name: "public", Args: []json.RawMessage{json.RawMessage(`1`)}, Expected: json.RawMessage(`2`)},
		{CaseMetadata: CaseMetadata{Kind: CaseKindExample, Hidden: true}, Name: "private", Args: []json.RawMessage{json.RawMessage(`2`)}, Expected: json.RawMessage(`4`)},
	}
	selected := SelectUnitCases(cases, false)
	if len(selected) != 1 || selected[0].Name != "public" {
		t.Fatalf("public selection = %#v", selected)
	}
	projected := ProjectPublicUnitCases(cases)
	if len(projected) != 1 || projected[0].Kind != CaseKindHidden || projected[0].Explanation == "" {
		t.Fatalf("public projection = %#v", projected)
	}
	encoded, err := json.Marshal(projected)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) == "" || json.Valid(encoded) == false {
		t.Fatalf("projection JSON = %q", encoded)
	}
}

func TestNormalizeCaseMetadataAndVisibilityMix(t *testing.T) {
	metadata, err := NormalizeCaseMetadata(CaseMetadata{Kind: " EDGE ", Hidden: true, Rationale: " boundary "})
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Kind != CaseKindEdge || metadata.Rationale != "boundary" || !metadata.Hidden {
		t.Fatalf("normalized metadata = %#v", metadata)
	}
	if _, err := NormalizeCaseMetadata(CaseMetadata{Kind: "unknown"}); err == nil {
		t.Fatal("unknown kind was accepted")
	}
	if err := ValidateCaseVisibilityMix(CaseVisibilityCounts{Public: 2, Hidden: 2}, 2); err != nil {
		t.Fatalf("balanced mix: %v", err)
	}
	if err := ValidateCaseVisibilityMix(CaseVisibilityCounts{Public: 4, Hidden: 0}, 2); err == nil {
		t.Fatal("zero-hidden mix was accepted")
	}
}
