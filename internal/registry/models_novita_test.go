package registry

import (
	"slices"
	"testing"
)

func TestNovitaCatalogSnapshot(t *testing.T) {
	provider, ok := Global().GetProvider("novita")
	if !ok {
		t.Fatal("Novita provider not found")
	}
	if provider.API != "https://api.novita.ai/openai/v1" {
		t.Fatalf("Novita API = %q", provider.API)
	}
	if got, want := len(provider.Models), 107; got != want {
		t.Fatalf("Novita model count = %d, want %d", got, want)
	}

	for modelID, model := range provider.Models {
		if model.ID != modelID {
			t.Errorf("%s ID = %q", modelID, model.ID)
		}
		if model.Cost == nil || model.Cost.Input < 0 || model.Cost.Output < 0 {
			t.Errorf("%s has invalid pricing: %#v", modelID, model.Cost)
		}
		if model.Limit == nil || model.Limit.Context <= 0 {
			t.Errorf("%s has invalid limits: %#v", modelID, model.Limit)
		}
		if model.Modalities == nil || !containsString(model.Modalities.Output, "text") {
			t.Errorf("%s is not a text-output model: %#v", modelID, model.Modalities)
		}
	}

	for _, modelID := range []string{
		"ai_infer_test_1",
		"ai_infer_test_2",
		"ai_infer_test_3",
		"bunny",
		"dev/glm46",
		"gt-4p",
	} {
		if _, ok := provider.Models[modelID]; ok {
			t.Errorf("internal/test model %q is present", modelID)
		}
	}
}

func TestNovitaRepresentativeMetadata(t *testing.T) {
	provider, ok := Global().GetProvider("novita")
	if !ok {
		t.Fatal("Novita provider not found")
	}

	tests := []struct {
		modelID                      string
		input, output, cache         float64
		context, maxOutput           int64
		reasoning, tools, structured bool
	}{
		{
			modelID:   "inclusionai/ling-3.0-flash",
			context:   262144,
			maxOutput: 32768,
			reasoning: true,
			tools:     true,
		},
		{
			modelID:    "deepseek/deepseek-v4-flash",
			input:      0.14,
			output:     0.28,
			cache:      0.028,
			context:    1048576,
			maxOutput:  393216,
			reasoning:  true,
			tools:      true,
			structured: true,
		},
		{
			modelID:    "stepfun/step-3.7-flash",
			input:      0.2,
			output:     1.15,
			cache:      0.04,
			context:    262144,
			maxOutput:  256000,
			reasoning:  true,
			tools:      true,
			structured: true,
		},
	}

	for _, test := range tests {
		t.Run(test.modelID, func(t *testing.T) {
			model, ok := provider.Models[test.modelID]
			if !ok {
				t.Fatalf("%s not found", test.modelID)
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
				t.Fatalf("limits = %#v", model.Limit)
			}
			if model.Reasoning != test.reasoning ||
				model.ToolCall != test.tools ||
				model.StructuredOutput != test.structured {
				t.Fatalf("features: reasoning=%v tools=%v structured=%v",
					model.Reasoning, model.ToolCall, model.StructuredOutput)
			}
		})
	}
}

