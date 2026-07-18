package registry

import "testing"

func TestProxyRoutesForModelDefaultsToOpenRouter(t *testing.T) {
	routes := Global().ProxyRoutesForModel("claude-sonnet-4.6")
	if len(routes) != 2 {
		t.Fatalf("route count = %d, want 2", len(routes))
	}
	if routes[0].ProviderID != "openrouter" {
		t.Fatalf("primary provider = %q, want openrouter", routes[0].ProviderID)
	}
	if routes[0].ModelID != "anthropic/claude-sonnet-4.6" {
		t.Fatalf("primary upstream model = %q", routes[0].ModelID)
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
