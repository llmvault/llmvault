package billing_test

import (
	"math"
	"testing"

	"github.com/usehivy/hivy/internal/billing"
)

func TestCostUSDToCredits(t *testing.T) {
	floatingWholeCost := 0.4
	floatingWholeCost += 0.8

	for _, tc := range []struct {
		name string
		cost float64
		want int64
	}{
		{name: "zero", cost: 0, want: 0},
		{name: "negative", cost: -0.01, want: 0},
		{name: "sub credit", cost: 0.00084592, want: 1},
		{name: "exact credit", cost: 0.031, want: 31},
		{name: "ceil", cost: 0.03071, want: 31},
		{name: "floating point whole credit", cost: floatingWholeCost, want: 1200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := billing.CostUSDToCredits(tc.cost); got != tc.want {
				t.Fatalf("CostUSDToCredits(%f) = %d, want %d", tc.cost, got, tc.want)
			}
		})
	}
}

func TestEstimateCostUSD_UsesRegistryRouteAndCachedTokens(t *testing.T) {
	cost, err := billing.EstimateCostUSD(nil, "novita", "deepseek-v4-flash", 5_740, 79, 5_248)
	if err != nil {
		t.Fatalf("EstimateCostUSD: %v", err)
	}
	if cost <= 0 {
		t.Fatalf("estimated cost = %f, want positive", cost)
	}
}

func TestEstimateCostUSD_UsesRegistryCacheReadPrice(t *testing.T) {
	cost, err := billing.EstimateCostUSD(nil, "novita", "deepseek-v4-flash", 1000, 500, 800)
	if err != nil {
		t.Fatalf("EstimateCostUSD: %v", err)
	}

	expected := (200*0.14 + 800*0.028 + 500*0.28) / 1_000_000
	if math.Abs(cost-expected) > 0.000000001 {
		t.Fatalf("cost = %.12f, want %.12f", cost, expected)
	}
}

func TestEstimateCostUSD_IncidentStepFlashCacheRead(t *testing.T) {
	const input, cached, output = 15607827, 12788976, 39079
	cost, err := billing.EstimateCostUSD(nil, "novita", "step-3.7-flash", input, output, cached)
	if err != nil {
		t.Fatalf("EstimateCostUSD: %v", err)
	}

	nonCached := float64(input - cached)
	want := (nonCached*0.2 + float64(cached)*0.04 + float64(output)*1.15) / 1_000_000
	if math.Abs(cost-want) > 1e-9 {
		t.Fatalf("cost = %.12f, want %.12f", cost, want)
	}

	credits := billing.CostUSDToCredits(cost)
	if credits < 1100 || credits > 1130 {
		t.Fatalf("credits = %d, want ~1120 (was 3167 at full input price)", credits)
	}

	buggy := (float64(input)*0.2 + float64(output)*1.15) / 1_000_000
	if math.Abs(cost-buggy) < 1e-6 {
		t.Fatalf("cached tokens still billed at full input rate: %.12f", cost)
	}
}

func TestEstimateCostUSD_NovitaDeepseekV4ProCacheRead(t *testing.T) {
	cost, err := billing.EstimateCostUSD(nil, "novita", "deepseek-v4-pro", 52454, 105, 52224)
	if err != nil {
		t.Fatalf("EstimateCostUSD: %v", err)
	}
	want := (230*1.6 + 52224*0.135 + 105*3.2) / 1_000_000
	if math.Abs(cost-want) > 1e-12 {
		t.Fatalf("cost = %.12f, want %.12f", cost, want)
	}
}

func TestEstimateCostUSD_XiaomiMiMoCountsReasoningInCompletionOnce(t *testing.T) {
	// Live MiMo fixture: completion_tokens is the billed output total and
	// reasoning_tokens is only a breakdown within that total.
	const input, cached, completion = 269, 192, 128
	cost, err := billing.EstimateCostUSD(nil, "xiaomi", "mimo-v2.5-pro", input, completion, cached)
	if err != nil {
		t.Fatalf("EstimateCostUSD: %v", err)
	}

	want := (77*0.435 + 192*0.0036 + 128*0.87) / 1_000_000
	if math.Abs(cost-want) > 1e-12 {
		t.Fatalf("cost = %.12f, want %.12f", cost, want)
	}

	doubleBilledReasoning := want + 129*0.87/1_000_000
	if math.Abs(cost-doubleBilledReasoning) < 1e-12 {
		t.Fatalf("reasoning token breakdown was added to completion tokens: %.12f", cost)
	}
}

