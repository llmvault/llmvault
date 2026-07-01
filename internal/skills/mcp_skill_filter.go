package skills

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// resolveSkillFilter resolves the agent's skill allow-filter from its own
// Skills config, else its catalog manifest. Mirrors
// agentruntime.resolveAgentSkillFilter.
func resolveSkillFilter(ctx context.Context, db *gorm.DB, agent *model.Agent) *model.SkillFilter {
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
	if err := db.WithContext(ctx).
		Select("manifest").
		Where("id = ? AND status = ?", *agent.AgentCatalogID, model.AgentCatalogStatusActive).
		First(&catalog).Error; err != nil {
		return nil
	}
	return skillFilterFromCatalogManifest(catalog.Manifest)
}

// skillAllowed reports whether a skill slug is permitted by the allow-filter.
// A nil filter (or nil Allow list) permits everything.
func skillAllowed(slug string, filter *model.SkillFilter) bool {
	if filter == nil || filter.Allow == nil {
		return true
	}
	for _, allowed := range filter.Allow {
		if allowed == slug {
			return true
		}
	}
	return false
}

type skillFilterJSON struct {
	Allow *[]string `json:"allow"`
}

func skillFilterFromAgentSkills(skills model.JSON) *model.SkillFilter {
	if len(skills) == 0 {
		return nil
	}
	if raw, ok := skills["skill_filter"]; ok {
		return decodeSkillFilter(raw)
	}
	if _, ok := skills["allow"]; ok {
		return decodeSkillFilter(map[string]any(skills))
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

func skillFilterFromPayload(payload *skillFilterJSON) *model.SkillFilter {
	if payload == nil || payload.Allow == nil {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(*payload.Allow))
	for _, name := range *payload.Allow {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	sort.Strings(out)
	return &model.SkillFilter{Allow: out}
}
