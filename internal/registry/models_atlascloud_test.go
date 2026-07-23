package registry

import (
	"slices"
	"testing"
)

func TestAtlasCloudCatalogSnapshot(t *testing.T) {
	provider, ok := Global().GetProvider("atlascloud")
	if !ok {
		t.Fatal("Atlas Cloud provider not found")
	}
	if got, want := len(provider.Models), 85; got != want {
		t.Fatalf("Atlas Cloud model count = %d, want %d", got, want)
	}

	for modelID, model := range provider.Models {
		if model.ID != modelID {
			t.Errorf("%s ID = %q", modelID, model.ID)
		}
		if model.Cost == nil || model.Cost.Input <= 0 || model.Cost.Output <= 0 {
			t.Errorf("%s has invalid pricing: %#v", modelID, model.Cost)
		}
		if model.Limit == nil || model.Limit.Context <= 0 || model.Limit.Output <= 0 {
			t.Errorf("%s has invalid limits: %#v", modelID, model.Limit)
		}
		if model.Modalities == nil || !containsString(model.Modalities.Output, "text") {
			t.Errorf("%s is not a text-output model: %#v", modelID, model.Modalities)
		}
	}

	for _, modelID := range []string{
		"openai/gpt-image-2",
		"google/gemini-3.1-flash-image",
		"openai/gpt-4o-mini",
	} {
		if _, ok := provider.Models[modelID]; ok {
			t.Errorf("unroutable or image model %q is present", modelID)
		}
	}
}

func TestAtlasCloudRepresentativeMetadata(t *testing.T) {
	provider, ok := Global().GetProvider("atlascloud")
	if !ok {
		t.Fatal("Atlas Cloud provider not found")
	}

	tests := []struct {
		modelID              string
		input, output, cache float64
		context, maxOutput   int64
		reasoning, tools     bool
	}{
		{
			modelID:   "tencent/hy3",
			input:     0.2,
			output:    0.8,
			cache:     0.05,
			context:   262144,
			maxOutput: 131072,
			reasoning: true,
			tools:     true,
		},
		{
			modelID:   "anthropic/claude-opus-4.7",
			input:     5,
			output:    25,
			cache:     0.5,
			context:   1000000,
			maxOutput: 128000,
		},
		{
			modelID:   "moonshotai/kimi-k3",
			input:     3,
			output:    15,
			cache:     0.3,
			context:   1048576,
			maxOutput: 1048576,
			reasoning: true,
			tools:     true,
		},
		{
			modelID:   "bytedance/doubao-seed-2.0-pro-260215",
			input:     0.5,
			output:    3,
			cache:     0.1,
			context:   262144,
			maxOutput: 131072,
		},
	}

	for _, test := range tests {
		t.Run(test.modelID, func(t *testing.T) {
			model := provider.Models[test.modelID]
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
			if model.Reasoning != test.reasoning || model.ToolCall != test.tools {
				t.Fatalf("features: reasoning=%v tools=%v", model.Reasoning, model.ToolCall)
			}
		})
	}
}

func TestAtlasCloudLongContextPricing(t *testing.T) {
	provider, ok := Global().GetProvider("atlascloud")
	if !ok {
		t.Fatal("Atlas Cloud provider not found")
	}
	model := provider.Models["openai/gpt-5.6-luna"]
	if model.Cost == nil || len(model.Cost.Tiers) != 1 {
		t.Fatalf("GPT 5.6 Luna tiers = %#v", model.Cost)
	}
	tier := model.Cost.Tiers[0]
	if tier.MinContext != 272000 ||
		tier.Input != 2 ||
		tier.Output != 9 ||
		tier.CacheRead != 0.2 {
		t.Fatalf("GPT 5.6 Luna long-context tier = %#v", tier)
	}
}

