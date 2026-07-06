package memory

const searchMemoriesDescription = `Search this channel's durable memories using a short semantic query.

Memories are scoped to the current channel; if the channel is configured to expose organization-wide memories, those are folded in automatically. Use query as a 2-6 word phrase, not a sentence. tags are optional exact lowercase slug filters.`

const retainMemoryDescription = `Store one stable memory for future work in this channel.

Use content for one durable fact, preference, rule, or decision. The memory is saved to the current channel; other agents working in this channel will recall it. tags are optional lowercase slugs for filtering later. Do not store secrets, raw logs, guesses, or transient status.`

const forgetMemoryDescription = `Archive a memory that is wrong, stale, duplicated, unsafe, or no longer useful.

Call search_memories first if you need the memory_id. You can only forget memories in this channel (or organization-wide memories when this channel is allowed to see them). reason is a brief note on why the memory should go.`

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
