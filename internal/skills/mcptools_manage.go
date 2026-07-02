package skills

import (
	"context"
	"encoding/json"
	"fmt"
	pathpkg "path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/model"
)

// Tool names for the skill-manager group. All four are privileged, opt-in
// mutating tools gated per calling agent (see skillManagerEnabled).
const (
	toolCreateOrgPlugin = "create_org_plugin"
	toolCreateSkill     = "create_skill"
	toolUpdateSkill     = "update_skill"
	toolArchiveSkill    = "archive_skill"
)

// Limits for agent-authored skills. Generous enough for real skills with
// references and scripts, small enough that a runaway tool call cannot bloat
// the skills table.
const (
	maxSkillContentBytes  = 256 * 1024
	maxSkillFileBytes     = 256 * 1024
	maxSkillTotalBytes    = 1024 * 1024
	maxSkillFiles         = 32
	maxSkillNameLen       = 120
	maxSkillDescriptionLen = 1024
)

var envVarNamePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// skillManagerEnabled reports whether the calling agent may use the privileged
// skill-manager tools. The default Hivy agent gets them automatically. Any
// other agent must explicitly allow-list one of them in McpToolFilter.Allow.
// Mirrors agentBuilderEnabled / orgMemoriesEnabled: these tools are never
// granted implicitly.
func skillManagerEnabled(agent *model.Agent) bool {
	if agent == nil {
		return false
	}
	if agent.IsDefault {
		return true
	}
	if agent.McpToolFilter == nil {
		return false
	}
	for _, allowed := range agent.McpToolFilter.Allow {
		switch strings.TrimSpace(allowed) {
		case toolCreateOrgPlugin, toolCreateSkill, toolUpdateSkill, toolArchiveSkill:
			return true
		}
	}
	return false
}

func registerSkillManagerTools(server *mcp.Server, db *gorm.DB, token *model.Token, frontendURL string) {
	registerCreateOrgPluginTool(server, db, token)
	registerCreateSkillTool(server, db, token, frontendURL)
	registerUpdateSkillTool(server, db, token, frontendURL)
	registerArchiveSkillTool(server, db, token)
}

// --- create_org_plugin ---------------------------------------------------------

type createOrgPluginArgs struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Icon        string `json:"icon"`
	IconColor   string `json:"icon_color"`
}

func registerCreateOrgPluginTool(server *mcp.Server, db *gorm.DB, token *model.Token) {
	server.AddTool(&mcp.Tool{
		Name:        toolCreateOrgPlugin,
		Description: "Create an org-owned plugin: a named group (e.g. \"Sales\", \"Engineering\") that holds this organization's custom skills. Skills always live inside a plugin — create or reuse one before calling create_skill. The plugin is installed on the org immediately; attach it to agents with update_agent(plugin_slugs). Check list_org_plugins first to reuse an existing group instead of creating a near-duplicate.",
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"name":        map[string]any{"type": "string", "description": "Display name for the plugin group, e.g. \"Sales\" or \"Engineering\". The slug is generated from it."},
				"description": map[string]any{"type": "string", "description": "Short description of what the skills in this group are for."},
				"category":    map[string]any{"type": "string", "description": "Optional catalog category. Defaults to \"Custom\"."},
				"icon":        map[string]any{"type": "string", "description": "Optional lucide icon name, e.g. \"briefcase\" or \"wrench\"."},
				"icon_color":  map[string]any{"type": "string", "description": "Optional hex icon color, e.g. \"#0EA5E9\"."},
			},
			"required": []string{"name"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args createOrgPluginArgs
		if err := decodeSkillToolArgs(req, &args); err != nil {
			return skillToolError(err.Error()), nil
		}
		return handleCreateOrgPlugin(ctx, db, token, args)
	})
}

