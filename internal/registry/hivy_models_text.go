package registry

var coreHivyModelsText = []HivyModel{
	{
		ID:      "gpt-5.6-luna",
		NewFrom: "2026-07-23T00:00:00Z",
		NewTo:   "2026-09-23T00:00:00Z",
		Routes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "openai/gpt-5.6-luna"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "openai/gpt-5.6-luna"},
		},
	},
	{
		ID:      "gpt-5.6-terra",
		NewFrom: "2026-07-23T00:00:00Z",
		NewTo:   "2026-09-23T00:00:00Z",
		Routes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "openai/gpt-5.6-terra"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "openai/gpt-5.6-terra"},
		},
	},
	{
		ID:      "gpt-5.6-sol",
		NewFrom: "2026-07-23T00:00:00Z",
		NewTo:   "2026-09-23T00:00:00Z",
		Routes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "openai/gpt-5.6-sol"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "openai/gpt-5.6-sol"},
		},
	},
	{
		ID: "gemini-3.5-flash",
		Routes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "google/gemini-3.5-flash"},
			{ProviderID: "google", ModelID: "gemini-3.5-flash"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "google/gemini-3.5-flash"},
			{ProviderID: "google", ModelID: "gemini-3.5-flash"},
		},
	},
	{
		ID: "gemini-3.1-flash-lite",
		Routes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "google/gemini-3.1-flash-lite"},
			{ProviderID: "google", ModelID: "gemini-3.1-flash-lite"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "google/gemini-3.1-flash-lite"},
			{ProviderID: "google", ModelID: "gemini-3.1-flash-lite"},
		},
	},
	{
		ID: "gemini-3.1-pro-preview",
		Routes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "google/gemini-3.1-pro-preview"},
			{ProviderID: "google", ModelID: "gemini-3.1-pro-preview"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "google/gemini-3.1-pro-preview"},
			{ProviderID: "google", ModelID: "gemini-3.1-pro-preview"},
		},
	},
	{
		ID: "gemini-3-flash-preview",
		Routes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "google/gemini-3-flash-preview"},
			{ProviderID: "google", ModelID: "gemini-3-flash-preview"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "google/gemini-3-flash-preview"},
			{ProviderID: "google", ModelID: "gemini-3-flash-preview"},
		},
	},
	{
		ID: "deepseek-v4-pro",
		Routes: []ModelRoute{
			{ProviderID: "novita", ModelID: "deepseek/deepseek-v4-pro"},
			{ProviderID: "atlascloud", ModelID: "deepseek-ai/deepseek-v4-pro"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "novita", ModelID: "deepseek/deepseek-v4-pro"},
			{ProviderID: "atlascloud", ModelID: "deepseek-ai/deepseek-v4-pro"},
		},
	},
	{
		ID: "deepseek-v4-flash",
		Routes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "deepseek-ai/deepseek-v4-flash"},
			{ProviderID: "novita", ModelID: "deepseek/deepseek-v4-flash"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "deepseek-ai/deepseek-v4-flash"},
			{ProviderID: "novita", ModelID: "deepseek/deepseek-v4-flash"},
		},
	},
	{
		ID: "step-3.7-flash",
		Routes: []ModelRoute{
			{ProviderID: "novita", ModelID: "stepfun/step-3.7-flash"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "novita", ModelID: "stepfun/step-3.7-flash"},
		},
	},
	{
		ID: "ling-2.6-1t",
		Routes: []ModelRoute{
			{ProviderID: "novita", ModelID: "inclusionai/ling-2.6-1t"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "novita", ModelID: "inclusionai/ling-2.6-1t"},
		},
	},
	{
		ID: "qwen3.7-max",
		Routes: []ModelRoute{
			{ProviderID: "novita", ModelID: "qwen/qwen3.7-max"},
			{ProviderID: "atlascloud", ModelID: "qwen/qwen3.7-max"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "novita", ModelID: "qwen/qwen3.7-max"},
			{ProviderID: "atlascloud", ModelID: "qwen/qwen3.7-max"},
		},
	},
	{
		ID: "qwen3.7-plus",
		Routes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "qwen/qwen3.7-plus"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "qwen/qwen3.7-plus"},
		},
	},
	{
		ID: "grok-4.3",
		Routes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "xai/grok-4.3"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "xai/grok-4.3"},
		},
	},
	{
		ID: "qwen3.6-35b-a3b",
		Routes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "qwen/qwen3.6-35b-a3b"},
			{ProviderID: "novita", ModelID: "qwen/qwen3.6-35b-a3b"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "qwen/qwen3.6-35b-a3b"},
			{ProviderID: "novita", ModelID: "qwen/qwen3.6-35b-a3b"},
		},
	},
	{
		ID: "qwen3.6-27b",
		Routes: []ModelRoute{
			{ProviderID: "novita", ModelID: "qwen/qwen3.6-27b"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "novita", ModelID: "qwen/qwen3.6-27b"},
		},
	},
	{
		ID: "kimi-k2.6",
		Routes: []ModelRoute{
			{ProviderID: "novita", ModelID: "moonshotai/kimi-k2.6"},
			{ProviderID: "atlascloud", ModelID: "moonshotai/kimi-k2.6"},
			{ProviderID: "moonshotai", ModelID: "kimi-k2.6"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "novita", ModelID: "moonshotai/kimi-k2.6"},
			{ProviderID: "atlascloud", ModelID: "moonshotai/kimi-k2.6"},
			{ProviderID: "moonshotai", ModelID: "kimi-k2.6"},
		},
	},
	{
		ID: "mimo-v2.5-pro-ultraspeed",
		Routes: []ModelRoute{
			{ProviderID: "xiaomi", ModelID: "mimo-v2.5-pro-ultraspeed"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "xiaomi", ModelID: "mimo-v2.5-pro-ultraspeed"},
		},
	},
	{
		ID: "mimo-v2.5-pro",
		Routes: []ModelRoute{
			{ProviderID: "xiaomi", ModelID: "mimo-v2.5-pro"},
			{ProviderID: "novita", ModelID: "xiaomimimo/mimo-v2.5-pro"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "xiaomi", ModelID: "mimo-v2.5-pro"},
			{ProviderID: "novita", ModelID: "xiaomimimo/mimo-v2.5-pro"},
		},
	},
	{
		ID: "mimo-v2.5",
		Routes: []ModelRoute{
			{ProviderID: "xiaomi", ModelID: "mimo-v2.5"},
			{ProviderID: "novita", ModelID: "xiaomimimo/mimo-v2.5"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "xiaomi", ModelID: "mimo-v2.5"},
			{ProviderID: "novita", ModelID: "xiaomimimo/mimo-v2.5"},
		},
	},
	{
		ID: "minimax-m2.7",
		Routes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "minimaxai/minimax-m2.7"},
			{ProviderID: "novita", ModelID: "minimax/minimax-m2.7"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "minimaxai/minimax-m2.7"},
			{ProviderID: "novita", ModelID: "minimax/minimax-m2.7"},
		},
	},
	{
		ID: "glm-5.1",
		Routes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "zai-org/glm-5.1"},
			{ProviderID: "novita", ModelID: "zai-org/glm-5.1"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "zai-org/glm-5.1"},
			{ProviderID: "novita", ModelID: "zai-org/glm-5.1"},
		},
	},
	{
		ID: "glm-5-turbo",
		Routes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "zai-org/glm-5-turbo"},
			{ProviderID: "novita", ModelID: "zai-org/glm-5-turbo"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "zai-org/glm-5-turbo"},
			{ProviderID: "novita", ModelID: "zai-org/glm-5-turbo"},
		},
	},
	{
		ID: "glm-5",
		Routes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "zai-org/glm-5"},
			{ProviderID: "novita", ModelID: "zai-org/glm-5"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "zai-org/glm-5"},
			{ProviderID: "novita", ModelID: "zai-org/glm-5"},
		},
	},
	{
		ID: "glm-4.7",
		Routes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "zai-org/glm-4.7"},
			{ProviderID: "novita", ModelID: "zai-org/glm-4.7"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "zai-org/glm-4.7"},
			{ProviderID: "novita", ModelID: "zai-org/glm-4.7"},
		},
	},
	{
		ID: "glm-4.7-flash",
		Routes: []ModelRoute{
			{ProviderID: "novita", ModelID: "zai-org/glm-4.7-flash"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "novita", ModelID: "zai-org/glm-4.7-flash"},
		},
	},
	{
		ID: "minimax-m2.5",
		Routes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "minimaxai/minimax-m2.5"},
			{ProviderID: "novita", ModelID: "minimax/minimax-m2.5"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "minimaxai/minimax-m2.5"},
			{ProviderID: "novita", ModelID: "minimax/minimax-m2.5"},
		},
	},
	{
		ID: "kimi-k2.5",
		Routes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "moonshotai/kimi-k2.5"},
			{ProviderID: "novita", ModelID: "moonshotai/kimi-k2.5"},
			{ProviderID: "moonshotai", ModelID: "kimi-k2.5"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "moonshotai/kimi-k2.5"},
			{ProviderID: "novita", ModelID: "moonshotai/kimi-k2.5"},
			{ProviderID: "moonshotai", ModelID: "kimi-k2.5"},
		},
	},
}
