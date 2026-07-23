package registry

import "testing"

func TestEngyCatalogMatchesLiveDirectory(t *testing.T) {
	provider, ok := Global().GetProvider("engy")
	if !ok {
		t.Fatal("Engy provider is missing")
	}
	if provider.API != "https://api.engy.ai/v1" {
		t.Fatalf("API = %q", provider.API)
	}

	tests := []struct {
		upstreamID, canonicalID string
		input, output, cache    float64
		context, maxOutput      int64
	}{
		{"glm-5.2", "engy-glm-5.2", 0.68, 1.5, 0.18, 262144, 131072},
		{"qwen3.6-35b-a3b", "engy-qwen3.6-35b-a3b", 0.045, 0.3, 0.015, 262144, 65536},
	}
	if len(provider.Models) != len(tests) {
		t.Fatalf("model count = %d, want %d", len(provider.Models), len(tests))
	}

	for _, test := range tests {
		t.Run(test.upstreamID, func(t *testing.T) {
			model, ok := provider.Models[test.upstreamID]
			if !ok {
				t.Fatal("model is missing")
			}
			if model.Cost == nil ||
				model.Cost.Input != test.input ||
				model.Cost.Output != test.output ||
				model.Cost.CacheRead != test.cache {
				t.Fatalf("cost = %#v", model.Cost)
			}
			if model.Limit == nil ||
				model.Limit.Context != test.context ||
				model.Limit.Output != test.maxOutput {
				t.Fatalf("limit = %#v", model.Limit)
			}

			wantRoute := ModelRoute{ProviderID: "engy", ModelID: test.upstreamID}
			routes := Global().ProxyRoutesForModel(test.canonicalID)
			if len(routes) != 1 || routes[0] != wantRoute {
				t.Fatalf("routes = %#v, want %#v", routes, []ModelRoute{wantRoute})
			}
		})
	}
}
