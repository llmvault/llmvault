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
