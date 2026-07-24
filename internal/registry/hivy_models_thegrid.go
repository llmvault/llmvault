package registry

// theGridHivyModels keeps The Grid instruments distinct from fixed underlying
// models. Every entry routes directly using the instrument ID as the upstream
// OpenAI-compatible model value.
var theGridHivyModels = []HivyModel{
	{
		ID:      "agent-max",
		NewFrom: "2026-07-24T00:00:00Z",
		NewTo:   "2026-09-24T00:00:00Z",
		Routes: []ModelRoute{
			{ProviderID: "thegrid", ModelID: "agent-max"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "thegrid", ModelID: "agent-max"},
		},
	},
	{
		ID:      "agent-prime",
		NewFrom: "2026-07-24T00:00:00Z",
		NewTo:   "2026-09-24T00:00:00Z",
		Routes: []ModelRoute{
			{ProviderID: "thegrid", ModelID: "agent-prime"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "thegrid", ModelID: "agent-prime"},
		},
	},
	{
		ID:      "agent-standard",
		NewFrom: "2026-07-24T00:00:00Z",
		NewTo:   "2026-09-24T00:00:00Z",
		Routes: []ModelRoute{
			{ProviderID: "thegrid", ModelID: "agent-standard"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "thegrid", ModelID: "agent-standard"},
		},
	},
	{
		ID:      "bytedance-pro-latest",
		NewFrom: "2026-07-24T00:00:00Z",
		NewTo:   "2026-09-24T00:00:00Z",
		Routes: []ModelRoute{
			{ProviderID: "thegrid", ModelID: "bytedance-pro-latest"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "thegrid", ModelID: "bytedance-pro-latest"},
		},
	},
	{
		ID:      "claude-opus-latest",
		NewFrom: "2026-07-24T00:00:00Z",
		NewTo:   "2026-09-24T00:00:00Z",
		Routes: []ModelRoute{
			{ProviderID: "thegrid", ModelID: "claude-opus-latest"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "thegrid", ModelID: "claude-opus-latest"},
		},
	},
	{
		ID:      "code-max",
		NewFrom: "2026-07-24T00:00:00Z",
		NewTo:   "2026-09-24T00:00:00Z",
		Routes: []ModelRoute{
			{ProviderID: "thegrid", ModelID: "code-max"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "thegrid", ModelID: "code-max"},
		},
	},
	{
		ID:      "code-prime",
		NewFrom: "2026-07-24T00:00:00Z",
		NewTo:   "2026-09-24T00:00:00Z",
		Routes: []ModelRoute{
			{ProviderID: "thegrid", ModelID: "code-prime"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "thegrid", ModelID: "code-prime"},
		},
	},
	{
		ID:      "code-standard",
		NewFrom: "2026-07-24T00:00:00Z",
		NewTo:   "2026-09-24T00:00:00Z",
		Routes: []ModelRoute{
			{ProviderID: "thegrid", ModelID: "code-standard"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "thegrid", ModelID: "code-standard"},
		},
	},
	{
		ID:      "deepseek-pro-latest",
		NewFrom: "2026-07-24T00:00:00Z",
		NewTo:   "2026-09-24T00:00:00Z",
		Routes: []ModelRoute{
			{ProviderID: "thegrid", ModelID: "deepseek-pro-latest"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "thegrid", ModelID: "deepseek-pro-latest"},
		},
	},
	{
		ID:      "gemini-pro-latest",
		NewFrom: "2026-07-24T00:00:00Z",
		NewTo:   "2026-09-24T00:00:00Z",
		Routes: []ModelRoute{
			{ProviderID: "thegrid", ModelID: "gemini-pro-latest"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "thegrid", ModelID: "gemini-pro-latest"},
		},
	},
	{
		ID:      "glm-latest",
		NewFrom: "2026-07-24T00:00:00Z",
		NewTo:   "2026-09-24T00:00:00Z",
		Routes: []ModelRoute{
			{ProviderID: "thegrid", ModelID: "glm-latest"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "thegrid", ModelID: "glm-latest"},
		},
	},
	{
		ID:      "gpt-sol-latest",
		NewFrom: "2026-07-24T00:00:00Z",
		NewTo:   "2026-09-24T00:00:00Z",
		Routes: []ModelRoute{
			{ProviderID: "thegrid", ModelID: "gpt-sol-latest"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "thegrid", ModelID: "gpt-sol-latest"},
		},
	},
	{
		ID:      "kimi-latest",
		NewFrom: "2026-07-24T00:00:00Z",
		NewTo:   "2026-09-24T00:00:00Z",
		Routes: []ModelRoute{
			{ProviderID: "thegrid", ModelID: "kimi-latest"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "thegrid", ModelID: "kimi-latest"},
		},
	},
	{
		ID:      "minimax-latest",
		NewFrom: "2026-07-24T00:00:00Z",
		NewTo:   "2026-09-24T00:00:00Z",
		Routes: []ModelRoute{
			{ProviderID: "thegrid", ModelID: "minimax-latest"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "thegrid", ModelID: "minimax-latest"},
		},
	},
	{
		ID:      "text-max",
		NewFrom: "2026-07-24T00:00:00Z",
		NewTo:   "2026-09-24T00:00:00Z",
		Routes: []ModelRoute{
			{ProviderID: "thegrid", ModelID: "text-max"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "thegrid", ModelID: "text-max"},
		},
	},
	{
		ID:      "text-prime",
		NewFrom: "2026-07-24T00:00:00Z",
		NewTo:   "2026-09-24T00:00:00Z",
		Routes: []ModelRoute{
			{ProviderID: "thegrid", ModelID: "text-prime"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "thegrid", ModelID: "text-prime"},
		},
	},
	{
		ID:      "text-standard",
		NewFrom: "2026-07-24T00:00:00Z",
		NewTo:   "2026-09-24T00:00:00Z",
		Routes: []ModelRoute{
			{ProviderID: "thegrid", ModelID: "text-standard"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "thegrid", ModelID: "text-standard"},
		},
	},
}
