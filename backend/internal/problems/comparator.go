package problems

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

const DefaultFloatEpsilon = 1e-6

type ComparatorKind string

const (
	ComparatorExact    ComparatorKind = "exact"
	ComparatorSet      ComparatorKind = "set"
	ComparatorMultiset ComparatorKind = "multiset"
	ComparatorSorted   ComparatorKind = "sorted"
	ComparatorFloat    ComparatorKind = "float"
	ComparatorChecker  ComparatorKind = "checker"
)

// Comparator is the language-neutral acceptance predicate attached to a
// problem or, as an override, to one hidden test case.
type Comparator struct {
	Kind    ComparatorKind `json:"kind"`
	Epsilon *float64       `json:"epsilon,omitempty"`
}

// NormalizeComparator validates a comparator and fills its documented
// defaults. The zero value is the backwards-compatible exact comparator.
func NormalizeComparator(input Comparator) (Comparator, error) {
	input.Kind = ComparatorKind(strings.ToLower(strings.TrimSpace(string(input.Kind))))
	if input.Kind == "" {
		input.Kind = ComparatorExact
	}

	switch input.Kind {
	case ComparatorExact, ComparatorSet, ComparatorMultiset, ComparatorSorted, ComparatorChecker:
		if input.Epsilon != nil {
			return Comparator{}, fmt.Errorf("comparator %q does not accept epsilon", input.Kind)
		}
	case ComparatorFloat:
		if input.Epsilon == nil {
			epsilon := DefaultFloatEpsilon
			input.Epsilon = &epsilon
		}
		if *input.Epsilon <= 0 || math.IsNaN(*input.Epsilon) || math.IsInf(*input.Epsilon, 0) {
			return Comparator{}, errors.New("float comparator epsilon must be finite and greater than zero")
		}
	default:
		return Comparator{}, fmt.Errorf("unknown comparator kind %q", input.Kind)
	}
	return input, nil
}

// ResolveComparator applies a case-level override to the problem default.
// A nil override inherits the problem comparator; a present zero-value
// override explicitly selects exact comparison.
func ResolveComparator(problem Comparator, caseOverride *Comparator) (Comparator, error) {
	resolved, err := NormalizeComparator(problem)
	if err != nil {
		return Comparator{}, fmt.Errorf("problem comparator: %w", err)
	}
	if caseOverride == nil {
		return resolved, nil
	}
	resolved, err = NormalizeComparator(*caseOverride)
	if err != nil {
		return Comparator{}, fmt.Errorf("case comparator: %w", err)
	}
	return resolved, nil
}
