package skills

import (
	"context"
	"encoding/json"
	"fmt"
	pathpkg "path"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// --- archive_skill -------------------------------------------------------------

type archiveSkillArgs struct {
	PluginSlug      string `json:"plugin_slug"`
	Skill           string `json:"skill"`
	UserApproved    bool   `json:"user_approved"`
	HivyActorUserID string `json:"_hivy_actor_user_id"`
}

func registerArchiveSkillTool(server *mcp.Server, db *gorm.DB, token *model.Token) {
	server.AddTool(&mcp.Tool{
		Name:        toolArchiveSkill,
		Description: "Archive a skill in an org-owned plugin, removing it from every agent whose plugins include it. Destructive: requires user_approved=true, which you may only set after showing the user exactly which skill will be removed and getting their explicit confirmation in this conversation.",
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"plugin_slug":   map[string]any{"type": "string", "description": "Slug of the org-owned plugin the skill lives in."},
				"skill":         map[string]any{"type": "string", "description": "Slug of the skill to archive."},
				"user_approved": map[string]any{"type": "boolean", "description": "Set true only after the user explicitly confirmed archiving this exact skill in this conversation."},
			},
			"required": []string{"plugin_slug", "skill", "user_approved"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args archiveSkillArgs
		if err := decodeSkillToolArgs(req, &args); err != nil {
			return skillToolError(err.Error()), nil
		}
		return handleArchiveSkill(ctx, db, token, args)
	})
}

func handleArchiveSkill(ctx context.Context, db *gorm.DB, token *model.Token, args archiveSkillArgs) (*mcp.CallToolResult, error) {
	if errResult := requireOrgManagerActor(ctx, db, token.OrgID, args.HivyActorUserID); errResult != nil {
		return errResult, nil
	}
	plugin, errResult := loadOrgOwnedPlugin(ctx, db, token.OrgID, args.PluginSlug)
	if errResult != nil {
		return errResult, nil
	}
	skill, errResult := loadOrgOwnedSkill(ctx, db, token.OrgID, plugin, args.Skill)
	if errResult != nil {
		return errResult, nil
	}
	if !args.UserApproved {
		desc := ""
		if skill.Description != nil {
			desc = *skill.Description
		}
		return skillToolError(fmt.Sprintf(
			"archiving %q removes it from every agent that has the %q plugin. Show the user the skill (name: %s, description: %s), get their explicit confirmation in this conversation, then retry with user_approved=true",
			skill.Slug, plugin.Slug, skill.Name, desc,
		)), nil
	}
	if err := db.WithContext(ctx).Model(skill).Update("status", model.SkillStatusArchived).Error; err != nil {
		return skillToolError("failed to archive skill: " + err.Error()), nil
	}
	return skillToolJSON(map[string]any{
		"success": true,
		"skill": map[string]any{
			"slug":        skill.Slug,
			"plugin_slug": plugin.Slug,
			"status":      model.SkillStatusArchived,
		},
		"hint": "Archived. update_skill on this slug republishes it.",
	})
}

// --- shared helpers ------------------------------------------------------------

// loadOrgOwnedPlugin resolves an ACTIVE plugin owned by this org. Global
// catalog plugins are deliberately rejected: agent-authored skills only ever
// live in org-owned plugins.
func loadOrgOwnedPlugin(ctx context.Context, db *gorm.DB, orgID uuid.UUID, slug string) (*model.Plugin, *mcp.CallToolResult) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return nil, skillToolError("plugin_slug is required")
	}
	var plugin model.Plugin
	err := db.WithContext(ctx).
		Where("slug = ? AND org_id = ? AND status = ?", slug, orgID, model.PluginStatusActive).
		First(&plugin).Error
	if err == gorm.ErrRecordNotFound {
		var globalCount int64
		db.WithContext(ctx).Model(&model.Plugin{}).
			Where("slug = ? AND org_id IS NULL AND status = ?", slug, model.PluginStatusActive).
			Count(&globalCount)
		if globalCount > 0 {
			return nil, skillToolError(fmt.Sprintf("plugin %q is a global catalog plugin; custom skills can only live in org-owned plugins — create one with create_org_plugin", slug))
		}
		return nil, skillToolError(fmt.Sprintf("org plugin %q not found; create it with create_org_plugin or check list_org_plugins", slug))
	}
	if err != nil {
		return nil, skillToolError("failed to load plugin: " + err.Error())
	}
	return &plugin, nil
}

func loadOrgOwnedSkill(ctx context.Context, db *gorm.DB, orgID uuid.UUID, plugin *model.Plugin, slug string) (*model.Skill, *mcp.CallToolResult) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return nil, skillToolError("skill is required")
	}
	var skill model.Skill
	err := db.WithContext(ctx).
		Where("plugin_id = ? AND slug = ? AND org_id = ?", plugin.ID, slug, orgID).
		First(&skill).Error
	if err == gorm.ErrRecordNotFound {
		return nil, skillToolError(fmt.Sprintf("skill %q not found in plugin %q", slug, plugin.Slug))
	}
	if err != nil {
		return nil, skillToolError("failed to load skill: " + err.Error())
	}
	return &skill, nil
}