func handleCreateOrgPlugin(ctx context.Context, db *gorm.DB, token *model.Token, args createOrgPluginArgs) (*mcp.CallToolResult, error) {
	name := strings.TrimSpace(args.Name)
	if name == "" {
		return skillToolError("name is required"), nil
	}
	if len(name) > maxSkillNameLen {
		return skillToolError(fmt.Sprintf("name must be at most %d characters", maxSkillNameLen)), nil
	}
	slug := model.GenerateSlug(name)
	category := strings.TrimSpace(args.Category)
	if category == "" {
		category = "Custom"
	}

	// Reject slugs that collide with any plugin visible to this org — a global
	// catalog plugin or one of the org's own (any status: the per-org unique
	// index covers archived rows too).
	var count int64
	if err := db.WithContext(ctx).Model(&model.Plugin{}).
		Where("slug = ? AND (org_id IS NULL OR org_id = ?)", slug, token.OrgID).
		Count(&count).Error; err != nil {
		return skillToolError("failed to check plugin slug: " + err.Error()), nil
	}
	if count > 0 {
		return skillToolError(fmt.Sprintf("a plugin with slug %q already exists for this org; reuse it (see list_org_plugins) or pick a different name", slug)), nil
	}

	manifest := map[string]any{
		"version":     1,
		"slug":        slug,
		"name":        name,
		"description": strings.TrimSpace(args.Description),
		"category":    category,
		"icon":        strings.TrimSpace(args.Icon),
		"icon_color":  strings.TrimSpace(args.IconColor),
		"org_plugin":  true,
	}
	rawManifest, err := json.Marshal(manifest)
	if err != nil {
		return skillToolError("failed to build plugin manifest"), nil
	}

	orgID := token.OrgID
	plugin := model.Plugin{
		ID:          uuid.New(),
		OrgID:       &orgID,
		Slug:        slug,
		Name:        name,
		Description: strings.TrimSpace(args.Description),
		Category:    category,
		Icon:        strings.TrimSpace(args.Icon),
		IconColor:   strings.TrimSpace(args.IconColor),
		Developer:   "Custom",
		Status:      model.PluginStatusActive,
		Manifest:    model.RawJSON(rawManifest),
	}
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&plugin).Error; err != nil {
			return fmt.Errorf("create plugin: %w", err)
		}
		install := model.OrgPluginInstall{ID: uuid.New(), OrgID: orgID, PluginID: plugin.ID}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&install).Error; err != nil {
			return fmt.Errorf("install plugin for org: %w", err)
		}
		return nil
	})
	if err != nil {
		return skillToolError(err.Error()), nil
	}

	return skillToolJSON(map[string]any{
		"success": true,
		"plugin": map[string]any{
			"id":       plugin.ID.String(),
			"slug":     plugin.Slug,
			"name":     plugin.Name,
			"category": plugin.Category,
		},
		"hint": "Plugin created and installed on the org. Add skills with create_skill(plugin_slug=\"" + plugin.Slug + "\"). To make agents use them, attach the plugin with update_agent — plugin_slugs REPLACES the agent's set, so call get_agent first and include its existing plugins.",
	})
}

// --- create_skill --------------------------------------------------------------

type createSkillArgs struct {
	PluginSlug                   string            `json:"plugin_slug"`
	Name                         string            `json:"name"`
	Description                  string            `json:"description"`
	HumanDescription             string            `json:"human_description"`
	Category                     string            `json:"category"`
	Tags                         []string          `json:"tags"`
	RequiredEnvironmentVariables []string          `json:"required_environment_variables"`
	Content                      string            `json:"content"`
	Files                        map[string]string `json:"files"`
}

func registerCreateSkillTool(server *mcp.Server, db *gorm.DB, token *model.Token, frontendURL string) {
	server.AddTool(&mcp.Tool{
		Name:        toolCreateSkill,
		Description: "Create a custom skill inside an org-owned plugin (create one first with create_org_plugin if needed). Provide the SKILL.md body as `content` (WITHOUT frontmatter — it is generated from name/description) and supporting files under references/, templates/, scripts/, or assets/. The skill is published immediately to every agent that has the plugin attached. Get the user's explicit approval of the final content before calling this.",
		InputSchema: createSkillSchema(),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args createSkillArgs
		if err := decodeSkillToolArgs(req, &args); err != nil {
			return skillToolError(err.Error()), nil
		}
		return handleCreateSkill(ctx, db, token, frontendURL, args)
	})
}

func createSkillSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"plugin_slug": map[string]any{"type": "string", "description": "Slug of the org-owned plugin this skill belongs to. Must be a plugin created with create_org_plugin, not a global catalog plugin."},
			"name":        map[string]any{"type": "string", "description": "Skill display name. The slug is generated from it."},
			"description": map[string]any{"type": "string", "description": "Agent-facing trigger text: when should an agent load this skill? Start with \"Use when...\". This is what agents see in skills_list, so make the triggering conditions concrete."},
			"human_description": map[string]any{"type": "string", "description": "Optional user-facing display copy for the UI."},
			"category":          map[string]any{"type": "string", "description": "Optional category shown in skills_list."},
			"tags": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional lowercase kebab-case tags."},
			"required_environment_variables": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Env var names the skill's instructions read at runtime, as injected into the sandbox (org variables are injected as HIVY_ORG_<NAME>). Declaring them here surfaces them in skill_view so agents and users know what to set."},
			"content": map[string]any{"type": "string", "description": "The SKILL.md body in markdown, WITHOUT YAML frontmatter (it is generated from name/description/category/tags)."},
			"files": map[string]any{
				"type":                 "object",
				"additionalProperties": map[string]any{"type": "string"},
				"description":          "Supporting files keyed by relative path under references/, templates/, scripts/, or assets/ (e.g. {\"references/api.md\": \"...\", \"scripts/check.sh\": \"...\"}). Materialized into .skills/<slug>/ when an agent loads the skill.",
			},
		},
		"required": []string{"plugin_slug", "name", "description", "content"},
	}
}

