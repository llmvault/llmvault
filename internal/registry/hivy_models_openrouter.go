package registry

var coreHivyModelsOpenRouter = []HivyModel{
	{
		ID: "gpt-5.6-luna",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "openai/gpt-5.6-luna-pro"},
		},
	},
	{
		ID: "gpt-5.6-terra",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "openai/gpt-5.6-terra-pro"},
		},
	},
	{
		ID: "gpt-5.6-sol",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "openai/gpt-5.6-sol-pro"},
		},
	},
	{
		ID: "gemini-3.5-flash",
		Routes: []ModelRoute{
			{ProviderID: "google", ModelID: "gemini-3.5-flash"},
			{ProviderID: "openrouter", ModelID: "google/gemini-3.5-flash"},
		},
	},
	{
		ID: "gemini-3.1-flash-lite",
		Routes: []ModelRoute{
			{ProviderID: "google", ModelID: "gemini-3.1-flash-lite"},
			{ProviderID: "openrouter", ModelID: "google/gemini-3.1-flash-lite"},
		},
	},
	{
		ID: "gemini-3.1-pro-preview",
		Routes: []ModelRoute{
			{ProviderID: "google", ModelID: "gemini-3.1-pro-preview"},
			{ProviderID: "openrouter", ModelID: "google/gemini-3.1-pro-preview"},
		},
	},
	{
		ID: "gemini-3-flash-preview",
		Routes: []ModelRoute{
			{ProviderID: "google", ModelID: "gemini-3-flash-preview"},
			{ProviderID: "openrouter", ModelID: "google/gemini-3-flash-preview"},
		},
	},
	{
		ID: "deepseek-v4-pro",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "deepseek/deepseek-v4-pro"},
		},
	},
	{
		ID: "deepseek-v4-flash",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "deepseek/deepseek-v4-flash"},
		},
	},
	{
		ID: "step-3.7-flash",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "stepfun/step-3.7-flash"},
		},
	},
	{
		ID: "ling-2.6-1t",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "inclusionai/ling-2.6-1t"},
		},
	},
	{
		ID: "qwen3.7-max",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "qwen/qwen3.7-max"},
		},
	},
	{
		ID: "qwen3.7-plus",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "qwen/qwen3.7-plus"},
		},
	},
	{
		ID: "grok-4.3",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "x-ai/grok-4.3"},
		},
	},
	{
		ID: "nemotron-3-ultra-550b-a55b",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "nvidia/nemotron-3-ultra-550b-a55b"},
		},
	},
	{
		ID: "qwen3.6-max-preview",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "qwen/qwen3.6-max-preview"},
		},
	},
	{
		ID: "qwen3.6-flash",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "qwen/qwen3.6-flash"},
		},
	},
	{
		ID: "qwen3.6-35b-a3b",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "qwen/qwen3.6-35b-a3b"},
		},
	},
	{
		ID: "qwen3.6-27b",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "qwen/qwen3.6-27b"},
		},
	},
	{
		ID: "kimi-k2.6",
		Routes: []ModelRoute{
			{ProviderID: "moonshotai", ModelID: "kimi-k2.6"},
			{ProviderID: "openrouter", ModelID: "moonshotai/kimi-k2.6"},
		},
	},
	{
		ID: "mimo-v2.5-pro",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "xiaomi/mimo-v2.5-pro"},
		},
	},
	{
		ID: "mimo-v2.5",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "xiaomi/mimo-v2.5"},
		},
	},
	{
		ID: "minimax-m2.7",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "minimax/minimax-m2.7"},
		},
	},
	{
		ID: "glm-5.1",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "z-ai/glm-5.1"},
		},
	},
	{
		ID: "glm-5-turbo",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "z-ai/glm-5-turbo"},
		},
	},
	{
		ID: "glm-5",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "z-ai/glm-5"},
		},
	},
	{
		ID: "glm-4.7",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "z-ai/glm-4.7"},
		},
	},
	{
		ID: "glm-4.7-flash",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "z-ai/glm-4.7-flash"},
		},
	},
	{
		ID: "mistral-small-4",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "mistralai/mistral-small-2603"},
		},
	},
	{
		ID: "minimax-m2.5",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "minimax/minimax-m2.5"},
		},
	},
	{
		ID: "step-3.5-flash",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "stepfun/step-3.5-flash"},
		},
	},
	{
		ID: "kimi-k2.5",
		Routes: []ModelRoute{
			{ProviderID: "moonshotai", ModelID: "kimi-k2.5"},
			{ProviderID: "openrouter", ModelID: "moonshotai/kimi-k2.5"},
		},
	},
}
