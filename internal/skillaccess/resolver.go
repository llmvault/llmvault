package skillaccess

import (
	"context"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

const (
	SourceTeamOwned = "team_owned"
	SourceTeamGrant = "team_grant"
)

type EffectiveSkill struct {
	Skill   model.Skill
	Sources []string
}

// ResolveAgent returns every published skill available to an agent through
// team ownership or a direct team grant.
func ResolveAgent(ctx context.Context, db *gorm.DB, agent model.Agent) ([]EffectiveSkill, error) {
	if db == nil || agent.OrgID == nil || agent.TeamID == uuid.Nil {
		return []EffectiveSkill{}, nil
	}
	resolved := make(map[uuid.UUID]*EffectiveSkill)
	add := func(rows []model.Skill, source string) {
		for _, row := range rows {
			entry := resolved[row.ID]
			if entry == nil {
				copy := row
				entry = &EffectiveSkill{Skill: copy}
				resolved[row.ID] = entry
			}
			if !contains(entry.Sources, source) {
				entry.Sources = append(entry.Sources, source)
			}
		}
	}

	var teamOwned []model.Skill
	if err := db.WithContext(ctx).
		Where("org_id = ? AND team_id = ? AND status = ?", *agent.OrgID, agent.TeamID, model.SkillStatusPublished).
		Find(&teamOwned).Error; err != nil {
		return nil, fmt.Errorf("load team-owned skills: %w", err)
	}
	add(teamOwned, SourceTeamOwned)

	var granted []model.Skill
	if err := db.WithContext(ctx).
		Joins("JOIN team_skill_grants tsg ON tsg.skill_id = skills.id").
		Where("tsg.org_id = ? AND tsg.team_id = ?", *agent.OrgID, agent.TeamID).
		Where("skills.status = ? AND skills.team_id IS NULL AND (skills.org_id IS NULL OR skills.org_id = ?)", model.SkillStatusPublished, *agent.OrgID).
		Find(&granted).Error; err != nil {
		return nil, fmt.Errorf("load team skill grants: %w", err)
	}
	add(granted, SourceTeamGrant)

	out := make([]EffectiveSkill, 0, len(resolved))
	for _, entry := range resolved {
		sort.Strings(entry.Sources)
		out = append(out, *entry)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Skill.Slug == out[j].Skill.Slug {
			return out[i].Skill.ID.String() < out[j].Skill.ID.String()
		}
		return out[i].Skill.Slug < out[j].Skill.Slug
	})
	return out, nil
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
