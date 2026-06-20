package registry

import "testing"

func TestCrofCatalogRoutes(t *testing.T) {
	reg := Global()

	provider, ok := reg.GetProvider("crof")
	if !ok {
		t.Fatal("crof provider not found")
	}
	if provider.API != "https://crof.ai/v1" {
		t.Fatalf("crof API = %q, want https://crof.ai/v1", provider.API)
	}

	tests := []struct {
		canonicalID string
		upstreamID  string
		input       float64
		output      float64
		cacheRead   float64
		context     int64
		outputLimit int64
	}{
		{
			canonicalID: "crof-deepseek-v4-pro",
			upstreamID:  "deepseek-v4-pro",
			input:       0.35,
			output:      0.8,
			cacheRead:   0.003,
			context:     1000000,
			outputLimit: 131072,
		},
		{
			canonicalID: "crof-deepseek-v4-flash",
			upstreamID:  "deepseek-v4-flash",
			input:       0.12,
			output:      0.21,
			cacheRead:   0.003,
			context:     1000000,
			outputLimit: 131072,
		},
		{
			canonicalID: "crof-mimo-v2.5-pro",
			upstreamID:  "mimo-v2.5-pro",
			input:       0.4,
			output:      0.8,
			cacheRead:   0.003,
			context:     1000000,
			outputLimit: 131072,
		},
		{
			canonicalID: "crof-glm-5.2",
			upstreamID:  "glm-5.2",
			input:       0.5,
			output:      2.2,
			cacheRead:   0.08,
			context:     1000000,
			outputLimit: 131072,
		},
		{
			canonicalID: "crof-kimi-k2.7-code",
			upstreamID:  "kimi-k2.7-code",
			input:       0.55,
			output:      2.25,
			cacheRead:   0.05,
			context:     262144,
			outputLimit: 262144,
		},
	}

	for _, tt := range tests {
		t.Run(tt.canonicalID, func(t *testing.T) {
			route, ok := reg.ResolveModel("crof", tt.canonicalID)
			if !ok {
				t.Fatal("crof route not found")
			}
			if route.UpstreamID != tt.upstreamID {
				t.Fatalf("upstream = %q, want %q", route.UpstreamID, tt.upstreamID)
			}
			if route.Model.ID != tt.canonicalID {
				t.Fatalf("canonical model id = %q, want %q", route.Model.ID, tt.canonicalID)
			}
			if route.Model.Cost == nil || route.Model.Cost.Input != tt.input || route.Model.Cost.Output != tt.output || route.Model.Cost.CacheRead != tt.cacheRead {
				t.Fatalf("cost = %#v", route.Model.Cost)
			}
			if route.Model.Limit == nil || route.Model.Limit.Context != tt.context || route.Model.Limit.Output != tt.outputLimit {
				t.Fatalf("limit = %#v", route.Model.Limit)
			}
			if !route.Model.Reasoning || !route.Model.ToolCall || !route.Model.OpenWeights {
				t.Fatalf("capabilities = reasoning:%v tool_call:%v open_weights:%v", route.Model.Reasoning, route.Model.ToolCall, route.Model.OpenWeights)
			}

			providers := reg.ProviderPreferenceForModel(tt.canonicalID)
			if len(providers) != 1 || providers[0] != "crof" {
				t.Fatalf("provider preference = %v, want [crof]", providers)
			}
		})
	}
}

func TestCrofCanonicalModelsAreProviderPrefixed(t *testing.T) {
	models := Global().CanonicalModelsForProviders([]string{"crof"})
	seen := map[string]bool{}
	for _, model := range models {
		seen[model.ID] = true
	}

	for _, id := range []string{
		"crof-deepseek-v4-pro",
		"crof-deepseek-v4-flash",
		"crof-mimo-v2.5-pro",
		"crof-glm-5.2",
		"crof-kimi-k2.7-code",
	} {
		if !seen[id] {
			t.Fatalf("missing Crof canonical model %q in provider model list", id)
		}
	}

	for _, unprefixed := range []string{
		"deepseek-v4-pro",
		"deepseek-v4-flash",
		"mimo-v2.5-pro",
		"glm-5.2",
		"kimi-k2.7-code",
	} {
		if seen[unprefixed] {
			t.Fatalf("unprefixed model %q appeared in Crof provider model list", unprefixed)
		}
	}
}
