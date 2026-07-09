package skills

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/usehivy/hivy/internal/model"
)

func resolveSkillFilter(agent *model.Agent) *model.SkillFilter {
	if agent == nil {
		return nil
	}
	return skillFilterFromAgentSkills(agent.Skills)
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
