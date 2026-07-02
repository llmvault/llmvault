package agents

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// orgAvailableSkillSlugs returns the set of published skill slugs available to
// the org: the skills owned by plugins the org has installed (revoked_at IS
// NULL). These are the slugs an agent-builder caller may put in a `skills`
// array, since a skill only resolves at runtime when its plugin is installed.
func orgAvailableSkillSlugs(ctx context.Context, db *gorm.DB, orgID uuid.UUID) ([]string, error) {
	if db == nil || orgID == uuid.Nil {
		return nil, nil
	}
	var pluginIDs []uuid.UUID
	if err := db.WithContext(ctx).Model(&model.OrgPluginInstall{}).
		Where("org_id = ? AND revoked_at IS NULL", orgID).
		Pluck("plugin_id", &pluginIDs).Error; err != nil {
		return nil, err
	}
	if len(pluginIDs) == 0 {
		return nil, nil
	}
	var slugs []string
	if err := db.WithContext(ctx).Model(&model.Skill{}).
		Where("plugin_id IN ? AND status = ?", pluginIDs, model.SkillStatusPublished).
		Distinct("slug").
		Pluck("slug", &slugs).Error; err != nil {
		return nil, err
	}
	sort.Strings(slugs)
	return slugs, nil
}

// validateSkillSlugs checks every requested slug against the org-available set,
// returning the de-duped sorted slugs. On an unknown slug it returns a helpful
// error listing the valid slugs (or directing the caller to list_org_plugins
// when none are available).
func validateSkillSlugs(ctx context.Context, db *gorm.DB, orgID uuid.UUID, requested []string) ([]string, error) {
	cleaned := dedupeNonEmpty(requested)
	if len(cleaned) == 0 {
		return nil, nil
	}
	available, err := orgAvailableSkillSlugs(ctx, db, orgID)
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
					"unknown skill %q: no skills are available to this org; install a plugin that provides skills (call list_org_plugins to see options)",
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
// an empty object (nil filter = all plugin skills allowed).
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
