package middleware

import (
	"math"
	"testing"

	"github.com/usehivy/hivy/internal/observe"
	"github.com/usehivy/hivy/internal/registry"
)

func TestCalculateCost_CachedTokensPricedAtCacheRead(t *testing.T) {
	reg := registry.Global()
	usage := observe.UsageData{
		InputTokens:  15607827,
		CachedTokens: 12788976,
		OutputTokens: 39079,
	}

	got := calculateCost(reg, "openrouter", "step-3.7-flash", usage)

	nonCached := float64(usage.InputTokens - usage.CachedTokens)
	want := (nonCached*0.2 + float64(usage.CachedTokens)*0.04 + float64(usage.OutputTokens)*1.15) / 1_000_000
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("calculateCost = %.12f, want %.12f", got, want)
	}

	buggyFullPrice := (float64(usage.InputTokens)*0.2 + float64(usage.OutputTokens)*1.15) / 1_000_000
	if math.Abs(got-buggyFullPrice) < 1e-6 {
		t.Fatalf("calculateCost still prices cached tokens at full input rate: %.12f", got)
	}
}

func TestCalculateCost_ReasoningTokensDoNotDoubleBill(t *testing.T) {
	reg := registry.Global()
	base := observe.UsageData{InputTokens: 1000, CachedTokens: 400, OutputTokens: 500}
	withReasoning := base
	withReasoning.ReasoningTokens = 300

	if a, b := calculateCost(reg, "openrouter", "step-3.7-flash", base),
		calculateCost(reg, "openrouter", "step-3.7-flash", withReasoning); a != b {
		t.Fatalf("reasoning tokens changed cost: %.12f vs %.12f", a, b)
	}
}

func TestCalculateCost_UnknownModelZero(t *testing.T) {
	if got := calculateCost(registry.Global(), "openrouter", "does-not-exist", observe.UsageData{InputTokens: 10}); got != 0 {
		t.Fatalf("calculateCost(unknown) = %f, want 0", got)
	}
}
