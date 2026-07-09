package skills

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/pluginresolve"
)

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
	HivyActorUserID              string            `json:"_hivy_actor_user_id"`
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
			"plugin_slug":                    map[string]any{"type": "string", "description": "Slug of the org-owned plugin this skill belongs to. Must be a plugin created with create_org_plugin, not a global catalog plugin."},
			"name":                           map[string]any{"type": "string", "description": "Skill display name. The slug is generated from it."},
			"description":                    map[string]any{"type": "string", "description": "Agent-facing trigger text: when should an agent load this skill? Start with \"Use when...\". This is what agents see in skills_list, so make the triggering conditions concrete."},
			"human_description":              map[string]any{"type": "string", "description": "Optional user-facing display copy for the UI."},
			"category":                       map[string]any{"type": "string", "description": "Optional category shown in skills_list."},
			"tags":                           map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional lowercase kebab-case tags."},
			"required_environment_variables": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Env var names the skill's instructions read at runtime, as injected into the sandbox (org variables are injected as HIVY_ORG_<NAME>). Declaring them here surfaces them in skill_view so agents and users know what to set."},
			"content":                        map[string]any{"type": "string", "description": "The SKILL.md body in markdown, WITHOUT YAML frontmatter (it is generated from name/description/category/tags)."},
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
	if errResult := requireOrgManagerActor(ctx, db, token.OrgID, args.HivyActorUserID); errResult != nil {
		return errResult, nil
	}
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
	_ = pluginresolve.RefreshPluginSkillInstallCounts(ctx, db, plugin.ID)

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
	HivyActorUserID              string             `json:"_hivy_actor_user_id"`
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
