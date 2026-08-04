package problems

import (
	"math"
	"testing"
)

func TestNormalizeComparator(t *testing.T) {
	t.Run("zero value defaults to exact", func(t *testing.T) {
		got, err := NormalizeComparator(Comparator{})
		if err != nil || got.Kind != ComparatorExact || got.Epsilon != nil {
			t.Fatalf("NormalizeComparator() = %#v, %v", got, err)
		}
	})

	t.Run("float defaults epsilon", func(t *testing.T) {
		got, err := NormalizeComparator(Comparator{Kind: ComparatorFloat})
		if err != nil || got.Epsilon == nil || *got.Epsilon != DefaultFloatEpsilon {
			t.Fatalf("NormalizeComparator() = %#v, %v", got, err)
		}
	})

	for _, test := range []struct {
		name       string
		comparator Comparator
	}{
		{name: "unknown", comparator: Comparator{Kind: "close-enough"}},
		{name: "zero epsilon", comparator: Comparator{Kind: ComparatorFloat, Epsilon: floatPointer(0)}},
		{name: "negative epsilon", comparator: Comparator{Kind: ComparatorFloat, Epsilon: floatPointer(-1)}},
		{name: "nan epsilon", comparator: Comparator{Kind: ComparatorFloat, Epsilon: floatPointer(math.NaN())}},
		{name: "infinite epsilon", comparator: Comparator{Kind: ComparatorFloat, Epsilon: floatPointer(math.Inf(1))}},
		{name: "epsilon on exact", comparator: Comparator{Kind: ComparatorExact, Epsilon: floatPointer(1e-3)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NormalizeComparator(test.comparator); err == nil {
				t.Fatalf("NormalizeComparator(%#v) unexpectedly succeeded", test.comparator)
			}
		})
	}
}

func TestResolveComparator(t *testing.T) {
	problem := Comparator{Kind: ComparatorSet}
	got, err := ResolveComparator(problem, nil)
	if err != nil || got.Kind != ComparatorSet {
		t.Fatalf("inherited comparator = %#v, %v", got, err)
	}

	override := Comparator{Kind: ComparatorMultiset}
	got, err = ResolveComparator(problem, &override)
	if err != nil || got.Kind != ComparatorMultiset {
		t.Fatalf("overridden comparator = %#v, %v", got, err)
	}
}

func floatPointer(value float64) *float64 { return &value }
