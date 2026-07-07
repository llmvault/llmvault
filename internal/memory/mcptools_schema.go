package memory

const searchMemoriesDescription = `Search this channel's durable memories using a short semantic query.

Results are consolidated observations: one canonical entry per fact with kind, entities, proof_count (how many times it was independently confirmed), and last_mentioned_at. When a memory's wording was rewritten over time, the result also carries history (superseded wordings, newest first, each with the date it stopped being current and the reason) — use it to explain how a belief evolved, resolve contradictions about past states, or answer "what did we use before X". History entries are former wordings: never present them as current facts. Memories are scoped to the current channel; if the channel is configured to expose organization-wide memories, those are folded in automatically. Ranking blends semantic similarity with memory strength (proof count, kind, recency), and matches below the relevance threshold are dropped server-side — an empty result means nothing relevant is stored, so act on that instead of retrying with looser wording.

How to write effective queries:
- Use a 2-6 word noun phrase that mirrors how the remembered fact would be worded, e.g. "preview deploy platform" or "refund approval policy" — never a full question or sentence.
- Name concrete entities verbatim (customer, project, person): "helio launch checklist", not "that launch thing".
- Search one concept per call; make separate calls for separate topics instead of combining them.
- Leave tags empty on the first call. tags are exact lowercase slug filters matched against observation entities, and a wrong slug silently returns nothing — narrow with a tag only after seeing it in results.

Most channel memory is already injected into your context (rules + memory digest); search for long-tail facts beyond the digest or for precedent before decisions. Memory is read-only to you: new memories are written automatically by background reflection over your sessions, and corrections or deletions happen in the memories UI — there is no tool to store or delete a memory. Set include_facts true only to debug the raw extracted facts behind the observations.`

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
