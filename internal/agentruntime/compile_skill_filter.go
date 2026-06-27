package agentruntime

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

type skillFilterJSON struct {
	Allow *[]string `json:"allow"`
}

type toolFilterJSON struct {
	Allow *[]string `json:"allow"`
	Deny  *[]string `json:"deny"`
}

func resolveAgentMCPToolFilter(ctx context.Context, db *gorm.DB, agent *model.Agent) *model.ToolFilter {
	if agent == nil {
		return nil
	}
	if agent.AgentCatalog != nil {
		if filter := mcpToolFilterFromCatalogManifest(agent.AgentCatalog.Manifest); filter != nil {
			return filter
		}
	}
	if db == nil || agent.AgentCatalogID == nil {
		return nil
	}
	var catalog model.AgentCatalog
	err := db.WithContext(ctx).
		Select("manifest").
		Where("id = ? AND status = ?", *agent.AgentCatalogID, model.AgentCatalogStatusActive).
		First(&catalog).Error
	if err != nil {
		return nil
	}
	return mcpToolFilterFromCatalogManifest(catalog.Manifest)
}

func resolveAgentSkillFilter(ctx context.Context, db *gorm.DB, agent *model.Agent) *model.SkillFilter {
	if agent == nil {
		return nil
	}
	if filter := skillFilterFromAgentSkills(agent.Skills); filter != nil {
		return filter
	}
	if agent.AgentCatalog != nil {
		if filter := skillFilterFromCatalogManifest(agent.AgentCatalog.Manifest); filter != nil {
			return filter
		}
	}
	if db == nil || agent.AgentCatalogID == nil {
		return nil
	}
	var catalog model.AgentCatalog
	err := db.WithContext(ctx).
		Select("manifest").
		Where("id = ? AND status = ?", *agent.AgentCatalogID, model.AgentCatalogStatusActive).
		First(&catalog).Error
	if err != nil {
		return nil
	}
	return skillFilterFromCatalogManifest(catalog.Manifest)
}

func mcpToolFilterFromCatalogManifest(raw model.RawJSON) *model.ToolFilter {
	if strings.TrimSpace(string(raw)) == "" {
		return nil
	}
	var payload struct {
		McpToolFilter *toolFilterJSON `json:"mcp_tool_filter"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil || payload.McpToolFilter == nil {
		return nil
	}
	return toolFilterFromPayload(payload.McpToolFilter)
}

func skillFilterFromAgentSkills(skills model.JSON) *model.SkillFilter {
	if len(skills) == 0 {
		return nil
	}
	if raw, ok := skills["skill_filter"]; ok {
		return decodeSkillFilter(raw)
	}
	if _, ok := skills["allow"]; ok {
		return decodeSkillFilter(skills)
	}
	return nil
}

func skillFilterFromCatalogManifest(raw model.RawJSON) *model.SkillFilter {
	if strings.TrimSpace(string(raw)) == "" {
		return nil
	}
	var payload struct {
		SkillFilter *skillFilterJSON `json:"skill_filter"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil || payload.SkillFilter == nil {
		return nil
	}
	return skillFilterFromPayload(payload.SkillFilter)
}

func decodeSkillFilter(raw any) *model.SkillFilter {
	switch typed := raw.(type) {
	case model.SkillFilter:
		return normalizeSkillFilter(&typed)
	case *model.SkillFilter:
		return normalizeSkillFilter(typed)
	}
	body, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var payload skillFilterJSON
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil
	}
	return skillFilterFromPayload(&payload)
}

func toolFilterFromPayload(payload *toolFilterJSON) *model.ToolFilter {
	if payload == nil || (payload.Allow == nil && payload.Deny == nil) {
		return nil
	}
	return &model.ToolFilter{
		Allow: normalizeOptionalStrings(payload.Allow),
		Deny:  normalizeOptionalStrings(payload.Deny),
	}
}

func skillFilterFromPayload(payload *skillFilterJSON) *model.SkillFilter {
	if payload == nil || payload.Allow == nil {
		return nil
	}
	return normalizeSkillFilter(&model.SkillFilter{Allow: *payload.Allow})
}

func normalizeToolFilter(filter *model.ToolFilter) *model.ToolFilter {
	if filter == nil || (filter.Allow == nil && filter.Deny == nil) {
		return nil
	}
	return &model.ToolFilter{
		Allow: normalizeStringsPreservingNil(filter.Allow),
		Deny:  normalizeStringsPreservingNil(filter.Deny),
	}
}

func normalizeSkillFilter(filter *model.SkillFilter) *model.SkillFilter {
	if filter == nil || filter.Allow == nil {
		return nil
	}
	return &model.SkillFilter{Allow: normalizeStringsPreservingNil(filter.Allow)}
}

func normalizeOptionalStrings(values *[]string) []string {
	if values == nil {
		return nil
	}
	return normalizeStringsPreservingNil(*values)
}

func normalizeStringsPreservingNil(values []string) []string {
	if values == nil {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, name := range values {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func copySkillSpecs(skills []SkillSpec) []SkillSpec {
	if len(skills) == 0 {
		return []SkillSpec{}
	}
	out := make([]SkillSpec, len(skills))
	copy(out, skills)
	return out
}
