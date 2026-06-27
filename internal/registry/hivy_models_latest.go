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
			{ProviderID: "moonshotai", ModelID: "kimi-k2.7-code"},
			{ProviderID: "openrouter", ModelID: "moonshotai/kimi-k2.7-code"},
		},
	},
	{
		ID: "minimax-m3",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "minimax/minimax-m3"},
		},
	},
	{
		ID: "glm-5.2",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "z-ai/glm-5.2"},
		},
	},
}
