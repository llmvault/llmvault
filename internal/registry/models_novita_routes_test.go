package registry

import (
	"slices"
	"testing"
)

func TestNovitaTieredPricing(t *testing.T) {
	provider, ok := Global().GetProvider("novita")
	if !ok {
		t.Fatal("Novita provider not found")
	}

	minimax := provider.Models["minimax/minimax-m3"]
	if minimax.Cost == nil || len(minimax.Cost.Tiers) != 1 {
		t.Fatalf("MiniMax M3 cost = %#v", minimax.Cost)
	}
	if minimax.Cost.Input != 0.3 || minimax.Cost.Output != 1.2 || minimax.Cost.CacheRead != 0.06 {
		t.Fatalf("MiniMax M3 base cost = %#v", minimax.Cost)
	}
	if got, want := minimax.Cost.Tiers[0], (CostTier{
		MinContext: 524288,
		Input:      0.6,
		Output:     2.4,
		CacheRead:  0.12,
	}); got != want {
		t.Fatalf("MiniMax M3 tier = %#v, want %#v", got, want)
	}

	qwen := provider.Models["qwen/qwen3-max"]
	if qwen.Cost == nil || len(qwen.Cost.Tiers) != 2 {
		t.Fatalf("Qwen3 Max cost = %#v", qwen.Cost)
	}
	if qwen.Cost.Input != 0.845 || qwen.Cost.Output != 3.38 {
		t.Fatalf("Qwen3 Max base cost = %#v", qwen.Cost)
	}
	if qwen.Cost.Tiers[0].MinContext != 32768 ||
		qwen.Cost.Tiers[0].Input != 1.4 ||
		qwen.Cost.Tiers[0].Output != 5.64 ||
		qwen.Cost.Tiers[1].MinContext != 131072 ||
		qwen.Cost.Tiers[1].Input != 2.11 ||
		qwen.Cost.Tiers[1].Output != 8.45 {
		t.Fatalf("Qwen3 Max tiers = %#v", qwen.Cost.Tiers)
	}
}

func TestNovitaRoutesAreDeclaredExplicitly(t *testing.T) {
	tests := []struct {
		canonicalID   string
		novitaModelID string
		primary       string
	}{
		{"deepseek-v4-flash", "deepseek/deepseek-v4-flash", "deepseek"},
		{"deepseek-v4-pro", "deepseek/deepseek-v4-pro", "deepseek"},
		{"glm-4.7", "zai-org/glm-4.7", "atlascloud"},
		{"glm-4.7-flash", "zai-org/glm-4.7-flash", "novita"},
		{"glm-5", "zai-org/glm-5", "atlascloud"},
		{"glm-5-turbo", "zai-org/glm-5-turbo", "atlascloud"},
		{"glm-5.1", "zai-org/glm-5.1", "atlascloud"},
		{"glm-5.2", "zai-org/glm-5.2", "novita"},
		{"hy3", "tencent/hy3", "novita"},
		{"kimi-k2.5", "moonshotai/kimi-k2.5", "atlascloud"},
		{"kimi-k2.6", "moonshotai/kimi-k2.6", "novita"},
		{"kimi-k2.7-code", "moonshotai/kimi-k2.7-code", "atlascloud"},
		{"kimi-k3", "moonshotai/kimi-k3", "novita"},
		{"ling-2.6-1t", "inclusionai/ling-2.6-1t", "novita"},
		{"ling-3.0-flash", "inclusionai/ling-3.0-flash", "novita"},
		{"mimo-v2.5", "xiaomimimo/mimo-v2.5", "xiaomi"},
		{"mimo-v2.5-pro", "xiaomimimo/mimo-v2.5-pro", "xiaomi"},
		{"minimax-m2.5", "minimax/minimax-m2.5", "atlascloud"},
		{"minimax-m2.7", "minimax/minimax-m2.7", "atlascloud"},
		{"minimax-m3", "minimax/minimax-m3", "novita"},
		{"deepseek-v3.2", "deepseek/deepseek-v3.2", "atlascloud"},
		{"nemotron-3-nano-30b-a3b", "nvidia/nemotron-3-nano-30b-a3b", "novita"},
		{"cobuddy", "baidu/cobuddy", "novita"},
		{"qwen3.6-27b", "qwen/qwen3.6-27b", "novita"},
		{"qwen3.6-35b-a3b", "qwen/qwen3.6-35b-a3b", "atlascloud"},
		{"qwen3.7-max", "qwen/qwen3.7-max", "novita"},
		{"step-3.7-flash", "stepfun/step-3.7-flash", "novita"},
	}

	for _, test := range tests {
		t.Run(test.canonicalID, func(t *testing.T) {
			hivyModel, ok := hivyModelsByID[test.canonicalID]
			if !ok {
				t.Fatalf("%s is not declared", test.canonicalID)
			}
			if !slices.Equal(hivyModel.Routes, hivyModel.ProxyRoutes) {
				t.Fatalf("routes and proxy routes differ: %#v != %#v",
					hivyModel.Routes, hivyModel.ProxyRoutes)
			}
			if len(hivyModel.Routes) == 0 || hivyModel.Routes[0].ProviderID != test.primary {
				t.Fatalf("routes = %#v, want %s primary", hivyModel.Routes, test.primary)
			}

			novitaIndex := slices.Index(hivyModel.Routes, ModelRoute{
				ProviderID: "novita",
				ModelID:    test.novitaModelID,
			})
			if novitaIndex < 0 {
				t.Fatalf("Novita route not declared: %#v", hivyModel.Routes)
			}
			if _, ok := Global().ResolveModel("novita", test.canonicalID); !ok {
				t.Fatal("Novita route does not resolve")
			}
		})
	}

	novitaRouteCount := 0
	for _, hivyModel := range supportedHivyModels {
		for _, route := range hivyModel.ProxyRoutes {
			if route.ProviderID == "novita" {
				novitaRouteCount++
			}
		}
	}
	if novitaRouteCount != len(tests) {
		t.Fatalf("explicit Novita route count = %d, want %d", novitaRouteCount, len(tests))
	}
}
