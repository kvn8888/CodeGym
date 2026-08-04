package usage

import (
	"strings"
)

// Rates are USD per 1,000,000 tokens. Reviewed 2026-07-15 from public lists:
//
//   - Meta Muse Spark 1.1: $1.25 in / $4.25 out
//   - OpenAI/Azure GPT-5.6 Terra: $2.50 in / $15.00 out (standard short context)
//   - Gemini Flash-class (gemini-flash-latest): ~$0.30 in / $2.50 out

const (
	// MicrosPerUSD is 1e6 micro-dollars per USD for integer math.
	MicrosPerUSD int64 = 1_000_000
	// TokensPerMillion for rate conversion.
	TokensPerMillion int64 = 1_000_000
)

// PricingAsOf documents when the embedded rate card was last reviewed.
const PricingAsOf = "2026-07-15"

// Rate is USD per million tokens for a model family.
type Rate struct {
	Provider      string  `json:"provider"`
	ModelMatch    string  `json:"model_match"` // substring match; empty = provider default
	InputPerMTok  float64 `json:"input_usd_per_mtok"`
	OutputPerMTok float64 `json:"output_usd_per_mtok"`
	Source        string  `json:"source,omitempty"`
}

// DefaultRates is the built-in rate card (specific model matches first).
var DefaultRates = []Rate{
	{
		Provider:      "meta",
		ModelMatch:    "muse-spark",
		InputPerMTok:  1.25,
		OutputPerMTok: 4.25,
		Source:        "Meta Model API launch pricing",
	},
	{
		Provider:      "azure",
		ModelMatch:    "gpt-5.6-terra",
		InputPerMTok:  2.50,
		OutputPerMTok: 15.00,
		Source:        "OpenAI GPT-5.6 Terra / Azure OpenAI list",
	},
	{
		Provider:      "azure",
		ModelMatch:    "gpt-5.6-luna",
		InputPerMTok:  1.00,
		OutputPerMTok: 6.00,
		Source:        "OpenAI GPT-5.6 Luna list",
	},
	{
		Provider:      "azure",
		ModelMatch:    "gpt-5.6-sol",
		InputPerMTok:  5.00,
		OutputPerMTok: 30.00,
		Source:        "OpenAI GPT-5.6 Sol list",
	},
	{
		Provider:      "gemini",
		ModelMatch:    "gemini",
		InputPerMTok:  0.30,
		OutputPerMTok: 2.50,
		Source:        "Gemini 2.5 Flash-class public list (approx for gemini-flash-latest)",
	},
	{
		Provider:      "meta",
		ModelMatch:    "",
		InputPerMTok:  1.25,
		OutputPerMTok: 4.25,
		Source:        "Meta default",
	},
	{
		Provider:      "azure",
		ModelMatch:    "",
		InputPerMTok:  2.50,
		OutputPerMTok: 15.00,
		Source:        "Azure/OpenAI mid-tier default (Terra)",
	},
	{
		Provider:      "gemini",
		ModelMatch:    "",
		InputPerMTok:  0.30,
		OutputPerMTok: 2.50,
		Source:        "Gemini default",
	},
	{
		Provider:      "openai_compat",
		ModelMatch:    "",
		InputPerMTok:  1.00,
		OutputPerMTok: 5.00,
		Source:        "Unknown OpenAI-compat fallback",
	},
}

// LookupRate finds the best matching rate for provider+model.
func LookupRate(provider, model string) Rate {
	provider = strings.ToLower(strings.TrimSpace(provider))
	model = strings.ToLower(strings.TrimSpace(model))

	var providerDefault Rate
	hasProviderDefault := false

	for _, rate := range DefaultRates {
		if rate.Provider != provider {
			continue
		}
		if rate.ModelMatch == "" {
			providerDefault = rate
			hasProviderDefault = true
			continue
		}
		if model != "" && strings.Contains(model, strings.ToLower(rate.ModelMatch)) {
			return rate
		}
	}
	if hasProviderDefault {
		return providerDefault
	}

	// Model slug match across providers (e.g. provider still openai_compat).
	for _, rate := range DefaultRates {
		if rate.ModelMatch != "" && model != "" && strings.Contains(model, strings.ToLower(rate.ModelMatch)) {
			return rate
		}
	}

	return Rate{
		Provider:      provider,
		InputPerMTok:  1.00,
		OutputPerMTok: 5.00,
		Source:        "unknown default",
	}
}

// EstimateCostMicros returns cost in micro-USD for the given token counts.
func EstimateCostMicros(provider, model string, tokensIn, tokensOut int) int64 {
	rate := LookupRate(provider, model)
	normalizedIn := maxInt(0, tokensIn)
	normalizedOut := maxInt(0, tokensOut)
	in := float64(normalizedIn) * rate.InputPerMTok / float64(TokensPerMillion)
	out := float64(normalizedOut) * rate.OutputPerMTok / float64(TokensPerMillion)
	cost := int64((in+out)*float64(MicrosPerUSD) + 0.5)
	if cost == 0 && normalizedIn+normalizedOut > 0 {
		return 1
	}
	return cost
}

// MicrosToUSD converts micro-USD to a float dollars value for API responses.
func MicrosToUSD(micros int64) float64 {
	return float64(micros) / float64(MicrosPerUSD)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
