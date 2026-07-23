package registry

import (
	"slices"
	"testing"
)

func TestAtlasCloudRoutesAreDeclaredExplicitly(t *testing.T) {
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
		if !slices.Contains(hivyModel.Routes, wantAtlas) {
			t.Errorf("%s declared routes = %#v, want Atlas route %#v", test.canonicalID, hivyModel.Routes, wantAtlas)
		}
		if !slices.Contains(hivyModel.ProxyRoutes, wantAtlas) {
			t.Errorf("%s declared proxy routes = %#v, want Atlas route %#v", test.canonicalID, hivyModel.ProxyRoutes, wantAtlas)
		}
		if !slices.Equal(hivyModel.Routes, hivyModel.ProxyRoutes) {
			t.Errorf("%s routes and proxy routes differ: %#v != %#v", test.canonicalID, hivyModel.Routes, hivyModel.ProxyRoutes)
		}

		routes := Global().ProxyRoutesForModel(test.canonicalID)
		if len(routes) < 1 {
			t.Errorf("%s routes = %#v", test.canonicalID, routes)
			continue
		}
		if !slices.Contains(routes, wantAtlas) {
			t.Errorf("%s proxy routes = %#v, want Atlas route %#v", test.canonicalID, routes, wantAtlas)
		}
		if _, ok := Global().ResolveModel("atlascloud", test.canonicalID); !ok {
			t.Errorf("%s Atlas route does not resolve", test.canonicalID)
		}
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
