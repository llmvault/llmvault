package billing_test

import (
	"math"
	"testing"

	"github.com/usehivy/hivy/internal/billing"
)

func TestEstimateCostUSD_NovitaUsesDirectoryPricing(t *testing.T) {
	// Sanitized live response: Novita reported 9 prompt and 32 completion
	// tokens for DeepSeek V4 Flash, but did not include a dollar-cost field.
	cost, err := billing.EstimateCostUSD(nil, "novita", "deepseek-v4-flash", 9, 32, 0)
	if err != nil {
		t.Fatalf("EstimateCostUSD: %v", err)
	}

	want := (9*0.14 + 32*0.28) / 1_000_000
	if math.Abs(cost-want) > 1e-12 {
		t.Fatalf("cost = %.12f, want %.12f", cost, want)
	}
	if math.Abs(want-0.00001022) > 1e-12 {
		t.Fatalf("fixture cost = %.12f, want 0.00001022", want)
	}
}

func TestEstimateCostUSD_NovitaUsesCacheReadPrice(t *testing.T) {
	const input, cached, output = int64(1000), int64(800), int64(500)
	cost, err := billing.EstimateCostUSD(nil, "novita", "deepseek-v4-flash", input, output, cached)
	if err != nil {
		t.Fatalf("EstimateCostUSD: %v", err)
	}

	want := (200*0.14 + 800*0.028 + 500*0.28) / 1_000_000
	if math.Abs(cost-want) > 1e-12 {
		t.Fatalf("cost = %.12f, want %.12f", cost, want)
	}
}

func TestEstimateCostUSD_NovitaReasoningIsNotDoubleBilled(t *testing.T) {
	// The live response reported reasoning_tokens=32 as a breakdown of the
	// same 32 completion_tokens. Only completion_tokens enters estimation.
	const input, completion, reasoning = int64(9), int64(32), int64(32)
	cost, err := billing.EstimateCostUSD(nil, "novita", "deepseek-v4-flash", input, completion, 0)
	if err != nil {
		t.Fatalf("EstimateCostUSD: %v", err)
	}

	want := (float64(input)*0.14 + float64(completion)*0.28) / 1_000_000
	doubleBilled := want + float64(reasoning)*0.28/1_000_000
	if math.Abs(cost-want) > 1e-12 {
		t.Fatalf("cost = %.12f, want %.12f", cost, want)
	}
	if math.Abs(cost-doubleBilled) < 1e-12 {
		t.Fatalf("reasoning tokens were billed twice: %.12f", cost)
	}
}

func TestEstimateCostUSD_NovitaLingIsTemporarilyFree(t *testing.T) {
	cost, err := billing.EstimateCostUSD(nil, "novita", "ling-3.0-flash", 27, 36, 0)
	if err != nil {
		t.Fatalf("EstimateCostUSD: %v", err)
	}
	if cost != 0 {
		t.Fatalf("cost = %.12f, want zero", cost)
	}
}

func TestEstimateCostUSD_NovitaUsesLongContextTier(t *testing.T) {
	const cached, output = int64(500_000), int64(1_000)

	baseInput := int64(524_287)
	baseCost, err := billing.EstimateCostUSD(nil, "novita", "minimax-m3", baseInput, output, cached)
	if err != nil {
		t.Fatalf("EstimateCostUSD base tier: %v", err)
	}
	baseWant := (float64(baseInput-cached)*0.3 + float64(cached)*0.06 + float64(output)*1.2) / 1_000_000
	if math.Abs(baseCost-baseWant) > 1e-12 {
		t.Fatalf("base cost = %.12f, want %.12f", baseCost, baseWant)
	}

	tierInput := int64(524_288)
	tierCost, err := billing.EstimateCostUSD(nil, "novita", "minimax-m3", tierInput, output, cached)
	if err != nil {
		t.Fatalf("EstimateCostUSD long-context tier: %v", err)
	}
	tierWant := (float64(tierInput-cached)*0.6 + float64(cached)*0.12 + float64(output)*2.4) / 1_000_000
	if math.Abs(tierCost-tierWant) > 1e-12 {
		t.Fatalf("tier cost = %.12f, want %.12f", tierCost, tierWant)
	}
}
