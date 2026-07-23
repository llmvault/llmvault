package registry

import "testing"

func TestProxyRoutesForAtlasModelPreferAtlasWithOpenRouterFallback(t *testing.T) {
	routes := Global().ProxyRoutesForModel("claude-sonnet-4.6")
	if len(routes) != 3 {
		t.Fatalf("route count = %d, want 3", len(routes))
	}
	if routes[0] != (ModelRoute{ProviderID: "atlascloud", ModelID: "anthropic/claude-sonnet-4.6"}) {
		t.Fatalf("primary route = %#v, want Atlas Cloud", routes[0])
	}
	if routes[1] != (ModelRoute{ProviderID: "openrouter", ModelID: "anthropic/claude-sonnet-4.6"}) {
		t.Fatalf("fallback route = %#v, want OpenRouter", routes[1])
	}
}

func TestProxyRoutesForModelRetainsOnlyAvailableDirectRoute(t *testing.T) {
	routes := Global().ProxyRoutesForModel("scribe-v2")
	if len(routes) != 1 || routes[0].ProviderID != "elevenlabs" {
		t.Fatalf("routes = %#v, want elevenlabs-only route", routes)
	}
}

func TestProxyRoutesForMiMoPreferXiaomiWithOpenRouterFallback(t *testing.T) {
	for _, modelID := range []string{"mimo-v2.5-pro", "mimo-v2.5"} {
		t.Run(modelID, func(t *testing.T) {
			routes := Global().ProxyRoutesForModel(modelID)
			if len(routes) != 2 {
				t.Fatalf("route count = %d, want 2", len(routes))
			}
			if routes[0] != (ModelRoute{ProviderID: "xiaomi", ModelID: modelID}) {
				t.Fatalf("primary route = %#v, want Xiaomi MiMo", routes[0])
			}
			if routes[1] != (ModelRoute{ProviderID: "openrouter", ModelID: "xiaomi/" + modelID}) {
				t.Fatalf("fallback route = %#v, want OpenRouter", routes[1])
			}
		})
	}
}

func TestProxyRoutesForMiMoUltraspeedUsesXiaomi(t *testing.T) {
	routes := Global().ProxyRoutesForModel("mimo-v2.5-pro-ultraspeed")
	if len(routes) != 1 {
		t.Fatalf("route count = %d, want 1", len(routes))
	}
	if routes[0] != (ModelRoute{ProviderID: "xiaomi", ModelID: "mimo-v2.5-pro-ultraspeed"}) {
		t.Fatalf("route = %#v, want Xiaomi MiMo Ultraspeed", routes[0])
	}
}

func TestProxyRoutesForMiMoCarryDirectProviderPricing(t *testing.T) {
	tests := []struct {
		modelID   string
		input     float64
		output    float64
		cacheRead float64
	}{
		{modelID: "mimo-v2.5-pro-ultraspeed", input: 1.305, output: 2.61, cacheRead: 0.0108},
		{modelID: "mimo-v2.5-pro", input: 0.435, output: 0.87, cacheRead: 0.0036},
		{modelID: "mimo-v2.5", input: 0.14, output: 0.28, cacheRead: 0.0028},
	}

	for _, test := range tests {
		t.Run(test.modelID, func(t *testing.T) {
			route, ok := Global().ResolveModel("xiaomi", test.modelID)
			if !ok {
				t.Fatalf("ResolveModel(xiaomi, %s) failed", test.modelID)
			}
			if route.Model.Cost == nil {
				t.Fatalf("Xiaomi %s has no cost metadata", test.modelID)
			}
			if route.Model.Cost.Input != test.input ||
				route.Model.Cost.Output != test.output ||
				route.Model.Cost.CacheRead != test.cacheRead {
				t.Fatalf("cost = %#v, want input=%v output=%v cache_read=%v",
					route.Model.Cost, test.input, test.output, test.cacheRead)
			}
		})
	}
}

func TestProxyRoutesForHy3PreferAtlasCloudWithOpenRouterFallback(t *testing.T) {
	routes := Global().ProxyRoutesForModel("hy3")
	if len(routes) != 2 {
		t.Fatalf("route count = %d, want 2", len(routes))
	}
	if routes[0] != (ModelRoute{ProviderID: "atlascloud", ModelID: "tencent/hy3"}) {
		t.Fatalf("primary route = %#v, want Atlas Cloud", routes[0])
	}
	if routes[1] != (ModelRoute{ProviderID: "openrouter", ModelID: "tencent/hy3"}) {
		t.Fatalf("fallback route = %#v, want OpenRouter", routes[1])
	}
}

func TestAtlasCloudHy3CatalogAndPricing(t *testing.T) {
	provider, ok := Global().GetProvider("atlascloud")
	if !ok {
		t.Fatal("Atlas Cloud provider not found")
	}
	if provider.API != "https://api.atlascloud.ai/v1" {
		t.Fatalf("Atlas Cloud API = %q", provider.API)
	}

	route, ok := Global().ResolveModel("atlascloud", "hy3")
	if !ok {
		t.Fatal("Atlas Cloud hy3 route not found")
	}
	if route.UpstreamID != "tencent/hy3" {
		t.Fatalf("upstream = %q, want tencent/hy3", route.UpstreamID)
	}
	if route.Model.Cost == nil ||
		route.Model.Cost.Input != 0.2 ||
		route.Model.Cost.Output != 0.8 ||
		route.Model.Cost.CacheRead != 0.05 {
		t.Fatalf("cost = %#v, want input=0.2 output=0.8 cache_read=0.05", route.Model.Cost)
	}
}
