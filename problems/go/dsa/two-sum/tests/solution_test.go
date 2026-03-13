package twosum

import (
	"sort"
	"testing"
)

func TestTwoSum_BasicCase(t *testing.T) {
	result := TwoSum([]int{2, 7, 11, 15}, 9)
	sort.Ints(result)
	if len(result) != 2 || result[0] != 0 || result[1] != 1 {
		t.Errorf("expected [0, 1], got %v", result)
	}
}

func TestTwoSum_MiddleElements(t *testing.T) {
	result := TwoSum([]int{3, 2, 4}, 6)
	sort.Ints(result)
	if len(result) != 2 || result[0] != 1 || result[1] != 2 {
		t.Errorf("expected [1, 2], got %v", result)
	}
}

func TestTwoSum_SameValues(t *testing.T) {
	result := TwoSum([]int{3, 3}, 6)
	sort.Ints(result)
	if len(result) != 2 || result[0] != 0 || result[1] != 1 {
		t.Errorf("expected [0, 1], got %v", result)
	}
}

func TestTwoSum_NegativeNumbers(t *testing.T) {
	result := TwoSum([]int{-1, -2, -3, -4, -5}, -8)
	sort.Ints(result)
	if len(result) != 2 || result[0] != 2 || result[1] != 4 {
		t.Errorf("expected [2, 4], got %v", result)
	}
}

func TestTwoSum_LargerArray(t *testing.T) {
	result := TwoSum([]int{1, 5, 8, 3, 9, 2}, 11)
	sort.Ints(result)
	if len(result) != 2 || result[0] != 1 || result[1] != 3 {
		t.Errorf("expected in sorted order to contain indices summing to 11, got %v", result)
	}
}
