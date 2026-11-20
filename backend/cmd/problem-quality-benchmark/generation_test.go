package main

import (
	"testing"

	"github.com/kvn8888/codegym/backend/internal/generation"
)

func TestEstimateGenerationCostMicros(t *testing.T) {
	usage := generation.GenerateResult{TokensIn: 473, TokensOut: 768}
	want := int64(473+768) * 2
	if got := estimateGenerationCostMicros(usage); got != want {
		t.Fatalf("estimateGenerationCostMicros = %d, want %d", got, want)
	}
}

func TestEstimateGenerationCostMicrosZeroUsage(t *testing.T) {
	if got := estimateGenerationCostMicros(generation.GenerateResult{}); got != 0 {
		t.Fatalf("estimateGenerationCostMicros = %d, want 0", got)
	}
}
