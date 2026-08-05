package problems

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const MaxCaseRationaleLength = 300

// CaseKind is descriptive coverage metadata. Hidden, not Kind, controls
// whether a case may be exposed before submission.
type CaseKind string

const (
	CaseKindExample    CaseKind = "example"
	CaseKindFunctional CaseKind = "functional"
	CaseKindEdge       CaseKind = "edge"
	CaseKindStress     CaseKind = "stress"
	CaseKindHidden     CaseKind = "hidden"
)

type CaseMetadata struct {
	Kind      CaseKind `json:"kind"`
	Hidden    bool     `json:"hidden"`
	Rationale string   `json:"rationale,omitempty"`
}

// UnitCase is one server-owned function invocation and expected return value.
type UnitCase struct {
	CaseMetadata
	Name       string            `json:"name"`
	Args       []json.RawMessage `json:"args"`
	Expected   json.RawMessage   `json:"expected"`
	Comparator *Comparator       `json:"comparator,omitempty"`
}

// PublicCase is the strategy-discriminated worked-example projection returned
// by the problem API. Expected contains a JSON value for unit cases and an
// HTTPExpectation for HTTP cases. Hidden is deliberately absent.
type PublicCase struct {
	Strategy    TestStrategy      `json:"strategy"`
	Name        string            `json:"name"`
	Kind        CaseKind          `json:"kind"`
	Args        []json.RawMessage `json:"args,omitempty"`
	Request     *HTTPRequest      `json:"request,omitempty"`
	Expected    any               `json:"expected"`
	Explanation string            `json:"explanation,omitempty"`
}

type CaseVisibilityCounts struct {
	Public int
	Hidden int
}

func NormalizeCaseMetadata(input CaseMetadata) (CaseMetadata, error) {
	input.Kind = CaseKind(strings.ToLower(strings.TrimSpace(string(input.Kind))))
	switch input.Kind {
	case CaseKindExample, CaseKindFunctional, CaseKindEdge, CaseKindStress, CaseKindHidden:
	default:
		return CaseMetadata{}, fmt.Errorf("case kind must be example, functional, edge, stress, or hidden, got %q", input.Kind)
	}
	input.Rationale = strings.TrimSpace(input.Rationale)
	if len(input.Rationale) > MaxCaseRationaleLength {
		return CaseMetadata{}, fmt.Errorf("case rationale must be at most %d characters", MaxCaseRationaleLength)
	}
	return input, nil
}

func SelectUnitCases(cases []UnitCase, includeHidden bool) []UnitCase {
	selected := make([]UnitCase, 0, len(cases))
	for _, testCase := range cases {
		if !testCase.Hidden || includeHidden {
			selected = append(selected, testCase)
		}
	}
	return selected
}

func SelectHTTPCases(cases []HTTPCase, includeHidden bool) []HTTPCase {
	selected := make([]HTTPCase, 0, len(cases))
	for _, testCase := range cases {
		if !testCase.Hidden || includeHidden {
			selected = append(selected, testCase)
		}
	}
	return selected
}

func CountUnitCaseVisibility(cases []UnitCase) CaseVisibilityCounts {
	counts := CaseVisibilityCounts{}
	for _, testCase := range cases {
		if testCase.Hidden {
			counts.Hidden++
		} else {
			counts.Public++
		}
	}
	return counts
}

func CountHTTPCaseVisibility(cases []HTTPCase) CaseVisibilityCounts {
	counts := CaseVisibilityCounts{}
	for _, testCase := range cases {
		if testCase.Hidden {
			counts.Hidden++
		} else {
			counts.Public++
		}
	}
	return counts
}

func ValidateCaseVisibilityMix(counts CaseVisibilityCounts, minimumEach int) error {
	if minimumEach < 1 {
		return errors.New("minimum case count must be positive")
	}
	if counts.Public < minimumEach || counts.Hidden < minimumEach {
		return fmt.Errorf("test cases must contain at least %d public and %d hidden cases (got %d public, %d hidden)", minimumEach, minimumEach, counts.Public, counts.Hidden)
	}
	if difference := counts.Public - counts.Hidden; difference > 2 || difference < -2 {
		return fmt.Errorf("test cases must be roughly half public and half hidden (got %d public, %d hidden)", counts.Public, counts.Hidden)
	}
	return nil
}

func ProjectPublicUnitCases(cases []UnitCase) []PublicCase {
	selected := SelectUnitCases(cases, false)
	projected := make([]PublicCase, 0, len(selected))
	for _, testCase := range selected {
		projected = append(projected, PublicCase{
			Strategy: TestStrategyUnit, Name: testCase.Name, Kind: testCase.Kind,
			Args: append([]json.RawMessage(nil), testCase.Args...), Expected: cloneRawMessage(testCase.Expected),
			Explanation: testCase.Rationale,
		})
	}
	return projected
}

func ProjectPublicHTTPCases(cases []HTTPCase) []PublicCase {
	selected := SelectHTTPCases(cases, false)
	projected := make([]PublicCase, 0, len(selected))
	for _, testCase := range selected {
		request := testCase.Request
		projected = append(projected, PublicCase{
			Strategy: TestStrategyHTTP, Name: testCase.Name, Kind: testCase.Kind,
			Request: &request, Expected: testCase.Expect, Explanation: testCase.Rationale,
		})
	}
	return projected
}

func cloneRawMessage(input json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), input...)
}
