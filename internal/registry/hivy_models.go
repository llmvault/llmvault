package registry

var coreHivyModelsPreferred = []HivyModel{
	{
		ID: "claude-opus-4.7",
		Routes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "anthropic/claude-opus-4.7"},
			{ProviderID: "openrouter", ModelID: "anthropic/claude-opus-4.7"},
			{ProviderID: "anthropic", ModelID: "claude-opus-4-7"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "anthropic/claude-opus-4.7"},
			{ProviderID: "openrouter", ModelID: "anthropic/claude-opus-4.7"},
			{ProviderID: "anthropic", ModelID: "claude-opus-4-7"},
		},
	},
	{
		ID: "claude-opus-4.7-fast",
		Routes: []ModelRoute{
			{ProviderID: "openrouter", ModelID: "anthropic/claude-opus-4.7-fast"},
		},
	},
	{
		ID: "claude-opus-4.6",
		Routes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "anthropic/claude-opus-4.6"},
			{ProviderID: "openrouter", ModelID: "anthropic/claude-opus-4.6"},
			{ProviderID: "anthropic", ModelID: "claude-opus-4-6"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "anthropic/claude-opus-4.6"},
			{ProviderID: "openrouter", ModelID: "anthropic/claude-opus-4.6"},
			{ProviderID: "anthropic", ModelID: "claude-opus-4-6"},
		},
	},
	{
		ID: "claude-opus-4.5",
		Routes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "anthropic/claude-opus-4.5-20251101"},
			{ProviderID: "openrouter", ModelID: "anthropic/claude-opus-4.5"},
			{ProviderID: "anthropic", ModelID: "claude-opus-4-5"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "anthropic/claude-opus-4.5-20251101"},
			{ProviderID: "openrouter", ModelID: "anthropic/claude-opus-4.5"},
			{ProviderID: "anthropic", ModelID: "claude-opus-4-5"},
		},
	},
	{
		ID: "claude-sonnet-5",
		Routes: []ModelRoute{
			{ProviderID: "anthropic", ModelID: "claude-sonnet-5"},
			{ProviderID: "openrouter", ModelID: "anthropic/claude-sonnet-5"},
		},
	},
	{
		ID: "claude-sonnet-4.6",
		Routes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "anthropic/claude-sonnet-4.6"},
			{ProviderID: "openrouter", ModelID: "anthropic/claude-sonnet-4.6"},
			{ProviderID: "anthropic", ModelID: "claude-sonnet-4-6"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "anthropic/claude-sonnet-4.6"},
			{ProviderID: "openrouter", ModelID: "anthropic/claude-sonnet-4.6"},
			{ProviderID: "anthropic", ModelID: "claude-sonnet-4-6"},
		},
	},
	{
		ID: "claude-sonnet-4.5",
		Routes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "anthropic/claude-sonnet-4.5-20250929"},
			{ProviderID: "openrouter", ModelID: "anthropic/claude-sonnet-4.5"},
			{ProviderID: "anthropic", ModelID: "claude-sonnet-4-5"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "anthropic/claude-sonnet-4.5-20250929"},
			{ProviderID: "openrouter", ModelID: "anthropic/claude-sonnet-4.5"},
			{ProviderID: "anthropic", ModelID: "claude-sonnet-4-5"},
		},
	},
	{
		ID: "claude-sonnet-4",
		Routes: []ModelRoute{
			{ProviderID: "anthropic", ModelID: "claude-sonnet-4-0"},
			{ProviderID: "openrouter", ModelID: "anthropic/claude-sonnet-4"},
		},
	},
	{
		ID: "gpt-5.5",
		Routes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "openai/gpt-5.5"},
			{ProviderID: "openrouter", ModelID: "openai/gpt-5.5"},
			{ProviderID: "openai", ModelID: "gpt-5.5"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "openai/gpt-5.5"},
			{ProviderID: "openrouter", ModelID: "openai/gpt-5.5"},
			{ProviderID: "openai", ModelID: "gpt-5.5"},
		},
	},
	{
		ID: "gpt-5.5-pro",
		Routes: []ModelRoute{
			{ProviderID: "openai", ModelID: "gpt-5.5-pro"},
			{ProviderID: "openrouter", ModelID: "openai/gpt-5.5-pro"},
		},
	},
	{
		ID: "gpt-5.4",
		Routes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "openai/gpt-5.4"},
			{ProviderID: "openrouter", ModelID: "openai/gpt-5.4"},
			{ProviderID: "openai", ModelID: "gpt-5.4"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "openai/gpt-5.4"},
			{ProviderID: "openrouter", ModelID: "openai/gpt-5.4"},
			{ProviderID: "openai", ModelID: "gpt-5.4"},
		},
	},
	{
		ID: "gpt-5.4-pro",
		Routes: []ModelRoute{
			{ProviderID: "openai", ModelID: "gpt-5.4-pro"},
			{ProviderID: "openrouter", ModelID: "openai/gpt-5.4-pro"},
		},
	},
	{
		ID: "gpt-5.4-mini",
		Routes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "openai/gpt-5.4-mini"},
			{ProviderID: "openrouter", ModelID: "openai/gpt-5.4-mini"},
			{ProviderID: "openai", ModelID: "gpt-5.4-mini"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "openai/gpt-5.4-mini"},
			{ProviderID: "openrouter", ModelID: "openai/gpt-5.4-mini"},
			{ProviderID: "openai", ModelID: "gpt-5.4-mini"},
		},
	},
	{
		ID: "gpt-5.4-nano",
		Routes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "openai/gpt-5.4-nano"},
			{ProviderID: "openrouter", ModelID: "openai/gpt-5.4-nano"},
			{ProviderID: "openai", ModelID: "gpt-5.4-nano"},
		},
		ProxyRoutes: []ModelRoute{
			{ProviderID: "atlascloud", ModelID: "openai/gpt-5.4-nano"},
			{ProviderID: "openrouter", ModelID: "openai/gpt-5.4-nano"},
			{ProviderID: "openai", ModelID: "gpt-5.4-nano"},
		},
	},
	{
		ID: "gpt-4o-mini",
		Routes: []ModelRoute{
			{ProviderID: "openai", ModelID: "gpt-4o-mini"},
			{ProviderID: "openrouter", ModelID: "openai/gpt-4o-mini"},
		},
	},
	{
		ID: "scribe-v2",
		Routes: []ModelRoute{
			{ProviderID: "elevenlabs", ModelID: "scribe_v2"},
		},
	},
	{
		ID: "gpt-5.3-codex",
		Routes: []ModelRoute{
			{ProviderID: "openai", ModelID: "gpt-5.3-codex"},
			{ProviderID: "openrouter", ModelID: "openai/gpt-5.3-codex"},
		},
	},
	{
		ID: "gpt-5.3-codex-spark",
		Routes: []ModelRoute{
			{ProviderID: "openai", ModelID: "gpt-5.3-codex-spark"},
		},
	},
}
