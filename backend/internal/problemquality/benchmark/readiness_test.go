package benchmark

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestValidateSavedCaseReadinessReportsMissingProvenance(t *testing.T) {
	dir := copyFixture(t)
	readiness, err := ValidateSavedCaseReadiness(dir, "current")
	if err != nil {
		t.Fatal(err)
	}
	if !readiness.Runnable {
		t.Fatal("runnable = false, want true")
	}
	if readiness.ProvenanceComplete {
		t.Fatal("provenance_complete = true, want false without generation provenance")
	}
	if !slices.Equal(readiness.MissingProvenance, []string{"generation_provenance"}) {
		t.Fatalf("missing provenance = %v", readiness.MissingProvenance)
	}
	if readiness.GovernedContentSHA256 == "" {
		t.Fatal("governed content digest is empty")
	}
	if !readiness.DevelopmentFixture {
		t.Fatal("development_fixture = false, want true")
	}
}

func TestValidateSavedCaseReadinessAcceptsCompleteProvenance(t *testing.T) {
	dir := copyFixture(t)
	writeGenerationProvenance(t, dir, completeProvenance())

	readiness, err := ValidateSavedCaseReadiness(dir, "current")
	if err != nil {
		t.Fatal(err)
	}
	if !readiness.Runnable || !readiness.ProvenanceComplete {
		t.Fatalf("readiness = %+v, want runnable with complete provenance", readiness)
	}
	if len(readiness.MissingProvenance) != 0 {
		t.Fatalf("missing provenance = %v, want none", readiness.MissingProvenance)
	}
	identity := loadIdentity(t, dir)
	if readiness.GovernedContentSHA256 != mustIdentityDigest(t, identity) {
		t.Fatal("readiness digest does not match the governed content identity")
	}
}

func TestValidateSavedCaseReadinessReportsPartialUsage(t *testing.T) {
	dir := copyFixture(t)
	provenance := completeProvenance()
	provenance["usage"] = map[string]any{
		"input_tokens": int64(1200),
		"source":       "saved-generation-record",
	}
	writeGenerationProvenance(t, dir, provenance)

	readiness, err := ValidateSavedCaseReadiness(dir, "current")
	if err != nil {
		t.Fatal(err)
	}
	if !readiness.Runnable {
		t.Fatal("runnable = false, want true")
	}
	if readiness.ProvenanceComplete {
		t.Fatal("provenance_complete = true, want false for partial usage")
	}
	for _, field := range []string{
		"generation_provenance.usage.output_tokens",
		"generation_provenance.usage.cost_micros",
	} {
		if !slices.Contains(readiness.MissingProvenance, field) {
			t.Fatalf("missing provenance = %v, want %s", readiness.MissingProvenance, field)
		}
	}
}

func TestValidateSavedCaseReadinessRequiresIndependentObservations(t *testing.T) {
	dir := copyFixture(t)
	writeGenerationProvenance(t, dir, completeProvenance())

	readiness, err := ValidateSavedCaseReadiness(dir, "independent")
	if err != nil {
		t.Fatal(err)
	}
	if readiness.ProvenanceComplete {
		t.Fatal("provenance_complete = true, want false without independent observations")
	}
	if !slices.Contains(readiness.MissingProvenance, "generation_provenance.observations") {
		t.Fatalf("missing provenance = %v, want observations", readiness.MissingProvenance)
	}
}

func TestValidateSavedCaseReadinessRejectsInvalidConfiguration(t *testing.T) {
	dir := copyFixture(t)
	path := filepath.Join(dir, RunConfigFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	config["case_id"] = "different-case"
	updated, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := ValidateSavedCaseReadiness(dir, "current"); err == nil {
		t.Fatal("readiness error = nil, want invalid configuration error")
	}
}

func completeProvenance() map[string]any {
	return map[string]any{
		"model_revision":  "test-model-2026-11",
		"prompt_revision": "test-prompt-v1",
		"config":          map[string]any{"temperature": 0},
		"usage": map[string]any{
			"input_tokens":  int64(1200),
			"output_tokens": int64(300),
			"cost_micros":   int64(4200),
			"currency":      "USD",
			"cost_status":   "provider_reported",
			"source":        "saved-generation-record",
		},
	}
}

func writeGenerationProvenance(t *testing.T, dir string, provenance map[string]any) {
	t.Helper()
	path := filepath.Join(dir, RunConfigFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	config["generation_provenance"] = provenance
	updated, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		t.Fatal(err)
	}
}