func handleCreateSkill(ctx context.Context, db *gorm.DB, token *model.Token, frontendURL string, args createSkillArgs) (*mcp.CallToolResult, error) {
	plugin, errResult := loadOrgOwnedPlugin(ctx, db, token.OrgID, args.PluginSlug)
	if errResult != nil {
		return errResult, nil
	}
	name := strings.TrimSpace(args.Name)
	desc := strings.TrimSpace(args.Description)
	content := args.Content
	if errResult := validateSkillFields(name, desc, content, args.Files, args.RequiredEnvironmentVariables); errResult != nil {
		return errResult, nil
	}
	slug := model.GenerateSlug(name)

	var count int64
	if err := db.WithContext(ctx).Model(&model.Skill{}).
		Where("plugin_id = ? AND slug = ?", plugin.ID, slug).
		Count(&count).Error; err != nil {
		return skillToolError("failed to check skill slug: " + err.Error()), nil
	}
	if count > 0 {
		return skillToolError(fmt.Sprintf("skill %q already exists in plugin %q; use update_skill to change it", slug, plugin.Slug)), nil
	}

	files := cleanSkillFiles(args.Files)
	envVars := normalizeStringList(args.RequiredEnvironmentVariables)
	rawBundle, err := marshalSkillBundle(slug, name, desc, content, files, envVars)
	if err != nil {
		return skillToolError(err.Error()), nil
	}

	now := time.Now()
	orgID := token.OrgID
	humanDesc := strings.TrimSpace(args.HumanDescription)
	skill := model.Skill{
		ID:               uuid.New(),
		PluginID:         &plugin.ID,
		OrgID:            &orgID,
		Slug:             slug,
		Name:             name,
		Description:      &desc,
		HumanDescription: &humanDesc,
		Category:         strings.TrimSpace(args.Category),
		SourceType:       model.SkillSourceInline,
		RepoRef:          "main",
		Bundle:           model.RawJSON(rawBundle),
		Tags:             pq.StringArray(normalizeStringList(args.Tags)),
		Status:           model.SkillStatusPublished,
		PublishedAt:      &now,
		HydratedAt:       &now,
	}
	if err := db.WithContext(ctx).Create(&skill).Error; err != nil {
		return skillToolError("failed to create skill: " + err.Error()), nil
	}
	refreshSkillInstallCount(ctx, db, plugin.ID)

	return skillToolJSON(map[string]any{
		"success": true,
		"skill": map[string]any{
			"slug":        skill.Slug,
			"name":        skill.Name,
			"plugin_slug": plugin.Slug,
			"status":      skill.Status,
		},
		"required_environment_variables": envVars,
		"environment_settings_url":       environmentSettingsURL(frontendURL),
		"hint":                           skillPublishHint(plugin.Slug),
	})
}

// --- update_skill --------------------------------------------------------------

type updateSkillArgs struct {
	PluginSlug                   string             `json:"plugin_slug"`
	Skill                        string             `json:"skill"`
	Name                         *string            `json:"name"`
	Description                  *string            `json:"description"`
	HumanDescription             *string            `json:"human_description"`
	Category                     *string            `json:"category"`
	Tags                         *[]string          `json:"tags"`
	RequiredEnvironmentVariables *[]string          `json:"required_environment_variables"`
	Content                      *string            `json:"content"`
	Files                        *map[string]string `json:"files"`
}

func registerUpdateSkillTool(server *mcp.Server, db *gorm.DB, token *model.Token, frontendURL string) {
	server.AddTool(&mcp.Tool{
		Name:        toolUpdateSkill,
		Description: "Update a skill in an org-owned plugin. A true patch: only provided fields change; the skill slug never changes. A provided `files` object REPLACES the full file set. Updating an archived skill republishes it. Get the user's explicit approval of the changes before calling this.",
		InputSchema: updateSkillSchema(),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args updateSkillArgs
		if err := decodeSkillToolArgs(req, &args); err != nil {
			return skillToolError(err.Error()), nil
		}
		return handleUpdateSkill(ctx, db, token, frontendURL, args)
	})
}

