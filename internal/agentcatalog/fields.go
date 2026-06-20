package agentcatalog

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/lib/pq"

	"github.com/usehivy/hivy/internal/model"
)

func catalogUpdates(manifest Manifest, raw model.RawJSON, hash, status string) map[string]any {
	developer := strings.TrimSpace(manifest.Developer)
	if developer == "" {
		developer = "Hivy"
	}
	strategy := strings.TrimSpace(manifest.Runtime.SandboxStrategy)
	if strategy == "" {
		strategy = "per_session"
	}
	sandboxImage := model.NormalizeSandboxImage(manifest.Runtime.SandboxImage)
	return map[string]any{
		"name":                strings.TrimSpace(manifest.Name),
		"description":         strings.TrimSpace(manifest.Description),
		"category":            strings.TrimSpace(manifest.Category),
		"avatar_url":          strings.TrimSpace(manifest.AvatarURL),
		"developer":           developer,
		"official":            boolValue(manifest.Official),
		"is_default":          boolValue(manifest.Default),
		"model":               strings.TrimSpace(manifest.Runtime.Model),
		"available_models":    pq.StringArray(normalizeCatalogAvailableModels(manifest.Runtime.Model, manifest.Runtime.AvailableModels)),
		"multimodal_model":    strings.TrimSpace(manifest.Runtime.MultimodalModel),
		"sandbox_strategy":    strategy,
		"sandbox_image":       sandboxImage,
		"instructions":        strings.TrimSpace(manifest.instructions),
		"tools":               normalizeToolSelection(manifest.Tools),
		"sub_agents":          catalogSubAgentsJSON(manifest),
		"required_plugins":    pq.StringArray(normalizeStrings(manifest.Plugins.Required)),
		"recommended_plugins": pq.StringArray(normalizeStrings(manifest.Plugins.Recommended)),
		"manifest":            raw,
		"source_hash":         hash,
		"status":              status,
	}
}

func applyCatalogUpdates(row *model.AgentCatalog, updates map[string]any) {
	row.Name = updates["name"].(string)
	row.Description = updates["description"].(string)
	row.Category = updates["category"].(string)
	row.AvatarURL = updates["avatar_url"].(string)
	row.Developer = updates["developer"].(string)
	row.Official = updates["official"].(bool)
	row.IsDefault = updates["is_default"].(bool)
	row.Model = updates["model"].(string)
	row.AvailableModels = updates["available_models"].(pq.StringArray)
	row.MultimodalModel = updates["multimodal_model"].(string)
	row.SandboxStrategy = updates["sandbox_strategy"].(string)
	row.SandboxImage = updates["sandbox_image"].(string)
	row.Instructions = updates["instructions"].(string)
	row.Tools = updates["tools"].(model.JSON)
	row.SubAgents = updates["sub_agents"].(model.RawJSON)
	row.RequiredPlugins = updates["required_plugins"].(pq.StringArray)
	row.RecommendedPlugins = updates["recommended_plugins"].(pq.StringArray)
	row.Manifest = updates["manifest"].(model.RawJSON)
	row.SourceHash = updates["source_hash"].(string)
	row.Status = updates["status"].(string)
}

func normalizeStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		clean := strings.TrimSpace(value)
		if clean == "" || seen[clean] {
			continue
		}
		seen[clean] = true
		out = append(out, clean)
	}
	return out
}

func normalizeCatalogAvailableModels(defaultModel string, values []string) []string {
	out := normalizeStrings(values)
	defaultModel = strings.TrimSpace(defaultModel)
	if defaultModel == "" {
		return out
	}
	for _, value := range out {
		if value == defaultModel {
			return out
		}
	}
	return append([]string{defaultModel}, out...)
}

func catalogSubAgentsJSON(manifest Manifest) model.RawJSON {
	if len(manifest.SubAgents) == 0 {
		return model.RawJSON("{}")
	}
	out := make(map[string]model.AgentCatalogSubAgent, len(manifest.SubAgents))
	for key, subAgent := range manifest.SubAgents {
		cleanKey := strings.TrimSpace(key)
		if cleanKey == "" {
			continue
		}
		out[cleanKey] = model.AgentCatalogSubAgent{
			Name:         strings.TrimSpace(subAgent.Name),
			Description:  strings.TrimSpace(subAgent.Description),
			Model:        strings.TrimSpace(subAgent.Model),
			Tools:        normalizeToolSelection(subAgent.Tools),
			Instructions: strings.TrimSpace(subAgent.instructions),
		}
	}
	raw, err := json.Marshal(out)
	if err != nil || len(raw) == 0 {
		return model.RawJSON("{}")
	}
	return model.RawJSON(raw)
}

func boolValue(value *bool) bool {
	return value != nil && *value
}

func normalizeToolSelection(values map[string]any) model.JSON {
	if len(values) == 0 {
		return model.JSON{}
	}
	out := model.JSON{}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		cleanKey := strings.TrimSpace(key)
		if cleanKey == "" || !toolSelectionEnabled(values[key]) {
			continue
		}
		if config, ok := values[key].(map[string]any); ok {
			out[cleanKey] = cloneToolConfig(config)
			continue
		}
		out[cleanKey] = true
	}
	return out
}

func toolSelectionEnabled(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case bool:
		return typed
	case map[string]any:
		if enabled, ok := typed["enabled"].(bool); ok && !enabled {
			return false
		}
		return true
	default:
		return true
	}
}

func cloneToolConfig(config map[string]any) map[string]any {
	out := make(map[string]any, len(config))
	for key, value := range config {
		out[key] = cloneToolConfigValue(value)
	}
	return out
}

func cloneToolConfigValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneToolConfig(typed)
	case []any:
		out := make([]any, len(typed))
		for index, item := range typed {
			out[index] = cloneToolConfigValue(item)
		}
		return out
	default:
		return typed
	}
}
