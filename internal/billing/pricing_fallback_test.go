package billing_test

import (
	"errors"
	"math"
	"testing"

	"github.com/usehivy/hivy/internal/billing"
	"github.com/usehivy/hivy/internal/registry"
)

func TestEstimateCostUSD_NoCacheReadFallbackDiscount(t *testing.T) {
	const modelID = "gpt-5.3-codex"
	const in, out = 1.75, 14.0
	cost, err := billing.EstimateCostUSD(registry.Global(), "openai", modelID, 1000, 100, 800)
	if err != nil {
		t.Fatalf("EstimateCostUSD: %v", err)
	}
	want := (200*in + 800*in*0.50 + 100*out) / 1_000_000
	if math.Abs(cost-want) > 1e-9 {
		t.Fatalf("cost = %.12f, want %.12f (OpenAI 0.50 cache discount for %s)", cost, want, modelID)
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
	cost, err := billing.EstimateCostUSD(nil, "novita", "deepseek-v4-flash", 0, 0, 0)
	if err != nil {
		t.Fatalf("EstimateCostUSD: %v", err)
	}
	if cost != 0 {
		t.Fatalf("zero tokens cost = %f, want 0", cost)
	}
}

func TestEstimateCostUSD_TogetherUsesLiveDirectoryPricing(t *testing.T) {
	cost, err := billing.EstimateCostUSD(
		nil,
		"together",
		"nemotron-3-ultra-550b-a55b",
		1_000_000,
		1_000_000,
		800_000,
	)
	if err != nil {
		t.Fatalf("EstimateCostUSD: %v", err)
	}
	// 200K regular input at $0.60/M + 800K cached input at $0.20/M +
	// 1M output at $3.60/M.
	const want = 3.88
	if math.Abs(cost-want) > 1e-12 {
		t.Fatalf("cost = %.12f, want %.12f", cost, want)
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
