package usage

import "testing"

func TestLookupRateMetaMuse(t *testing.T) {
	rate := LookupRate("meta", "muse-spark-1.1")
	if rate.InputPerMTok != 1.25 || rate.OutputPerMTok != 4.25 {
		t.Fatalf("meta rate = %+v", rate)
	}
}

func TestLookupRateAzureTerra(t *testing.T) {
	rate := LookupRate("azure", "gpt-5.6-terra-2026-07-09")
	if rate.InputPerMTok != 2.50 || rate.OutputPerMTok != 15.00 {
		t.Fatalf("azure terra rate = %+v", rate)
	}
}

func TestEstimateCostMicros(t *testing.T) {
	// 1M in + 1M out on meta = $1.25 + $4.25 = $5.50
	micros := EstimateCostMicros("meta", "muse-spark-1.1", 1_000_000, 1_000_000)
	if micros != 5_500_000 {
		t.Fatalf("micros = %d, want 5500000", micros)
	}
	if MicrosToUSD(micros) != 5.5 {
		t.Fatalf("usd = %v", MicrosToUSD(micros))
	}
}

func TestServiceAggregate(t *testing.T) {
	svc := NewService(NewInMemoryStore(), nil)
	ctx := testUsageContext(t)

	if _, err := svc.Record(ctx, RecordInput{
		Provider: "azure", Model: "gpt-5.6-terra", Kind: "mcq",
		TokensIn: 1000, TokensOut: 500,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Record(ctx, RecordInput{
		Provider: "meta", Model: "muse-spark-1.1", Kind: "mcq",
		TokensIn: 2000, TokensOut: 1000,
	}); err != nil {
		t.Fatal(err)
	}

	agg, err := svc.Aggregate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if agg.CallCount != 2 {
		t.Fatalf("calls = %d", agg.CallCount)
	}
	if agg.TotalTokensIn != 3000 || agg.TotalTokensOut != 1500 {
		t.Fatalf("tokens = %d/%d", agg.TotalTokensIn, agg.TotalTokensOut)
	}
	if agg.TotalCostUSD <= 0 {
		t.Fatalf("cost = %v", agg.TotalCostUSD)
	}
	if len(agg.ByProvider) != 2 {
		t.Fatalf("by_provider = %#v", agg.ByProvider)
	}
}
