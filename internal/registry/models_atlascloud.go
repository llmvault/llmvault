package registry

// atlasCloudProvider is a snapshot of Atlas Cloud's GET /v1/models response
// from 2026-07-23, limited to text-generating models that passed a live
// /v1/chat/completions route check. Pricing is stored per million tokens.
var atlasCloudProvider = Provider{
	ID:   "atlascloud",
	Name: "Atlas Cloud",
	API:  "https://api.atlascloud.ai/v1",
	Doc:  "https://www.atlascloud.ai/docs/models/llm",
	Models: map[string]Model{
		"anthropic/claude-haiku-4.5-20251001": {
			ID:          "anthropic/claude-haiku-4.5-20251001",
			Name:        "Claude Haiku 4.5 20251001",
			Description: "Claude Haiku 4.5 20251001",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     1,
				Output:    5,
				CacheRead: 0.1,
			},
			Limit: &Limit{
				Context: 200000,
				Output:  64000,
			},
		},
		"anthropic/claude-opus-4.1-20250805": {
			ID:          "anthropic/claude-opus-4.1-20250805",
			Name:        "Claude Opus 4.1 20250805",
			Description: "Claude Opus 4.1 20250805",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     15,
				Output:    75,
				CacheRead: 1.5,
			},
			Limit: &Limit{
				Context: 200000,
				Output:  32000,
			},
		},
		"anthropic/claude-opus-4.5-20251101": {
			ID:          "anthropic/claude-opus-4.5-20251101",
			Name:        "Claude Opus 4.5 20251101",
			Description: "Claude Opus 4.5 20251101",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     5,
				Output:    25,
				CacheRead: 0.5,
			},
			Limit: &Limit{
				Context: 200000,
				Output:  32000,
			},
		},
		"anthropic/claude-opus-4.5-20251101-coding": {
			ID:          "anthropic/claude-opus-4.5-20251101-coding",
			Name:        "Claude Opus 4.5 20251101 Coding",
			Description: "Claude Opus 4.5 20251101 Coding",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     5,
				Output:    25,
				CacheRead: 0.5,
			},
			Limit: &Limit{
				Context: 200000,
				Output:  32000,
			},
		},
		"anthropic/claude-opus-4.6": {
			ID:          "anthropic/claude-opus-4.6",
			Name:        "Claude Opus 4.6",
			Description: "Claude Opus 4.6",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     5,
				Output:    25,
				CacheRead: 0.5,
			},
			Limit: &Limit{
				Context: 1000000,
				Output:  128000,
			},
		},
		"anthropic/claude-opus-4.6-coding": {
			ID:          "anthropic/claude-opus-4.6-coding",
			Name:        "Claude Opus 4.6 Coding",
			Description: "Claude Opus 4.6 Coding",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     5,
				Output:    25,
				CacheRead: 0.5,
			},
			Limit: &Limit{
				Context: 1000000,
				Output:  128000,
			},
		},
		"anthropic/claude-opus-4.7": {
			ID:          "anthropic/claude-opus-4.7",
			Name:        "Claude Opus 4.7",
			Description: "Claude Opus 4.7",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     5,
				Output:    25,
				CacheRead: 0.5,
			},
			Limit: &Limit{
				Context: 1000000,
				Output:  128000,
			},
		},
		"anthropic/claude-opus-4.7-coding": {
			ID:          "anthropic/claude-opus-4.7-coding",
			Name:        "Claude Opus 4.7 Coding",
			Description: "Claude Opus 4.7 Coding",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     5,
				Output:    25,
				CacheRead: 0.5,
			},
			Limit: &Limit{
				Context: 1000000,
				Output:  128000,
			},
		},
		"anthropic/claude-opus-4.8": {
			ID:          "anthropic/claude-opus-4.8",
			Name:        "Claude Opus 4.8",
			Description: "Claude Opus 4.8",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     5,
				Output:    25,
				CacheRead: 0.5,
			},
			Limit: &Limit{
				Context: 1000000,
				Output:  128000,
			},
		},
		"anthropic/claude-opus-4.8-ccmax": {
			ID:          "anthropic/claude-opus-4.8-ccmax",
			Name:        "Claude Opus 4.8 CC Max",
			Description: "Claude Opus 4.8 CC Max",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     5,
				Output:    25,
				CacheRead: 0.5,
			},
			Limit: &Limit{
				Context: 1000000,
				Output:  128000,
			},
		},
		"anthropic/claude-opus-4.8-coding": {
			ID:          "anthropic/claude-opus-4.8-coding",
			Name:        "Claude Opus 4.8 Coding",
			Description: "Claude Opus 4.8 Coding",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     5,
				Output:    25,
				CacheRead: 0.5,
			},
			Limit: &Limit{
				Context: 1000000,
				Output:  128000,
			},
		},
		"anthropic/claude-sonnet-4.5-20250929": {
			ID:          "anthropic/claude-sonnet-4.5-20250929",
			Name:        "Claude Sonnet 4.5 20250929",
			Description: "Claude Sonnet 4.5 20250929",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     3,
				Output:    15,
				CacheRead: 0.3,
				Tiers: []CostTier{
					{
						MinContext: 200000,
						Input:      6,
						Output:     22.5,
						CacheRead:  0.6,
					},
				},
			},
			Limit: &Limit{
				Context: 200000,
				Output:  64000,
			},
		},
		"anthropic/claude-sonnet-4.5-20250929-coding": {
			ID:          "anthropic/claude-sonnet-4.5-20250929-coding",
			Name:        "Claude Sonnet 4.5 20250929 Coding",
			Description: "Claude Sonnet 4.5 20250929 Coding",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     3,
				Output:    15,
				CacheRead: 0.3,
				Tiers: []CostTier{
					{
						MinContext: 200000,
						Input:      6,
						Output:     22.5,
						CacheRead:  0.6,
					},
				},
			},
			Limit: &Limit{
				Context: 200000,
				Output:  64000,
			},
		},
		"anthropic/claude-sonnet-4.6": {
			ID:          "anthropic/claude-sonnet-4.6",
			Name:        "Claude Sonnet 4.6",
			Description: "Claude Sonnet 4.6",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     3,
				Output:    15,
				CacheRead: 0.3,
			},
			Limit: &Limit{
				Context: 200000,
				Output:  64000,
			},
		},
		"anthropic/claude-sonnet-4.6-coding": {
			ID:          "anthropic/claude-sonnet-4.6-coding",
			Name:        "Claude Sonnet 4.6 Coding",
			Description: "Claude Sonnet 4.6 Coding",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     3,
				Output:    15,
				CacheRead: 0.3,
			},
			Limit: &Limit{
				Context: 200000,
				Output:  64000,
			},
		},
		"anthropic/claude-sonnet-5-ccmax": {
			ID:          "anthropic/claude-sonnet-5-ccmax",
			Name:        "Claude Sonnet 5 CC Max",
			Description: "Claude Sonnet 5 CC Max",
			Modalities: &Modalities{
				Input:  []string{"text", "image"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     2,
				Output:    10,
				CacheRead: 0.2,
			},
			Limit: &Limit{
				Context: 1000000,
				Output:  128000,
			},
		},
		"bytedance/doubao-seed-1.6-251015": {
			ID:          "bytedance/doubao-seed-1.6-251015",
			Name:        "Doubao Seed 1.6",
			Description: "Doubao Seed 1.6",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.25,
				Output:    2,
				CacheRead: 0.05,
				Tiers: []CostTier{
					{
						MinContext: 131072,
						Input:      0.5,
						Output:     4,
						CacheRead:  0.05,
					},
				},
			},
			Limit: &Limit{
				Context: 262144,
				Output:  65536,
			},
		},
		"bytedance/doubao-seed-1.6-flash-250828": {
			ID:          "bytedance/doubao-seed-1.6-flash-250828",
			Name:        "Doubao Seed 1.6 Flash",
			Description: "Doubao Seed 1.6 Flash",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.075,
				Output:    0.3,
				CacheRead: 0.015,
				Tiers: []CostTier{
					{
						MinContext: 131072,
						Input:      0.1,
						Output:     0.8,
						CacheRead:  0.015,
					},
				},
			},
			Limit: &Limit{
				Context: 262144,
				Output:  32768,
			},
		},
		"bytedance/doubao-seed-1.8-251228": {
			ID:          "bytedance/doubao-seed-1.8-251228",
			Name:        "Doubao Seed 1.8",
			Description: "Doubao Seed 1.8",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.25,
				Output:    2,
				CacheRead: 0.05,
				Tiers: []CostTier{
					{
						MinContext: 131072,
						Input:      0.5,
						Output:     4,
						CacheRead:  0.05,
					},
				},
			},
			Limit: &Limit{
				Context: 262144,
				Output:  65536,
			},
		},
		"bytedance/doubao-seed-2.0-code-preview-260215": {
			ID:          "bytedance/doubao-seed-2.0-code-preview-260215",
			Name:        "Doubao Seed 2.0 Code Preview",
			Description: "Doubao Seed 2.0 Code Preview",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.5,
				Output:    3,
				CacheRead: 0.1,
				Tiers: []CostTier{
					{
						MinContext: 131072,
						Input:      1,
						Output:     6,
						CacheRead:  0.2,
					},
				},
			},
			Limit: &Limit{
				Context: 262144,
				Output:  131072,
			},
		},
		"bytedance/doubao-seed-2.0-lite-260428": {
			ID:          "bytedance/doubao-seed-2.0-lite-260428",
			Name:        "Doubao Seed 2.0 Lite",
			Description: "Doubao Seed 2.0 Lite",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.25,
				Output:    2,
				CacheRead: 0.05,
				Tiers: []CostTier{
					{
						MinContext: 131072,
						Input:      0.5,
						Output:     4,
						CacheRead:  0.1,
					},
				},
			},
			Limit: &Limit{
				Context: 262144,
				Output:  131072,
			},
		},
		"bytedance/doubao-seed-2.0-mini-260428": {
			ID:          "bytedance/doubao-seed-2.0-mini-260428",
			Name:        "Doubao Seed 2.0 Mini",
			Description: "Doubao Seed 2.0 Mini",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.1,
				Output:    0.4,
				CacheRead: 0.02,
				Tiers: []CostTier{
					{
						MinContext: 131072,
						Input:      0.2,
						Output:     0.8,
						CacheRead:  0.04,
					},
				},
			},
			Limit: &Limit{
				Context: 262144,
				Output:  131072,
			},
		},
		"bytedance/doubao-seed-2.0-pro-260215": {
			ID:          "bytedance/doubao-seed-2.0-pro-260215",
			Name:        "Doubao Seed 2.0 Pro",
			Description: "Doubao Seed 2.0 Pro",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.5,
				Output:    3,
				CacheRead: 0.1,
				Tiers: []CostTier{
					{
						MinContext: 131072,
						Input:      1,
						Output:     6,
						CacheRead:  0.2,
					},
				},
			},
			Limit: &Limit{
				Context: 262144,
				Output:  131072,
			},
		},
		"bytedance/doubao-seed-2.1-pro-260628": {
			ID:          "bytedance/doubao-seed-2.1-pro-260628",
			Name:        "Doubao Seed 2.1 Pro",
			Description: "Doubao Seed 2.1 Pro",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.9,
				Output:    4.5,
				CacheRead: 0.18,
			},
			Limit: &Limit{
				Context: 262144,
				Output:  262144,
			},
		},
		"bytedance/doubao-seed-2.1-turbo-260628": {
			ID:          "bytedance/doubao-seed-2.1-turbo-260628",
			Name:        "Doubao Seed 2.1 Turbo",
			Description: "Doubao Seed 2.1 Turbo",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.45,
				Output:    2.25,
				CacheRead: 0.09,
			},
			Limit: &Limit{
				Context: 262144,
				Output:  262144,
			},
		},
		"bytedance/doubao-seed-evolving": {
			ID:          "bytedance/doubao-seed-evolving",
			Name:        "Doubao Seed Evolving",
			Description: "Doubao Seed Evolving",
			Modalities: &Modalities{
				Input:  []string{"text", "image", "video"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.9,
				Output:    4.5,
				CacheRead: 0.18,
			},
			Limit: &Limit{
				Context: 262144,
				Output:  262144,
			},
		},
		"deepseek-ai/deepseek-ocr": {
			ID:          "deepseek-ai/deepseek-ocr",
			Name:        "DeepSeek OCR",
			Description: "DeepSeek OCR",
			Modalities: &Modalities{
				Input:  []string{"text", "image"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.04,
				Output:    0.08,
				CacheRead: 0.04,
			},
			Limit: &Limit{
				Context: 8192,
				Output:  8192,
			},
		},
		"deepseek-ai/DeepSeek-V3.1": {
			ID:               "deepseek-ai/DeepSeek-V3.1",
			Name:             "DeepSeek-V3.1",
			Reasoning:        true,
			ToolCall:         true,
			StructuredOutput: true,
			Description:      "DeepSeek-V3.1",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.3,
				Output:    0.95,
				CacheRead: 0.13,
			},
			Limit: &Limit{
				Context: 131072,
				Output:  65536,
			},
		},
		"deepseek-ai/DeepSeek-V3.1-Terminus": {
			ID:               "deepseek-ai/DeepSeek-V3.1-Terminus",
			Name:             "DeepSeek V3.1 Terminus",
			Reasoning:        true,
			ToolCall:         true,
			StructuredOutput: true,
			Description:      "DeepSeek V3.1 Terminus",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.3,
				Output:    0.95,
				CacheRead: 0.13,
			},
			Limit: &Limit{
				Context: 131072,
				Output:  65536,
			},
		},
		"deepseek-ai/deepseek-v3.2": {
			ID:               "deepseek-ai/deepseek-v3.2",
			Name:             "DeepSeek V3.2",
			Reasoning:        true,
			ToolCall:         true,
			StructuredOutput: true,
			Description:      "DeepSeek V3.2",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.26,
				Output:    0.38,
				CacheRead: 0.13,
			},
			Limit: &Limit{
				Context: 163840,
				Output:  163840,
			},
		},
		"deepseek-ai/DeepSeek-V3.2-Exp": {
			ID:               "deepseek-ai/DeepSeek-V3.2-Exp",
			Name:             "DeepSeek V3.2 Exp",
			Reasoning:        true,
			ToolCall:         true,
			StructuredOutput: true,
			Description:      "DeepSeek V3.2 Exp",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.27,
				Output:    0.41,
				CacheRead: 0.27,
			},
			Limit: &Limit{
				Context: 163840,
				Output:  163840,
			},
		},
		"deepseek-ai/deepseek-v4-flash": {
			ID:               "deepseek-ai/deepseek-v4-flash",
			Name:             "DeepSeek V4 Flash",
			ToolCall:         true,
			StructuredOutput: true,
			Description:      "DeepSeek V4 Flash",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.14,
				Output:    0.28,
				CacheRead: 0.028,
			},
			Limit: &Limit{
				Context: 1048576,
				Output:  393216,
			},
		},
		"deepseek-ai/deepseek-v4-pro": {
			ID:               "deepseek-ai/deepseek-v4-pro",
			Name:             "DeepSeek V4 Pro",
			ToolCall:         true,
			StructuredOutput: true,
			Description:      "DeepSeek V4 Pro",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     1.68,
				Output:    3.38,
				CacheRead: 0.13,
			},
			Limit: &Limit{
				Context: 1048576,
				Output:  393216,
			},
		},
		"google/gemini-2.0-flash": {
			ID:          "google/gemini-2.0-flash",
			Name:        "Gemini 2.0 Flash",
			Description: "Gemini 2.0 Flash",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.1,
				Output:    0.4,
				CacheRead: 0.025,
			},
			Limit: &Limit{
				Context: 1048576,
				Output:  8192,
			},
		},
		"google/gemini-2.5-flash": {
			ID:          "google/gemini-2.5-flash",
			Name:        "Gemini 2.5 Flash",
			Description: "Gemini 2.5 Flash",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.3,
				Output:    2.5,
				CacheRead: 0.03,
			},
			Limit: &Limit{
				Context: 1048576,
				Output:  65536,
			},
		},
		"google/gemini-2.5-flash-lite": {
			ID:          "google/gemini-2.5-flash-lite",
			Name:        "Gemini 2.5 Flash Lite",
			Description: "Gemini 2.5 Flash Lite",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.1,
				Output:    0.4,
				CacheRead: 0.01,
			},
			Limit: &Limit{
				Context: 1048576,
				Output:  65536,
			},
		},
		"google/gemini-2.5-pro": {
			ID:          "google/gemini-2.5-pro",
			Name:        "Gemini 2.5 Pro",
			Description: "Gemini 2.5 Pro",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     1.25,
				Output:    10,
				CacheRead: 0.125,
				Tiers: []CostTier{
					{
						MinContext: 200000,
						Input:      2.5,
						Output:     15,
						CacheRead:  0.25,
					},
				},
			},
			Limit: &Limit{
				Context: 1000000,
				Output:  65536,
			},
		},
		"google/gemini-3-flash-preview": {
			ID:          "google/gemini-3-flash-preview",
			Name:        "Gemini 3 Flash Preview",
			Description: "Gemini 3 Flash Preview",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.5,
				Output:    3,
				CacheRead: 0.05,
			},
			Limit: &Limit{
				Context: 200000,
				Output:  65536,
			},
		},
		"google/gemini-3.1-flash-lite": {
			ID:          "google/gemini-3.1-flash-lite",
			Name:        "Gemini 3.1 Flash Lite",
			Description: "Gemini 3.1 Flash Lite",
			Modalities: &Modalities{
				Input:  []string{"text", "image", "video"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.25,
				Output:    1.5,
				CacheRead: 0.025,
			},
			Limit: &Limit{
				Context: 1048576,
				Output:  65536,
			},
		},
		"google/gemini-3.1-pro-preview": {
			ID:          "google/gemini-3.1-pro-preview",
			Name:        "Gemini 3.1 Pro Preview",
			Description: "Gemini 3.1 Pro Preview",
			Modalities: &Modalities{
				Input:  []string{"text", "image", "video"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     2,
				Output:    12,
				CacheRead: 0.2,
				Tiers: []CostTier{
					{
						MinContext: 200000,
						Input:      4,
						Output:     18,
						CacheRead:  0.4,
					},
				},
			},
			Limit: &Limit{
				Context: 1000000,
				Output:  64000,
			},
		},
		"google/gemini-3.5-flash": {
			ID:          "google/gemini-3.5-flash",
			Name:        "Gemini 3.5 Flash",
			Description: "Gemini 3.5 Flash",
			Modalities: &Modalities{
				Input:  []string{"text", "image", "video"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     1.5,
				Output:    9,
				CacheRead: 0.15,
			},
			Limit: &Limit{
				Context: 1048576,
				Output:  65536,
			},
		},
		"kwaipilot/kat-coder-air-v2.5": {
			ID:               "kwaipilot/kat-coder-air-v2.5",
			Name:             "KAT Coder Air V2.5",
			Reasoning:        true,
			ToolCall:         true,
			StructuredOutput: true,
			Description:      "KAT Coder Air V2.5",
			Modalities: &Modalities{
				Input:  []string{"text", "image", "video"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.15,
				Output:    0.6,
				CacheRead: 0.03,
			},
			Limit: &Limit{
				Context: 262144,
				Output:  262144,
			},
		},
		"kwaipilot/kat-coder-pro-v2": {
			ID:               "kwaipilot/kat-coder-pro-v2",
			Name:             "KAT Coder Pro V2",
			ToolCall:         true,
			StructuredOutput: true,
			Description:      "KAT Coder Pro V2",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.3,
				Output:    1.2,
				CacheRead: 0.06,
			},
			Limit: &Limit{
				Context: 262144,
				Output:  144000,
			},
		},
		"kwaipilot/kat-coder-pro-v2.5": {
			ID:               "kwaipilot/kat-coder-pro-v2.5",
			Name:             "KAT Coder Pro V2.5",
			Reasoning:        true,
			ToolCall:         true,
			StructuredOutput: true,
			Description:      "KAT Coder Pro V2.5",
			Modalities: &Modalities{
				Input:  []string{"text", "image", "video"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.74,
				Output:    2.96,
				CacheRead: 0.15,
			},
			Limit: &Limit{
				Context: 262144,
				Output:  262144,
			},
		},
		"meituan-longcat/longcat-2.0": {
			ID:               "meituan-longcat/longcat-2.0",
			Name:             "LongCat 2.0",
			Reasoning:        true,
			ToolCall:         true,
			StructuredOutput: true,
			Description:      "LongCat 2.0",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.75,
				Output:    3,
				CacheRead: 0.015,
			},
			Limit: &Limit{
				Context: 1048756,
				Output:  262144,
			},
		},
		"minimaxai/minimax-m2.5": {
			ID:               "minimaxai/minimax-m2.5",
			Name:             "MiniMax M2.5",
			Reasoning:        true,
			ToolCall:         true,
			StructuredOutput: true,
			Description:      "MiniMax M2.5",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.295,
				Output:    1.2,
				CacheRead: 0.06,
			},
			Limit: &Limit{
				Context: 196608,
				Output:  196608,
			},
		},
		"minimaxai/minimax-m2.7": {
			ID:               "minimaxai/minimax-m2.7",
			Name:             "MiniMax M2.7",
			Reasoning:        true,
			ToolCall:         true,
			StructuredOutput: true,
			Description:      "MiniMax M2.7",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.3,
				Output:    1.2,
				CacheRead: 0.06,
			},
			Limit: &Limit{
				Context: 196608,
				Output:  196608,
			},
		},
		"minimaxai/minimax-m3": {
			ID:          "minimaxai/minimax-m3",
			Name:        "MiniMax M3",
			Description: "MiniMax M3",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.3,
				Output:    1.2,
				CacheRead: 0.06,
				Tiers: []CostTier{
					{
						MinContext: 512000,
						Input:      0.6,
						Output:     2.4,
						CacheRead:  0.12,
					},
				},
			},
			Limit: &Limit{
				Context: 524300,
				Output:  524288,
			},
		},
		"moonshotai/kimi-k2.5": {
			ID:               "moonshotai/kimi-k2.5",
			Name:             "Kimi K2.5",
			Reasoning:        true,
			ToolCall:         true,
			StructuredOutput: true,
			Description:      "Kimi K2.5",
			Modalities: &Modalities{
				Input:  []string{"text", "image", "video"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.49,
				Output:    2.5,
				CacheRead: 0.2,
			},
			Limit: &Limit{
				Context: 262144,
				Output:  262144,
			},
		},
		"moonshotai/kimi-k2.6": {
			ID:               "moonshotai/kimi-k2.6",
			Name:             "Kimi K2.6",
			Reasoning:        true,
			ToolCall:         true,
			StructuredOutput: true,
			Description:      "Kimi K2.6",
			Modalities: &Modalities{
				Input:  []string{"text", "image", "video"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.95,
				Output:    4,
				CacheRead: 0.16,
			},
			Limit: &Limit{
				Context: 262144,
				Output:  262144,
			},
		},
		"moonshotai/kimi-k2.7-code": {
			ID:               "moonshotai/kimi-k2.7-code",
			Name:             "Kimi K2.7 Code",
			Reasoning:        true,
			ToolCall:         true,
			StructuredOutput: true,
			Description:      "Kimi K2.7 Code",
			Modalities: &Modalities{
				Input:  []string{"text", "image"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.95,
				Output:    4,
				CacheRead: 0.16,
			},
			Limit: &Limit{
				Context: 262144,
				Output:  262144,
			},
		},
		"moonshotai/kimi-k3": {
			ID:               "moonshotai/kimi-k3",
			Name:             "Kimi K3",
			Reasoning:        true,
			ToolCall:         true,
			StructuredOutput: true,
			Description:      "Kimi K3",
			Modalities: &Modalities{
				Input:  []string{"text", "image", "video"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     3,
				Output:    15,
				CacheRead: 0.3,
			},
			Limit: &Limit{
				Context: 1048576,
				Output:  1048576,
			},
		},
		"openai/gpt-4.1-mini": {
			ID:          "openai/gpt-4.1-mini",
			Name:        "GPT 4.1 mini",
			Description: "GPT 4.1 mini",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.4,
				Output:    1.6,
				CacheRead: 0.1,
			},
			Limit: &Limit{
				Context: 1047576,
				Output:  32768,
			},
		},
		"openai/gpt-5.4": {
			ID:          "openai/gpt-5.4",
			Name:        "GPT 5.4",
			Description: "GPT 5.4",
			Modalities: &Modalities{
				Input:  []string{"text", "image"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     2.5,
				Output:    15,
				CacheRead: 0.25,
				Tiers: []CostTier{
					{
						MinContext: 272000,
						Input:      5,
						Output:     22.5,
						CacheRead:  0.5,
					},
				},
			},
			Limit: &Limit{
				Context: 400000,
				Output:  128000,
			},
		},
		"openai/gpt-5.4-mini": {
			ID:          "openai/gpt-5.4-mini",
			Name:        "GPT 5.4 mini",
			Description: "GPT 5.4 mini",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.75,
				Output:    4.5,
				CacheRead: 0.075,
			},
			Limit: &Limit{
				Context: 400000,
				Output:  131072,
			},
		},
		"openai/gpt-5.4-nano": {
			ID:          "openai/gpt-5.4-nano",
			Name:        "GPT 5.4 nano",
			Description: "GPT 5.4 nano",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.2,
				Output:    1.25,
				CacheRead: 0.02,
			},
			Limit: &Limit{
				Context: 400000,
				Output:  131072,
			},
		},
		"openai/gpt-5.5": {
			ID:          "openai/gpt-5.5",
			Name:        "GPT 5.5",
			Description: "GPT 5.5",
			Modalities: &Modalities{
				Input:  []string{"text", "image"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     5,
				Output:    30,
				CacheRead: 0.5,
				Tiers: []CostTier{
					{
						MinContext: 272000,
						Input:      10,
						Output:     45,
						CacheRead:  1,
					},
				},
			},
			Limit: &Limit{
				Context: 1050000,
				Output:  128000,
			},
		},
		"openai/gpt-5.6-luna": {
			ID:          "openai/gpt-5.6-luna",
			Name:        "GPT 5.6 Luna",
			Description: "GPT 5.6 Luna",
			Modalities: &Modalities{
				Input:  []string{"text", "image"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     1,
				Output:    6,
				CacheRead: 0.1,
				Tiers: []CostTier{
					{
						MinContext: 272000,
						Input:      2,
						Output:     9,
						CacheRead:  0.2,
					},
				},
			},
			Limit: &Limit{
				Context: 1050000,
				Output:  131072,
			},
		},
		"openai/gpt-5.6-sol": {
			ID:          "openai/gpt-5.6-sol",
			Name:        "GPT 5.6 Sol",
			Description: "GPT 5.6 Sol",
			Modalities: &Modalities{
				Input:  []string{"text", "image"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     5,
				Output:    30,
				CacheRead: 0.5,
				Tiers: []CostTier{
					{
						MinContext: 272000,
						Input:      10,
						Output:     45,
						CacheRead:  1,
					},
				},
			},
			Limit: &Limit{
				Context: 1050000,
				Output:  131072,
			},
		},
		"openai/gpt-5.6-terra": {
			ID:          "openai/gpt-5.6-terra",
			Name:        "GPT 5.6 Terra",
			Description: "GPT 5.6 Terra",
			Modalities: &Modalities{
				Input:  []string{"text", "image"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     2.5,
				Output:    15,
				CacheRead: 0.25,
				Tiers: []CostTier{
					{
						MinContext: 272000,
						Input:      5,
						Output:     22.5,
						CacheRead:  0.5,
					},
				},
			},
			Limit: &Limit{
				Context: 1050000,
				Output:  131072,
			},
		},
		"Qwen/Qwen3-235B-A22B-Instruct-2507": {
			ID:               "Qwen/Qwen3-235B-A22B-Instruct-2507",
			Name:             "Qwen3-235B-A22B-Instruct-2507",
			ToolCall:         true,
			StructuredOutput: true,
			Description:      "Qwen3-235B-A22B-Instruct-2507",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.2,
				Output:    0.88,
				CacheRead: 0.2,
			},
			Limit: &Limit{
				Context: 131072,
				Output:  131072,
			},
		},
		"qwen/qwen3-vl-235b-a22b-thinking": {
			ID:               "qwen/qwen3-vl-235b-a22b-thinking",
			Name:             "Qwen3 VL 235B A22B Thinking",
			Reasoning:        true,
			ToolCall:         true,
			StructuredOutput: true,
			Description:      "Qwen3 VL 235B A22B Thinking",
			Modalities: &Modalities{
				Input:  []string{"text", "image"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.5,
				Output:    2.5,
				CacheRead: 0.5,
			},
			Limit: &Limit{
				Context: 131072,
				Output:  65536,
			},
		},
		"qwen/qwen3.5-122b-a10b": {
			ID:               "qwen/qwen3.5-122b-a10b",
			Name:             "Qwen3.5 122B A10B",
			Reasoning:        true,
			ToolCall:         true,
			StructuredOutput: true,
			Description:      "Qwen3.5 122B A10B",
			Modalities: &Modalities{
				Input:  []string{"text", "image", "video"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.3,
				Output:    2.4,
				CacheRead: 0.3,
			},
			Limit: &Limit{
				Context: 262144,
				Output:  65536,
			},
		},
		"qwen/qwen3.5-27b": {
			ID:               "qwen/qwen3.5-27b",
			Name:             "Qwen3.5 27B",
			Reasoning:        true,
			ToolCall:         true,
			StructuredOutput: true,
			Description:      "Qwen3.5 27B",
			Modalities: &Modalities{
				Input:  []string{"text", "image", "video"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.27,
				Output:    2.16,
				CacheRead: 0.27,
			},
			Limit: &Limit{
				Context: 262144,
				Output:  65536,
			},
		},
		"qwen/qwen3.5-35b-a3b": {
			ID:               "qwen/qwen3.5-35b-a3b",
			Name:             "Qwen3.5 35B A3B",
			Reasoning:        true,
			ToolCall:         true,
			StructuredOutput: true,
			Description:      "Qwen3.5 35B A3B",
			Modalities: &Modalities{
				Input:  []string{"text", "image", "video"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.225,
				Output:    1.8,
				CacheRead: 0.225,
			},
			Limit: &Limit{
				Context: 262144,
				Output:  65536,
			},
		},
		"qwen/qwen3.5-397b-a17b": {
			ID:               "qwen/qwen3.5-397b-a17b",
			Name:             "Qwen3.5 397BA17B",
			Reasoning:        true,
			ToolCall:         true,
			StructuredOutput: true,
			Description:      "Qwen3.5 397BA17B",
			Modalities: &Modalities{
				Input:  []string{"text", "image", "video"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.55,
				Output:    3.5,
				CacheRead: 0.55,
			},
			Limit: &Limit{
				Context: 262144,
				Output:  65536,
			},
		},
		"qwen/qwen3.5-flash": {
			ID:          "qwen/qwen3.5-flash",
			Name:        "Qwen3.5 Flash",
			Description: "Qwen3.5 Flash",
			Modalities: &Modalities{
				Input:  []string{"text", "image"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.1,
				Output:    0.4,
				CacheRead: 0.1,
			},
			Limit: &Limit{
				Context: 1000000,
				Output:  67072,
			},
		},
		"qwen/qwen3.5-plus": {
			ID:          "qwen/qwen3.5-plus",
			Name:        "Qwen3.5 Plus",
			Description: "Qwen3.5 Plus",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.4,
				Output:    2.4,
				CacheRead: 0.04,
				Tiers: []CostTier{
					{
						MinContext: 262144,
						Input:      0.5,
						Output:     3,
						CacheRead:  0.05,
					},
				},
			},
			Limit: &Limit{
				Context: 1000000,
				Output:  67072,
			},
		},
		"qwen/qwen3.6-35b-a3b": {
			ID:          "qwen/qwen3.6-35b-a3b",
			Name:        "Qwen3.6 35B A3B",
			Reasoning:   true,
			ToolCall:    true,
			Description: "Qwen3.6 35B A3B",
			Modalities: &Modalities{
				Input:  []string{"text", "image", "video"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.186,
				Output:    1.11375,
				CacheRead: 0.186,
			},
			Limit: &Limit{
				Context: 262144,
				Output:  65536,
			},
		},
		"qwen/qwen3.6-plus": {
			ID:          "qwen/qwen3.6-plus",
			Name:        "Qwen3.6 Plus",
			Reasoning:   true,
			ToolCall:    true,
			Description: "Qwen3.6 Plus",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.325,
				Output:    1.95,
				CacheRead: 0.0325,
				Tiers: []CostTier{
					{
						MinContext: 256000,
						Input:      1.3,
						Output:     3.9,
						CacheRead:  0.13,
					},
				},
			},
			Limit: &Limit{
				Context: 1000000,
				Output:  65536,
			},
		},
		"qwen/qwen3.7-max": {
			ID:          "qwen/qwen3.7-max",
			Name:        "Qwen3.7 Max",
			Description: "Qwen3.7 Max",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     2.5,
				Output:    7.5,
				CacheRead: 0.5,
			},
			Limit: &Limit{
				Context: 1000000,
				Output:  67072,
			},
		},
		"qwen/qwen3.7-plus": {
			ID:          "qwen/qwen3.7-plus",
			Name:        "Qwen3.7 Plus",
			Description: "Qwen3.7 Plus",
			Modalities: &Modalities{
				Input:  []string{"text", "image"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.4,
				Output:    1.6,
				CacheRead: 0.08,
				Tiers: []CostTier{
					{
						MinContext: 262144,
						Input:      1.2,
						Output:     4.8,
						CacheRead:  0.24,
					},
				},
			},
			Limit: &Limit{
				Context: 1000000,
				Output:  67072,
			},
		},
		"tencent/hy3": {
			ID:               "tencent/hy3",
			Name:             "Hunyuan 3",
			Reasoning:        true,
			ToolCall:         true,
			StructuredOutput: true,
			Description:      "Hunyuan 3",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.2,
				Output:    0.8,
				CacheRead: 0.05,
			},
			Limit: &Limit{
				Context: 262144,
				Output:  131072,
			},
		},
		"xai/grok-4.3": {
			ID:          "xai/grok-4.3",
			Name:        "Grok 4.3",
			Description: "Grok 4.3",
			Modalities: &Modalities{
				Input:  []string{"text", "image"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     1.25,
				Output:    2.5,
				CacheRead: 0.2,
				Tiers: []CostTier{
					{
						MinContext: 200000,
						Input:      2.5,
						Output:     5,
						CacheRead:  0.4,
					},
				},
			},
			Limit: &Limit{
				Context: 1000000,
				Output:  1000000,
			},
		},
		"xai/grok-4.5": {
			ID:          "xai/grok-4.5",
			Name:        "Grok 4.5",
			Description: "Grok 4.5",
			Modalities: &Modalities{
				Input:  []string{"text", "image"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     2,
				Output:    6,
				CacheRead: 0.5,
			},
			Limit: &Limit{
				Context: 500000,
				Output:  500000,
			},
		},
		"xai/grok-build-0.1": {
			ID:          "xai/grok-build-0.1",
			Name:        "Grok Build 0.1",
			Description: "Grok Build 0.1",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     1,
				Output:    2,
				CacheRead: 0.2,
				Tiers: []CostTier{
					{
						MinContext: 204800,
						Input:      2,
						Output:     4,
						CacheRead:  0.4,
					},
				},
			},
			Limit: &Limit{
				Context: 262144,
				Output:  262144,
			},
		},
		"xiaomi/mimo-v2.5": {
			ID:          "xiaomi/mimo-v2.5",
			Name:        "MiMo V2.5",
			Description: "MiMo V2.5",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.14,
				Output:    0.28,
				CacheRead: 0.0028,
			},
			Limit: &Limit{
				Context: 1024000,
				Output:  131072,
			},
		},
		"xiaomi/mimo-v2.5-pro": {
			ID:          "xiaomi/mimo-v2.5-pro",
			Name:        "MiMo V2.5 Pro",
			Description: "MiMo V2.5 Pro",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.435,
				Output:    0.87,
				CacheRead: 0.0036,
			},
			Limit: &Limit{
				Context: 1024000,
				Output:  131072,
			},
		},
		"zai-org/GLM-4.6": {
			ID:               "zai-org/GLM-4.6",
			Name:             "GLM 4.6",
			Reasoning:        true,
			ToolCall:         true,
			StructuredOutput: true,
			Description:      "GLM 4.6",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.6,
				Output:    2.2,
				CacheRead: 0.11,
			},
			Limit: &Limit{
				Context: 202752,
				Output:  202752,
			},
		},
		"zai-org/glm-4.7": {
			ID:               "zai-org/glm-4.7",
			Name:             "GLM 4.7",
			Reasoning:        true,
			ToolCall:         true,
			StructuredOutput: true,
			Description:      "GLM 4.7",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.52,
				Output:    1.85,
				CacheRead: 0.12,
			},
			Limit: &Limit{
				Context: 202752,
				Output:  202752,
			},
		},
		"zai-org/glm-5": {
			ID:               "zai-org/glm-5",
			Name:             "GLM 5",
			Reasoning:        true,
			ToolCall:         true,
			StructuredOutput: true,
			Description:      "GLM 5",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     0.95,
				Output:    3.15,
				CacheRead: 0.19,
			},
			Limit: &Limit{
				Context: 202752,
				Output:  202752,
			},
		},
		"zai-org/glm-5-turbo": {
			ID:          "zai-org/glm-5-turbo",
			Name:        "GLM 5 Turbo",
			Reasoning:   true,
			ToolCall:    true,
			Description: "GLM 5 Turbo",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     1.2,
				Output:    4,
				CacheRead: 0.24,
			},
			Limit: &Limit{
				Context: 262144,
				Output:  131072,
			},
		},
		"zai-org/glm-5.1": {
			ID:               "zai-org/glm-5.1",
			Name:             "GLM 5.1",
			Reasoning:        true,
			ToolCall:         true,
			StructuredOutput: true,
			Description:      "GLM 5.1",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     1.26,
				Output:    3.96,
				CacheRead: 0.234,
			},
			Limit: &Limit{
				Context: 202752,
				Output:  202752,
			},
		},
		"zai-org/glm-5.2": {
			ID:               "zai-org/glm-5.2",
			Name:             "GLM 5.2",
			Reasoning:        true,
			ToolCall:         true,
			StructuredOutput: true,
			Description:      "GLM 5.2",
			Modalities: &Modalities{
				Input:  []string{"text"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     1.4,
				Output:    4.4,
				CacheRead: 0.26,
			},
			Limit: &Limit{
				Context: 1048576,
				Output:  131072,
			},
		},
		"zai-org/glm-5v-turbo": {
			ID:          "zai-org/glm-5v-turbo",
			Name:        "GLM 5v Turbo",
			Reasoning:   true,
			ToolCall:    true,
			Description: "GLM 5v Turbo",
			Modalities: &Modalities{
				Input:  []string{"text", "image", "video"},
				Output: []string{"text"},
			},
			Cost: &Cost{
				Input:     1.2,
				Output:    4,
				CacheRead: 0.24,
			},
			Limit: &Limit{
				Context: 202752,
				Output:  131072,
			},
		},
	},
}