func TestAtlasCloudKatCoderModels(t *testing.T) {
	tests := []struct {
		canonicalID          string
		upstreamID           string
		input, output, cache float64
		context, maxOutput   int64
		reasoning            bool
	}{
		{"kat-coder-air-v2.5", "kwaipilot/kat-coder-air-v2.5", 0.15, 0.6, 0.03, 262144, 262144, true},
		{"kat-coder-pro-v2", "kwaipilot/kat-coder-pro-v2", 0.3, 1.2, 0.06, 262144, 144000, false},
		{"kat-coder-pro-v2.5", "kwaipilot/kat-coder-pro-v2.5", 0.74, 2.96, 0.15, 262144, 262144, true},
	}

	for _, test := range tests {
		t.Run(test.canonicalID, func(t *testing.T) {
			routes := Global().ProxyRoutesForModel(test.canonicalID)
			wantRoute := ModelRoute{ProviderID: "atlascloud", ModelID: test.upstreamID}
			if len(routes) != 1 || routes[0] != wantRoute {
				t.Fatalf("routes = %#v, want %#v", routes, []ModelRoute{wantRoute})
			}

			resolved, ok := Global().ResolveModel("atlascloud", test.canonicalID)
			if !ok {
				t.Fatal("Atlas Cloud route does not resolve")
			}
			if resolved.UpstreamID != test.upstreamID {
				t.Fatalf("upstream = %q, want %q", resolved.UpstreamID, test.upstreamID)
			}
			if resolved.Model.Cost == nil ||
				resolved.Model.Cost.Input != test.input ||
				resolved.Model.Cost.Output != test.output ||
				resolved.Model.Cost.CacheRead != test.cache {
				t.Fatalf("cost = %#v", resolved.Model.Cost)
			}
			if resolved.Model.Limit == nil ||
				resolved.Model.Limit.Context != test.context ||
				resolved.Model.Limit.Output != test.maxOutput {
				t.Fatalf("limits = %#v", resolved.Model.Limit)
			}
			if !resolved.Model.ToolCall || !resolved.Model.StructuredOutput ||
				resolved.Model.Reasoning != test.reasoning {
				t.Fatalf("features: reasoning=%v tools=%v structured=%v",
					resolved.Model.Reasoning, resolved.Model.ToolCall, resolved.Model.StructuredOutput)
			}
		})
	}
}

func TestAtlasCloudLongCatModel(t *testing.T) {
	const canonicalID = "longcat-2.0"
	wantRoute := ModelRoute{
		ProviderID: "atlascloud",
		ModelID:    "meituan-longcat/longcat-2.0",
	}

	routes := Global().ProxyRoutesForModel(canonicalID)
	if len(routes) != 1 || routes[0] != wantRoute {
		t.Fatalf("routes = %#v, want %#v", routes, []ModelRoute{wantRoute})
	}

	resolved, ok := Global().ResolveModel("atlascloud", canonicalID)
	if !ok {
		t.Fatal("Atlas Cloud route does not resolve")
	}
	if resolved.UpstreamID != wantRoute.ModelID {
		t.Fatalf("upstream = %q, want %q", resolved.UpstreamID, wantRoute.ModelID)
	}
	if resolved.Model.Cost == nil ||
		resolved.Model.Cost.Input != 0.75 ||
		resolved.Model.Cost.Output != 3 ||
		resolved.Model.Cost.CacheRead != 0.015 {
		t.Fatalf("cost = %#v", resolved.Model.Cost)
	}
	if resolved.Model.Limit == nil ||
		resolved.Model.Limit.Context != 1048756 ||
		resolved.Model.Limit.Output != 262144 {
		t.Fatalf("limits = %#v", resolved.Model.Limit)
	}
	if !resolved.Model.Reasoning ||
		!resolved.Model.ToolCall ||
		!resolved.Model.StructuredOutput {
		t.Fatalf("features: reasoning=%v tools=%v structured=%v",
			resolved.Model.Reasoning,
			resolved.Model.ToolCall,
			resolved.Model.StructuredOutput,
		)
	}
}

