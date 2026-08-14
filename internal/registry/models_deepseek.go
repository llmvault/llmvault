package registry

// deepSeekProvider follows DeepSeek's OpenAI-compatible API catalog. The
// stable API model IDs resolve to the dated releases shown in model names.
var deepSeekProvider = Provider{
	ID:   "deepseek",
	Name: "DeepSeek",
	API:  "https://api.deepseek.com",
	Doc:  "https://api-docs.deepseek.com/quick_start/pricing",
	Models: map[string]Model{
		"deepseek-v4-flash": {
			ID:               "deepseek-v4-flash",
			Name:             "DeepSeek V4 Flash 0731",
			Family:           "deepseek",
			Reasoning:        true,
			ToolCall:         true,
			StructuredOutput: true,
			ReleaseDate:      "2026-07-31",
			Modalities:       &Modalities{Input: []string{"text"}, Output: []string{"text"}},
			Cost:             &Cost{Input: 0.14, Output: 0.28, CacheRead: 0.0028},
			Limit:            &Limit{Context: 1048576, Output: 393216},
		},
		"deepseek-v4-pro": {
			ID:               "deepseek-v4-pro",
			Name:             "DeepSeek V4 Pro 0813",
			Family:           "deepseek",
			Reasoning:        true,
			ToolCall:         true,
			StructuredOutput: true,
			ReleaseDate:      "2026-08-13",
			Modalities:       &Modalities{Input: []string{"text"}, Output: []string{"text"}},
			Cost:             &Cost{Input: 0.435, Output: 0.87, CacheRead: 0.003625},
			Limit:            &Limit{Context: 1048576, Output: 393216},
		},
	},
}
