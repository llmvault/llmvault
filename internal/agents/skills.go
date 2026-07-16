package agents

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/skillaccess"
)

// teamAvailableSkillSlugs returns the published skills effective for an agent:
// team-owned skills and direct grants of org-wide skills.
func teamAvailableSkillSlugs(ctx context.Context, db *gorm.DB, orgID, teamID uuid.UUID) ([]string, error) {
	if db == nil || orgID == uuid.Nil || teamID == uuid.Nil {
		return nil, nil
	}
	resolved, err := skillaccess.ResolveAgent(ctx, db, model.Agent{OrgID: &orgID, TeamID: teamID})
	if err != nil {
		return nil, err
	}
	slugs := make([]string, 0, len(resolved))
	for _, item := range resolved {
		slugs = append(slugs, item.Skill.Slug)
	}
	sort.Strings(slugs)
	return slugs, nil
}

// validateSkillSlugs checks every requested slug against the team-available set,
// returning the de-duped sorted slugs. On an unknown slug it returns a helpful
// error listing the valid slugs.
func validateSkillSlugs(ctx context.Context, db *gorm.DB, orgID, teamID uuid.UUID, requested []string) ([]string, error) {
	cleaned := dedupeNonEmpty(requested)
	if len(cleaned) == 0 {
		return nil, nil
	}
	available, err := teamAvailableSkillSlugs(ctx, db, orgID, teamID)
	if err != nil {
		return nil, fmt.Errorf("failed to load available skills: %w", err)
	}
	allowed := make(map[string]bool, len(available))
	for _, slug := range available {
		allowed[slug] = true
	}
	for _, slug := range cleaned {
		if !allowed[slug] {
			if len(available) == 0 {
				return nil, fmt.Errorf(
					"unknown skill %q: no skills are available to this team",
					slug,
				)
			}
			return nil, fmt.Errorf(
				"unknown skill %q: available skills are: %s",
				slug, strings.Join(available, ", "),
			)
		}
	}
	return cleaned, nil
}

// skillsJSON builds the agent Skills jsonb from a validated slug list. It uses
// the {"skill_filter": {"allow": [...]}} shape consumed by
// skills.resolveSkillFilter / skillFilterFromAgentSkills. An empty list yields
// an empty object (nil filter = all effective skills allowed).
func skillsJSON(slugs []string) model.JSON {
	if len(slugs) == 0 {
		return model.JSON{}
	}
	allow := make([]any, len(slugs))
	for i, slug := range slugs {
		allow[i] = slug
	}
	return model.JSON{
		"skill_filter": map[string]any{
			"allow": allow,
		},
	}
}

func dedupeNonEmpty(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
