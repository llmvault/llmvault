package registry

var coreHivyModelsTextLegacy = []HivyModel{
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
