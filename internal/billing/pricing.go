package billing

import (
	"errors"
	"fmt"
	"math"

	"github.com/usehivy/hivy/internal/registry"
)

const (
	// CreditUSDValue is the customer-facing value of one credit for metered usage.
	CreditUSDValue = 0.001

	// CreditOverdraftFloor is the most negative balance an org may reach before
	// platform inference is cut off. Inference stays admitted until the balance
	// hits this floor; the batch debiter never drives it below.
	CreditOverdraftFloor int64 = -50

	CostSourceProvider = "provider_reported"
	CostSourceRegistry = "registry_estimated"

	// WebsitePagePriceCredits is the flat per-page charge for a website crawl.
	WebsitePagePriceCredits = 1
)

// ErrUnknownModel is returned when a generation has no provider-reported cost
// and the registry cannot estimate the model/provider route.
var ErrUnknownModel = errors.New("billing: unknown model")

var cachedTokenDiscount = map[string]float64{
	"anthropic":     0.10,
	"openai":        0.50,
	"google":        0.25,
	"google-vertex": 0.25,
}

const defaultCacheReadDiscount = 0.25

func CostUSDToCredits(cost float64) int64 {
	if cost <= 0 {
		return 0
	}
	credits := cost / CreditUSDValue
	// Registry estimates add several fractional token charges. Normalize
	// sub-nanocredit floating-point noise before rounding up so an exact
	// whole-credit charge such as $1.20 cannot become 1,201 credits.
	credits = math.Round(credits*1_000_000_000) / 1_000_000_000
	return int64(math.Ceil(credits))
}

func EstimateCostUSD(reg *registry.Registry, providerID, modelID string, inputTokens, outputTokens, cachedTokens int64) (float64, error) {
	if reg == nil {
		reg = registry.Global()
	}
	if inputTokens < 0 {
		inputTokens = 0
	}
	if outputTokens < 0 {
		outputTokens = 0
	}
	if cachedTokens < 0 {
		cachedTokens = 0
	}
	if inputTokens == 0 && outputTokens == 0 {
		return 0, nil
	}

	route, ok := reg.ResolveModel(providerID, modelID)
	if !ok || route.Model.Cost == nil {
		return 0, fmt.Errorf("%w: provider=%q model=%q", ErrUnknownModel, providerID, modelID)
	}

	if cachedTokens > inputTokens {
		cachedTokens = inputTokens
	}
	modelCost := costForInputTokens(*route.Model.Cost, inputTokens)
	nonCachedInput := inputTokens - cachedTokens
	inputCost := float64(nonCachedInput) * modelCost.Input / 1_000_000
	cacheReadPrice := modelCost.CacheRead
	if cacheReadPrice == 0 && cachedTokens > 0 {
		discount, ok := cachedTokenDiscount[providerID]
		if !ok {
			discount = defaultCacheReadDiscount
		}
		cacheReadPrice = modelCost.Input * discount
	}
	cachedCost := float64(cachedTokens) * cacheReadPrice / 1_000_000
	outputCost := float64(outputTokens) * modelCost.Output / 1_000_000
	return inputCost + cachedCost + outputCost, nil
}

func costForInputTokens(base registry.Cost, inputTokens int64) registry.Cost {
	selected := base
	for _, tier := range base.Tiers {
		if inputTokens < tier.MinContext {
			continue
		}
		selected.Input = tier.Input
		selected.Output = tier.Output
		selected.CacheRead = tier.CacheRead
		selected.CacheWrite = tier.CacheWrite
	}
	return selected
}

func IsKnownModel(model string) bool {
	_, ok := registry.Global().CanonicalModel(model)
	return ok
}
