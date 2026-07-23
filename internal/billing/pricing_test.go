package billing_test

import (
	"errors"
	"math"
	"testing"

	"github.com/usehivy/hivy/internal/billing"
	"github.com/usehivy/hivy/internal/registry"
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
	cost, err := billing.EstimateCostUSD(nil, "openrouter", "deepseek-v4-flash", 5_740, 79, 5_248)
	if err != nil {
		t.Fatalf("EstimateCostUSD: %v", err)
	}
	if cost <= 0 {
		t.Fatalf("estimated cost = %f, want positive", cost)
	}
}

func TestEstimateCostUSD_UsesRegistryCacheReadPrice(t *testing.T) {
	cost, err := billing.EstimateCostUSD(nil, "openrouter", "deepseek-v4-flash", 1000, 500, 800)
	if err != nil {
		t.Fatalf("EstimateCostUSD: %v", err)
	}

	expected := (200*0.0983 + 800*0.0197 + 500*0.1966) / 1_000_000
	if math.Abs(cost-expected) > 0.000000001 {
		t.Fatalf("cost = %.12f, want %.12f", cost, expected)
	}
}

func TestEstimateCostUSD_IncidentStepFlashCacheRead(t *testing.T) {
	const input, cached, output = 15607827, 12788976, 39079
	cost, err := billing.EstimateCostUSD(nil, "openrouter", "step-3.7-flash", input, output, cached)
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

func TestEstimateCostUSD_DeepseekV4ProVerifiedCacheRead(t *testing.T) {
	cost, err := billing.EstimateCostUSD(nil, "openrouter", "deepseek-v4-pro", 52454, 105, 52224)
	if err != nil {
		t.Fatalf("EstimateCostUSD: %v", err)
	}
	want := (230*0.435 + 52224*0.003625 + 105*0.87) / 1_000_000
	if math.Abs(cost-want) > 1e-12 {
		t.Fatalf("cost = %.12f, want %.12f", cost, want)
	}
	if math.Abs(want-0.000380712) > 1e-9 {
		t.Fatalf("fixture drifted from provider-verified charge: %.9f", want)
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

func TestEstimateCostUSD_NoCacheReadFallbackDiscount(t *testing.T) {
	reg := registry.Global()
	prov, ok := reg.GetProvider("openrouter")
	if !ok {
		t.Skip("openrouter provider not in registry")
	}
	var modelID string
	var in, out float64
	for _, m := range prov.Models {
		if m.Cost == nil || m.Cost.CacheRead != 0 || m.Cost.Input <= 0 {
			continue
		}
		if _, ok := reg.ResolveModel("openrouter", m.ID); !ok {
			continue
		}
		modelID, in, out = m.ID, m.Cost.Input, m.Cost.Output
		break
	}
	if modelID == "" {
		t.Skip("no registry model without CacheRead to exercise fallback")
	}

	cost, err := billing.EstimateCostUSD(reg, "openrouter", modelID, 1000, 100, 800)
	if err != nil {
		t.Fatalf("EstimateCostUSD: %v", err)
	}
	want := (200*in + 800*in*0.25 + 100*out) / 1_000_000
	if math.Abs(cost-want) > 1e-9 {
		t.Fatalf("cost = %.12f, want %.12f (0.25 fallback discount for %s)", cost, want, modelID)
	}
	full := (1000*in + 100*out) / 1_000_000
	if math.Abs(cost-full) < 1e-12 {
		t.Fatalf("cached tokens billed at full input price without CacheRead: %.12f", cost)
	}
}

func TestEstimateCostUSD_UnknownModel(t *testing.T) {
	_, err := billing.EstimateCostUSD(nil, "openrouter", "claude-3-nonexistent", 1000, 100, 0)
	if !errors.Is(err, billing.ErrUnknownModel) {
		t.Fatalf("expected ErrUnknownModel, got %v", err)
	}
}

func TestEstimateCostUSD_ZeroTokensZeroCost(t *testing.T) {
	cost, err := billing.EstimateCostUSD(nil, "openrouter", "deepseek-v4-flash", 0, 0, 0)
	if err != nil {
		t.Fatalf("EstimateCostUSD: %v", err)
	}
	if cost != 0 {
		t.Fatalf("zero tokens cost = %f, want 0", cost)
	}
}

func TestIsKnownModel(t *testing.T) {
	if !billing.IsKnownModel("deepseek-v4-flash") {
		t.Error("IsKnownModel(deepseek-v4-flash) = false, want true")
	}
	if billing.IsKnownModel("claude-3-nonexistent") {
		t.Error("IsKnownModel(unknown) = true, want false")
	}
}
