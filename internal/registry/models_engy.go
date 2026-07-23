package registry

// engyProvider mirrors Engy's live /v1/models directory. Pricing is expressed
// in USD per million tokens; Engy's API publishes the same values per token.
var engyProvider = Provider{
	ID:   "engy",
	Name: "Engy",
	API:  "https://api.engy.ai/v1",
	Doc:  "https://engy.ai",
	Models: map[string]Model{
		"glm-5.2": {
			ID:               "glm-5.2",
			Name:             "Engy GLM 5.2",
			Family:           "glm",
			Reasoning:        true,
			ToolCall:         true,
			StructuredOutput: true,
			ReleaseDate:      "2026-06-12",
			Modalities:       &Modalities{Input: []string{"text"}, Output: []string{"text"}},
			Cost:             &Cost{Input: 0.68, Output: 1.5, CacheRead: 0.18},
			Limit:            &Limit{Context: 262144, Output: 131072},
		},
		"qwen3.6-35b-a3b": {
			ID:               "qwen3.6-35b-a3b",
			Name:             "Engy Qwen3.6 35B A3B",
			Family:           "qwen",
			Reasoning:        true,
			ToolCall:         true,
			StructuredOutput: true,
			ReleaseDate:      "2026-04-21",
			Modalities:       &Modalities{Input: []string{"text"}, Output: []string{"text"}},
			Cost:             &Cost{Input: 0.045, Output: 0.3, CacheRead: 0.015},
			Limit:            &Limit{Context: 262144, Output: 65536},
		},
	},
}
