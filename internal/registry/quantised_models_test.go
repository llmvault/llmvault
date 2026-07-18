package registry

import "testing"

func TestQuantisedCatalogIncludesOnlySupportedQuantisations(t *testing.T) {
	provider, ok := Global().GetProvider("quantised")
	if !ok {
		t.Fatal("quantised provider is missing")
	}

	want := map[string]string{
		"deepseek-v3.2":       "quantised-deepseek-v3.2",
		"deepseek-v4-pro":     "quantised-deepseek-v4-pro",
		"glm-4.7":             "quantised-glm-4.7",
		"glm-4.7-flash":       "quantised-glm-4.7-flash",
		"glm-5.1":             "quantised-glm-5.1",
		"glm-5.2":             "quantised-glm-5.2",
		"kimi-k2.5":           "quantised-kimi-k2.5",
		"kimi-k2.5-lightning": "quantised-kimi-k2.5-lightning",
		"kimi-k2.6":           "quantised-kimi-k2.6",
		"kimi-k2.7-code":      "quantised-kimi-k2.7-code",
		"mimo-v2.5-pro":       "quantised-mimo-v2.5-pro",
		"minimax-m2.5":        "quantised-minimax-m2.5",
		"qwen3.5-9b":          "quantised-qwen3.5-9b",
	}
	if len(provider.Models) != len(want) {
		t.Fatalf("model count = %d, want %d", len(provider.Models), len(want))
	}
	for upstreamID, canonicalID := range want {
		if _, ok := provider.Models[upstreamID]; !ok {
			t.Errorf("provider model %q is missing", upstreamID)
		}
		routes := Global().ProxyRoutesForModel(canonicalID)
		if len(routes) != 1 || routes[0] != (ModelRoute{ProviderID: "quantised", ModelID: upstreamID}) {
			t.Errorf("routes for %q = %#v", canonicalID, routes)
		}
	}

	for _, excluded := range []string{
		"deepseek-v4-flash",
		"deepseek-v4-pro-lightning",
		"gemma-4-31b-it",
		"glm-5",
		"greg-1-mini",
		"greg-2-super",
		"greg-2-ultra",
		"greg-rp",
		"qwen3.5-397b-a17b",
		"qwen3.6-27b",
	} {
		if _, ok := provider.Models[excluded]; ok {
			t.Errorf("excluded model %q is present", excluded)
		}
	}
}
