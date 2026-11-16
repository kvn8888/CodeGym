package benchmark

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kvn8888/codegym/backend/internal/benchmarkbudget"
	"github.com/kvn8888/codegym/backend/internal/environmentverify"
)

func TestValidateGenerationUsage(t *testing.T) {
	zero := int64(0)
	negative := int64(-1)
	tests := []struct {
		name    string
		usage   *GenerationUsage
		wantErr string
	}{
		{
			name: "provider reported",
			usage: &GenerationUsage{
				InputTokens: int64Value(1200), OutputTokens: int64Value(340),
				CostMicros: int64Value(4500), Currency: "usd",
				CostStatus: CostStatusProviderReported, Source: "provider export",
			},
		},
		{
			name: "estimated",
			usage: &GenerationUsage{
				InputTokens: int64Value(1200), OutputTokens: int64Value(340),
				CostMicros: int64Value(4500), Currency: "USD",
				CostStatus: CostStatusEstimated, Source: "saved run metadata",
				PricingBasis: "usage.DefaultRates reviewed 2026-07-15",
			},
		},
		{
			name: "tokens only",
			usage: &GenerationUsage{
				InputTokens: int64Value(1200), OutputTokens: int64Value(340),
				Source: "provider export",
			},
		},
		{
			name: "measured zero",
			usage: &GenerationUsage{
				InputTokens: &zero, OutputTokens: &zero,
				CostMicros: &zero, Currency: "USD",
				CostStatus: CostStatusProviderReported, Source: "provider export",
			},
		},
		{
			name: "missing source",
			usage: &GenerationUsage{
				InputTokens: int64Value(1),
			},
			wantErr: "source is required",
		},
		{
			name: "negative input",
			usage: &GenerationUsage{
				InputTokens: &negative, Source: "provider export",
			},
			wantErr: "input_tokens must be non-negative",
		},
		{
			name: "negative output",
			usage: &GenerationUsage{
				OutputTokens: &negative, Source: "provider export",
			},
			wantErr: "output_tokens must be non-negative",
		},
		{
			name: "negative cost",
			usage: &GenerationUsage{
				CostMicros: &negative, Currency: "USD",
				CostStatus: CostStatusProviderReported, Source: "provider export",
			},
			wantErr: "cost_micros must be non-negative",
		},
		{
			name:    "no measured values",
			usage:   &GenerationUsage{Source: "provider export"},
			wantErr: "at least one measured value",
		},
		{
			name: "cost without currency",
			usage: &GenerationUsage{
				CostMicros: int64Value(1), CostStatus: CostStatusProviderReported,
				Source: "provider export",
			},
			wantErr: "currency",
		},
		{
			name: "unsupported currency",
			usage: &GenerationUsage{
				CostMicros: int64Value(1), Currency: "EUR",
				CostStatus: CostStatusProviderReported, Source: "provider export",
			},
			wantErr: "only USD is accepted",
		},
		{
			name: "cost without status",
			usage: &GenerationUsage{
				CostMicros: int64Value(1), Currency: "USD", Source: "provider export",
			},
			wantErr: "cost_status",
		},
		{
			name: "provider reported with pricing basis",
			usage: &GenerationUsage{
				CostMicros: int64Value(1), Currency: "USD",
				CostStatus: CostStatusProviderReported, Source: "provider export",
				PricingBasis: "rate card",
			},
			wantErr: "must be omitted",
		},
		{
			name: "estimated without pricing basis",
			usage: &GenerationUsage{
				InputTokens: int64Value(1), OutputTokens: int64Value(1),
				CostMicros: int64Value(1), Currency: "USD",
				CostStatus: CostStatusEstimated, Source: "saved run metadata",
			},
			wantErr: "pricing_basis is required",
		},
		{
			name: "estimated without token counts",
			usage: &GenerationUsage{
				CostMicros: int64Value(1), Currency: "USD",
				CostStatus: CostStatusEstimated, Source: "saved run metadata",
				PricingBasis: "rate card",
			},
			wantErr: "requires both input_tokens and output_tokens",
		},
		{
			name: "currency without cost",
			usage: &GenerationUsage{
				InputTokens: int64Value(1), Currency: "USD", Source: "provider export",
			},
			wantErr: "currency requires cost_micros",
		},
		{
			name: "status without cost",
			usage: &GenerationUsage{
				InputTokens: int64Value(1), CostStatus: CostStatusProviderReported,
				Source: "provider export",
			},
			wantErr: "cost_status requires cost_micros",
		},
		{
			name: "pricing basis without cost",
			usage: &GenerationUsage{
				InputTokens: int64Value(1), PricingBasis: "rate card",
				Source: "provider export",
			},
			wantErr: "pricing_basis requires cost_micros",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateGenerationUsage(test.usage)
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("validateGenerationUsage: %v", err)
				}
				if test.usage.CostMicros != nil && test.usage.Currency != "USD" {
					t.Fatalf("currency = %q, want normalized USD", test.usage.Currency)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestDecodeRunConfigDistinguishesUnknownAndMeasuredZeroUsage(t *testing.T) {
	unknown, err := decodeRunConfig([]byte(`{"generation_provenance":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	if unknown.GenerationProvenance == nil || unknown.GenerationProvenance.Usage != nil {
		t.Fatalf("unknown usage = %+v, want nil usage", unknown.GenerationProvenance)
	}

	zero, err := decodeRunConfig([]byte(`{
		"generation_provenance": {
			"usage": {
				"input_tokens": 0,
				"output_tokens": 0,
				"cost_micros": 0,
				"currency": "USD",
				"cost_status": "provider_reported",
				"source": "provider export"
			}
		}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	usage := zero.GenerationProvenance.Usage
	if usage == nil || usage.InputTokens == nil || *usage.InputTokens != 0 ||
		usage.OutputTokens == nil || *usage.OutputTokens != 0 ||
		usage.CostMicros == nil || *usage.CostMicros != 0 {
		t.Fatalf("measured zero usage = %+v, want explicit zero pointers", usage)
	}

	_, err = decodeRunConfig([]byte(`{
		"generation_provenance": {
			"usage": {
				"input_tokens": -1,
				"source": "provider export"
			}
		}
	}`))
	if err == nil || !strings.Contains(err.Error(), "input_tokens must be non-negative") {
		t.Fatalf("invalid usage error = %v", err)
	}
}

func TestBuildReportSeparatesSavedExerciseAndAssessmentUsage(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	snapshot := &benchmarkbudget.Snapshot{
		Currency:         benchmarkbudget.Currency,
		LimitMicros:      benchmarkbudget.MilestoneLimitMicros,
		CommittedMicros:  1_000_000,
		RemainingMicros:  benchmarkbudget.MilestoneLimitMicros - 1_000_000,
		PriorSpendMicros: 1_000_000,
		Reservations:     []benchmarkbudget.Reservation{},
	}
	run := testCaseRun("saved-usage", false, false, environmentverify.BenchmarkEligibilityUnresolved, environmentverify.AssessmentCollection{
		Reference: completedReference(true),
	})
	run.Config.GenerationProvenance = &GenerationProvenanceConfig{
		ModelRevision:  "model-x",
		PromptRevision: "prompt-y",
		Usage: &GenerationUsage{
			InputTokens: int64Value(1200), OutputTokens: int64Value(340),
			CostMicros: int64Value(4500), Currency: "USD",
			CostStatus: CostStatusProviderReported, Source: "provider export 2026-11-12",
		},
	}
	report := BuildReport(Meta{
		RunID:          "pqb-usage",
		StartedAt:      now,
		CompletedAt:    now.Add(time.Second),
		TestedCommit:   "abc123",
		BaselineKind:   "other",
		Timeout:        time.Second,
		BudgetSnapshot: snapshot,
	}, []CaseRun{run})

	provenance := report.Cases[0].Provenance
	if provenance == nil {
		t.Fatal("provenance is missing")
	}
	saved := provenance.SavedExerciseUsage
	if saved.Status != UsageStatusKnown || saved.InputTokens == nil || *saved.InputTokens != 1200 ||
		saved.OutputTokens == nil || *saved.OutputTokens != 340 ||
		saved.CostMicros == nil || *saved.CostMicros != 4500 ||
		saved.Currency != "USD" || saved.CostStatus != CostStatusProviderReported ||
		saved.Source != "provider export 2026-11-12" {
		t.Fatalf("saved usage = %+v", saved)
	}
	assessment := provenance.AssessmentRunUsage
	if assessment.Status != UsageStatusNotApplicable || assessment.InputTokens != nil ||
		assessment.OutputTokens != nil || assessment.CostMicros != nil {
		t.Fatalf("assessment usage = %+v, want separate not_applicable fact", assessment)
	}
	if report.Execution.ModelOperations || report.Execution.PaidOperations {
		t.Fatalf("execution = %+v, want no paid or model operations", report.Execution)
	}
	if report.Budget == nil || report.Budget.CommittedMicros != snapshot.CommittedMicros ||
		report.Budget.RemainingMicros != snapshot.RemainingMicros {
		t.Fatalf("budget changed by historical usage: %+v", report.Budget)
	}
	if report.GenerationConfig["assessment_run_usage"] != UsageStatusNotApplicable {
		t.Fatalf("assessment usage config = %v", report.GenerationConfig["assessment_run_usage"])
	}
	if !hasRecommendation(report.Recommendations, "not charged to the benchmark budget") {
		t.Fatalf("recommendations = %v, want historical-usage caveat", report.Recommendations)
	}
	validateReportSchema(t, report)
}

func TestBuildReportMeasuredZeroUsageIsKnownNotUnavailable(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	zero := int64(0)
	run := testCaseRun("zero-usage", false, false, environmentverify.BenchmarkEligibilityUnresolved, environmentverify.AssessmentCollection{
		Reference: completedReference(true),
	})
	run.Config.GenerationProvenance = &GenerationProvenanceConfig{
		Usage: &GenerationUsage{
			InputTokens: &zero, OutputTokens: &zero, CostMicros: &zero,
			Currency: "USD", CostStatus: CostStatusProviderReported,
			Source: "provider export",
		},
	}
	report := BuildReport(Meta{
		RunID:        "pqb-zero",
		StartedAt:    now,
		CompletedAt:  now.Add(time.Second),
		TestedCommit: "abc123",
		BaselineKind: "other",
		Timeout:      time.Second,
	}, []CaseRun{run})
	if report.Cases[0].Provenance.SavedExerciseUsage.Status != UsageStatusKnown {
		t.Fatalf("saved usage = %+v, want known measured zero", report.Cases[0].Provenance.SavedExerciseUsage)
	}

	data, err := MarshalReport(report)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	cases := decoded["cases"].([]any)
	provenance := cases[0].(map[string]any)["provenance"].(map[string]any)
	usage := provenance["saved_exercise_usage"].(map[string]any)
	if usage["status"] != UsageStatusKnown || usage["input_tokens"] != float64(0) ||
		usage["output_tokens"] != float64(0) || usage["cost_micros"] != float64(0) {
		t.Fatalf("encoded usage = %+v, want explicit measured zeros", usage)
	}
	validateReportSchema(t, report)
}

func TestBuildReportSavedUsageUnavailableAndFixtureNotApplicable(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	report := BuildReport(Meta{
		RunID:        "pqb-usage-status",
		StartedAt:    now,
		CompletedAt:  now.Add(time.Second),
		TestedCommit: "abc123",
		BaselineKind: "other",
		Timeout:      time.Second,
	}, []CaseRun{
		{Config: &RunConfig{}},
		{Config: &RunConfig{DevelopmentFixture: true}},
	})
	if report.Cases[0].Provenance.SavedExerciseUsage.Status != UsageStatusUnavailable {
		t.Fatalf("saved usage = %+v, want unavailable", report.Cases[0].Provenance.SavedExerciseUsage)
	}
	if report.Cases[1].Provenance.SavedExerciseUsage.Status != UsageStatusNotApplicable {
		t.Fatalf("fixture usage = %+v, want not_applicable", report.Cases[1].Provenance.SavedExerciseUsage)
	}
	for _, caseReport := range report.Cases {
		if caseReport.Provenance.AssessmentRunUsage.Status != UsageStatusNotApplicable {
			t.Fatalf("assessment usage = %+v, want not_applicable", caseReport.Provenance.AssessmentRunUsage)
		}
	}
	validateReportSchema(t, report)
}

func int64Value(value int64) *int64 {
	return &value
}

func validateReportSchema(t *testing.T, report Report) {
	t.Helper()
	schema := filepath.Join("..", "..", "..", "..", "benchmarks", "problem-quality", "schema", "report.schema.json")
	data, err := MarshalReport(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateReportJSON(data, schema); err != nil {
		t.Fatalf("report does not validate: %v", err)
	}
}
