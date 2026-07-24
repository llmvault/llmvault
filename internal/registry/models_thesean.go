package registry

// theseanProvider mirrors Thesean's live GET /v1/models directory from
// 2026-07-24. Pricing is converted from the API's per-token values to USD per
// million tokens.
var theseanProvider = Provider{
	ID:   "thesean",
	Name: "Thesean",
	API:  "https://api.thesean.ai/v1",
	Doc:  "https://docs.thesean.ai",
	Models: map[string]Model{
		"ship-like/claude-haiku-4-5": {
			ID:          "ship-like/claude-haiku-4-5",
			Name:        "Ship like Claude Haiku 4.5",
			Family:      "claude-haiku",
			Reasoning:   true,
			ToolCall:    true,
			Modalities:  &Modalities{Input: []string{"text", "image"}, Output: []string{"text"}},
			Cost:        &Cost{Input: 0.5, Output: 2.5, CacheRead: 0.05, CacheWrite: 0.625},
			Limit:       &Limit{Context: 200000, Output: 64000},
			ReleaseDate: "2025-10-01",
		},
		"ship-like/claude-opus-4-8": {
			ID:         "ship-like/claude-opus-4-8",
			Name:       "Ship like Claude Opus 4.8",
			Family:     "claude-opus",
			Reasoning:  true,
			ToolCall:   true,
			Modalities: &Modalities{Input: []string{"text", "image"}, Output: []string{"text"}},
			Cost:       &Cost{Input: 2.5, Output: 12.5, CacheRead: 0.25, CacheWrite: 3.125},
			Limit:      &Limit{Context: 1000000, Output: 128000},
		},
		"ship-like/claude-sonnet-5": {
			ID:          "ship-like/claude-sonnet-5",
			Name:        "Ship like Claude Sonnet 5",
			Family:      "claude-sonnet",
			Reasoning:   true,
			ToolCall:    true,
			Modalities:  &Modalities{Input: []string{"text", "image"}, Output: []string{"text"}},
			Cost:        &Cost{Input: 1, Output: 5, CacheRead: 0.1, CacheWrite: 1.25},
			Limit:       &Limit{Context: 1000000, Output: 128000},
			ReleaseDate: "2026-06-30",
		},
		"ship-like/gpt-5.6-sol": {
			ID:         "ship-like/gpt-5.6-sol",
			Name:       "Ship like GPT 5.6 Sol",
			Family:     "gpt",
			Reasoning:  true,
			ToolCall:   true,
			Modalities: &Modalities{Input: []string{"text", "image"}, Output: []string{"text"}},
			Cost:       &Cost{Input: 2.5, Output: 15, CacheRead: 0.25, CacheWrite: 3.125},
			Limit:      &Limit{Context: 1050000, Output: 128000},
		},
	},
}