// validateSkillFields enforces the shared create/update invariants. Returns a
// tool error result describing the first violation, or nil.
func validateSkillFields(name, description, content string, files map[string]string, envVars []string) *mcp.CallToolResult {
	if name == "" {
		return skillToolError("name is required")
	}
	if len(name) > maxSkillNameLen {
		return skillToolError(fmt.Sprintf("name must be at most %d characters", maxSkillNameLen))
	}
	if description == "" {
		return skillToolError("description is required — it is the trigger text agents use to decide when to load the skill")
	}
	if len(description) > maxSkillDescriptionLen {
		return skillToolError(fmt.Sprintf("description must be at most %d characters", maxSkillDescriptionLen))
	}
	if strings.TrimSpace(content) == "" {
		return skillToolError("content is required — the SKILL.md body in markdown")
	}
	if strings.HasPrefix(strings.TrimSpace(content), "---") {
		return skillToolError("content must be the SKILL.md body WITHOUT YAML frontmatter; name/description/category/tags are supplied as fields and the frontmatter is generated")
	}
	if len(content) > maxSkillContentBytes {
		return skillToolError(fmt.Sprintf("content must be at most %d bytes", maxSkillContentBytes))
	}
	if len(files) > maxSkillFiles {
		return skillToolError(fmt.Sprintf("at most %d files are allowed", maxSkillFiles))
	}
	total := len(content)
	for path, body := range files {
		if err := validateSkillFilePath(path); err != nil {
			return skillToolError(err.Error())
		}
		if len(body) > maxSkillFileBytes {
			return skillToolError(fmt.Sprintf("file %q must be at most %d bytes", path, maxSkillFileBytes))
		}
		total += len(body)
	}
	if total > maxSkillTotalBytes {
		return skillToolError(fmt.Sprintf("skill content plus files must total at most %d bytes", maxSkillTotalBytes))
	}
	for _, envVar := range envVars {
		trimmed := strings.TrimSpace(envVar)
		if trimmed == "" {
			continue
		}
		if !envVarNamePattern.MatchString(trimmed) {
			return skillToolError(fmt.Sprintf("environment variable name %q is invalid: use UPPER_SNAKE_CASE (org variables are injected as HIVY_ORG_<NAME>)", envVar))
		}
	}
	return nil
}

// validateSkillFilePath enforces the same allow-list the runtime materializer
// uses: clean relative paths under references/, templates/, scripts/, assets/.
func validateSkillFilePath(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("file paths must not be empty")
	}
	if strings.HasPrefix(path, "/") || strings.Contains(path, "\\") {
		return fmt.Errorf("file path %q must be a relative path using forward slashes", path)
	}
	if clean := pathpkg.Clean(path); clean != path || strings.HasPrefix(clean, "..") {
		return fmt.Errorf("file path %q must be a clean relative path without \"..\" or \".\" segments", path)
	}
	top, rest, ok := strings.Cut(path, "/")
	if !ok || rest == "" || !containsString(allowedSkillFileDirs, top) {
		return fmt.Errorf("file path %q must live under one of: %s/", path, strings.Join(allowedSkillFileDirs, "/, "))
	}
	return nil
}

func cleanSkillFiles(files map[string]string) map[string]string {
	out := make(map[string]string, len(files))
	for path, body := range files {
		out[strings.TrimSpace(path)] = body
	}
	return out
}

// marshalSkillBundle builds the Bundle jsonb exactly like plugin sync does for
// filesystem skills, so agent-authored skills flow through skill_view and
// materialization unchanged.
func marshalSkillBundle(slug, name, description, content string, files map[string]string, envVars []string) ([]byte, error) {
	bundle := &Bundle{
		ID:          slug,
		Title:       name,
		Description: description,
		Content:     content,
		Manifest: map[string]any{
			"name":        name,
			"description": description,
		},
		Files:                        files,
		References:                   referencesFromFileMap(files),
		RequiredEnvironmentVariables: envVars,
	}
	raw, err := json.Marshal(bundle)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize skill bundle: %w", err)
	}
	return raw, nil
}

func referencesFromFileMap(files map[string]string) []Reference {
	if len(files) == 0 {
		return nil
	}
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	refs := make([]Reference, 0, len(paths))
	for _, path := range paths {
		refs = append(refs, Reference{Path: path, Body: files[path]})
	}
	return refs
}

func normalizeStringList(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func environmentSettingsURL(frontendURL string) string {
	base := strings.TrimRight(strings.TrimSpace(frontendURL), "/")
	return base + "/w/settings/environments"
}

func skillPublishHint(pluginSlug string) string {
	return "Published. Agents on a team that has the \"" + pluginSlug + "\" plugin enabled see it in skills_list immediately (their static prompt hint refreshes next session). Plugins are team-managed: grant the plugin to a team in team settings to reach more agents."
}
