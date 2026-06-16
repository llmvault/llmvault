package agentcatalog

import (
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
	return map[string]any{
		"name":                strings.TrimSpace(manifest.Name),
		"description":         strings.TrimSpace(manifest.Description),
		"category":            strings.TrimSpace(manifest.Category),
		"avatar_url":          strings.TrimSpace(manifest.AvatarURL),
		"developer":           developer,
		"official":            boolValue(manifest.Official),
		"is_default":          boolValue(manifest.Default),
		"model":               strings.TrimSpace(manifest.Runtime.Model),
		"multimodal_model":    strings.TrimSpace(manifest.Runtime.MultimodalModel),
		"sandbox_strategy":    strategy,
		"instructions":        strings.TrimSpace(manifest.instructions),
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
	row.MultimodalModel = updates["multimodal_model"].(string)
	row.SandboxStrategy = updates["sandbox_strategy"].(string)
	row.Instructions = updates["instructions"].(string)
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

func boolValue(value *bool) bool {
	return value != nil && *value
}
