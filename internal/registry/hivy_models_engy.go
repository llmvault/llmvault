package registry

// engyHivyModels keeps Engy's models separate from equivalent models offered
// by other providers. Callers opt into Engy explicitly through the engy- IDs.
var engyHivyModels = []HivyModel{
	{
		ID:      "engy-glm-5.2",
		NewFrom: "2026-07-24T00:00:00Z",
		NewTo:   "2026-09-24T00:00:00Z",
		Routes: []ModelRoute{
			{ProviderID: "engy", ModelID: "glm-5.2"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "engy", ModelID: "glm-5.2"},
		},
	},
	{
		ID:      "engy-qwen3.6-35b-a3b",
		NewFrom: "2026-07-24T00:00:00Z",
		NewTo:   "2026-09-24T00:00:00Z",
		Routes: []ModelRoute{
			{ProviderID: "engy", ModelID: "qwen3.6-35b-a3b"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "engy", ModelID: "qwen3.6-35b-a3b"},
		},
	},
}