func TestNovitaShowcaseModelsMatchLiveDirectory(t *testing.T) {
	provider, ok := Global().GetProvider("novita")
	if !ok {
		t.Fatal("Novita provider not found")
	}

	tests := []struct {
		modelID              string
		input, output, cache float64
		context, maxOutput   int64
	}{
		{"moonshotai/kimi-k3", 3, 15, 0.3, 1048576, 1048576},
		{"tencent/hy3", 0.14, 0.58, 0.035, 262144, 262144},
		{"zai-org/glm-5.2", 1.4, 4.4, 0.26, 1048576, 131072},
		{"moonshotai/kimi-k2.7-code", 0.95, 4, 0.19, 262144, 262144},
		{"minimax/minimax-m3", 0.3, 1.2, 0.06, 1000000, 131072},
		{"deepseek/deepseek-v4-pro", 1.6, 3.2, 0.135, 1048576, 393216},
		{"deepseek/deepseek-v4-flash", 0.14, 0.28, 0.028, 1048576, 393216},
		{"deepseek/deepseek-v3.2", 0.269, 0.4, 0.1345, 163840, 65536},
		{"inclusionai/ling-3.0-flash", 0, 0, 0, 262144, 32768},
		{"stepfun/step-3.7-flash", 0.2, 1.15, 0.04, 262144, 256000},
		{"nvidia/nemotron-3-nano-30b-a3b", 0.05, 0.2, 0, 262144, 32768},
		{"baidu/cobuddy", 0.28, 1.13, 0.07, 131072, 65536},
		{"xiaomimimo/mimo-v2.5", 0.168, 0.336, 0.0034, 1048576, 131072},
		{"qwen/qwen3.7-max", 1.25, 3.75, 0.25, 1000000, 65536},
		{"xiaomimimo/mimo-v2.5-pro", 0.522, 1.044, 0.0043, 1048576, 131072},
		{"qwen/qwen3.6-27b", 0.6, 3.6, 0, 262144, 65536},
	}

	for _, test := range tests {
		t.Run(test.modelID, func(t *testing.T) {
			model, ok := provider.Models[test.modelID]
			if !ok {
				t.Fatalf("%s not found", test.modelID)
			}
			if model.Cost == nil ||
				model.Cost.Input != test.input ||
				model.Cost.Output != test.output ||
				model.Cost.CacheRead != test.cache {
				t.Fatalf("cost = %#v, want input=%v output=%v cache_read=%v",
					model.Cost, test.input, test.output, test.cache)
			}
			if model.Limit == nil ||
				model.Limit.Context != test.context ||
				model.Limit.Output != test.maxOutput {
				t.Fatalf("limits = %#v, want context=%d output=%d",
					model.Limit, test.context, test.maxOutput)
			}
		})
	}
}

func TestNovitaLowestPricedShowcaseModelsArePrimary(t *testing.T) {
	for _, canonicalID := range []string{
		"kimi-k3",
		"hy3",
		"glm-5.2",
		"minimax-m3",
		"ling-3.0-flash",
		"step-3.7-flash",
		"nemotron-3-nano-30b-a3b",
		"cobuddy",
		"qwen3.7-max",
	} {
		t.Run(canonicalID, func(t *testing.T) {
			hivyModel, ok := hivyModelsByID[canonicalID]
			if !ok {
				t.Fatalf("%s is not declared", canonicalID)
			}
			if len(hivyModel.ProxyRoutes) == 0 || hivyModel.ProxyRoutes[0].ProviderID != "novita" {
				t.Fatalf("routes = %#v, want Novita primary", hivyModel.ProxyRoutes)
			}

			novita, ok := Global().ResolveModel("novita", canonicalID)
			if !ok || novita.Model.Cost == nil {
				t.Fatalf("Novita pricing for %s is unavailable", canonicalID)
			}
			for _, route := range hivyModel.ProxyRoutes[1:] {
				competitor, ok := Global().ResolveModel(route.ProviderID, canonicalID)
				if !ok || competitor.Model.Cost == nil {
					t.Fatalf("%s pricing for %s is unavailable", route.ProviderID, canonicalID)
				}
				if novita.Model.Cost.Input > competitor.Model.Cost.Input ||
					novita.Model.Cost.Output > competitor.Model.Cost.Output ||
					novita.Model.Cost.CacheRead > competitor.Model.Cost.CacheRead {
					t.Fatalf("Novita cost %#v exceeds %s cost %#v",
						novita.Model.Cost, route.ProviderID, competitor.Model.Cost)
				}
			}
		})
	}
}

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
		{"deepseek-v4-flash", "deepseek/deepseek-v4-flash", "atlascloud"},
		{"deepseek-v4-pro", "deepseek/deepseek-v4-pro", "atlascloud"},
		{"glm-4.7", "zai-org/glm-4.7", "atlascloud"},
		{"glm-4.7-flash", "zai-org/glm-4.7-flash", "novita"},
		{"glm-5", "zai-org/glm-5", "atlascloud"},
		{"glm-5-turbo", "zai-org/glm-5-turbo", "atlascloud"},
		{"glm-5.1", "zai-org/glm-5.1", "atlascloud"},
		{"glm-5.2", "zai-org/glm-5.2", "novita"},
		{"hy3", "tencent/hy3", "novita"},
		{"kimi-k2.5", "moonshotai/kimi-k2.5", "atlascloud"},
		{"kimi-k2.6", "moonshotai/kimi-k2.6", "atlascloud"},
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
