package registry

import "testing"

func TestTheseanCatalogMatchesLiveDirectory(t *testing.T) {
	provider, ok := Global().GetProvider("thesean")
	if !ok {
		t.Fatal("Thesean provider is missing")
	}
	if provider.API != "https://api.thesean.ai/v1" {
		t.Fatalf("API = %q", provider.API)
	}

	tests := []struct {
		upstreamID, canonicalID, name        string
		input, output, cacheRead, cacheWrite float64
		context, maxOutput                   int64
	}{
		{"ship-like/claude-haiku-4-5", "thesean-claude-haiku-4.5", "Ship like Claude Haiku 4.5", 0.5, 2.5, 0.05, 0.625, 200000, 64000},
		{"ship-like/claude-opus-4-8", "thesean-claude-opus-4.8", "Ship like Claude Opus 4.8", 2.5, 12.5, 0.25, 3.125, 1000000, 128000},
		{"ship-like/claude-sonnet-5", "thesean-claude-sonnet-5", "Ship like Claude Sonnet 5", 1, 5, 0.1, 1.25, 1000000, 128000},
		{"ship-like/gpt-5.6-sol", "thesean-gpt-5.6-sol", "Ship like GPT 5.6 Sol", 2.5, 15, 0.25, 3.125, 1050000, 128000},
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
			if model.Name != test.name {
				t.Fatalf("name = %q, want %q", model.Name, test.name)
			}
			if model.Cost == nil ||
				model.Cost.Input != test.input ||
				model.Cost.Output != test.output ||
				model.Cost.CacheRead != test.cacheRead ||
				model.Cost.CacheWrite != test.cacheWrite {
				t.Fatalf("cost = %#v", model.Cost)
			}
			if model.Limit == nil ||
				model.Limit.Context != test.context ||
				model.Limit.Output != test.maxOutput {
				t.Fatalf("limit = %#v", model.Limit)
			}

			wantRoute := ModelRoute{ProviderID: "thesean", ModelID: test.upstreamID}
			routes := Global().ProxyRoutesForModel(test.canonicalID)
			if len(routes) != 1 || routes[0] != wantRoute {
				t.Fatalf("routes = %#v, want %#v", routes, []ModelRoute{wantRoute})
			}
		})
	}
}
