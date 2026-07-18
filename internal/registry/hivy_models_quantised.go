package registry

// quantisedHivyModels intentionally exposes CrofAI's non-Q4_0 models under
// distinct canonical IDs. This keeps the quantised variants selectable without
// changing the provider chain of the equivalent first-party model.
var quantisedHivyModels = []HivyModel{
	{
		ID: "quantised-deepseek-v3.2",
		Routes: []ModelRoute{
			{ProviderID: "quantised", ModelID: "deepseek-v3.2"},
		},
	},
	{
		ID: "quantised-deepseek-v4-pro",
		Routes: []ModelRoute{
			{ProviderID: "quantised", ModelID: "deepseek-v4-pro"},
		},
	},
	{
		ID: "quantised-glm-4.7",
		Routes: []ModelRoute{
			{ProviderID: "quantised", ModelID: "glm-4.7"},
		},
	},
	{
		ID: "quantised-glm-4.7-flash",
		Routes: []ModelRoute{
			{ProviderID: "quantised", ModelID: "glm-4.7-flash"},
		},
	},
	{
		ID: "quantised-glm-5.1",
		Routes: []ModelRoute{
			{ProviderID: "quantised", ModelID: "glm-5.1"},
		},
	},
	{
		ID: "quantised-glm-5.2",
		Routes: []ModelRoute{
			{ProviderID: "quantised", ModelID: "glm-5.2"},
		},
	},
	{
		ID: "quantised-kimi-k2.5",
		Routes: []ModelRoute{
			{ProviderID: "quantised", ModelID: "kimi-k2.5"},
		},
	},
	{
		ID: "quantised-kimi-k2.5-lightning",
		Routes: []ModelRoute{
			{ProviderID: "quantised", ModelID: "kimi-k2.5-lightning"},
		},
	},
	{
		ID: "quantised-kimi-k2.6",
		Routes: []ModelRoute{
			{ProviderID: "quantised", ModelID: "kimi-k2.6"},
		},
	},
	{
		ID: "quantised-kimi-k2.7-code",
		Routes: []ModelRoute{
			{ProviderID: "quantised", ModelID: "kimi-k2.7-code"},
		},
	},
	{
		ID: "quantised-mimo-v2.5-pro",
		Routes: []ModelRoute{
			{ProviderID: "quantised", ModelID: "mimo-v2.5-pro"},
		},
	},
	{
		ID: "quantised-minimax-m2.5",
		Routes: []ModelRoute{
			{ProviderID: "quantised", ModelID: "minimax-m2.5"},
		},
	},
	{
		ID: "quantised-qwen3.5-9b",
		Routes: []ModelRoute{
			{ProviderID: "quantised", ModelID: "qwen3.5-9b"},
		},
	},
}
