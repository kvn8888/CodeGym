package main

import (
	"fmt"
	"strconv"
	"testing"
)

func checkCount(t *testing.T, testID string, numbers []int, expected int) {
	t.Helper()
	actual := CountEven(numbers)
	if actual == expected {
		fmt.Printf("CODEGYM_TEST %s PASS\n", testID)
		return
	}
	fmt.Printf("CODEGYM_TEST %s FAIL code=WRONG_COUNT expected=%s actual=%s\n",
		testID, strconv.Itoa(expected), strconv.Itoa(actual))
	t.Errorf("CountEven(%v) = %d, want %d", numbers, actual, expected)
}

func TestHiddenMixedValues(t *testing.T) {
	checkCount(t, "hidden-mixed-values", []int{1, 2, 3, 4, 5}, 2)
}

func TestHiddenNegativeValues(t *testing.T) {
	checkCount(t, "hidden-negative-values", []int{-2, 2, -3, 3}, 2)
}

func TestHiddenEmptyValues(t *testing.T) {
	checkCount(t, "hidden-empty-values", []int{}, 0)
}

func TestHiddenZeroValue(t *testing.T) {
	checkCount(t, "hidden-zero-value", []int{0}, 1)
}
