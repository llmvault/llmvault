package hindsight

import "github.com/usehivy/hivy/internal/model"

func memoryTagsSchema(requireMemoryType bool) map[string]any {
	required := []string{"scope", "provider"}
	if requireMemoryType {
		required = append(required, "memory_type")
	}
	return map[string]any{
		"type":        "object",
		"description": "Structured memory tags. Use provider scope for provider-wide facts and resource scope for facts about a specific provider resource.",
		"properties": map[string]any{
			"scope": map[string]any{
				"type":        "string",
				"enum":        []string{MemoryScopeProvider, MemoryScopeResource},
				"description": "Use resource when the memory is about a specific repository/project/channel/etc.; otherwise use provider.",
			},
			"provider": map[string]any{
				"type":        "string",
				"description": "Connected provider key, for example github-app, slack, linear, railway, or notion. Must match an active org connection.",
			},
			"resource_type": map[string]any{
				"type":        "string",
				"description": "Required for resource scope. Example: repository.",
			},
			"resource_id": map[string]any{
				"type":        "string",
				"description": "Required for resource scope. For GitHub repositories use owner/repo, for example usehivy/usehivy.com.",
			},
			"memory_type": map[string]any{
				"type":        "string",
				"enum":        SupportedMemoryTypes,
				"description": "Required for retain. Optional on recall/reflect to narrow retrieval.",
			},
		},
		"required": required,
	}
}

func hasLegacyMemoryTagFields(raw map[string]any) bool {
	if raw == nil {
		return false
	}
	for _, key := range []string{"provider", "source", "resource_type", "resource_id", "memory_type"} {
		if _, ok := raw[key]; ok {
			return true
		}
	}
	return false
}

func memoryMetadata(agent *model.Agent, documentID string, tags MemoryTagInput) map[string]string {
	metadata := map[string]string{
		"agent_id":      agent.ID.String(),
		"document_id":   documentID,
		"scope":         tags.Scope,
		"provider":      tags.Provider,
		"source":        "manual",
		"memory_type":   tags.MemoryType,
		"resource_id":   tags.ResourceID,
		"resource_type": tags.ResourceType,
	}
	for key, value := range metadata {
		if value == "" {
			delete(metadata, key)
		}
	}
	return metadata
}

func memoryObservationScopes(tags MemoryTagInput) [][]string {
	scope := []string{"scope:" + tags.Scope, "provider:" + tags.Provider}
	if tags.Scope == MemoryScopeResource {
		scope = append(scope,
			"resource_type:"+tags.ResourceType,
			"resource:"+tags.Provider+":"+tags.ResourceType+":"+tags.ResourceID,
		)
	}
	return [][]string{scope}
}
