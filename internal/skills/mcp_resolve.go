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
)

// allowedSkillFileDirs are the top-level directories a skill bundle may ship
// linked files under. Mirrors the runtime materialize allow-list.
var allowedSkillFileDirs = []string{"references", "templates", "scripts", "assets"}

// loadAgentPublishedSkills returns the published skills owned by the plugins
// installed on the agent. This mirrors agentruntime.buildSkills so the MCP
// tools surface exactly the skills the agent is entitled to.
func loadAgentPublishedSkills(ctx context.Context, db *gorm.DB, agentID uuid.UUID) ([]model.Skill, error) {
	if db == nil {
		return nil, nil
	}
	var pluginIDs []uuid.UUID
	if err := db.WithContext(ctx).Model(&model.AgentPluginInstall{}).
		Where("agent_id = ?", agentID).Pluck("plugin_id", &pluginIDs).Error; err != nil {
		return nil, err
	}
	if len(pluginIDs) == 0 {
		return nil, nil
	}
	var skills []model.Skill
	if err := db.WithContext(ctx).
		Where("plugin_id IN ? AND status = ?", pluginIDs, model.SkillStatusPublished).
		Find(&skills).Error; err != nil {
		return nil, err
	}
	sort.SliceStable(skills, func(i, j int) bool {
		if skills[i].Slug == skills[j].Slug {
			return skills[i].ID.String() < skills[j].ID.String()
		}
		return skills[i].Slug < skills[j].Slug
	})
	return skills, nil
}

// SkillSummary is a lightweight name/description/category view of a skill,
// used by both the skills_list MCP tool and the compile-time prompt hint so
// they always agree.
type SkillSummary struct {
	Name        string
	Description string
	Category    string
}

// AgentSkillSummaries returns the agent's allowed skills as summaries. It is
// the single source of truth shared by the MCP skills_list tool and the
// runtime prompt hint.
func AgentSkillSummaries(ctx context.Context, db *gorm.DB, agent *model.Agent) ([]SkillSummary, error) {
	if agent == nil {
		return nil, nil
	}
	all, err := loadAgentPublishedSkills(ctx, db, agent.ID)
	if err != nil {
		return nil, err
	}
	filter := resolveSkillFilter(ctx, db, agent)
	out := make([]SkillSummary, 0, len(all))
	for _, skill := range all {
		if !skillAllowed(skill.Slug, filter) {
			continue
		}
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
