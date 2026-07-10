package memory

func memoryIncludeFactsSchema() map[string]any {
	return map[string]any{
		"type":        "boolean",
		"description": "Debug only: search the raw extracted facts layer (legacy agent memories) instead of consolidated observations. Default false.",
	}
}

func memoryTagsSchema(description string) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": description,
		"items": map[string]any{
			"type":        "string",
			"pattern":     "^[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])?$",
			"maxLength":   64,
			"description": "Lowercase kebab-case slug. Use short labels like preference, billing, or project-helio.",
		},
		"maxItems":    memoryToolMaxTags,
		"uniqueItems": true,
	}
}
