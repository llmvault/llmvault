package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/skillaccess"
)

// allowedSkillFileDirs are the top-level directories a skill bundle may ship
// linked files under. Mirrors the runtime materialize allow-list.
var allowedSkillFileDirs = []string{"references", "templates", "scripts", "assets"}

// loadAgentPublishedSkills returns the published skills available through the
// agent's team ownership, direct grants, and granted connections.
func loadAgentPublishedSkills(ctx context.Context, db *gorm.DB, agentID uuid.UUID) ([]model.Skill, error) {
	if db == nil {
		return nil, nil
	}
	var agent model.Agent
	if err := db.WithContext(ctx).
		Preload("AgentCatalog").
		Where("id = ?", agentID).
		First(&agent).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	effective, err := skillaccess.ResolveAgent(ctx, db, agent)
	if err != nil {
		return nil, err
	}
	skills := make([]model.Skill, 0, len(effective))
	for _, entry := range effective {
		skills = append(skills, entry.Skill)
	}
	return skills, nil
}

// SkillSummary is a lightweight name/description/category view of a skill,
// used by skill_view resolution and the compile-time prompt hint so they
// always agree.
type SkillSummary struct {
	Name        string
	Description string
	Category    string
}

// AgentSkillSummaries returns the agent's allowed skills as summaries. It is
// the single source of truth shared by skill_view and the runtime prompt hint.
func AgentSkillSummaries(ctx context.Context, db *gorm.DB, agent *model.Agent) ([]SkillSummary, error) {
	if agent == nil {
		return nil, nil
	}
	all, err := loadAgentPublishedSkills(ctx, db, agent.ID)
	if err != nil {
		return nil, err
	}
	out := make([]SkillSummary, 0, len(all))
	for _, skill := range all {
		description := ""
		if bundle, bundleErr := decodeSkillBundle(skill); bundleErr == nil {
			description = skillDescription(skill, bundle)
		} else if skill.Description != nil {
			description = *skill.Description
		}
		out = append(out, SkillSummary{
			Name:        skill.Slug,
			Description: description,
			Category:    skill.Category,
		})
	}
	return out, nil
}

// loadActiveAgent loads the agent scoped to the token's org, failing if it is
// archived or not in the org. Used to resolve the skill allow-filter.
func loadActiveAgent(ctx context.Context, db *gorm.DB, orgID, agentID uuid.UUID) (*model.Agent, error) {
	var agent model.Agent
	if err := db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND status <> ?", agentID, orgID, "archived").
		First(&agent).Error; err != nil {
		return nil, fmt.Errorf("agent is not active in this org")
	}
	return &agent, nil
}

// skillDescription returns the agent-facing description, preferring the DB
// column and falling back to the bundle.
func skillDescription(skill model.Skill, bundle Bundle) string {
	if skill.Description != nil && *skill.Description != "" {
		return *skill.Description
	}
	return bundle.Description
}

// composeSkillMarkdown renders the SKILL.md the runtime materializes: YAML
// frontmatter followed by the bundle body. Mirrors
// agentruntime.composeInstructions plus a little extra metadata.
func composeSkillMarkdown(skill model.Skill, bundle Bundle) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("name: ")
	b.WriteString(skill.Slug)
	b.WriteString("\n")
	if desc := skillDescription(skill, bundle); desc != "" {
		encoded, _ := json.Marshal(desc)
		b.WriteString("description: ")
		b.Write(encoded)
		b.WriteString("\n")
	}
	if skill.Category != "" {
		b.WriteString("category: ")
		b.WriteString(skill.Category)
		b.WriteString("\n")
	}
	if len(skill.Tags) > 0 {
		b.WriteString("tags: [")
		b.WriteString(strings.Join(skill.Tags, ", "))
		b.WriteString("]\n")
	}
	b.WriteString("---\n\n")
	b.WriteString(bundle.Content)
	if !strings.HasSuffix(bundle.Content, "\n") {
		b.WriteString("\n")
	}
	return b.String()
}

// skillBundleFiles returns every supporting file in the bundle keyed by its
// path relative to the skill directory, from both the Files map and the
// References list.
func skillBundleFiles(bundle Bundle) map[string]string {
	files := make(map[string]string)
	for path, body := range bundle.Files {
		files[path] = body
	}
	for _, ref := range bundle.References {
		if ref.Path == "" {
			continue
		}
		files[ref.Path] = ref.Body
	}
	return files
}

// materializePayload builds the {root, files} instruction the runtime writes to
// disk. Includes the composed SKILL.md plus all supporting files.
func materializePayload(skill model.Skill, bundle Bundle) map[string]any {
	files := skillBundleFiles(bundle)
	files["SKILL.md"] = composeSkillMarkdown(skill, bundle)
	return map[string]any{
		"root":  ".skills/" + skill.Slug,
		"files": files,
	}
}

// linkedFileGroups groups supporting file paths by their top-level directory
// for the model-facing summary.
func linkedFileGroups(bundle Bundle) map[string][]string {
	groups := map[string][]string{}
	for path := range skillBundleFiles(bundle) {
		top := path
		if idx := strings.Index(path, "/"); idx >= 0 {
			top = path[:idx]
		}
		if !containsString(allowedSkillFileDirs, top) {
			continue
		}
		groups[top] = append(groups[top], path)
	}
	for _, files := range groups {
		sort.Strings(files)
	}
	return groups
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func decodeSkillBundle(skill model.Skill) (Bundle, error) {
	var bundle Bundle
	if len(skill.Bundle) == 0 {
		return bundle, fmt.Errorf("skill %q has an empty bundle", skill.Slug)
	}
	if err := json.Unmarshal(skill.Bundle, &bundle); err != nil {
		return bundle, fmt.Errorf("skill %q has an unreadable bundle", skill.Slug)
	}
	return bundle, nil
}
