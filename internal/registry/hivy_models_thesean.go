package registry

// theseanHivyModels keeps Thesean's Ship models separate from the equivalent
// base models. Callers opt into Ship explicitly through the thesean- IDs.
var theseanHivyModels = []HivyModel{
	{
		ID:      "thesean-claude-haiku-4.5",
		NewFrom: "2026-07-24T00:00:00Z",
		NewTo:   "2026-09-24T00:00:00Z",
		Routes: []ModelRoute{
			{ProviderID: "thesean", ModelID: "ship-like/claude-haiku-4-5"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "thesean", ModelID: "ship-like/claude-haiku-4-5"},
		},
	},
	{
		ID:      "thesean-claude-opus-4.8",
		NewFrom: "2026-07-24T00:00:00Z",
		NewTo:   "2026-09-24T00:00:00Z",
		Routes: []ModelRoute{
			{ProviderID: "thesean", ModelID: "ship-like/claude-opus-4-8"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "thesean", ModelID: "ship-like/claude-opus-4-8"},
		},
	},
	{
		ID:      "thesean-claude-sonnet-5",
		NewFrom: "2026-07-24T00:00:00Z",
		NewTo:   "2026-09-24T00:00:00Z",
		Routes: []ModelRoute{
			{ProviderID: "thesean", ModelID: "ship-like/claude-sonnet-5"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "thesean", ModelID: "ship-like/claude-sonnet-5"},
		},
	},
	{
		ID:      "thesean-gpt-5.6-sol",
		NewFrom: "2026-07-24T00:00:00Z",
		NewTo:   "2026-09-24T00:00:00Z",
		Routes: []ModelRoute{
			{ProviderID: "thesean", ModelID: "ship-like/gpt-5.6-sol"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "thesean", ModelID: "ship-like/gpt-5.6-sol"},
		},
	},
}
