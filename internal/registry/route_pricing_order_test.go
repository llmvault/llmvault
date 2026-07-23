package registry

import (
	"slices"
	"testing"
)

func TestCatalogRoutesCheapestProviderFirst(t *testing.T) {
	registry := Global()

	for _, catalogModel := range supportedHivyModels {
		if routePricingOrderExcluded(catalogModel.Routes) ||
			len(catalogModel.Routes) < 2 {
			continue
		}

		t.Run(catalogModel.ID, func(t *testing.T) {
			if !slices.Equal(catalogModel.Routes, catalogModel.ProxyRoutes) {
				t.Fatalf("Routes and ProxyRoutes must use the same price order: routes=%#v proxy_routes=%#v",
					catalogModel.Routes, catalogModel.ProxyRoutes)
			}

			firstRoute := catalogModel.Routes[0]
			firstModel, ok := registry.providerModel(firstRoute.ProviderID, firstRoute.ModelID)
			if !ok || firstModel.Cost == nil {
				t.Fatalf("first route %s/%s has no pricing metadata", firstRoute.ProviderID, firstRoute.ModelID)
			}

			for _, candidateRoute := range catalogModel.Routes[1:] {
				candidateModel, ok := registry.providerModel(candidateRoute.ProviderID, candidateRoute.ModelID)
				if !ok || candidateModel.Cost == nil {
					t.Fatalf("candidate route %s/%s has no pricing metadata", candidateRoute.ProviderID, candidateRoute.ModelID)
				}
				if routeCostLess(candidateModel.Cost, firstModel.Cost) {
					t.Errorf(
						"%s route costs input=$%.6f output=$%.6f cache_read=$%.6f cache_write=$%.6f; "+
							"it must precede %s at input=$%.6f output=$%.6f cache_read=$%.6f cache_write=$%.6f",
						candidateRoute.ProviderID,
						candidateModel.Cost.Input,
						candidateModel.Cost.Output,
						candidateModel.Cost.CacheRead,
						candidateModel.Cost.CacheWrite,
						firstRoute.ProviderID,
						firstModel.Cost.Input,
						firstModel.Cost.Output,
						firstModel.Cost.CacheRead,
						firstModel.Cost.CacheWrite,
					)
				}
			}
		})
	}
}

func routePricingOrderExcluded(routes []ModelRoute) bool {
	for _, route := range routes {
		switch route.ProviderID {
		case "quantised", "engy":
			return true
		}
	}
	return false
}

// routeCostLess compares the base cost of a representative uncached request:
// one million input tokens plus one million output tokens. A zero cache price
// is ambiguous in the curated source data (some direct providers omit cache
// metadata), so cache pricing must not break an otherwise equal base-price tie.
func routeCostLess(candidate, current *Cost) bool {
	const epsilon = 1e-12

	candidateTotal := candidate.Input + candidate.Output
	currentTotal := current.Input + current.Output
	return currentTotal-candidateTotal > epsilon
}
