package registry

// engyHivyModels keeps Engy's models separate from equivalent models offered
// by other providers. Callers opt into Engy explicitly through the engy- IDs.
var engyHivyModels = []HivyModel{
	{
		ID: "engy-glm-5.2",
		Routes: []ModelRoute{
			{ProviderID: "engy", ModelID: "glm-5.2"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "engy", ModelID: "glm-5.2"},
		},
	},
	{
		ID: "engy-qwen3.6-35b-a3b",
		Routes: []ModelRoute{
			{ProviderID: "engy", ModelID: "qwen3.6-35b-a3b"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "engy", ModelID: "qwen3.6-35b-a3b"},
		},
	},
}