func TestEstimateCostUSD_AtlasCloudHy3UsesVerifiedCacheReadRate(t *testing.T) {
	// Sanitized daily ledger fixture: Atlas reported 6,988 fresh input,
	// 20,736 cache-read, 2,134 output, and $0.004142 after display rounding.
	const fresh, cached, completion = 6_988, 20_736, 2_134
	const input = fresh + cached
	cost, err := billing.EstimateCostUSD(nil, "atlascloud", "hy3", input, completion, cached)
	if err != nil {
		t.Fatalf("EstimateCostUSD: %v", err)
	}

	want := (fresh*0.2 + cached*0.05 + completion*0.8) / 1_000_000
	if math.Abs(cost-want) > 1e-12 {
		t.Fatalf("cost = %.12f, want %.12f", cost, want)
	}
	if math.Abs(want-0.0041416) > 1e-12 {
		t.Fatalf("fixture cost = %.12f, want Atlas ledger value 0.0041416", want)
	}
}

func TestEstimateCostUSD_AtlasCloudKatCoderUsesVerifiedCacheReadRate(t *testing.T) {
	const input, cached, completion = int64(1_000), int64(800), int64(500)
	cost, err := billing.EstimateCostUSD(
		nil,
		"atlascloud",
		"kat-coder-air-v2.5",
		input,
		completion,
		cached,
	)
	if err != nil {
		t.Fatalf("EstimateCostUSD: %v", err)
	}

	want := (200*0.15 + 800*0.03 + 500*0.6) / 1_000_000
	if math.Abs(cost-want) > 1e-12 {
		t.Fatalf("cost = %.12f, want %.12f", cost, want)
	}
}

func TestEstimateCostUSD_AtlasCloudLongCatUsesVerifiedCacheReadRate(t *testing.T) {
	// Live Atlas response reported reasoning tokens as a breakdown within
	// completion_tokens, so only the completion total enters billing.
	const input, cached, completion = int64(1_000), int64(800), int64(500)
	cost, err := billing.EstimateCostUSD(
		nil,
		"atlascloud",
		"longcat-2.0",
		input,
		completion,
		cached,
	)
	if err != nil {
		t.Fatalf("EstimateCostUSD: %v", err)
	}

	want := (200*0.75 + 800*0.015 + 500*3) / 1_000_000
	if math.Abs(cost-want) > 1e-12 {
		t.Fatalf("cost = %.12f, want %.12f", cost, want)
	}
}

func TestEstimateCostUSD_AtlasCloudUsesLongContextTier(t *testing.T) {
	const cached, output = int64(200_000), int64(1_000)

	baseInput := int64(271_999)
	baseCost, err := billing.EstimateCostUSD(nil, "atlascloud", "gpt-5.4", baseInput, output, cached)
	if err != nil {
		t.Fatalf("EstimateCostUSD base tier: %v", err)
	}
	baseWant := (float64(baseInput-cached)*2.5 + float64(cached)*0.25 + float64(output)*15) / 1_000_000
	if math.Abs(baseCost-baseWant) > 1e-12 {
		t.Fatalf("base cost = %.12f, want %.12f", baseCost, baseWant)
	}

	tierInput := int64(272_000)
	tierCost, err := billing.EstimateCostUSD(nil, "atlascloud", "gpt-5.4", tierInput, output, cached)
	if err != nil {
		t.Fatalf("EstimateCostUSD long-context tier: %v", err)
	}
	tierWant := (float64(tierInput-cached)*5 + float64(cached)*0.5 + float64(output)*22.5) / 1_000_000
	if math.Abs(tierCost-tierWant) > 1e-12 {
		t.Fatalf("tier cost = %.12f, want %.12f", tierCost, tierWant)
	}
}

func TestEstimateCostUSD_EngyUsesLiveDirectoryPricing(t *testing.T) {
	tests := []struct {
		model                               string
		input, output, cached               int64
		inputPrice, outputPrice, cachePrice float64
	}{
		{"engy-glm-5.2", 17, 32, 0, 0.68, 1.5, 0.18},
		{"engy-qwen3.6-35b-a3b", 15, 32, 0, 0.045, 0.3, 0.015},
	}

	for _, test := range tests {
		t.Run(test.model, func(t *testing.T) {
			cost, err := billing.EstimateCostUSD(
				nil,
				"engy",
				test.model,
				test.input,
				test.output,
				test.cached,
			)
			if err != nil {
				t.Fatalf("EstimateCostUSD: %v", err)
			}
			fresh := test.input - test.cached
			want := (float64(fresh)*test.inputPrice +
				float64(test.cached)*test.cachePrice +
				float64(test.output)*test.outputPrice) / 1_000_000
			if math.Abs(cost-want) > 1e-12 {
				t.Fatalf("cost = %.12f, want %.12f", cost, want)
			}
		})
	}
}
