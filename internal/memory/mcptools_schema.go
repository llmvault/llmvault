package memory

const searchMemoriesDescription = `Search durable memories using a short semantic query.

Use query as a 2-6 word phrase, not a sentence. target.owner selects org, user, or both. target.visibility selects this_agent, all_agents, or both. tags are optional exact lowercase slug filters.`

const retainMemoryDescription = `Store one stable memory for future work.

Use content for one durable fact, preference, rule, or decision. target.owner chooses user or org. target.visibility chooses this_agent or all_agents; prefer this_agent unless other agents should use it. To seed a memory for another agent in your org, set target.agent_id to that agent's UUID with owner org instead of visibility; only that agent can later forget it. tags are optional lowercase slugs. Do not store secrets, raw logs, guesses, or transient status.`

const forgetMemoryDescription = `Archive a memory that is wrong, stale, duplicated, unsafe, or no longer useful.

Call search_memories first if you need the memory_id. Agents may forget org memories. User memories may only be forgotten in a session for that same user. A memory bound to another agent can only be archived by the org's default agent, and only with user_approved true after showing the user the memory and its owning agent and receiving explicit confirmation.`

func memorySearchTargetSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"description":          "Optional memory scope filter. Defaults to owner=both and visibility=both.",
		"additionalProperties": false,
		"properties": map[string]any{
			"owner": map[string]any{
				"type":        "string",
				"enum":        []string{memoryToolOwnerOrg, memoryToolOwnerUser, memoryToolOwnerBoth},
				"description": "org searches organization memories; user searches memories for the current session creator; both searches accessible org and user memories.",
			},
			"visibility": map[string]any{
				"type":        "string",
				"enum":        []string{AgentVisibilityAllAgents, AgentVisibilityThisAgent, AgentVisibilityBoth},
				"description": "all_agents searches shared memories; this_agent searches only this agent's private memories; both searches both.",
			},
		},
	}
}

func memoryRetainTargetSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"description":          "Required write target. Pick the narrowest durable audience that should remember this. Provide exactly one of visibility or agent_id.",
		"additionalProperties": false,
		"properties": map[string]any{
			"owner": map[string]any{
				"type":        "string",
				"enum":        []string{memoryToolOwnerOrg, memoryToolOwnerUser},
				"description": "user means the current session creator; org means the memory belongs to the whole organization.",
			},
			"visibility": map[string]any{
				"type":        "string",
				"enum":        []string{AgentVisibilityAllAgents, AgentVisibilityThisAgent},
				"description": "this_agent keeps the memory private to this agent; all_agents makes it shared across agents for the selected owner. Mutually exclusive with agent_id.",
			},
			"agent_id": map[string]any{
				"type":        "string",
				"description": "Optional UUID of another agent in this org; binds the memory to that specific agent. Requires owner org, is mutually exclusive with visibility, and the target agent alone can later forget it.",
			},
		},
		"required": []string{"owner"},
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
