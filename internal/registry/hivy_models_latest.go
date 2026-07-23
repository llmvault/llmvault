package registry

var supportedHivyModels = combinedHivyModels()

func combinedHivyModels() []HivyModel {
	models := make([]HivyModel, 0, len(coreHivyModels)+len(latestHivyModels))
	models = append(models, coreHivyModels...)
	models = append(models, latestHivyModels...)
	return models
}

var latestHivyModels = []HivyModel{
	{
		ID: DefaultRasterImageGenerationModelID,
		Routes: []ModelRoute{
			{ProviderID: "reve", ModelID: "reve-image"},
		},
	},
	{
		ID: "flux.2-klein-4b",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "black-forest-labs/flux.2-klein-4b"},
		},
	},
	{
		ID: "riverflow-v2.5-fast",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "sourceful/riverflow-v2.5-fast"},
		},
	},
	{
		ID: "riverflow-v2.5-pro",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "sourceful/riverflow-v2.5-pro"},
		},
	},
	{
		ID: DefaultVectorImageGenerationModelID,
		Routes: []ModelRoute{
			{ProviderID: "quiver", ModelID: "arrow-1.1"},
		},
	},
	{
		ID: "recraft-v4.1-vector",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "recraft/recraft-v4.1-vector"},
		},
	},
	{
		ID: "recraft-v4.1-pro-vector",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "recraft/recraft-v4.1-pro-vector"},
		},
	},
	{
		ID: "kimi-k2.7-code",
		Routes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "moonshotai/kimi-k2.7-code"},
			{ProviderID: "novita", ModelID: "moonshotai/kimi-k2.7-code"},
			{ProviderID: "openrouter", ModelID: "moonshotai/kimi-k2.7-code"},
			{ProviderID: "moonshotai", ModelID: "kimi-k2.7-code"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "moonshotai/kimi-k2.7-code"},
			{ProviderID: "novita", ModelID: "moonshotai/kimi-k2.7-code"},
			{ProviderID: "openrouter", ModelID: "moonshotai/kimi-k2.7-code"},
			{ProviderID: "moonshotai", ModelID: "kimi-k2.7-code"},
		},
	},
	{
		ID: "minimax-m3",
		Routes: []ModelRoute{
			{ProviderID: "novita", ModelID: "minimax/minimax-m3"},
			{ProviderID: "atlascloud", ModelID: "minimaxai/minimax-m3"},
			{ProviderID: "openrouter", ModelID: "minimax/minimax-m3"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "novita", ModelID: "minimax/minimax-m3"},
			{ProviderID: "atlascloud", ModelID: "minimaxai/minimax-m3"},
			{ProviderID: "openrouter", ModelID: "minimax/minimax-m3"},
		},
	},
	{
		ID: "glm-5.2",
		Routes: []ModelRoute{
			{ProviderID: "novita", ModelID: "zai-org/glm-5.2"},
			{ProviderID: "atlascloud", ModelID: "zai-org/glm-5.2"},
			{ProviderID: "openrouter", ModelID: "z-ai/glm-5.2"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "novita", ModelID: "zai-org/glm-5.2"},
			{ProviderID: "atlascloud", ModelID: "zai-org/glm-5.2"},
			{ProviderID: "openrouter", ModelID: "z-ai/glm-5.2"},
		},
	},
	{
		ID: "hy3",
		Routes: []ModelRoute{
			{ProviderID: "novita", ModelID: "tencent/hy3"},
			{ProviderID: "atlascloud", ModelID: "tencent/hy3"},
			{ProviderID: "openrouter", ModelID: "tencent/hy3"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "novita", ModelID: "tencent/hy3"},
			{ProviderID: "atlascloud", ModelID: "tencent/hy3"},
			{ProviderID: "openrouter", ModelID: "tencent/hy3"},
		},
	},
	{
		ID: "ling-3.0-flash",
		Routes: []ModelRoute{
			{ProviderID: "novita", ModelID: "inclusionai/ling-3.0-flash"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "novita", ModelID: "inclusionai/ling-3.0-flash"},
		},
	},
	{
		ID: "kimi-k3",
		Routes: []ModelRoute{
			{ProviderID: "novita", ModelID: "moonshotai/kimi-k3"},
			{ProviderID: "atlascloud", ModelID: "moonshotai/kimi-k3"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "novita", ModelID: "moonshotai/kimi-k3"},
			{ProviderID: "atlascloud", ModelID: "moonshotai/kimi-k3"},
		},
	},
	{
		ID: "deepseek-v3.2",
		Routes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "deepseek-ai/deepseek-v3.2"},
			{ProviderID: "novita", ModelID: "deepseek/deepseek-v3.2"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "deepseek-ai/deepseek-v3.2"},
			{ProviderID: "novita", ModelID: "deepseek/deepseek-v3.2"},
		},
	},
	{
		ID: "nemotron-3-nano-30b-a3b",
		Routes: []ModelRoute{
			{ProviderID: "novita", ModelID: "nvidia/nemotron-3-nano-30b-a3b"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "novita", ModelID: "nvidia/nemotron-3-nano-30b-a3b"},
		},
	},
	{
		ID: "cobuddy",
		Routes: []ModelRoute{
			{ProviderID: "novita", ModelID: "baidu/cobuddy"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "novita", ModelID: "baidu/cobuddy"},
		},
	},
	{
		ID: "grok-4.5",
		Routes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "xai/grok-4.5"},
			{ProviderID: "openrouter", ModelID: "x-ai/grok-4.5"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "xai/grok-4.5"},
			{ProviderID: "openrouter", ModelID: "x-ai/grok-4.5"},
		},
	},
	{
		ID: "laguna-m.1",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "poolside/laguna-m.1"},
		},
	},
}
