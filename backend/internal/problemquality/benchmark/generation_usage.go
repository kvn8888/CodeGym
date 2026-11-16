package benchmark

import (
	"errors"
	"fmt"
	"strings"
)

const (
	UsageStatusKnown         = "known"
	UsageStatusUnavailable   = "unavailable"
	UsageStatusNotApplicable = "not_applicable"

	CostStatusProviderReported = "provider_reported"
	CostStatusEstimated        = "estimated"

	UsageCurrencyUSD = "USD"
)

// GenerationUsage is optional historical usage supplied with saved generation
// provenance. Nil token or cost pointers mean the value is unknown; a pointer
// to zero means the value was measured as zero. It is imported evidence only
// and is never written to the benchmark budget ledger.
type GenerationUsage struct {
	InputTokens  *int64 `json:"input_tokens,omitempty"`
	OutputTokens *int64 `json:"output_tokens,omitempty"`
	CostMicros   *int64 `json:"cost_micros,omitempty"`
	Currency     string `json:"currency,omitempty"`
	CostStatus   string `json:"cost_status,omitempty"`
	Source       string `json:"source,omitempty"`
	PricingBasis string `json:"pricing_basis,omitempty"`
}

// UsageFact is a reportable usage value. Status distinguishes unavailable
// historical data and not-applicable assessment usage from measured zero.
type UsageFact struct {
	Status string `json:"status"`
	GenerationUsage
}

func validateGenerationUsage(usage *GenerationUsage) error {
	if usage == nil {
		return nil
	}
	if strings.TrimSpace(usage.Source) == "" {
		return errors.New("source is required for a usage record")
	}
	usage.Source = strings.TrimSpace(usage.Source)
	if usage.InputTokens != nil && *usage.InputTokens < 0 {
		return errors.New("input_tokens must be non-negative")
	}
	if usage.OutputTokens != nil && *usage.OutputTokens < 0 {
		return errors.New("output_tokens must be non-negative")
	}
	if usage.CostMicros != nil && *usage.CostMicros < 0 {
		return errors.New("cost_micros must be non-negative")
	}
	if usage.InputTokens == nil && usage.OutputTokens == nil && usage.CostMicros == nil {
		return errors.New("a usage record must include at least one measured value")
	}
	if usage.CostMicros == nil {
		if strings.TrimSpace(usage.Currency) != "" {
			return errors.New("currency requires cost_micros")
		}
		if strings.TrimSpace(usage.CostStatus) != "" {
			return errors.New("cost_status requires cost_micros")
		}
		if strings.TrimSpace(usage.PricingBasis) != "" {
			return errors.New("pricing_basis requires cost_micros")
		}
		return nil
	}

	currency := strings.ToUpper(strings.TrimSpace(usage.Currency))
	if currency != UsageCurrencyUSD {
		return fmt.Errorf("currency %q is unsupported; only %s is accepted", usage.Currency, UsageCurrencyUSD)
	}
	usage.Currency = currency
	costStatus := strings.ToLower(strings.TrimSpace(usage.CostStatus))
	switch costStatus {
	case CostStatusProviderReported:
		if strings.TrimSpace(usage.PricingBasis) != "" {
			return errors.New("pricing_basis must be omitted when cost_status is provider_reported")
		}
	case CostStatusEstimated:
		if strings.TrimSpace(usage.PricingBasis) == "" {
			return errors.New("pricing_basis is required when cost_status is estimated")
		}
		if usage.InputTokens == nil || usage.OutputTokens == nil {
			return errors.New("estimated cost requires both input_tokens and output_tokens")
		}
	default:
		return fmt.Errorf("cost_status %q must be %s or %s",
			usage.CostStatus, CostStatusProviderReported, CostStatusEstimated)
	}
	usage.CostStatus = costStatus
	usage.PricingBasis = strings.TrimSpace(usage.PricingBasis)
	return nil
}

func savedExerciseUsageFact(config *RunConfig, fixture bool) UsageFact {
	if config != nil && config.GenerationProvenance != nil && config.GenerationProvenance.Usage != nil {
		return UsageFact{
			Status:          UsageStatusKnown,
			GenerationUsage: *config.GenerationProvenance.Usage,
		}
	}
	if fixture {
		return UsageFact{Status: UsageStatusNotApplicable}
	}
	return UsageFact{Status: UsageStatusUnavailable}
}

func assessmentRunUsageFact() UsageFact {
	return UsageFact{Status: UsageStatusNotApplicable}
}

func hasKnownSavedExerciseUsage(cases []CaseReport) bool {
	for _, caseReport := range cases {
		if caseReport.Provenance != nil && caseReport.Provenance.SavedExerciseUsage.Status == UsageStatusKnown {
			return true
		}
	}
	return false
}