func TestAtlasCloudPrimaryRoutesAreDeclaredExplicitly(t *testing.T) {
	tests := []struct {
		canonicalID  string
		atlasModelID string
	}{
		{canonicalID: "claude-opus-4.5", atlasModelID: "anthropic/claude-opus-4.5-20251101"},
		{canonicalID: "claude-opus-4.6", atlasModelID: "anthropic/claude-opus-4.6"},
		{canonicalID: "claude-opus-4.7", atlasModelID: "anthropic/claude-opus-4.7"},
		{canonicalID: "claude-sonnet-4.5", atlasModelID: "anthropic/claude-sonnet-4.5-20250929"},
		{canonicalID: "claude-sonnet-4.6", atlasModelID: "anthropic/claude-sonnet-4.6"},
		{canonicalID: "deepseek-v3.2", atlasModelID: "deepseek-ai/deepseek-v3.2"},
		{canonicalID: "deepseek-v4-flash", atlasModelID: "deepseek-ai/deepseek-v4-flash"},
		{canonicalID: "deepseek-v4-pro", atlasModelID: "deepseek-ai/deepseek-v4-pro"},
		{canonicalID: "gemini-3-flash-preview", atlasModelID: "google/gemini-3-flash-preview"},
		{canonicalID: "gemini-3.1-flash-lite", atlasModelID: "google/gemini-3.1-flash-lite"},
		{canonicalID: "gemini-3.1-pro-preview", atlasModelID: "google/gemini-3.1-pro-preview"},
		{canonicalID: "gemini-3.5-flash", atlasModelID: "google/gemini-3.5-flash"},
		{canonicalID: "glm-4.7", atlasModelID: "zai-org/glm-4.7"},
		{canonicalID: "glm-5", atlasModelID: "zai-org/glm-5"},
		{canonicalID: "glm-5-turbo", atlasModelID: "zai-org/glm-5-turbo"},
		{canonicalID: "glm-5.1", atlasModelID: "zai-org/glm-5.1"},
		{canonicalID: "gpt-5.4", atlasModelID: "openai/gpt-5.4"},
		{canonicalID: "gpt-5.4-mini", atlasModelID: "openai/gpt-5.4-mini"},
		{canonicalID: "gpt-5.4-nano", atlasModelID: "openai/gpt-5.4-nano"},
		{canonicalID: "gpt-5.5", atlasModelID: "openai/gpt-5.5"},
		{canonicalID: "gpt-5.6-luna", atlasModelID: "openai/gpt-5.6-luna"},
		{canonicalID: "gpt-5.6-sol", atlasModelID: "openai/gpt-5.6-sol"},
		{canonicalID: "gpt-5.6-terra", atlasModelID: "openai/gpt-5.6-terra"},
		{canonicalID: "grok-4.3", atlasModelID: "xai/grok-4.3"},
		{canonicalID: "grok-4.5", atlasModelID: "xai/grok-4.5"},
		{canonicalID: "kat-coder-air-v2.5", atlasModelID: "kwaipilot/kat-coder-air-v2.5"},
		{canonicalID: "kat-coder-pro-v2", atlasModelID: "kwaipilot/kat-coder-pro-v2"},
		{canonicalID: "kat-coder-pro-v2.5", atlasModelID: "kwaipilot/kat-coder-pro-v2.5"},
		{canonicalID: "kimi-k2.5", atlasModelID: "moonshotai/kimi-k2.5"},
		{canonicalID: "kimi-k2.6", atlasModelID: "moonshotai/kimi-k2.6"},
		{canonicalID: "kimi-k2.7-code", atlasModelID: "moonshotai/kimi-k2.7-code"},
		{canonicalID: "longcat-2.0", atlasModelID: "meituan-longcat/longcat-2.0"},
		{canonicalID: "minimax-m2.5", atlasModelID: "minimaxai/minimax-m2.5"},
		{canonicalID: "minimax-m2.7", atlasModelID: "minimaxai/minimax-m2.7"},
		{canonicalID: "qwen3.6-35b-a3b", atlasModelID: "qwen/qwen3.6-35b-a3b"},
		{canonicalID: "qwen3.7-plus", atlasModelID: "qwen/qwen3.7-plus"},
	}

	for _, test := range tests {
		hivyModel, ok := hivyModelsByID[test.canonicalID]
		if !ok {
			t.Errorf("%s is not declared", test.canonicalID)
			continue
		}
		wantAtlas := ModelRoute{ProviderID: "atlascloud", ModelID: test.atlasModelID}
		if len(hivyModel.Routes) == 0 || hivyModel.Routes[0] != wantAtlas {
			t.Errorf("%s declared routes = %#v, want Atlas first", test.canonicalID, hivyModel.Routes)
		}
		if len(hivyModel.ProxyRoutes) == 0 || hivyModel.ProxyRoutes[0] != wantAtlas {
			t.Errorf("%s declared proxy routes = %#v, want Atlas first", test.canonicalID, hivyModel.ProxyRoutes)
		}
		if !slices.Equal(hivyModel.Routes, hivyModel.ProxyRoutes) {
			t.Errorf("%s routes and proxy routes differ: %#v != %#v", test.canonicalID, hivyModel.Routes, hivyModel.ProxyRoutes)
		}

		routes := Global().ProxyRoutesForModel(test.canonicalID)
		if len(routes) < 1 {
			t.Errorf("%s routes = %#v", test.canonicalID, routes)
			continue
		}
		if routes[0] != wantAtlas {
			t.Errorf("%s primary route = %#v", test.canonicalID, routes[0])
		}
		if _, ok := Global().ResolveModel("atlascloud", test.canonicalID); !ok {
			t.Errorf("%s Atlas route does not resolve", test.canonicalID)
		}
	}

	atlasPrimaryCount := 0
	for _, hivyModel := range supportedHivyModels {
		if len(hivyModel.ProxyRoutes) > 0 && hivyModel.ProxyRoutes[0].ProviderID == "atlascloud" {
			atlasPrimaryCount++
		}
	}
	if atlasPrimaryCount != len(tests) {
		t.Fatalf("explicit Atlas primary route count = %d, want %d", atlasPrimaryCount, len(tests))
	}
}

func TestAtlasCloudDoesNotReplaceDirectPrimaryRoutes(t *testing.T) {
	routes := Global().ProxyRoutesForModel("gpt-4o-mini")
	if len(routes) == 0 || routes[0].ProviderID != "openai" {
		t.Fatalf("gpt-4o-mini routes = %#v, want OpenAI primary", routes)
	}

	routes = Global().ProxyRoutesForModel("mimo-v2.5-pro")
	if len(routes) == 0 || routes[0].ProviderID != "xiaomi" {
		t.Fatalf("mimo-v2.5-pro routes = %#v, want Xiaomi primary", routes)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