func updateSkillSchema() map[string]any {
	schema := createSkillSchema()
	props := schema["properties"].(map[string]any)
	props["skill"] = map[string]any{"type": "string", "description": "Slug of the skill to update (see skills_list or list_org_plugins)."}
	props["name"] = map[string]any{"type": "string", "description": "New display name. The slug is NOT regenerated."}
	delete(props, "plugin_slug")
	props["plugin_slug"] = map[string]any{"type": "string", "description": "Slug of the org-owned plugin the skill lives in."}
	schema["required"] = []string{"plugin_slug", "skill"}
	return schema
}

func handleUpdateSkill(ctx context.Context, db *gorm.DB, token *model.Token, frontendURL string, args updateSkillArgs) (*mcp.CallToolResult, error) {
	plugin, errResult := loadOrgOwnedPlugin(ctx, db, token.OrgID, args.PluginSlug)
	if errResult != nil {
		return errResult, nil
	}
	skill, errResult := loadOrgOwnedSkill(ctx, db, token.OrgID, plugin, args.Skill)
	if errResult != nil {
		return errResult, nil
	}
	bundle, err := decodeSkillBundle(*skill)
	if err != nil {
		return skillToolError(err.Error()), nil
	}

	name := skill.Name
	if args.Name != nil && strings.TrimSpace(*args.Name) != "" {
		name = strings.TrimSpace(*args.Name)
	}
	desc := ""
	if skill.Description != nil {
		desc = *skill.Description
	}
	if args.Description != nil {
		desc = strings.TrimSpace(*args.Description)
	}
	content := bundle.Content
	if args.Content != nil {
		content = *args.Content
	}
	files := skillBundleFiles(bundle)
	if args.Files != nil {
		files = cleanSkillFiles(*args.Files)
	}
	envVars := bundle.RequiredEnvironmentVariables
	if args.RequiredEnvironmentVariables != nil {
		envVars = normalizeStringList(*args.RequiredEnvironmentVariables)
	}
	if errResult := validateSkillFields(name, desc, content, files, envVars); errResult != nil {
		return errResult, nil
	}

	rawBundle, err := marshalSkillBundle(skill.Slug, name, desc, content, files, envVars)
	if err != nil {
		return skillToolError(err.Error()), nil
	}

	now := time.Now()
	updates := map[string]any{
		"name":        name,
		"description": &desc,
		"bundle":      model.RawJSON(rawBundle),
		"status":      model.SkillStatusPublished,
		"hydrated_at": &now,
	}
	if args.HumanDescription != nil {
		humanDesc := strings.TrimSpace(*args.HumanDescription)
		updates["human_description"] = &humanDesc
	}
	if args.Category != nil {
		updates["category"] = strings.TrimSpace(*args.Category)
	}
	if args.Tags != nil {
		updates["tags"] = pq.StringArray(normalizeStringList(*args.Tags))
	}
	if err := db.WithContext(ctx).Model(skill).Updates(updates).Error; err != nil {
		return skillToolError("failed to update skill: " + err.Error()), nil
	}

	return skillToolJSON(map[string]any{
		"success": true,
		"skill": map[string]any{
			"slug":        skill.Slug,
			"name":        name,
			"plugin_slug": plugin.Slug,
			"status":      model.SkillStatusPublished,
		},
		"required_environment_variables": envVars,
		"environment_settings_url":       environmentSettingsURL(frontendURL),
		"hint":                           skillPublishHint(plugin.Slug),
	})
}

// --- archive_skill -------------------------------------------------------------

type archiveSkillArgs struct {
	PluginSlug   string `json:"plugin_slug"`
	Skill        string `json:"skill"`
	UserApproved bool   `json:"user_approved"`
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

func refreshSkillInstallCount(ctx context.Context, db *gorm.DB, pluginID uuid.UUID) {
	_ = db.WithContext(ctx).Model(&model.Skill{}).
		Where("plugin_id = ?", pluginID).
		UpdateColumn("install_count", gorm.Expr(
			"(SELECT COUNT(*) FROM agent_plugin_installs WHERE plugin_id = ?)", pluginID,
		)).Error
}

func environmentSettingsURL(frontendURL string) string {
	base := strings.TrimRight(strings.TrimSpace(frontendURL), "/")
	return base + "/w/settings/environments"
}

func skillPublishHint(pluginSlug string) string {
	return "Published. Agents with the \"" + pluginSlug + "\" plugin attached see it in skills_list immediately (their static prompt hint refreshes next session). Attach the plugin to more agents via get_agent + update_agent (plugin_slugs replaces the set)."
}
